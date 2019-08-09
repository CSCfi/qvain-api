package psql

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/CSCfi/qvain-api/pkg/models"
	"github.com/wvh/uuid"
)

var filterDatasetOwner = uuid.MustFromString("3d403637-7e86-4dc8-80e4-7983a42b6efb") // random uuid

func createDataset(t *testing.T, db *DB, created string, user string, org string,
	accessType string, schema string, published bool) uuid.UUID {
	blob := []byte(fmt.Sprintf(`{
		"title": "test_dataset",
		"metadata_provider_user": "%s",
		"metadata_provider_org": "%s",
		"research_dataset": {
			"access_rights": {
				"access_type": {
					"identifier": "%s"
				}
			}
		}
	}`, user, org, accessType))
	dataset, err := models.NewDataset(filterDatasetOwner)
	if err != nil {
		t.Fatal("models.NewDataset():", err)
	}
	tim, err := time.Parse(time.RFC3339, created)
	if err != nil {
		t.Fatal("time.Parse:", err)
	}
	dataset.SetData(2, schema, blob)
	dataset.Created = tim
	dataset.Modified = tim
	dataset.Published = published

	err = db.CreateWithMetadata(dataset)
	if err != nil {
		t.Fatal("db.Create:", err)
	}

	return dataset.Id
}

func TestDatasetFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	db, err := NewPoolServiceFromEnv()
	if err != nil {
		t.Fatal("psql:", err)
	}

	// remove test data
	cleanUp := func() {
		datasets, err := db.GetAllForUid(filterDatasetOwner)
		if err != nil {
			t.Fatal("db.GetAllForUid:", err)
		}

		for _, d := range datasets {
			db.Delete(d.Id, &d.Owner)
		}
	}
	cleanUp()
	defer cleanUp()

	createDataset(t, db, "9999-01-01T10:00:00.11Z", "testimatti", "testiorg", "/access_type/open", "metax-ida", true)
	createDataset(t, db, "9999-02-01T10:00:00.12Z", "testimatti", "testiorg", "/access_type/open", "metax-att", false)
	createDataset(t, db, "9999-02-01T10:30:00.12Z", "testimatti", "testiorg", "/access_type/open", "metax-att", false)
	createDataset(t, db, "9999-02-02T10:00:00.17Z", "testiteppo", "testiorg", "/access_type/restricted", "metax-att", false)
	createDataset(t, db, "9999-03-20T10:00:00.16Z", "testimatti", "testiorg", "/access_type/restricted", "metax-ida", false)
	createDataset(t, db, "9999-03-20T10:00:00.16Z", "testimatti", "testiorg", "/access_type/open", "metax-ida", true)
	createDataset(t, db, "9999-03-20T23:30:00.16Z", "testimatti", "otherorg", "/access_type/restricted", "metax-ida", false)
	createDataset(t, db, "9999-07-25T10:00:00.16Z", "testiteppo", "otherorg", "/access_type/restricted", "metax-ida", false)
	createDataset(t, db, "9999-08-25T10:00:00.16Z", "testiteppo", "otherorg", "/access_type/restricted", "metax-ida", false)
	createDataset(t, db, "9999-08-25T10:00:10.16Z", "testimatti", "thirdorg", "/access_type/topsecret", "metax-ida", false)

	getCount := func(filter DatasetFilter) int {
		var count struct {
			Count int `json:"count"`
		}
		filter.QvainOwner = filterDatasetOwner.String()
		result, err := db.CountDatasets(&filter)
		if err != nil {
			t.Error(err)
		}
		json.Unmarshal(result, &count)
		return count.Count
	}

	getGroupCount := func(filter DatasetFilter) map[string]int {
		var counts []map[string]interface{}

		filter.QvainOwner = filterDatasetOwner.String()
		filter.GroupTimeZone = "UTC"
		result, err := db.CountDatasets(&filter)
		if err != nil {
			t.Error(err)
		}
		json.Unmarshal(result, &counts)

		// Turn array in to a map:
		// [{ organization: testorg, count: 1 }] ==> {testorg: 1}
		mapped := make(map[string]int)
		for _, row := range counts {
			for key, val := range row {
				if key != "count" {
					mapped[val.(string)] = int(row["count"].(float64))
				}
			}
		}
		return mapped
	}

	// Flag
	count := getCount(DatasetFilter{OnlyAtt: true})
	if count != 3 {
		t.Errorf("expected 3 ATT datasets")
	}
	count = getCount(DatasetFilter{OnlyIda: true})
	if count != 7 {
		t.Errorf("expected 7 IDA datasets")
	}
	count = getCount(DatasetFilter{OnlyPublished: true})
	if count != 2 {
		t.Errorf("expected 2 published datasets")
	}
	count = getCount(DatasetFilter{OnlyDrafts: true})
	if count != 8 {
		t.Errorf("expected 8 draft datasets")
	}

	// TimeFilters
	count = getCount(DatasetFilter{DateCreated: []TimeFilter{ParseTimeFilter("_eq", "9999Z")}})
	if count != 10 {
		t.Errorf("expected 10 datasets in year 9999")
	}
	count = getCount(DatasetFilter{DateCreated: []TimeFilter{ParseTimeFilter("_eq", "9999-01Z")}})
	if count != 1 {
		t.Errorf("expected 1 dataset in month 9999-02")
	}
	count = getCount(DatasetFilter{DateCreated: []TimeFilter{ParseTimeFilter("_eq", "9999-02Z")}})
	if count != 3 {
		t.Errorf("expected 3 datasets in month 9999-02")
	}
	count = getCount(DatasetFilter{DateCreated: []TimeFilter{ParseTimeFilter("_eq", "9999-03-20Z")}})
	if count != 3 {
		t.Errorf("expected 3 datasets in day 9999-03-20")
	}
	count = getCount(DatasetFilter{DateCreated: []TimeFilter{ParseTimeFilter("", "9999-08-25T10:00Z")}})
	if count != 2 {
		t.Errorf("expected 2 datasets in hour 9999-08-25 10:00")
	}

	count = getCount(DatasetFilter{DateCreated: []TimeFilter{ParseTimeFilter("_lt", "9999-07Z")}})
	if count != 7 {
		t.Errorf("expected 7 datasets before 9999-07")
	}
	count = getCount(DatasetFilter{DateCreated: []TimeFilter{ParseTimeFilter("_le", "9999-07Z")}})
	if count != 8 {
		t.Errorf("expected 8 datasets before and including 9999-07")
	}

	count = getCount(DatasetFilter{DateCreated: []TimeFilter{ParseTimeFilter("_gt", "9999-02-01Z")}})
	if count != 7 {
		t.Errorf("expected 7 datasets after 9999-02-01")
	}
	count = getCount(DatasetFilter{DateCreated: []TimeFilter{ParseTimeFilter("_ge", "9999-02-01Z")}})
	if count != 9 {
		t.Errorf("expected 9 datasets after and including 9999-02-01")
	}

	count = getCount(DatasetFilter{DateCreated: []TimeFilter{ParseTimeFilter("_eq", "9999-08-25T10:00:00Z")}})
	if count != 1 {
		t.Errorf("expected 1 dataset in 9999-08-25T10:00:00")
	}

	// GroupBy
	counts := getGroupCount(DatasetFilter{GroupBy: "organization"})
	if counts["testiorg"] != 6 || counts["otherorg"] != 3 || counts["thirdorg"] != 1 {
		t.Errorf("error in GroupBy: organization [%v]", counts)
	}
	counts = getGroupCount(DatasetFilter{GroupBy: "schema"})
	if counts["metax-att"] != 3 || counts["metax-ida"] != 7 {
		t.Errorf("error in GroupBy: schema [%v]", counts)
	}
	counts = getGroupCount(DatasetFilter{GroupBy: "access_type"})
	if counts["/access_type/open"] != 4 || counts["/access_type/restricted"] != 5 || counts["/access_type/topsecret"] != 1 {
		t.Errorf("error in GroupBy: schema [%v]", counts)
	}

	counts = getGroupCount(DatasetFilter{GroupBy: "year_created"})
	if len(counts) != 1 {
		t.Errorf("expected 1 group for year_created [%v]", counts)
	}
	counts = getGroupCount(DatasetFilter{GroupBy: "month_created"})
	if len(counts) != 5 {
		t.Errorf("expected 5 groups for month_created [%v]", counts)
	}
	counts = getGroupCount(DatasetFilter{GroupBy: "day_created"})
	if len(counts) != 6 {
		t.Errorf("expected 6 groups for day_created [%v]", counts)
	}

	t.Run("create", func(t *testing.T) {

	})

}
