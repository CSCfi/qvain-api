package main

import (
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/CSCfi/qvain-api/internal/psql"
	"github.com/CSCfi/qvain-api/internal/sessions"
	"github.com/CSCfi/qvain-api/internal/version"

	"github.com/francoispqt/gojay"
	"github.com/rs/zerolog"
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
// Also logs error message to backend terminal
func dbError(w http.ResponseWriter, err error, logger *zerolog.Logger) *zerolog.Event {
	switch err {
	case nil:
		return nil
	// meta
	case psql.ErrExists:
		return loggedJSONError(w, "resource exists already", http.StatusConflict, logger)
	case psql.ErrNotFound:
		return loggedJSONError(w, "resource not found", http.StatusNotFound, logger)
	case psql.ErrNotOwner:
		return loggedJSONError(w, "not resource owner", http.StatusForbidden, logger)
	case psql.ErrInvalidJson:
		return loggedJSONError(w, "invalid input", http.StatusBadRequest, logger)
	// connection
	case psql.ErrConnection:
		return loggedJSONError(w, "no database connection", http.StatusServiceUnavailable, logger)
	case psql.ErrTimeout:
		return loggedJSONError(w, "database timeout", http.StatusServiceUnavailable, logger)
	case psql.ErrTemporary:
		return loggedJSONError(w, "temporary database error", http.StatusServiceUnavailable, logger)
	// generic
	default:
		return loggedJSONError(w, "database error", http.StatusInternalServerError, logger)
	}
}

// sessionError handles session errors by returning appropriate HTTP status codes and logging into backend terminal.
func sessionError(w http.ResponseWriter, err error, logger *zerolog.Logger) *zerolog.Event {
	switch err {
	case nil:
		return nil
	// session errors
	case sessions.ErrSessionNotFound:
		return loggedJSONError(w, err.Error(), http.StatusUnauthorized, logger)
	case sessions.ErrCreatingSid:
		return loggedJSONError(w, err.Error(), http.StatusInternalServerError, logger)
	case sessions.ErrUnknownUser:
		return loggedJSONError(w, err.Error(), http.StatusServiceUnavailable, logger)
	// catch-all
	default:
		return loggedJSONError(w, err.Error(), http.StatusInternalServerError, logger)
	}
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

// QueryParser provides helper functions for converting query parameters into Go types.
type QueryParser struct {
	query         url.Values
	checkedParams map[string]bool
	invalidParams []string
}

// NewQueryParser creates a new QueryParser for a query.
func NewQueryParser(query url.Values) *QueryParser {
	return &QueryParser{
		query:         query,
		checkedParams: map[string]bool{},
		invalidParams: make([]string, 0),
	}
}

// Flag returns true when param is "true" or is present but has no value.
func (q *QueryParser) Flag(param string) bool {
	q.checkedParams[param] = true
	val, exists := q.query[param]
	if !exists {
		return false
	}
	if val[0] == "" || val[0] == "true" {
		return true
	}

	q.invalidParams = append(q.invalidParams, param+"="+val[0])
	return false
}

// TimeFilters converts parameters with a suffix and a (optionally truncated) RFC3339 time value into
// an TimeFilter array representing time comparisons. See psql.ParseTimeFilter for further information.
func (q *QueryParser) TimeFilters(param string) (filters []psql.TimeFilter) {
	for suffix := range psql.ComparisonSuffixes {
		q.checkedParams[param+suffix] = true
		val, exists := q.query[param+suffix]
		if !exists {
			continue
		}

		if filter := psql.ParseTimeFilter(suffix, val[0]); !filter.IsZero() {
			filters = append(filters, filter)
		} else {
			q.invalidParams = append(q.invalidParams, param+suffix+"="+val[0])
		}
	}

	return filters
}

// String returns a string parameter.
func (q *QueryParser) String(param string) string {
	q.checkedParams[param] = true
	val, exists := q.query[param]
	if !exists {
		return ""
	}
	return val[0]
}

// StringOption returns the string parameter only if it is a key in the options map.
func (q *QueryParser) StringOption(param string, options map[string]string) string {
	q.checkedParams[param] = true
	val, exists := q.query[param]
	if !exists {
		return ""
	}
	_, isKey := options[val[0]]
	if !isKey {
		q.invalidParams = append(q.invalidParams, param+"="+val[0])
		return ""
	}
	return val[0]
}

// Skip marks parameter as used but ignores its value.
func (q *QueryParser) Skip(param string) {
	q.checkedParams[param] = true
}

// Validate returns query parameters that either:
// - have an invalid value
// - have multiple values
// - are unused
func (q *QueryParser) Validate() (invalidParams []string) {
	for param, values := range q.query {
		if !q.checkedParams[param] {
			q.invalidParams = append(q.invalidParams, param+" (unknown parameter)")
		} else if len(values) > 1 {
			q.invalidParams = append(q.invalidParams, param+" (multiple values)")
		}
	}
	return q.invalidParams
}

// loggedJSONError creates a new error UUID and writes an error API response. Use the chaining methods
// of the returned zerolog event to add more error context and finally call its Msg method to log the error.
func loggedJSONError(w http.ResponseWriter, msg string, status int, logger *zerolog.Logger) *zerolog.Event {
	generatedErrorID := uuid.MustNewUUID().String()
	apiWriteHeaders(w)
	w.WriteHeader(status)

	enc := gojay.BorrowEncoder(w)
	defer enc.Release()

	enc.AppendByte('{')
	enc.AddIntKey("status", status)
	enc.AddStringKey("msg", msg)
	enc.AddStringKey("error_id", generatedErrorID)
	enc.AppendByte('}')
	enc.Write()

	return logger.Error().Str("errorId ", generatedErrorID)
}

// loggedJSONErrorWithPayload writes an error API response like loggedJsonError, but allows adding a source and extra (pre-serialised) json value.
func loggedJSONErrorWithPayload(w http.ResponseWriter, msg string, status int, logger *zerolog.Logger, origin string, payload []byte) *zerolog.Event {
	generatedErrorID := uuid.MustNewUUID().String()
	apiWriteHeaders(w)
	w.WriteHeader(status)

	enc := gojay.BorrowEncoder(w)
	defer enc.Release()

	enc.AppendByte('{')
	enc.AddIntKey("status", status)
	enc.AddStringKey("msg", msg)
	enc.AddStringKey("error_id", generatedErrorID)
	enc.AddStringKey("origin", origin)
	enc.AddEmbeddedJSONKeyOmitEmpty("more", (*gojay.EmbeddedJSON)(&payload))
	enc.AppendByte('}')
	enc.Write()

	return logger.Error().Str("errorId ", generatedErrorID).Str("origin", origin)
}
