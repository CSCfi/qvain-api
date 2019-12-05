package shared

import (
	"context"
	"fmt"

	"github.com/CSCfi/qvain-api/internal/psql"
	"github.com/CSCfi/qvain-api/pkg/metax"
	"github.com/CSCfi/qvain-api/pkg/models"
	"github.com/rs/zerolog"
	"github.com/CSCfi/qvain-api/uuid"
)

// RefreshDatasetDirectoryContent uses a Metax RPC call to update directory contents for a directory in a
// dataset with the given Metax identifier. The updated dataset is fetched from Metax and it replaces the current version in the DB,
// so any unpublished changes are lost. If a new dataset version was created, returns the new Qvain identifier.
func RefreshDatasetDirectoryContent(api *metax.MetaxService, db *psql.DB, logger *zerolog.Logger, owner *models.User, id uuid.UUID, directoryIdentifier string) (newQVersionId *uuid.UUID, err error) {
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

	newMetaxIdentifier, err := api.RefreshDirectoryContent(ctx, identifier, directoryIdentifier)
	if err != nil {
		return nil, err
	}
	logger.Debug().Str("identifier", identifier).
		Str("dir_identifier", directoryIdentifier).Str("new_version_identifier", newMetaxIdentifier).Msg("refresh directory")

	qvainId, err := FetchDataset(api, db, *logger, owner.Uid, identifier)
	if err != nil {
		return nil, err
	}
	logger.Debug().Str("identifier", identifier).Str("id", qvainId.String()).Msg("fetched updated dataset")

	if newMetaxIdentifier != "" {
		newQVersionId, err = FetchDataset(api, db, *logger, owner.Uid, newMetaxIdentifier)
		if err != nil {
			return nil, err
		}
		logger.Debug().Str("identifier", newMetaxIdentifier).Str("id", newQVersionId.String()).Msg("fetched new dataset version")
	}

	return newQVersionId, err
}
