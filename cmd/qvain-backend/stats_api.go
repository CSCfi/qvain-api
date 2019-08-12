package main

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/CSCfi/qvain-api/internal/psql"

	"github.com/francoispqt/gojay"
	"github.com/rs/zerolog"
)

// StatsApi provides statistics for Qvain.
type StatsApi struct {
	db         *psql.DB
	logger     zerolog.Logger
	identity   string
	apiKey     string
	requireKey bool
}

// NewStatsApi creates a new StatsApi.
func NewStatsApi(db *psql.DB, logger zerolog.Logger, apiKey string, requireKey bool) *StatsApi {
	return &StatsApi{
		db:         db,
		logger:     logger,
		identity:   DefaultIdentity,
		apiKey:     apiKey,
		requireKey: requireKey,
	}
}

func (api *StatsApi) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if api.apiKey == "" && api.requireKey {
		api.logger.Error().Msg("missing api key")
		jsonError(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	key := r.URL.Query().Get("key")
	if key != api.apiKey {
		api.logger.Error().Msg("invalid api key")
		jsonError(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	head := ShiftUrlWithTrailing(r)
	api.logger.Debug().Str("head", head).Str("path", r.URL.Path).Str("method", r.Method).Msg("stats")

	if r.Method != http.MethodGet {
		jsonError(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	if head == "datasets" {
		api.Datasets(w, r)
		return
	}

	jsonError(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
}

func getDatasetFilter(query url.Values) (*psql.DatasetFilter, []string) {
	parser := NewQueryParser(query)
	filter := &psql.DatasetFilter{
		OnlyDrafts:    parser.Flag("only_drafts"),
		OnlyPublished: parser.Flag("only_published"),
		OnlyAtt:       parser.Flag("only_att"),
		OnlyIda:       parser.Flag("only_ida"),
		DateCreated:   parser.TimeFilters("date_created"),
		User:          parser.String("user_created"),
		Organization:  parser.String("organization"),
		GroupBy:       parser.StringOption("group_by", psql.DatasetFilterGroupByPaths),
	}
	parser.Skip("key")
	return filter, parser.Validate()
}

// Datasets provides dataset counts.
func (api *StatsApi) Datasets(w http.ResponseWriter, r *http.Request) {
	filter, invalidParams := getDatasetFilter(r.URL.Query())

	apiWriteHeaders(w)
	enc := gojay.BorrowEncoder(w)
	defer enc.Release()

	if len(invalidParams) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		enc.AppendByte('{')
		enc.AddStringKey("error", "invalid values: "+strings.Join(invalidParams, ","))
		enc.AppendByte('}')
		enc.Write()
		return
	}

	result, err := api.db.CountDatasets(filter)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		enc.AppendByte('{')
		enc.AddStringKey("error", "an error occurred")
		enc.AppendByte('}')
		enc.Write()
		return
	}
	w.WriteHeader(http.StatusOK)
	enc.AppendBytes(result)
	enc.Write()
}
