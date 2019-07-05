package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"

	"github.com/CSCfi/qvain-api/internal/sessions"
	"github.com/CSCfi/qvain-api/pkg/models"
	"github.com/rs/zerolog"
	"github.com/wvh/uuid"
)

var (
	responses = map[string]string{
		"1": `{"project_identifier": "1"}`,
		"2": `{"results": [{"project_identifier": "1"}, {"project_identifier": "2"}]}`,
		"3": `{"project_identifier": "3"}`,
		"4": `{"results": [{"project_identifier": "1"}, {"project_identifier": "4"}]}`,
		"5": `{project_identifier: "1"}`, // invalid json; missing quotes in key
		"6": `{"directories": [{"project_identifier": "1"}, {"project_identifier": "4"}]}`,
		"7": `{"testing": {"nesting": [ {"foo": "bar"}, {"project_identifier": "5"}]}}`,
		"8": `{"testing": {"nesting": [ {"foo": "bar"}, {"project_identifier": "1"}]}}`,
		"9": `{"testing": {"nesting": [ {foo: "bar"}, {"project_identifier": "1"}]}}`, // missing quotes in key
	}

	requestBodies = map[string]string{
		"object": `{"identifier":"1", "file_characteristics": {"title":"Whee"}}`,
		"array": `[{"identifier":"1", "file_characteristics": {"title":"Whee"}},` +
			`{"identifier":"2", "file_characteristics": {"title":"Whoo"}}]`,
	}

	userIdentity = "user"
	userProjects = []string{"1", "2"}
)

func errorResponse(request *http.Request, msg string, code int) *http.Response {
	recorder := httptest.NewRecorder()
	recorder.WriteString(msg)
	response := recorder.Result()
	response.StatusCode = code
	response.Request = request
	return response
}

// checkProperty checks the json request root object contains a property with a specific value. If
// the json object contains an array, each object in the array is checked.
func checkProperty(r *http.Request, key string, value string) bool {
	// read body
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return false
	}
	r.Body.Close()

	// parse json
	var data interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return false
	}

	// check key-value pair
	switch data := data.(type) {
	case map[string]interface{}: // object
		if v, ok := data[key].(string); !ok || v != value {
			return false
		}

	case []interface{}: // array of objects
		for _, object := range data {
			if object, isObject := object.(map[string]interface{}); isObject {
				if v, ok := object[key].(string); !ok || v != value {
					return false
				}
			}
		}
	}

	return true
}

// DummyRoundTripper acts as a dummy replacement for the external Metax server.
type DummyRoundTripper struct{}

// RoundTrip performs checks on the request generated by the proxy and
// returns a response corresponding to the response query parameter.
func (rt *DummyRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	query := request.URL.Query()

	// make sure that /directories/ and /files/ are not called with wrong project query parameter
	if strings.HasPrefix(request.URL.Path, "/directories/") && query.Get("project_identifier") != "" {
		response := errorResponse(request, "project_identifier not allowed with /directories/", http.StatusBadRequest)
		return response, nil
	}
	if strings.HasPrefix(request.URL.Path, "/files/") && query.Get("project") != "" {
		response := errorResponse(request, "project not allowed with /files/", http.StatusBadRequest)
		return response, nil
	}

	// expect allowed_projects if method is not GET
	if request.Method != http.MethodGet {
		allowedProjectsStr := query.Get("allowed_projects")
		if allowedProjectsStr == "" {
			response := errorResponse(request, "non-get request should have allowed_projects", http.StatusBadRequest)
			return response, nil
		}

		// check that allowed_projects contains the same projects as user.Projects
		allowedProjects := strings.Split(allowedProjectsStr, ",")
		found := 0
		for _, project := range allowedProjects {
			for _, userProject := range userProjects {
				if userProject == project {
					found++
					break
				}
			}
		}
		if found != len(allowedProjects) {
			response := errorResponse(request, "allowed_projects does not match user projects", http.StatusBadRequest)
			return response, nil
		}
	}

	if request.Method != http.MethodGet {
		field := "user_created"
		if request.Method != http.MethodPost {
			field = "user_modified"
		}
		if !checkProperty(request, field, userIdentity) {
			response := errorResponse(request, fmt.Sprintf("missing %s in request", field), http.StatusBadRequest)
			return response, nil
		}
	}

	recorder := httptest.NewRecorder()
	recorder.WriteString(responses[query.Get("response")])
	response := recorder.Result()
	response.Request = request
	response.Header.Add("X-Dummy-Header", "remove_me")
	return response, nil
}

func NewDummyProxy(logger zerolog.Logger, sessionsManager *sessions.Manager) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Director:       func(r *http.Request) {}, // don't modify the request
		Transport:      &DummyRoundTripper{},
		ErrorHandler:   makeProxyErrorHandler(logger),
		ModifyResponse: makeProxyModifyResponse(logger, sessionsManager)}
}

type RequestConfig struct {
	NoSession bool
	Method    string
	Body      string
}

// tryRequest creates a request for ApiProxy.ServeHTTP and checks the response
func tryRequest(t *testing.T, url string, config RequestConfig, expectedStatus int) string {
	t.Helper() // ignore this function when printing line numbers for errors

	logger := zerolog.Nop() // don't print logs
	sessionsManager := sessions.NewManager()
	api := &ApiProxy{
		proxy:    NewDummyProxy(logger, sessionsManager),
		sessions: sessionsManager,
		logger:   logger,
	}

	// create session
	uuid, _ := uuid.NewUUID()
	sid, _ := sessionsManager.NewLogin(
		&uuid,
		&models.User{
			Projects: userProjects,
			Identity: userIdentity,
		},
	)

	method := http.MethodGet
	if config.Method != "" {
		method = config.Method
	}

	request, _ := http.NewRequest(method, url, new(bytes.Buffer)) // create request
	if !config.NoSession {
		request.Header.Add("Cookie", "sid="+sid) // add session cookie
	}

	// add body to request
	if config.Body != "" {
		request.Body = ioutil.NopCloser(bytes.NewBuffer([]byte(config.Body)))
	}

	writer := httptest.NewRecorder() // writer will contain the response
	api.ServeHTTP(writer, request)   // make the request

	// read response and check status code
	body, _ := ioutil.ReadAll(writer.Body)
	statusCode := writer.Result().StatusCode
	if statusCode != expectedStatus {
		t.Errorf("%s: expected %d, got %d %s", url, expectedStatus, statusCode, string(body))
	}

	if dummyHeader := writer.Header().Get("X-Dummy-Header"); dummyHeader != "" {
		t.Errorf("%s: headers from remote service should be removed", url)
	}

	return string(body)
}

func TestApiProxy(t *testing.T) {
	// ok, return single object
	tryRequest(t,
		"/files/fakeurl?project=1&response=1",
		RequestConfig{},
		http.StatusOK,
	)

	// ok, return result object array
	tryRequest(t,
		"/directories/fakeurl?project=2&response=2",
		RequestConfig{},
		http.StatusOK,
	)

	// ok, return nested result
	tryRequest(t,
		"/directories/fakeurl?project=1&response=8",
		RequestConfig{},
		http.StatusOK,
	)

	// ok, no project specified, return result object array
	tryRequest(t,
		"/directories/fakeurl?response=2",
		RequestConfig{},
		http.StatusOK,
	)

	// ok, use PATCH method
	tryRequest(t,
		"/files/fakeurl?project=1&response=1",
		RequestConfig{Method: http.MethodPatch, Body: requestBodies["object"]},
		http.StatusOK,
	)

	// ok, use PATCH method for array
	tryRequest(t,
		"/files/fakeurl?project=1&response=1",
		RequestConfig{Method: http.MethodPatch, Body: requestBodies["array"]},
		http.StatusOK,
	)

	// ok, use POST method
	tryRequest(t,
		"/files/fakeurl?project=1&response=1",
		RequestConfig{Method: http.MethodPost, Body: requestBodies["object"]},
		http.StatusOK,
	)

	// fail, path does not start with /directories/ or /files/
	tryRequest(t,
		"/invalidpath/fakeurl?project=1&response=1",
		RequestConfig{},
		http.StatusForbidden,
	)

	// fail, missing session cookie
	tryRequest(t,
		"/directories/fakeurl?project=1",
		RequestConfig{NoSession: true},
		http.StatusUnauthorized,
	)

	// fail, project_identifier is not allowed (proxy takes care of it)
	tryRequest(t,
		"/directories/fakeurl?project_identifier=1&response=1",
		RequestConfig{},
		http.StatusBadRequest,
	)

	// fail, invalid project in query
	tryRequest(t,
		"/directories/fakeurl?project=4&response=3",
		RequestConfig{},
		http.StatusForbidden,
	)

	// fail, invalid project in single object response
	tryRequest(t,
		"/directories/fakeurl?project=2&response=3",
		RequestConfig{},
		http.StatusForbidden,
	)

	// fail, invalid project in response results
	tryRequest(t,
		"/directories/fakeurl?project=2&response=4",
		RequestConfig{},
		http.StatusForbidden,
	)

	// fail, invalid project in response directories array
	tryRequest(t,
		"/directories/fakeurl?project=2&response=6",
		RequestConfig{},
		http.StatusForbidden,
	)

	// fail, invalid project in nested response
	tryRequest(t,
		"/directories/fakeurl?project=2&response=7",
		RequestConfig{},
		http.StatusForbidden,
	)

	// fail, repeated project parameter
	tryRequest(t,
		"/directories/fakeurl?project=1&project=10",
		RequestConfig{},
		http.StatusBadRequest,
	)

	// fail, allowed_projects in query
	tryRequest(t,
		"/directories/fakeurl?allowed_projects=1,2,3,4,5",
		RequestConfig{},
		http.StatusBadRequest,
	)

	// fail, invalid json
	tryRequest(t,
		"/directories/fakeurl?response=5",
		RequestConfig{},
		http.StatusInternalServerError,
	)

	// fail, invalid json
	tryRequest(t,
		"/directories/fakeurl?response=9",
		RequestConfig{},
		http.StatusInternalServerError,
	)
}
