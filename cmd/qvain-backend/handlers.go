package main

import (
	"fmt"
	"net/http"

	"github.com/NatLibFi/qvain-api/jwt"
	"github.com/NatLibFi/qvain-api/version"
	"github.com/felixge/httpsnoop"
	"github.com/rs/zerolog"
)

func welcome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Write([]byte("Welcome to the Qvain API server.\n"))
}

func myVersion(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "qvain %s at tag %s hash %s\n", version.SemVer, version.CommitTag, version.CommitHash)
}

func protected(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Welcome to a protected url.\n"))
}

func serveApp(w http.ResponseWriter, req *http.Request) {
}

// makeLoggingHandler takes a handler and logger and then wraps the given handler with request logging middleware.
func makeLoggingHandler(wrapped http.Handler, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := httpsnoop.CaptureMetrics(wrapped, w, r)

		var uid string
		if jwt, ok := jwt.FromContext(r.Context()); ok {
			uid = jwt.Subject()
		}

		logger.Log().Str("method", r.Method).Str("url", r.URL.String()).Int("status", h.Code).Dur("⌛", h.Duration).Str("Δt", h.Duration.String()).Int64("written", h.Written).Str("user", uid).Msg("request")
	})
}

// LoggingHandler wraps a handler with request logging middleware.
/*
func LoggingHandler(wrapped http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := httpsnoop.CaptureMetrics(wrapped, w, r)
		//_ = m
		logger.Log().Str("method", r.Method).Str("url", r.URL.String()).Int("status", h.Code).Dur("⌛", h.Duration).Str("Δt", h.Duration.String()).Int64("written", h.Written).Msg("request")
	})
}
*/
