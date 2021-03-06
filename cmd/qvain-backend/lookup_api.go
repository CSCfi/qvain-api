package main

import (
	"encoding/json"
	"net/http"

	"github.com/CSCfi/qvain-api/internal/psql"
	"github.com/rs/zerolog"
	"github.com/wvh/uuid"
)

// LookupApi holds the configuration for the lookup API.
type LookupApi struct {
	db     *psql.DB
	logger zerolog.Logger
	apiKey string
}

// NewLookupApi sets up a lookup API.
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
	key := r.Header.Get("x-api-key")
	if key != api.apiKey {
		loggedJSONError(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized, &api.logger).Msg("invalid api key")
		return
	}

	if r.Method == http.MethodGet {
		if head == "dataset" {
			api.Dataset(w, r)
			return
		}
		loggedJSONError(w, http.StatusText(http.StatusNotFound), http.StatusNotFound, &api.logger).Msg("invalid lookup path")
		return
	}

	loggedJSONError(w, http.StatusText(http.StatusNotFound), http.StatusNotFound, &api.logger).Msg("Invalid method")
}

// Dataset retrieves information for a single dataset.
func (api *LookupApi) Dataset(w http.ResponseWriter, r *http.Request) {
	hasTrailing := r.URL.Path == "/"
	head := ShiftUrlWithTrailing(r)
	api.logger.Debug().Bool("hasTrailing", hasTrailing).Str("path", r.URL.Path)

	if head != "" || hasTrailing {
		loggedJSONError(w, http.StatusText(http.StatusNotFound), http.StatusNotFound, &api.logger).Msg("Unhandled request")
		return
	}

	qvainId := r.URL.Query().Get("qvain_id") // qvain id of the dataset
	metaxId := r.URL.Query().Get("metax_id") // metax identifier of the dataset

	if qvainId == "" && metaxId == "" {
		loggedJSONError(w, "either 'qvain_id' or 'metax_id' required in query", http.StatusBadRequest, &api.logger).Msg("Lookup")
		return
	}

	if qvainId != "" && metaxId != "" {
		loggedJSONError(w, "both 'qvain_id' and 'metax_id' in query", http.StatusBadRequest, &api.logger).Msg("Lookup")
		return
	}

	var (
		res json.RawMessage
		err error
	)
	if qvainId != "" {
		if _, err = uuid.FromString(qvainId); err != nil { // avoid db error on invalid uuid
			loggedJSONError(w, "invalid dataset id", http.StatusBadRequest, &api.logger).Str("qvain_id", qvainId).Msg("invalid dataset id")
			return
		}
		res, err = api.db.ViewDatasetInfoByIdentifier("id", qvainId)
	} else if metaxId != "" {
		res, err = api.db.ViewDatasetInfoByIdentifier("identifier", metaxId)
	}
	if err != nil {
		dbError(w, err, &api.logger).Msg("error retrieving dataset info")
		return
	}

	apiWriteHeaders(w)
	w.Write(res)
}
