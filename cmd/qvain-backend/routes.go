package main

import (
	"net/http"

	"github.com/CSCfi/qvain-api/internal/oidc"
)

// makeMux sets up the default handlers and returns a mux that can also be used for testing.
func makeMux(config *Config) *http.ServeMux {
	mux := http.NewServeMux()

	// static endpoints
	mux.HandleFunc("/", welcome)

	// api endpoint
	mux.HandleFunc("/api", apiHello)
	mux.HandleFunc("/api/", apiHello)

	// api endpoint, show version
	mux.HandleFunc("/api/version", apiVersion)

	// api endpoint, database check
	mux.Handle("/api/db", apiDatabaseCheck(config.db))

	// OIDC client
	oidcLogger := config.NewLogger("oidc")
	oidcClient, err := oidc.NewOidcClient(
		config.oidcProviderName,
		config.oidcClientID,
		config.oidcClientSecret,
		"https://"+config.Hostname+"/api/auth/cb",
		config.oidcProviderUrl,
		"/login",
	)
	if err != nil {
		oidcLogger.Error().Err(err).Msg("oidc configuration failed")
	} else {
		oidcClient.SetLogger(oidcLogger)
		oidcClient.OnLogin = MakeSessionHandlerForFairdata(config.sessions, config.db, nil, config.Logger, "fd")
		mux.HandleFunc("/api/auth/login", oidcClient.Auth())
		mux.HandleFunc("/api/auth/cb", oidcClient.Callback())
	}

	return mux
}
