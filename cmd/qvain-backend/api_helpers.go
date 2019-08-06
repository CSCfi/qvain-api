package main

import (
	"net/http"
	"path"
	"strings"

	"github.com/CSCfi/qvain-api/internal/psql"
	"github.com/CSCfi/qvain-api/internal/sessions"
	"github.com/CSCfi/qvain-api/internal/version"

	"github.com/francoispqt/gojay"
	"github.com/wvh/uuid"
)

// apiWriteHeaders writes standard header fields for all JSON api responses.
func apiWriteHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// apiWriteOptions is a convenience function to add an OPTIONS response to API endpoints.
//
// [CORS] headers in use: Retry-After (rate limiting), Range/Accept-Ranges (paging)
// [CORS] headers in consideration: X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset (rate limiting)
func apiWriteOptions(w http.ResponseWriter, opts string) {
	apiWriteHeaders(w)
	// pre-flight
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Retry-After, Accept-Ranges")
	w.Header().Set("Access-Control-Allow-Methods", "*") // wildcard in spec but not implemented by all browsers yet
	w.Header().Set("Access-Control-Max-Age", "3600")

	// actual OPTIONS response
	w.Header().Set("Allow", opts)
}

// jsonError takes an error string and status code and writes them to the response.
func jsonError(w http.ResponseWriter, msg string, status int) {
	apiWriteHeaders(w)
	w.WriteHeader(status)

	enc := gojay.BorrowEncoder(w)
	defer enc.Release()

	// we could also wrap this with EncodeObject instead of manually handling the object, but this is zero alloc
	//   enc.EncodeObject(gojay.EncodeObjectFunc(func(enc *gojay.Encoder) {...}))
	enc.AppendByte('{')
	enc.AddIntKey("status", status)
	enc.AddStringKey("msg", msg)
	enc.AppendByte('}')
	enc.Write()
}

// jsonErrorWithDescription writes an error API response like jsonError does, but adds a friendly explanation and optional URL.
// jsonError takes an error string and status code and writes them to the response.
func jsonErrorWithDescription(w http.ResponseWriter, msg string, help string, url string, status int) {
	apiWriteHeaders(w)
	w.WriteHeader(status)

	enc := gojay.BorrowEncoder(w)
	defer enc.Release()

	enc.AppendByte('{')
	enc.AddIntKey("status", status)
	enc.AddStringKey("msg", msg)
	enc.AddStringKeyOmitEmpty("help", help)
	enc.AddStringKeyOmitEmpty("url", url)
	enc.AppendByte('}')
	enc.Write()
}

// jsonErrorWithPayload writes an error API response like jsonError, but allows adding a source and extra (pre-serialised) json value.
func jsonErrorWithPayload(w http.ResponseWriter, msg string, origin string, payload []byte, status int) {
	apiWriteHeaders(w)
	w.WriteHeader(status)

	enc := gojay.BorrowEncoder(w)
	defer enc.Release()

	enc.AppendByte('{')
	enc.AddIntKey("status", status)
	enc.AddStringKey("msg", msg)
	enc.AddStringKey("origin", origin)
	enc.AddEmbeddedJSONKeyOmitEmpty("more", (*gojay.EmbeddedJSON)(&payload))
	enc.AppendByte('}')
	enc.Write()
}

// smartError checks if the request needs a JSON or HTML response and calls the right error function.
func smartError(w http.ResponseWriter, r *http.Request, msg string, status int) {
	if strings.HasPrefix(r.Header.Get("Accept"), "application/json") {
		jsonError(w, msg, status)
		return
	}
	http.Error(w, msg, status)
}

// ifGet is a convenience function that serves http requests only if the method is GET.
func ifGet(w http.ResponseWriter, r *http.Request, h http.HandlerFunc) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	h(w, r)
}

// checkMethod returns true if the request matches the given HTTP method,
// handles the OPTIONS method, or responds with a MethodNotAllowed error.
//
// This function is meant for handlers that accept only one HTTP method.
func checkMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}

	if r.Method == http.MethodOptions {
		apiWriteOptions(w, "OPTIONS, "+method)
		return false
	}

	//http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	jsonError(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	return false
}

func apiVersion(w http.ResponseWriter, r *http.Request) {
	apiWriteHeaders(w)
	w.Header().Set("ETag", `"`+version.CommitHash+`"`)

	enc := gojay.BorrowEncoder(w)
	defer enc.Release()

	enc.AppendByte('{')
	enc.AddStringKey("name", version.Name)
	enc.AddStringKey("description", version.Description)
	enc.AddStringKey("version", version.SemVer)
	enc.AddStringKey("tag", version.CommitTag)
	enc.AddStringKey("hash", version.CommitHash)
	enc.AddStringKey("repo", version.CommitRepo)
	enc.AppendByte('}')
	enc.Write()
}

// dbError handles database errors. It returns more specific API messages for predefined errors
// that might be relevant for the user. Other errors return `database error` with a 500 status code.
func dbError(w http.ResponseWriter, err error) bool {
	switch err {
	case nil:
		return false
	// meta
	case psql.ErrExists:
		jsonError(w, "resource exists already", http.StatusConflict)
	case psql.ErrNotFound:
		jsonError(w, "resource not found", http.StatusNotFound)
	case psql.ErrNotOwner:
		jsonError(w, "not resource owner", http.StatusForbidden)
	case psql.ErrInvalidJson:
		jsonError(w, "invalid input", http.StatusBadRequest)
	// connection
	case psql.ErrConnection:
		jsonError(w, "no database connection", http.StatusServiceUnavailable)
	case psql.ErrTimeout:
		jsonError(w, "database timeout", http.StatusServiceUnavailable)
	case psql.ErrTemporary:
		jsonError(w, "temporary database error", http.StatusServiceUnavailable)
	// generic
	default:
		jsonError(w, "database error", http.StatusInternalServerError)
	}
	return true
}

// sessionError handles session errors by returning appropriate HTTP status codes.
func sessionError(w http.ResponseWriter, err error) bool {
	switch err {
	case nil:
		return false
	// session errors
	case sessions.ErrSessionNotFound:
		jsonError(w, err.Error(), http.StatusUnauthorized)
	case sessions.ErrCreatingSid:
		jsonError(w, err.Error(), http.StatusInternalServerError)
	case sessions.ErrUnknownUser:
		jsonError(w, err.Error(), http.StatusServiceUnavailable)
	// catch-all
	default:
		jsonError(w, err.Error(), http.StatusInternalServerError)
	}
	return true
}

// convertExternalStatusCode tries to convert a status code from an eternal service to one this application can provide.
func convertExternalStatusCode(code int) int {
	switch {
	case code >= 300 && code < 400:
		return 200
	case code == 401 || code == 403:
		return 500
	case code == 500:
		return 502
	case code == 503:
		return 504
	default:
		return code
	}
}

func ShiftPath(p string) (head, tail string) {
	if p == "" {
		return "", "/"
	}
	p = strings.TrimPrefix(path.Clean(p), "/")
	i := strings.Index(p, "/")
	if i < 0 {
		return p, "/"
	}
	return p[:i], p[i:]
}

func ShiftUrlWithTrailing(r *http.Request) (head string) {
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/")
	i := strings.Index(r.URL.Path, "/")
	if i < 0 {
		head = r.URL.Path
		r.URL.Path = ""
		return
	}
	head = r.URL.Path[:i+1]
	r.URL.Path = r.URL.Path[i:]
	return
}

func ShiftUrl(r *http.Request) (head string) {
	head, r.URL.Path = ShiftPath(r.URL.Path)
	return
}

func StripPrefix(prefix string, r *http.Request) {
	if r.URL.Path == "" {
		return
	}

	r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
}

func HasSubroutes(head string) bool {
	return strings.HasSuffix(head, "/")
}

func GetStringParam(head string) string {
	if strings.HasSuffix(head, "/") {
		return head[:len(head)-1]
	}
	return head
}

func GetUuidParam(head string) (uuid.UUID, error) {
	return uuid.FromString(GetStringParam(head))
}

func TrimSlash(s string) string {
	return strings.TrimRight(s, "/")
}
