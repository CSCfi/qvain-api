package shared

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/CSCfi/qvain-api/internal/psql"
	"github.com/CSCfi/qvain-api/pkg/env"
	"github.com/CSCfi/qvain-api/pkg/metax"
	"github.com/CSCfi/qvain-api/pkg/models"
	"github.com/tidwall/gjson"

	"github.com/wvh/uuid"
)

var (
	ownerUuid        = uuid.MustFromString("053bffbcc41edad4853bea91fc42ea18")
	ownerIdentity    = "owner"
	modifierIdentity = "modifier"
)

func readFile(tb testing.TB, fn string) []byte {
	path := filepath.Join("..", "..", "pkg", "metax", "testdata", fn)
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	return bytes
}

func modifyTitleFromDataset(db *psql.DB, id uuid.UUID, title string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	ct, err := tx.Exec("UPDATE datasets SET blob = jsonb_set(blob, '{research_dataset,title,en}', to_jsonb($2::text)), modified = now() WHERE id = $1", id.Array(), title)
	if err != nil {
		return err
	}

	if ct.RowsAffected() != 1 {
		return psql.ErrNotFound
	}

	return tx.Commit()
}

func deleteFilesFromFairdataDataset(db *psql.DB, id uuid.UUID) error {
	return deletePathFromDataset(db, id, "{research_dataset,files}")
}

func deletePathFromDataset(db *psql.DB, id uuid.UUID, path string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	ct, err := tx.Exec("UPDATE datasets SET blob = blob #- $2, modified = now() WHERE id = $1", id.Array(), path)
	if err != nil {
		return err
	}

	if ct.RowsAffected() != 1 {
		return psql.ErrNotFound
	}

	return tx.Commit()
}

// TestPublish creates a Qvain dataset, saves it, publishes it to metax, and saves the resulting version.
func TestPublish(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	tests := []struct {
		fn string
	}{
		{
			fn: "unpublished.json",
		},
	}

	db, err := psql.NewPoolServiceFromEnv()
	if err != nil {
		t.Fatal("psql:", err)
	}

	api := metax.NewMetaxService(
		env.Get("APP_METAX_API_HOST"),
		metax.WithCredentials(env.Get("APP_METAX_API_USER"), env.Get("APP_METAX_API_PASS")),
		metax.WithInsecureCertificates(env.GetBool("APP_DEV_MODE")),
	)

	owner := &models.User{
		Uid:      ownerUuid,
		Identity: ownerIdentity,
		Projects: []string{"project_x"},
	}

	modifier := &models.User{
		Uid:      ownerUuid,
		Identity: modifierIdentity,
		Projects: []string{"project_x"},
	}

	wrongProjectOwner := &models.User{
		Uid:      ownerUuid,
		Identity: ownerIdentity,
		Projects: []string{"wrong_project_x_y_z"},
	}

	noProjectOwner := &models.User{
		Uid:      ownerUuid,
		Identity: ownerIdentity,
		Projects: []string{},
	}

	for _, test := range tests {
		blob := readFile(t, test.fn)

		dataset, err := models.NewDataset(owner.Uid)
		if err != nil {
			t.Fatal("models.NewDataset():", err)
		}
		dataset.SetData(2, metax.SchemaIda, blob)

		id := dataset.Id

		err = db.Create(dataset)
		if err != nil {
			t.Fatal("db.Create():", err)
		}
		//defer db.Delete(id, nil)

		var versionId string

		// tests that should fail with *metax.ApiError 403 due to project permissions
		t.Run(test.fn+"(wrong project)", func(t *testing.T) {
			_, _, _, err := Publish(api, db, id, wrongProjectOwner)
			if apiErr, ok := err.(*metax.ApiError); !ok || apiErr.StatusCode() != 403 {
				t.Error("error: wrongProjectOwner should have failed with 403")
			}
		})

		t.Run(test.fn+"(no project)", func(t *testing.T) {
			_, _, _, err := Publish(api, db, id, noProjectOwner)
			if apiErr, ok := err.(*metax.ApiError); !ok || apiErr.StatusCode() != 403 {
				t.Error("error: noProjectOwner should have failed with 403")
			}
		})

		// test that should publish succesfully
		t.Run(test.fn+"(new)", func(t *testing.T) {
			vId, nId, _, err := Publish(api, db, id, owner)
			if err != nil {
				if apiErr, ok := err.(*metax.ApiError); ok {
					t.Errorf("API error: [%d] %s", apiErr.StatusCode(), apiErr.Error())
				}
				t.Error("error:", err)
			}

			if nId != "" {
				t.Errorf("API created a new version: expected %q, got %q", "", nId)
			}

			t.Logf("published with version id %q", vId)
			versionId = vId

			// check that the dataset has been updated with a user_created field
			publishedDataset, err := db.Get(id)
			if err != nil {
				t.Error("error retrieving dataset:", err)
			}
			if userCreated := gjson.GetBytes(publishedDataset.Blob(), "user_created").String(); userCreated != owner.Identity {
				t.Error("missing or wrong user_created", userCreated)
			}
		})

		err = modifyTitleFromDataset(db, id, "Less Wonderful Title")
		if err != nil {
			t.Fatal("modifyTitleFromDataset():", err)
		}

		// test that should update
		t.Run(test.fn+"(update)", func(t *testing.T) {
			vId, nId, _, err := Publish(api, db, id, modifier)
			if err != nil {
				if apiErr, ok := err.(*metax.ApiError); ok {
					t.Errorf("API error: [%d] %s", apiErr.StatusCode(), apiErr.Error())
				}
				t.Error("error:", err)
			}

			if nId != "" {
				t.Errorf("API created a new version: expected %q, got %q", "", nId)
			}

			if vId != versionId {
				t.Errorf("API version id changed: expected %q, got %q", versionId, vId)
			}

			t.Logf("(re)published with version id %q", vId)

			// check that the dataset has been updated with a user_modified field
			publishedDataset, err := db.Get(id)
			if err != nil {
				t.Error("error retrieving dataset:", err)
			}

			if userModified := gjson.GetBytes(publishedDataset.Blob(), "user_modified").String(); userModified != modifier.Identity {
				t.Error("missing or wrong user_modified:", userModified)
			}
		})

		err = deleteFilesFromFairdataDataset(db, id)
		if err != nil {
			t.Fatal("deleteFilesFromFairdataDataset():", err)
		}

		// test that should remove files and create a new version
		t.Run(test.fn+"(files)", func(t *testing.T) {
			vId, nId, qId, err := Publish(api, db, id, modifier)
			if err != nil {
				if apiErr, ok := err.(*metax.ApiError); ok {
					t.Errorf("API error: [%d] %s", apiErr.StatusCode(), apiErr.Error())
				}
				t.Error("error:", err)
			}

			if vId != versionId {
				t.Errorf("API version id changed: expected %q, got %q", versionId, vId)
			}

			if nId == "" {
				t.Errorf("API didn't create a new version: expected identifier, got %q", nId)
			} else {
				t.Logf("created new version with metax id %q and qvain id %q", nId, qId)
			}

			t.Logf("(re)published with version id %q", vId)

			// the new version should have the user who modified the dataset as user_created
			publishedDataset, err := db.Get(*qId)
			if err != nil {
				t.Error("error retrieving dataset:", err)
			}
			if userCreated := gjson.GetBytes(publishedDataset.Blob(), "user_created").String(); userCreated != modifier.Identity {
				t.Error("missing or wrong user_created: ", userCreated)
			}
		})

		// test that should unpublish and delete
		t.Run(test.fn+"(new)", func(t *testing.T) {

			dataset, err := db.GetWithOwner(id, owner.Uid)
			if err != nil {
				t.Error("error:", err)
			}
			identifier := metax.GetIdentifier(dataset.Blob())

			err = UnpublishAndDelete(api, db, id, owner.Uid)
			if err != nil {
				if apiErr, ok := err.(*metax.ApiError); ok {
					t.Errorf("API error: [%d] %s", apiErr.StatusCode(), apiErr.Error())
				}
				t.Error("error:", err)
			}

			// retrieve the deleted dataset from Metax
			removedDataset, err := api.GetIdRemoved(identifier)
			if err != nil {
				t.Error("error:", err)
				return
			}

			if removed := gjson.GetBytes(removedDataset, "removed").Bool(); !removed {
				t.Errorf("dataset was not removed from Metax")
				return
			}

			// try to retrieve the deleted dataset from Qvain db
			_, err = db.GetWithOwner(id, owner.Uid)
			if err == nil {
				t.Errorf("dataset was not deleted from Qvain")
			}

			t.Logf("unpublished and removed dataset %s", id)
		})

		// if we want to make it invalid again...
		/*
			err = deletePathFromDataset(db, id, "{research_dataset,title,en}")
			if err != nil {
				t.Fatal("deletePathFromDataset():", err)
			}
		*/

	}
}
