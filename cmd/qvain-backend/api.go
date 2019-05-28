package main

import (
	"expvar"
	"net/http"
	"strings"

	"github.com/CSCfi/qvain-api/pkg/metax"
	"github.com/rs/zerolog"
)

// Root configures a http.Handler for routing HTTP requests to the root URL.
func Root(config *Config) http.Handler {
	apis := NewApis(config)
	apiHandler := http.Handler(apis)
	if config.LogRequests {
		// wrap apiHandler with request logging middleware
		apiHandler = makeLoggingHandler("/api", apiHandler, config.NewLogger("request"))
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch ShiftUrlWithTrailing(r) {
		case "api/":
			apiHandler.ServeHTTP(w, r)
		case "":
			ifGet(w, r, welcome)
		default:
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		}
		return
	})
}

// Apis holds configured API endpoints.
type Apis struct {
	config *Config
	logger zerolog.Logger

	datasets *DatasetApi
	sessions *SessionApi
	auth     *AuthApi
	proxy    *ApiProxy
	lookup   *LookupApi
}

// NewApis constructs a collection of APIs with a given configuration.
func NewApis(config *Config) *Apis {
	apis := &Apis{
		config: config,
		logger: config.NewLogger("apis"),
	}

	metax := metax.NewMetaxService(config.MetaxApiHost,
		metax.WithCredentials(config.metaxApiUser, config.metaxApiPass),
		metax.WithInsecureCertificates(config.DevMode))

	apis.datasets = NewDatasetApi(config.db, config.sessions, metax, config.NewLogger("datasets"))
	apis.sessions = NewSessionApi(config.sessions, config.NewLogger("sessions"))
	apis.auth = NewAuthApi(config, makeOnFairdataLogin(metax, config.db, config.NewLogger("sync")), config.NewLogger("auth"))
	apis.proxy = NewApiProxy(
		"https://"+config.MetaxApiHost+"/rest/",
		config.metaxApiUser,
		config.metaxApiPass,
		config.sessions,
		config.NewLogger("proxy"),
	)
	apis.lookup = NewLookupApi(config.db)

	return apis
}

// ServeHTTP is a http.Handler that delegates to the requested API endpoint.
func (apis *Apis) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	head := ShiftUrlWithTrailing(r)
	apis.logger.Debug().Str("head", head).Str("path", r.URL.Path).Msg("apis")

	switch head {
	case "datasets/":
		datasetsC.Add(1)
		apis.datasets.ServeHTTP(w, r)
	case "sessions/":
		sessionsC.Add(1)
		apis.sessions.ServeHTTP(w, r)
	case "auth/":
		authC.Add(1)
		apis.auth.ServeHTTP(w, r)
	case "proxy/":
		proxyC.Add(1)
		// only allow access to /directories and /files; path has been cleaned by Go on instantiation
		// TODO: make prefix filter in proxy package?
		if strings.HasPrefix(r.URL.Path, "/directories/") || strings.HasPrefix(r.URL.Path, "/files/") {
			apis.proxy.ServeHTTP(w, r)
		} else {
			jsonError(w, "access denied", http.StatusForbidden)
		}
	case "lookup/":
		lookupC.Add(1)
		apis.lookup.ServeHTTP(w, r)
	case "version":
		versionC.Add(1)
		ifGet(w, r, apiVersion)
	case "vars":
		expvar.Handler().ServeHTTP(w, r)
	case "":
		ifGet(w, r, welcome)
	default:
		jsonError(w, "unknown api called: "+TrimSlash(head), http.StatusNotFound)
	}
}
