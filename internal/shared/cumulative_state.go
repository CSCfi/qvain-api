package shared

import (
	"context"
	"fmt"

	"github.com/CSCfi/qvain-api/internal/psql"
	"github.com/CSCfi/qvain-api/pkg/metax"
	"github.com/CSCfi/qvain-api/pkg/models"
	"github.com/rs/zerolog"
	"github.com/wvh/uuid"
)

// ChangeDatasetCumulativeState uses a Metax RPC call to change cumulative_state for a dataset with the given
// Metax identifier. The updated dataset is fetched from Metax and it replaces the current version in the DB,
// so any unpublished changes are lost. If a new dataset version was created, returns the new Qvain identifier.
func ChangeDatasetCumulativeState(api *metax.MetaxService, db *psql.DB, logger *zerolog.Logger, owner *models.User, id uuid.UUID, cumulativeState string) (newQVersionId *uuid.UUID, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), PublishTimeout)
	defer cancel()

	dataset, err := db.GetWithOwner(id, owner.Uid)
	if err != nil {
		return nil, err
	}

	if dataset.Unwrap().Family() != metax.MetaxDatasetFamily {
		return nil, fmt.Errorf("not a metax dataset")
	}

	identifier := metax.GetIdentifier(dataset.Blob())
	if identifier == "" {
		return nil, fmt.Errorf("dataset Metax identifier not found")
	}

	newMetaxIdentifier, err := api.ChangeCumulativeState(ctx, identifier, cumulativeState)
	if err != nil {
		return nil, err
	}
	logger.Debug().Str("identifier", identifier).
		Str("cumulative_state", cumulativeState).Str("new_version_identifier", newMetaxIdentifier).Msg("changed cumulative_state")

	qvainId, err := FetchDataset(api, db, *logger, owner.Uid, identifier, true)
	if err != nil {
		return nil, err
	}
	logger.Debug().Str("identifier", identifier).Str("id", qvainId.String()).Msg("fetched updated dataset")

	if newMetaxIdentifier != "" {
		newQVersionId, err = FetchDataset(api, db, *logger, owner.Uid, newMetaxIdentifier, true)
		if err != nil {
			return nil, err
		}
		logger.Debug().Str("identifier", newMetaxIdentifier).Str("id", newQVersionId.String()).Msg("fetched new dataset version")
	}

	return newQVersionId, err
}
