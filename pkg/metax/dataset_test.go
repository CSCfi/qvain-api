package metax

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	// testBaseDataset is a (published) Metax dataset with identifier, owner and date fields set.
	testBaseDataset = "published.json"

	// testPathToId is the dotted path to the application id in the test dataset.
	testPathToId = "editor.record_id"

	// testPathToAppIdent is the dotted path to the application ident(ifier) in the test dataset.
	testPathToAppIdent = "editor.identifier"

	// testPathToUser is the dotted path to the user identity in the test dataset.
	testPathToUser = "metadata_provider_user"

	// testNoIdString is the string representation of an unset uuid.
	testNoIdString = "00000000000000000000000000000000"
)

func TestMetaxDatasetParsing(t *testing.T) {
	// note: readTestFile is a helper function defined in another test file
	datasetBytes := readTestFile(t, testBaseDataset)

	qid := gjson.GetBytes(datasetBytes, testPathToId).Str
	if qid == "" {
		t.Fatalf("can't find a valid Qvain id from test dataset; expected at path: %q", testPathToId)
	}

	if app := gjson.GetBytes(datasetBytes, testPathToAppIdent).Str; app != appIdent {
		t.Fatalf("wrong application identifier in test dataset; expected: %q, got: %q", appIdent, app)
	}

	identity := gjson.GetBytes(datasetBytes, testPathToUser).Str
	if identity == "" {
		t.Fatal("can't find a valid Qvain id from test dataset")
	}

	tests := []struct {
		// name of test and function to modify the dataset before test
		name   string
		before func([]byte) []byte

		// result fields to check against
		isNew bool
		id    string
		err   error
	}{
		{
			name:   "existing dataset",
			before: nil,
			isNew:  false,
			id:     qid,
			err:    nil,
		},
		{
			name:   "new dataset without editor",
			before: func(data []byte) []byte { data, _ = sjson.DeleteBytes(data, "editor"); return data },
			isNew:  true,
			id:     testNoIdString,
			err:    nil,
		},
		{
			name:   "existing dataset but missing id",
			before: func(data []byte) []byte { data, _ = sjson.DeleteBytes(data, testPathToId); return data },
			isNew:  true,
			id:     testNoIdString,
			err:    nil,
		},
		{
			name:   "existing dataset but invalid id (not uuid)",
			before: func(data []byte) []byte { data, _ = sjson.SetBytes(data, testPathToId, "invalid-id-666"); return data },
			isNew:  false,
			id:     testNoIdString,
			err:    ErrInvalidId,
		},
		{
			name:   "new dataset because of wrong application identifier",
			before: func(data []byte) []byte { data, _ = sjson.SetBytes(data, testPathToAppIdent, "not_qvain"); return data },
			isNew:  true,
			id:     testNoIdString,
			err:    nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := datasetBytes
			if test.before != nil {
				// no need to copy, sjson will allocate a new slice for us
				data = test.before(data)
			}

			unparsed := MetaxRawRecord{json.RawMessage(data)}

			// parse
			dataset, isNew, err := unparsed.ToQvain()
			if err != nil {
				// make sure these error condition tests do not continue but return
				if test.err == nil {
					t.Fatalf("expected no error, got: %v", err)
				} else if err != test.err {
					t.Fatalf("unexpected error, expected: %q, got: %q", test.err, err)
				} else {
					// expected error matched, pass
					return
				}
			}

			// check `new` detection
			if isNew != test.isNew {
				t.Errorf("isNew: expected %t, got %t", test.isNew, isNew)
			}

			// check id
			if dataset.Id.String() != test.id {
				t.Errorf("id doesn't match: expected %v, got %v", test.id, dataset.Id)
			}

			// only check creation time for new datasets; we don't care about existing ones
			if test.isNew && dataset.Created.IsZero() {
				t.Error("dataset.Created should be set to upstream creation time but is zero")
			}
		})
	}
}

func TestValidateUpdatedDataset(t *testing.T) {
	rawDataset := `{
		"data_catalog":{"id":1,"identifier":"urn:nbn:fi:att:data-catalog-ida"},
		"identifier":"urn:nbn:fi:att:bfe2d120-6ceb-4949-9755-882ab54c45b2",
		"cumulative_state": 0,
		"research_dataset":{
			"title":{"en":"Wonderful Title"},
			"total_files_byte_size":200,
			"preferred_identifier":"urn:nbn:fi:att:fe7ed696-2a60-4d0c-b707-ee02c2bcd616",
			"metadata_version_identifier":"urn:nbn:fi:att:be138915-2b91-4dbe-91b0-8f4e72e816b4",
			"directories":[{"directory_path":"/directory", "some_extra_thing": 10}],
			"files":[
				{"file_path":"/directory/file1"},
				{"file_path":"/directory/file2"}
			]
		}
	}`
	unparsed := MetaxRawRecord{json.RawMessage(rawDataset)}
	dataset, _, err := unparsed.ToQvain()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	metaxDataset := &MetaxDataset{Dataset: dataset}

	TestField := func(t *testing.T, field string, raw []byte) error {
		t.Helper()
		unparsed := MetaxRawRecord{json.RawMessage(rawDataset)}
		updated, _, _ := unparsed.ToQvain()
		newBlob, err := sjson.SetRawBytes(metaxDataset.Blob(), field, raw)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		updated.SetData(updated.Family(), updated.Schema(), newBlob)
		return metaxDataset.ValidateUpdated(updated)
	}

	TestDeleteField := func(t *testing.T, field string) error {
		t.Helper()
		unparsed := MetaxRawRecord{json.RawMessage(rawDataset)}
		updated, _, _ := unparsed.ToQvain()
		newBlob, err := sjson.DeleteBytes(metaxDataset.Blob(), field)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		updated.SetData(updated.Family(), updated.Schema(), newBlob)
		return metaxDataset.ValidateUpdated(updated)
	}

	// change preferred identifier
	err = TestField(t, "research_dataset.preferred_identifier", []byte(`""`))
	if err == nil {
		t.Fatalf("expected an error")
	}

	// change total_files_byte_size
	err = TestField(t, "research_dataset.total_files_byte_size", []byte(`201`))
	if err == nil {
		t.Fatalf("expected an error")
	}
	err = TestField(t, "research_dataset.total_files_byte_size", []byte(`200`))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// change metadata_version_identifier
	err = TestField(t, "research_dataset.metadata_version_identifier", []byte(`""`))
	if err == nil {
		t.Fatalf("expected an error")
	}

	// delete field
	err = TestDeleteField(t, "research_dataset.metadata_version_identifier")
	if err == nil {
		t.Fatalf("expected an error")
	}

	// change files
	err = TestField(t, "research_dataset.files", []byte(`[{"file_path":"/directory/file3"}]`))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// change cumulative_state
	err = TestField(t, "cumulative_state", []byte(`0`))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	err = TestField(t, "cumulative_state", []byte(`1`))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	err = TestField(t, "cumulative_state", []byte(`2`)) // 2 not valid initial state
	if err == nil {
		t.Fatalf("expected an error")
	}

	// set cumulative_state for published dataset
	metaxDataset.Published = true
	err = TestField(t, "cumulative_state", []byte(`0`)) // ok because state didn't change
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	err = TestField(t, "cumulative_state", []byte(`1`)) // error, state change not allowed
	if err == nil {
		t.Fatalf("expected an error")
	}

	// set metaxDataset catalog to PAS, which will prevent altering files/directories
	pasBlob, _ := sjson.SetRawBytes(metaxDataset.Blob(), "data_catalog", []byte(`{"id":3,"identifier":"urn:nbn:fi:att:data-catalog-pas"}`))
	metaxDataset.SetData(metaxDataset.Family(), metaxDataset.Schema(), pasBlob)

	// change files
	err = TestField(t, "research_dataset.files", []byte(`[{"file_path":"/directory/file3"}]`))
	if err == nil {
		t.Fatalf("expected an error")
	}

	// change order of files in array
	err = TestField(t, "research_dataset.files", []byte(`[
		{"file_path":"/directory/file2"},{"file_path":"/directory/file1"}
	]`))
	if err == nil {
		t.Fatalf("expected an error")
	}

	// change value of directory
	err = TestField(t, "research_dataset.directories",
		[]byte(`[{"directory_path":"/directory", "some_extra_thing": 12}]`))
	if err == nil {
		t.Fatalf("expected an error")
	}

	// change order of keys in directory object
	err = TestField(t, "research_dataset.directories",
		[]byte(`[{"some_extra_thing": 10, "directory_path":"/directory"}]`))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// repeated keys in directory object, only the last one is taken into account
	err = TestField(t, "research_dataset.directories",
		[]byte(`[{"directory_path":"/directory", "some_extra_thing": 20, "some_extra_thing": 10}]`))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// add preservation state to metaxDataset
	pasBlob, _ = sjson.SetRawBytes(metaxDataset.Blob(), "preservation_state", []byte(`80`))
	metaxDataset.SetData(metaxDataset.Family(), metaxDataset.Schema(), pasBlob)

	// if preservation_state >= 80 and not 100 or 130, all updates should be forbidden
	err = TestField(t, "research_dataset.some_field", []byte(`"some_value"`))
	if err == nil {
		t.Fatalf("expected an error")
	}

	// change to another non-editable preservation state
	pasBlob, _ = sjson.SetRawBytes(metaxDataset.Blob(), "preservation_state", []byte(`120`))
	metaxDataset.SetData(metaxDataset.Family(), metaxDataset.Schema(), pasBlob)

	// if preservation_state >= 80 and not 100 or 130, all updates should be forbidden
	err = TestField(t, "research_dataset.some_field", []byte(`"some_value"`))
	if err == nil {
		t.Fatalf("expected an error")
	}

	// change to an editable preservation state
	pasBlob, _ = sjson.SetRawBytes(metaxDataset.Blob(), "preservation_state", []byte(`100`))
	metaxDataset.SetData(metaxDataset.Family(), metaxDataset.Schema(), pasBlob)

	// preservation_states 100 and 130 should allow editing
	err = TestField(t, "research_dataset.some_field", []byte(`"some_value"`))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// change to another editable preservation state
	pasBlob, _ = sjson.SetRawBytes(metaxDataset.Blob(), "preservation_state", []byte(`130`))
	metaxDataset.SetData(metaxDataset.Family(), metaxDataset.Schema(), pasBlob)

	// preservation_states 100 and 130 should allow editing
	err = TestField(t, "research_dataset.some_field", []byte(`"some_value"`))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}
