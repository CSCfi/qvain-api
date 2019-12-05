package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/CSCfi/qvain-api/internal/psql"

	"bytes"
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"testing"
)

type JsonError struct {
	Status int              `json:"status"`
	Msg    string           `json:"msg"`
	Origin string           `json:"origin"`
	Extra  *json.RawMessage `json:"more,omitempty"`
}

func TestJsonErrors(t *testing.T) {
	var tests = []struct {
		status int
		msg    string
		origin string
		extra  []byte
	}{
		{
			status: 200,
			msg:    "OK",
			extra:  nil,
		},
		{
			status: 200,
			msg:    "that worked",
			origin: "someservice",
			extra:  []byte(`"extra string field"`),
		},
		{
			status: 400,
			msg:    "boom",
			origin: "someservice",
			extra:  []byte(`{"much":"key","so":"data"}`),
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			w := httptest.NewRecorder()
			jsonError(w, test.msg, test.status)

			response := w.Result()
			body, _ := ioutil.ReadAll(response.Body)

			if response.StatusCode != test.status {
				t.Errorf("statuscode (header): expected %d, got %d", test.status, response.StatusCode)
			}

			if response.Header.Get("Content-Type") != "application/json" {
				t.Errorf("content-type: expected %s, got %s", "application/json", response.Header.Get("Content-Type"))
			}

			var parsed JsonError
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Error("error response failed to unmarshal:", err)
			}

			if parsed.Status != test.status {
				t.Errorf("status (body): expected %d, got %d", test.status, parsed.Status)
			}

			if parsed.Msg != test.msg {
				t.Errorf("message: expected %s, got %s", test.msg, parsed.Msg)
			}
		})

		t.Run(test.msg+"_payload", func(t *testing.T) {
			w := httptest.NewRecorder()
			jsonErrorWithPayload(w, test.msg, test.origin, test.extra, test.status)

			response := w.Result()
			body, _ := ioutil.ReadAll(response.Body)

			if response.StatusCode != test.status {
				t.Errorf("statuscode (header): expected %d, got %d", test.status, response.StatusCode)
			}

			if response.Header.Get("Content-Type") != "application/json" {
				t.Errorf("content-type: expected %s, got %s", "application/json", response.Header.Get("Content-Type"))
			}

			var parsed JsonError
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Error("error response failed to unmarshal:", err)
			}

			t.Log(string(body))

			if parsed.Status != test.status {
				t.Errorf("status (body): expected %d, got %d", test.status, parsed.Status)
			}

			if parsed.Msg != test.msg {
				t.Errorf("message: expected %s, got %s", test.msg, parsed.Msg)
			}

			if parsed.Origin != test.origin {
				t.Errorf("origin: expected %s, got %s", test.origin, parsed.Origin)
			}

			if parsed.Extra != nil && !bytes.Equal(*parsed.Extra, test.extra) {
				t.Errorf("extra: expected %v, got %v", test.extra, parsed.Extra)
			}
		})

	}
}

func TestQueryParser(t *testing.T) {
	options := map[string]string{
		"imakey": "yes",
	}
	timeTest := time.Date(2011, 10, 5, 16, 48, 1, 0, time.UTC)

	expectedErrors := 0

	query := url.Values{}
	query.Add("flag_present", "")
	query.Add("flag_true", "true")
	query.Add("flag_invalid", "porkkanalaatikko")
	expectedErrors++

	query.Add("time_test", "2011-10-05T16:48:01.000Z") // == timeTest
	query.Add("time", "2011-10-04T16:48:01.000Z")
	query.Add("time_eq", "2011-10-04T16:48:01.000Z")
	query.Add("time_le", "2011-10-05T16:48:01.000+03:00")
	query.Add("time_ge", "2011-10-04T16:48:01.000 02:15")
	query.Add("time_lt", "2011-08-05T16:48:01.123-02:00")
	query.Add("time_gt", "2011-10-04T16:48:01.000Z")
	query.Add("time_lt", "2011-13-05T16:48:01.000Z") // invalid month
	expectedErrors++

	query.Add("string", "just a string")
	query.Add("string_option", "imakey")
	query.Add("string_option_invalid", "imnotakey")
	expectedErrors++

	query.Add("unused", "hmmm")
	expectedErrors++
	query.Add("multiple", "hmm1")
	query.Add("multiple", "hmm2")
	expectedErrors++
	query.Add("skipme", "ignored_value")

	parser := NewQueryParser(query)

	// Flag
	if !parser.Flag("flag_present") {
		t.Errorf("flag_present != true")
	}
	if !parser.Flag("flag_true") {
		t.Errorf("flag_true != true")
	}
	if parser.Flag("flag_invalid_value") {
		t.Errorf("flag_true == true")
	}
	if parser.Flag("flag_missing") {
		t.Errorf("flag_missing == true")
	}

	// TimeFilters
	if !parser.TimeFilters("time_test")[0].Time.Equal(timeTest) {
		t.Errorf("time_test != timeTest")
	}
	timeFilters := parser.TimeFilters("time")
	if len(timeFilters) != 6 {
		t.Errorf("len(timeFilters) != 6")
	}
	countComparisons := func(comparison int) int {
		count := 0
		for _, tf := range timeFilters {
			if tf.Comparison == comparison {
				count++
			}
		}
		return count
	}
	if countComparisons(psql.CompareEq) != 2 {
		t.Errorf("countComparisons(psql.CompareEq) != 2")
	}
	if countComparisons(psql.CompareLe) != 1 {
		t.Errorf("countComparisons(psql.CompareLe) != 1")
	}
	if countComparisons(psql.CompareGe) != 1 {
		t.Errorf("countComparisons(psql.CompareGe) != 1")
	}
	if countComparisons(psql.CompareLt) != 1 {
		t.Errorf("countComparisons(psql.CompareLt) != 1")
	}
	if countComparisons(psql.CompareGt) != 1 {
		t.Errorf("countComparisons(psql.CompareGt) != 1")
	}
	if len(parser.TimeFilters("time_missing")) != 0 {
		t.Errorf("len(time_missing) != 0")
	}

	// String
	if parser.String("string") != "just a string" {
		t.Errorf(`string != "just a string"`)
	}
	if parser.String("string_missing") != "" {
		t.Errorf(`string_missing != ""`)
	}

	// StringOption
	if parser.StringOption("string_option", options) != "imakey" {
		t.Errorf("string != stringTest()")
	}
	if parser.StringOption("string_option_invalid", options) != "" {
		t.Errorf(`string_option_invalid != ""`)
	}
	if parser.StringOption("string_option_missing", options) != "" {
		t.Errorf(`string_option_missing != ""`)
	}

	// Skip
	parser.Skip("skipme")
	parser.Skip("multiple")

	invalid := parser.Validate()
	if len(invalid) != expectedErrors {
		t.Errorf(`len(invalid) != expectedErrors`)
		fmt.Println("invalid: ")
		fmt.Println(strings.Join(invalid, ", \n"))
	}
}
