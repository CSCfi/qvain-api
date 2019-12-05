package main

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/CSCfi/qvain-api/internal/psql"
	"github.com/CSCfi/qvain-api/internal/sessions"
	"github.com/CSCfi/qvain-api/internal/shared"
	"github.com/CSCfi/qvain-api/pkg/metax"
	"github.com/CSCfi/qvain-api/pkg/models"
	"github.com/tidwall/gjson"

	"github.com/francoispqt/gojay"
	"github.com/rs/zerolog"
	"github.com/CSCfi/qvain-api/uuid"
)

// DefaultIdentity is the user identity to show to the outside world.
const DefaultIdentity = "fairdata"

type DatasetApi struct {
	db       *psql.DB
	sessions *sessions.Manager
	metax    *metax.MetaxService
	logger   zerolog.Logger

	identity string
}

func NewDatasetApi(db *psql.DB, sessions *sessions.Manager, metax *metax.MetaxService, logger zerolog.Logger) *DatasetApi {
	return &DatasetApi{
		db:       db,
		sessions: sessions,
		metax:    metax,
		logger:   logger,
		identity: DefaultIdentity,
	}
}

// SetIdentity sets the identity to show to the outside world.
// It is not safe to call this method after instantiation.
func (api *DatasetApi) SetIdentity(identity string) {
	api.identity = identity
}

func (api *DatasetApi) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// authenticated api
	session, err := api.sessions.SessionFromRequest(r)
	if err != nil {
		loggedJSONError(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized, &api.logger).Err(err).Msg("no session from request")
		return
	}

	user := session.User

	head := ShiftUrlWithTrailing(r)
	api.logger.Debug().Str("head", head).Str("path", r.URL.Path).Str("method", r.Method).Msg("datasets")

	// root
	if head == "" {
		// handle self
		switch r.Method {
		case http.MethodGet:
			api.ListDatasets(w, r, user)
		case http.MethodPost:
			api.createDataset(w, r, user)
		case http.MethodOptions:
			apiWriteOptions(w, "GET, POST, OPTIONS")
			return
		default:
			loggedJSONError(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed, &api.logger).Str("head", head).Msg("Invalid head value in api call")
		}
		return
	}

	// dataset uuid
	id, err := GetUuidParam(head)
	if err != nil {
		loggedJSONError(w, "bad format for uuid path parameter", http.StatusBadRequest, &api.logger).Err(err).Str("head", head).Msg("failed getting dataset uuid")
		return
	}

	// delegate to dataset handler
	api.Dataset(w, r, user, id)
}

func (api *DatasetApi) ListDatasets(w http.ResponseWriter, r *http.Request, user *models.User) {
	switch r.URL.RawQuery {
	case "":
	case "fetch":
		api.logger.Debug().Str("op", "fetch").Msg("datasets")
		err := shared.Fetch(api.metax, api.db, api.logger, user.Uid, user.Identity)
		if err != nil {
			// TODO: handle mixed error
			loggedJSONError(w, err.Error(), http.StatusBadRequest, &api.logger).Err(err).Msg("Listing dataset failed")
			//dbError(w, err)
			return
		}
	case "fetchall":
		api.logger.Debug().Str("op", "fetchall").Msg("datasets")
		shared.FetchAll(api.metax, api.db, api.logger, user.Uid, user.Identity)
	default:
		loggedJSONError(w, "invalid parameter", http.StatusBadRequest, &api.logger).Msg("Unhandled parameter")
		return
	}

	jsondata, err := api.db.ViewDatasetsByOwner(user.Uid)
	if err != nil {
		dbError(w, err, &api.logger).Err(err).Str("uid", user.Uid.String()).Msg("ViewDatasetsByOwner failed")
		return
	}

	apiWriteHeaders(w)
	//w.Header().Set("Cache-Control", "private, max-age=300")
	w.Write(jsondata)
}

// Dataset handles requests for a dataset by UUID. It dispatches to request method specific handlers.
func (api *DatasetApi) Dataset(w http.ResponseWriter, r *http.Request, user *models.User, id uuid.UUID) {
	api.logger.Debug().Str("head", "").Str("path", r.URL.Path).Msg("dataset")
	hasTrailing := r.URL.Path == "/"
	op := ShiftUrlWithTrailing(r)
	api.logger.Debug().Bool("hasTrailing", hasTrailing).Str("head", op).Str("path", r.URL.Path).Str("dataset", id.String()).Msg("dataset")

	// root; don't accept trailing
	if op == "" && !hasTrailing {
		// handle self
		switch r.Method {
		case http.MethodGet:
			api.getDataset(w, r, user.Uid, id, "")
			return
		case http.MethodPut:
			api.updateDataset(w, r, user, id)
			return
		case http.MethodDelete:
			api.deleteDataset(w, r, user.Uid, id)
			return
		case http.MethodOptions:
			apiWriteOptions(w, "GET, PUT, DELETE, OPTIONS")
			return

		default:
			loggedJSONError(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed, &api.logger).Msg("Unhandled request method")
			return
		}
	}

	// dataset operations
	switch op {
	case "export":
		// TODO: assess security implementations before enabling this
		loggedJSONError(w, "export not implemented", http.StatusNotImplemented, &api.logger).Msg("export not implemented")
		return
	case "versions":
		if checkMethod(w, r, http.MethodGet) {
			api.ListVersions(w, r, user.Uid, id)
		}
		return
	case "publish":
		if checkMethod(w, r, http.MethodPost) {
			api.publishDataset(w, r, user, id)
		}
		return
	case "change_cumulative_state":
		if checkMethod(w, r, http.MethodPost) {
			api.changeDatasetCumulativeState(w, r, user, id)
		}
		return
	case "refresh_directory_content":
		if checkMethod(w, r, http.MethodPost) {
			api.refreshDatasetDirectoryContent(w, r, user, id)
		}
		return
	default:
		loggedJSONError(w, "invalid dataset operation", http.StatusNotFound, &api.logger).Msg("Unhandled dataset operation")
		return
	}
}

// getDataset retrieves a dataset's whole blob or part thereof depending on the path.
// Not all datasets are fully viewable through the API.
func (api *DatasetApi) getDataset(w http.ResponseWriter, r *http.Request, owner uuid.UUID, id uuid.UUID, path string) {
	// whole dataset is not visible through this API
	// TODO: plug super-user check
	if false && path == "" {
		jsonError(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	res, err := api.db.ViewDatasetWithOwner(id, owner, api.identity)
	if err != nil {
		dbError(w, err, &api.logger).Err(err).Str("dataset", id.String()).Str("user", owner.String()).Msg("retrieval of dataset failed")
		return
	}

	apiWriteHeaders(w)
	w.Write(res)
}

func (api *DatasetApi) createDataset(w http.ResponseWriter, r *http.Request, creator *models.User) {
	var err error

	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		loggedJSONError(w, http.StatusText(http.StatusUnsupportedMediaType), http.StatusUnsupportedMediaType, &api.logger).Msg("Unsupported content-type")
		return
	}

	if r.Body == nil || r.Body == http.NoBody {
		loggedJSONError(w, "empty body", http.StatusBadRequest, &api.logger).Msg("Dataset creation failed due to empty request body")
		return
	}

	defer r.Body.Close()

	extra := map[string]string{
		"identity": creator.Identity,
		"org":      creator.Organisation,
	}

	bodyBytes, _ := ioutil.ReadAll(r.Body)
	r.Body.Close()
	r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	if cumulativeState := gjson.GetBytes(bodyBytes, "cumulative_state").String(); cumulativeState != "" {
		extra["cumulative_state"] = cumulativeState
	}

	typed, err := models.CreateDatasetFromJson(creator.Uid, r.Body, extra)
	if err != nil {
		loggedJSONError(w, err.Error(), http.StatusBadRequest, &api.logger).Err(err).Msg("Dataset creation failed from JSON")
		return
	}

	if typed.Unwrap().Family() == metax.MetaxDatasetFamily {
		metaxDataset := &metax.MetaxDataset{Dataset: typed.Unwrap()}
		err = metaxDataset.ValidateCreated()
		if err != nil {
			loggedJSONError(w, err.Error(), http.StatusBadRequest, &api.logger).Err(err).Msg("Dataset validation failed")
			return
		}
	}

	err = api.db.Create(typed.Unwrap())
	if err != nil {
		dbError(w, err, &api.logger).Err(err).Msg("Creation of dataset failed")
		return
	}

	api.Created(w, r, typed.Unwrap().Id)
}

func (api *DatasetApi) updateDataset(w http.ResponseWriter, r *http.Request, owner *models.User, id uuid.UUID) {
	var err error

	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		loggedJSONError(w, http.StatusText(http.StatusUnsupportedMediaType), http.StatusUnsupportedMediaType, &api.logger).Str("dataset id", id.String()).Msg("Update dataset failed")
		return
	}

	if r.Body == nil || r.Body == http.NoBody {
		loggedJSONError(w, "empty body", http.StatusBadRequest, &api.logger).Str("dataset id", id.String()).Msg("Update Dataset failed")
		return
	}

	defer r.Body.Close()

	extra := map[string]string{}

	bodyBytes, _ := ioutil.ReadAll(r.Body)
	r.Body.Close()
	r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	if cumulativeState := gjson.GetBytes(bodyBytes, "cumulative_state").String(); cumulativeState != "" {
		extra["cumulative_state"] = cumulativeState
	}

	typed, err := models.UpdateDatasetFromJson(owner.Uid, r.Body, extra)
	if err != nil {
		loggedJSONError(w, err.Error(), http.StatusBadRequest, &api.logger).Err(err).Str("dataset", id.String()).Str("user", owner.Uid.String()).Msg("update dataset failed")
		return
	}

	api.logger.Debug().Str("json", string(typed.Unwrap().Blob())).Msg("new json")

	api.logger.Debug().Str("owner", owner.Uid.String()).Msg("owner")

	// perform checks on the updated dataset before saving it
	dataset, err := api.db.GetWithOwner(id, owner.Uid)
	if err != nil {
		dbError(w, err, &api.logger).Err(err).Str("Dataset id", id.String()).Str("Owner Uid", owner.Uid.String()).Msg("Update dataset failed")
		return
	}

	updated := typed.Unwrap()
	updated.Published = dataset.Published // get Published from the original so validation handles it properly

	if dataset.Family() == metax.MetaxDatasetFamily {
		metaxDataset := &metax.MetaxDataset{Dataset: dataset}
		err = metaxDataset.ValidateUpdated(updated)
		if err != nil {
			loggedJSONError(w, err.Error(), http.StatusBadRequest, &api.logger).Err(err).Msg("Updated dataset validation failed")
			return
		}
	}

	err = api.db.SmartUpdateWithOwner(id, typed.Unwrap().Blob(), owner.Uid)
	if err != nil {
		dbError(w, err, &api.logger).Err(err).Str("dataset", id.String()).Str("user", owner.Uid.String()).Msg("SmartUpdateWithOwner failed")
		return
	}

	api.Created(w, r, typed.Unwrap().Id)
}

func (api *DatasetApi) handlePublishError(w http.ResponseWriter, ownerId uuid.UUID, id uuid.UUID, err error) {
	switch t := err.(type) {
	case *metax.ApiError:
		api.logger.Warn().Err(err).Str("dataset", id.String()).Str("owner", ownerId.String()).Str("origin", "api").Msg("publish failed")
		jsonErrorWithPayload(w, t.Error(), "metax", t.OriginalError(), convertExternalStatusCode(t.StatusCode()))
	case *psql.DatabaseError:
		dbError(w, err, &api.logger).Err(err).Str("dataset", id.String()).Str("owner", ownerId.String()).Str("origin", "database").Msg("publish failed")
	default:
		loggedJSONError(w, err.Error(), http.StatusInternalServerError, &api.logger).Err(err).Str("dataset", id.String()).Str("owner", ownerId.String()).Str("origin", "other").Msg("publish failed")
	}
}

func (api *DatasetApi) publishDataset(w http.ResponseWriter, r *http.Request, owner *models.User, id uuid.UUID) {
	vId, nId, qId, err := shared.Publish(api.metax, api.db, id, owner)
	if err != nil {
		api.handlePublishError(w, owner.Uid, id, err)
		return
	}

	api.Published(w, r, id, vId, qId, nId)
}

func (api *DatasetApi) deleteDataset(w http.ResponseWriter, r *http.Request, owner uuid.UUID, id uuid.UUID) {
	dataset, err := api.db.GetWithOwner(id, owner)
	if err != nil {
		dbError(w, err, &api.logger).Err(err).Str("dataset", id.String()).Str("user", owner.String()).Msg("deletion of dataset failed")
		return
	}

	if dataset.Published {
		err := shared.UnpublishAndDelete(api.metax, api.db, id, owner)
		if err != nil {
			api.handlePublishError(w, owner, id, err)
			return
		}
	} else {
		err = api.db.Delete(id, &owner)
		if err != nil {
			dbError(w, err, &api.logger).Err(err).Str("dataset", id.String()).Str("user", owner.String()).Msg("deletion of dataset failed")
			return
		}
	}

	// deleted, return 204 No Content
	apiWriteHeaders(w)
	w.WriteHeader(http.StatusNoContent)
}

func (api *DatasetApi) changeDatasetCumulativeState(w http.ResponseWriter, r *http.Request, owner *models.User, id uuid.UUID) {
	cumulativeState := r.URL.Query().Get("cumulative_state")
	if cumulativeState == "" {
		loggedJSONError(w, "missing value for cumulative_state", http.StatusBadRequest, &api.logger).
			Str("uid", owner.Uid.String()).Str("dataset", id.String()).Msg("changing cumulative state failed")
		return
	}

	nextVersionQvainId, err := shared.ChangeDatasetCumulativeState(api.metax, api.db, &api.logger, owner, id, cumulativeState)
	if err != nil {
		switch t := err.(type) {
		case *metax.ApiError:
			loggedJSONErrorWithPayload(w, t.Error(), convertExternalStatusCode(t.StatusCode()), &api.logger, "metax", t.OriginalError()).
				Str("dataset", id.String()).Msg("changing cumulative state failed")
		case *psql.DatabaseError:
			dbError(w, err, &api.logger).
				Err(err).Str("owner", owner.Uid.String()).Str("dataset", id.String()).Str("origin", "database").Msg("changing cumulative state failed")
		default:
			loggedJSONError(w, err.Error(), http.StatusNotFound, &api.logger).
				Err(err).Str("owner", owner.Uid.String()).Str("dataset", id.String()).Msg("changing cumulative state failed")
		}
		return
	}

	apiWriteHeaders(w)
	enc := gojay.BorrowEncoder(w)
	defer enc.Release()

	enc.AppendByte('{')
	enc.AddIntKey("status", http.StatusOK)
	enc.AddStringKey("msg", "dataset cumulative state changed to "+cumulativeState)
	enc.AddStringKey("id", id.String())
	if nextVersionQvainId != nil {
		enc.AddStringKey("new_id", nextVersionQvainId.String())
	}
	enc.AppendByte('}')
	enc.Write()
}

func (api *DatasetApi) refreshDatasetDirectoryContent(w http.ResponseWriter, r *http.Request, owner *models.User, id uuid.UUID) {
	directoryIdentifier := r.URL.Query().Get("dir_identifier")
	if directoryIdentifier == "" {
		loggedJSONError(w, "missing value for dir_identifier", http.StatusBadRequest, &api.logger).
			Str("uid", owner.Uid.String()).Str("dataset", id.String()).Msg("refreshing directory content failed")
		return
	}

	nextVersionQvainId, err := shared.RefreshDatasetDirectoryContent(api.metax, api.db, &api.logger, owner, id, directoryIdentifier)
	if err != nil {
		switch t := err.(type) {
		case *metax.ApiError:
			loggedJSONErrorWithPayload(w, t.Error(), convertExternalStatusCode(t.StatusCode()), &api.logger, "metax", t.OriginalError()).
				Str("dataset", id.String()).Msg("refreshing directory content failed")
		case *psql.DatabaseError:
			dbError(w, err, &api.logger).
				Err(err).Str("owner", owner.Uid.String()).Str("dataset", id.String()).Str("origin", "database").Msg("refreshing directory content failed")
		default:
			loggedJSONError(w, err.Error(), http.StatusNotFound, &api.logger).
				Err(err).Str("owner", owner.Uid.String()).Str("dataset", id.String()).Msg("refreshing directory content failed")
		}
		return
	}

	apiWriteHeaders(w)
	enc := gojay.BorrowEncoder(w)
	defer enc.Release()

	enc.AppendByte('{')
	enc.AddIntKey("status", http.StatusOK)
	enc.AddStringKey("msg", "directory content refreshed")
	enc.AddStringKey("id", id.String())
	if nextVersionQvainId != nil {
		enc.AddStringKey("new_id", nextVersionQvainId.String())
	}
	enc.AppendByte('}')
	enc.Write()

}

// ListVersions lists an array of existing versions for a given dataset and owner.
func (api *DatasetApi) ListVersions(w http.ResponseWriter, r *http.Request, user uuid.UUID, id uuid.UUID) {
	jsondata, err := api.db.ViewVersions(user, id)
	if err != nil {
		dbError(w, err, &api.logger).Err(err).Str("uid", user.String()).Str("dataset", id.String()).Msg("error getting versions")
		return
	}

	apiWriteHeaders(w)
	w.Write(jsondata)
}

// redirectToNew redirects to the location of a newly created (POST) or updated (PUT) resource.
// Note that http.Redirect() will write and send the headers, so set ours before.
func (api *DatasetApi) redirectToNew(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	api.logger.Debug().Str("r.URL.Path", r.URL.Path).Str("r.RequestURI", r.RequestURI).Str("r.URL.RawPath", r.URL.RawPath).Str("r.Method", r.Method).Msg("available URL information")

	// either: POST /parent/ or PUT /parent/id; so check we can check either the method or if we've got a trailing slash
	// NOTE: r.RequestURI is insecure, but http.Redirect escapes it for us anyway.
	if r.Method == http.MethodPost {
		http.Redirect(w, r, r.RequestURI+id.String(), http.StatusCreated)
		return
	}
	http.Redirect(w, r, r.RequestURI, http.StatusCreated)
}

// Created returns a success response for created (POST, 201) or updated (PUT, 204) resources.
//
// TODO: Perhaps 200 OK with a body is a better response to a PUT request?
func (api *DatasetApi) Created(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	apiWriteHeaders(w)

	// new, return 201 Created
	if r.Method == http.MethodPost {
		api.redirectToNew(w, r, id)

		enc := gojay.BorrowEncoder(w)
		defer enc.Release()

		enc.AppendByte('{')
		enc.AddIntKey("status", http.StatusCreated)
		enc.AddStringKey("msg", "created")
		enc.AddStringKey("id", id.String())
		enc.AppendByte('}')
		enc.Write()

		return
	}

	// update, return 204 No Content
	w.WriteHeader(http.StatusNoContent)
}

func (api *DatasetApi) Published(w http.ResponseWriter, r *http.Request, id uuid.UUID, extid string, newId *uuid.UUID, newExtid string) {
	apiWriteHeaders(w)
	enc := gojay.BorrowEncoder(w)
	defer enc.Release()

	enc.AppendByte('{')
	enc.AddIntKey("status", http.StatusOK)
	enc.AddStringKey("msg", "dataset published")
	enc.AddStringKey("id", id.String())
	enc.AddStringKey("extid", extid)
	if newId != nil {
		enc.AddStringKey("new_id", newId.String())
	}
	if newExtid != "" {
		enc.AddStringKey("new_extid", newExtid)
	}
	enc.AppendByte('}')
	enc.Write()
}
