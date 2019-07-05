package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"syscall"

	"github.com/CSCfi/qvain-api/internal/sessions"
	"github.com/CSCfi/qvain-api/internal/version"
	"github.com/CSCfi/qvain-api/pkg/proxy"
	"github.com/rs/zerolog"
)

// ApiProxy is a reverse proxy.
type ApiProxy struct {
	proxy    *httputil.ReverseProxy
	sessions *sessions.Manager
	logger   zerolog.Logger
}

// makeProxyErrorHandler makes a callback function to handle errors happening inside the proxy.
func makeProxyErrorHandler(logger zerolog.Logger) func(http.ResponseWriter, *http.Request, error) {
	// log only every N proxy error
	//logger = logger.Sample(&zerolog.BasicSampler{N: 3})
	return func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Info().Err(err).Msg("upstream error")
		jsonError(w, convertNetError(err), http.StatusBadGateway)
	}
}

// recorderToResponse is a helper function to get fields from a ResponseRecorder to a Response.
func recorderToResponse(recorder *httptest.ResponseRecorder, response *http.Response) {
	result := recorder.Result()
	response.StatusCode = result.StatusCode
	response.Body = result.Body
	response.Header = result.Header
	response.Trailer = result.Trailer
}

// checkProjectIdentifierMap checks project_identifiers in map recursively.
func checkProjectIdentifierMap(session *sessions.Session, m map[string]interface{}) bool {
	for key, v := range m {
		switch vv := v.(type) {
		case string:
			if key == "project_identifier" && !session.User.HasProject(vv) {
				return false
			}
		case map[string]interface{}:
			if !checkProjectIdentifierMap(session, vv) {
				return false
			}
		case []interface{}:
			if !checkProjectIdentifierArray(session, vv) {
				return false
			}
		}
	}
	return true
}

// checkProjectIdentifierArray checks project_identifiers in array recursively.
func checkProjectIdentifierArray(session *sessions.Session, a []interface{}) bool {
	for _, v := range a {
		switch vv := v.(type) {
		case map[string]interface{}:
			if !checkProjectIdentifierMap(session, vv) {
				return false
			}
		case []interface{}:
			if !checkProjectIdentifierArray(session, vv) {
				return false
			}
		}
	}
	return true
}

// addPropertyToRequest adds a propetry to the root object in a json request,
// or if the root is an array, to each of the objects in the array
func addPropertyToRequest(r *http.Request, key string, value string) error {
	// read body
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	r.Body.Close()

	// parse json
	var data interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return err
	}

	// set the property for the objects
	switch data := data.(type) {
	case map[string]interface{}: // object
		data[key] = value

	case []interface{}: // array of objects
		for _, object := range data {
			if object, isObject := object.(map[string]interface{}); isObject {
				object[key] = value
			}
		}
	}

	// create new body with the modified data, update ContentLength
	body, err = json.Marshal(data)
	if err != nil {
		return err
	}
	r.ContentLength = int64(len(body))

	r.Body = ioutil.NopCloser(bytes.NewBuffer(body)) // make body readable again
	return nil
}

// makeModifyResponse makes a callback function to handle the response. This is used for
// checking that a Metax response does not contain invalid projects.
func makeProxyModifyResponse(logger zerolog.Logger, sessions *sessions.Manager) func(*http.Response) error {
	return func(response *http.Response) error {
		response.Header = make(http.Header) // clear response headers

		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil // respond with original error
		}

		// read body
		body, err := ioutil.ReadAll(response.Body)
		if err != nil {
			recorder := httptest.NewRecorder()
			jsonError(recorder, "failed to read response body", http.StatusInternalServerError)
			recorderToResponse(recorder, response)
			return nil
		}
		response.Body.Close()
		response.Body = ioutil.NopCloser(bytes.NewBuffer(body)) // make body readable again

		// parse json
		var data map[string]interface{}
		err = json.Unmarshal(body, &data)
		if err != nil {
			recorder := httptest.NewRecorder()
			jsonError(recorder, "response is not json", http.StatusInternalServerError)
			recorderToResponse(recorder, response)
			return nil
		}

		// get user session
		session, err := sessions.UserSessionFromRequest(response.Request)
		if err != nil {
			// Our error helper functions need a ResponseWriter so we cannot use response directly.
			// Instead, we'll write to a ResponseRecorder and copy the result to the response.
			recorder := httptest.NewRecorder()
			sessionError(recorder, err)
			recorderToResponse(recorder, response)
			return nil
		}

		// check response for project_identifier strings recursively
		if !checkProjectIdentifierMap(session, data) {
			recorder := httptest.NewRecorder()
			jsonError(recorder, "invalid project in response", http.StatusForbidden)
			recorderToResponse(recorder, response)
			return nil
		}

		return nil
	}
}

// NewApiProxy creates a reverse web proxy that uses HTTP Basic Authentication. Used for allowing
// the front-end user access to the Metax files api. Since this allows the user to access Metax using
// Qvain service credentials, care needs to be taken that users cannot perform actions they shouldn't
// have access to.
func NewApiProxy(upstreamURL string, user string, pass string, sessions *sessions.Manager, logger zerolog.Logger, devMode bool) *ApiProxy {
	upUrl, err := url.Parse(upstreamURL)
	if err != nil {
		logger.Error().Err(err).Str("url", upstreamURL).Msg("can't parse upstream url")
	}

	return &ApiProxy{
		proxy: proxy.NewSingleHostReverseProxy(
			upUrl,
			proxy.WithBasicAuth(user, pass),
			proxy.WithErrorHandler(makeProxyErrorHandler(logger)),
			proxy.WithModifyResponse(makeProxyModifyResponse(logger, sessions)),
			proxy.WithUserAgent(version.Id+"/"+version.CommitTag),
			proxy.WithInsecureCertificates(devMode),
		),
		sessions: sessions,
		logger:   logger,
	}
}

// ServeHTTP proxies user requests to Metax so the front-end can query project information from Metax.
// The query is checked against the user session to make sure that users can only query projects
// they have access to.
func (api *ApiProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	api.logger.Debug().Str("path", r.URL.Path).Msg("request path")

	// only allow access to /directories/ and /files/; path has been cleaned by Go on instantiation
	if !(strings.HasPrefix(r.URL.Path, "/directories/") || strings.HasPrefix(r.URL.Path, "/files/")) {
		jsonError(w, "access denied", http.StatusForbidden)
	}

	// make sure the user is authenticated
	session, err := api.sessions.UserSessionFromRequest(r)
	if err != nil {
		sessionError(w, err)
		return
	}

	// allowed_projects should be set by the proxy, not in the original request
	query := r.URL.Query()
	if _, found := query["allowed_projects"]; found {
		jsonError(w, "bad request: allowed_projects is not allowed", http.StatusBadRequest)
		return
	}

	// proxy takes care of converting project to project_identifier as needed
	if _, found := query["project_identifier"]; found {
		jsonError(w, "bad request: project_identifier is not allowed", http.StatusBadRequest)
		return
	}

	// check optional project query parameter
	if projectQueries, found := query["project"]; found {
		if len(projectQueries) > 1 {
			jsonError(w, "bad request: multiple projects in query", http.StatusBadRequest)
			return
		}
		if len(session.User.Projects) < 1 {
			jsonError(w, "access denied: user has no projects", http.StatusForbidden)
			return
		}
		project := projectQueries[0]
		if !session.User.HasProject(project) {
			api.logger.Debug().Strs("projects", session.User.Projects).Str("wanted", project).Msg("project check")
			jsonError(w, "access denied: invalid project", http.StatusForbidden)
			return
		}

		// /files/ expects that project query parameter is called project_identifier
		if strings.HasPrefix(r.URL.Path, "/files/") {
			query.Del("project")
			query.Add("project_identifier", project)
			r.URL.RawQuery = query.Encode()
		}
	}

	// use allowed_projects parameter for non-GET requests
	if r.Method != http.MethodGet {
		r.URL.RawQuery = session.User.AddAllowedProjects(r.URL.RawQuery)

		// assume new objects are being created if method is POST, so user_created will be used
		key := "user_created"
		if r.Method != http.MethodPost {
			key = "user_modified"
		}

		if err := addPropertyToRequest(r, key, session.User.Identity); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	api.proxy.ServeHTTP(w, r)
}

// convertNetError tries to catch (package) net and syscall errors and give a friendlier description.
// TODO: move this elsewhere?
func convertNetError(err error) string {
	if err == nil {
		return "no error"
	}

	if netError, ok := err.(net.Error); ok && netError.Timeout() {
		return "connection timeout"
	}

	switch t := err.(type) {
	case *net.OpError:
		if t.Op == "dial" {
			return "unknown host"
		}
		if t.Op == "read" {
			return "connection refused"
		}
	case syscall.Errno:
		if t == syscall.ECONNREFUSED {
			return "connection refused"
		}
	}

	// fallback to simple Bad Gateway error
	return http.StatusText(http.StatusBadGateway)
}
