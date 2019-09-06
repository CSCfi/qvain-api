package main

import (
	"encoding/json"
	"net/http"

	"github.com/CSCfi/qvain-api/internal/psql"
	"github.com/rs/zerolog"
)

// LookupApi holds the configuration for the identifier lookup service.
type LookupApi struct {
	db     *psql.DB
	logger zerolog.Logger
	apiKey string
}

// NewLookupApi sets up a dataset lookup service.
func NewLookupApi(db *psql.DB, logger zerolog.Logger, apiKey string) *LookupApi {
	return &LookupApi{
		db:     db,
		logger: logger,
		apiKey: apiKey,
	}
}

// ServeHTTP is the main entry point for the Lookup API.
func (api *LookupApi) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	head := ShiftUrlWithTrailing(r)
	api.logger.Debug().Str("head", head).Str("path", r.URL.Path).Str("method", r.Method).Msg("lookup")

	// api for services
	key := r.URL.Query().Get("key")
	if key != api.apiKey {
		jsonError(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		api.logger.Error().Msg("invalid api key")
		return
	}

	if r.Method == http.MethodGet {
		if head == "dataset" {
			api.Dataset(w, r)
			return
		}
	}

	jsonError(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
}

// Dataset gets information for a single dataset.
func (api *LookupApi) Dataset(w http.ResponseWriter, r *http.Request) {
	hasTrailing := r.URL.Path == "/"
	head := ShiftUrlWithTrailing(r)
	api.logger.Debug().Bool("hasTrailing", hasTrailing).Str("path", r.URL.Path)

	if head != "" || hasTrailing {
		jsonError(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	id := r.URL.Query().Get("id")                 // qvain id of the dataset
	identifier := r.URL.Query().Get("identifier") // external (Metax) identifier of the dataset

	if id == "" && identifier == "" {
		jsonError(w, "missing either 'id' or 'identifier' in query", http.StatusBadRequest)
		return
	}

	if id != "" && identifier != "" {
		jsonError(w, "both 'id' and 'identifier' in query", http.StatusBadRequest)
		return
	}

	var (
		res json.RawMessage
		err error
	)
	if id != "" {
		res, err = api.db.ViewDatasetInfoByIdentifier("id", id)
	} else if identifier != "" {
		res, err = api.db.ViewDatasetInfoByIdentifier("identifier", identifier)
	}
	if err != nil {
		dbError(w, err)
		api.logger.Error().Msg("error retrieving dataset info")
		return
	}

	apiWriteHeaders(w)
	w.Write(res)
}
