package main

import (
	"net/http"

	"github.com/CSCfi/qvain-api/internal/sessions"
	"github.com/francoispqt/gojay"
	"github.com/rs/zerolog"
)

// SessionApi allows users to access their session information.
type SessionApi struct {
	sessions *sessions.Manager
	logger   zerolog.Logger
}

// NewSessionApi creates a new SessionApi.
func NewSessionApi(sessions *sessions.Manager, logger zerolog.Logger) *SessionApi {
	return &SessionApi{sessions: sessions}
}

// Current dumps the (public) data from the current session in json format to the response.
func (api *SessionApi) Current(w http.ResponseWriter, r *http.Request) {
	session, err := api.sessions.SessionFromRequest(r)
	if err != nil {
		api.logger.Debug().Err(err).Msg("no current session")
		sessionError(w, sessions.ErrSessionNotFound)
		return
	}

	enc := gojay.BorrowEncoder(w)
	defer enc.Release()

	apiWriteHeaders(w)
	err = enc.EncodeObject(session.Public())
	if err != nil {
		api.logger.Error().Err(err).Msg("failed to encode public session")
		jsonError(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}

// Logout deletes the current user session and returns a json response.
func (api *SessionApi) Logout(w http.ResponseWriter, r *http.Request) {
	sid, err := sessions.GetSessionCookie(r)
	if err != nil {
		api.logger.Debug().Err(err).Msg("no session cookie found")
		sessionError(w, sessions.ErrSessionNotFound)
		return
	}
	success := api.sessions.DestroyWithCookie(w, sid)
	if !success {
		api.logger.Debug().Msg("failed to destroy session")
		sessionError(w, sessions.ErrSessionNotFound)
		return
	}

	apiWriteHeaders(w)
	w.WriteHeader(http.StatusOK)

	enc := gojay.BorrowEncoder(w)
	defer enc.Release()

	enc.AppendByte('{')
	enc.AddStringKey("msg", "User logged out succesfully")
	enc.AppendByte('}')
	enc.Write()
}

// ServeHTTP satisfies the http.Handler interface; it is the main endpoint for the session api.
func (api *SessionApi) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	api.logger.Debug().Str("path", r.URL.Path).Msg("request path")
	head := ShiftUrlWithTrailing(r)

	switch head {
	case "":
		switch r.Method {
		case http.MethodGet:
			api.Current(w, r)
		case http.MethodOptions:
			apiWriteOptions(w, "GET, OPTIONS")
		default:
			jsonError(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	case "logout":
		switch r.Method {
		case http.MethodPost:
			api.Logout(w, r)
		case http.MethodOptions:
			apiWriteOptions(w, "POST, OPTIONS")
		default:
			jsonError(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}
}
