package shared

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/CSCfi/qvain-api/internal/psql"
	"github.com/CSCfi/qvain-api/pkg/metax"
	"github.com/CSCfi/qvain-api/pkg/models"
	"github.com/tidwall/sjson"
	"github.com/wvh/uuid"
)

const (
	// PublishTimeout is how long we wait for response when publishling or unpublishing
	PublishTimeout = 10 * time.Second
)

var (
	// ErrNoIdentifier means we can't find the Metax dataset identifier in created or updated datasets.
	ErrNoIdentifier = errors.New("no identifier in dataset")
)

// ChangeDatasetCumulativeState uses a Metax RPC call to change cumulative_state for a dataset with the given
// Metax identifier. May create a new dataset version.
func ChangeDatasetCumulativeState(api *metax.MetaxService, db *psql.DB, identifier string, cumulativeState string) (newQVersionId *uuid.UUID, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), PublishTimeout)
	defer cancel()
	if err = api.ChangeCumulativeState(ctx, identifier, cumulativeState); err != nil {
		return nil, err
	}

	// newVersion, err = api.GetId(newVersionId)
	// if err != nil {
	// 	fmt.Println("error getting new version:", err)
	// 	//return err
	// 	return versionId, newVersionId, nil, err
	// }
	// fmt.Printf("new: %s\n\n", newVersion)

	// // create a Qvain id for the new version
	// var tmp uuid.UUID
	// tmp, err = uuid.NewUUID()
	// if err != nil {
	// 	return
	// }
	// newQVersionId = &tmp

	// synced := metax.GetModificationDate(newVersion)
	// if synced.IsZero() {
	// 	fmt.Fprintln(os.Stderr, "Could not find date_modified or date_created from new version!")
	// 	synced = time.Now()
	// }

	// // store the new version
	// err = db.WithTransaction(func(tx *psql.Tx) error {
	// 	return tx.StoreNewVersion(id, *newQVersionId, synced, newVersion)
	// })
	// if err != nil {
	// 	return
	// }

	// TODO: Determine new id if possible, fetch stuff?

	return nil, err
}

// Publish stores a dataset in Metax and updates the Qvain database.
// It returns the Metax identifier for the dataset, the new version idenifier if such was created, and an error.
// The error returned can be a Metax ApiError, a Qvain database error, or a basic Go error.
func Publish(api *metax.MetaxService, db *psql.DB, id uuid.UUID, owner *models.User) (versionId string, newVersionId string, newQVersionId *uuid.UUID, err error) {

	dataset, err := db.GetWithOwner(id, owner.Uid)
	if err != nil {
		return
	}

	// Add user_created or user_modified based on whether this was already published
	blob := dataset.Blob()
	if dataset.Published {
		blob, err = sjson.SetBytes(blob, "user_modified", owner.Identity)
	} else {
		blob, err = sjson.SetBytes(blob, "user_created", owner.Identity)
	}
	if err != nil {
		return
	}

	fmt.Fprintln(os.Stderr, "About to publish:", id)

	ctx, cancel := context.WithTimeout(context.Background(), PublishTimeout)
	defer cancel()

	res, err := api.Store(ctx, blob, owner)
	if err != nil {
		fmt.Fprintf(os.Stderr, "type: %T\n", err)
		if apiErr, ok := err.(*metax.ApiError); ok {
			fmt.Fprintf(os.Stderr, "metax error: [%d] %s\n", apiErr.StatusCode(), apiErr.OriginalError())
		}
		//return err
		return
	}

	fmt.Fprintln(os.Stderr, "Success! Response follows:")
	fmt.Printf("%s", res)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Marking dataset as published...")

	versionId = metax.GetIdentifier(res)
	if versionId == "" {
		return "", "", nil, ErrNoIdentifier
	}

	synced := metax.GetModificationDate(res)
	if synced.IsZero() {
		fmt.Fprintln(os.Stderr, "Could not find date_modified or date_created from dataset!")
		synced = time.Now()
	}

	err = db.StorePublished(id, res, synced)
	if err != nil {
		//return err
		return
	}

	if newVersionId = metax.MaybeNewVersionId(res); newVersionId != "" {
		fmt.Println("created new version:", newVersionId)

		var newVersion []byte
		// get the new version from the Metax api
		newVersion, err = api.GetId(newVersionId)
		if err != nil {
			fmt.Println("error getting new version:", err)
			//return err
			return versionId, newVersionId, nil, err
		}
		fmt.Printf("new: %s\n\n", newVersion)

		// create a Qvain id for the new version
		var tmp uuid.UUID
		tmp, err = uuid.NewUUID()
		if err != nil {
			return
		}
		newQVersionId = &tmp

		synced := metax.GetModificationDate(newVersion)
		if synced.IsZero() {
			fmt.Fprintln(os.Stderr, "Could not find date_modified or date_created from new version!")
			synced = time.Now()
		}

		// store the new version
		err = db.WithTransaction(func(tx *psql.Tx) error {
			return tx.StoreNewVersion(id, *newQVersionId, synced, newVersion)
		})
		if err != nil {
			return
		}
	}

	fmt.Fprintln(os.Stderr, "success")
	return
}

// UnpublishAndDelete marks a dataset as removed in Metax and deletes it from the Qvain db.
// The dataset will no longer be visible in Metax queries unless the ?removed=true parameter is used.
func UnpublishAndDelete(api *metax.MetaxService, db *psql.DB, id uuid.UUID, owner uuid.UUID) error {
	dataset, err := db.GetWithOwner(id, owner)
	if err != nil {
		return err
	}

	// mark as removed in Metax
	ctx, cancel := context.WithTimeout(context.Background(), PublishTimeout)
	defer cancel()
	if err := api.Delete(ctx, dataset.Blob()); err != nil {
		fmt.Fprintf(os.Stderr, "type: %T\n", err)
		if apiErr, ok := err.(*metax.ApiError); ok {
			fmt.Fprintf(os.Stderr, "metax error: [%d] %s\n", apiErr.StatusCode(), apiErr.OriginalError())
		}
		return err
	}

	// delete from db
	err = db.Delete(id, &owner)
	if err != nil {
		return err
	}

	return nil
}
