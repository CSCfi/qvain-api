package psql

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Constants for comparison types.
const (
	CompareEq = iota // ==
	CompareLe = iota // <=
	CompareGe = iota // >=
	CompareLt = iota // <
	CompareGt = iota // >
)

// TimeZoneRegex is used to determine if a date string ends with a timezone offset.
var TimeZoneRegex = regexp.MustCompile(`(Z|([\+-]\d\d:\d\d))$`)

// ComparisonSuffixes are used to determine the type of comparison used in a query parameter.
var ComparisonSuffixes = map[string]int{
	"":    CompareEq,
	"_eq": CompareEq,
	"_le": CompareLe,
	"_ge": CompareGe,
	"_lt": CompareLt,
	"_gt": CompareGt,
}

// TimeFilter contains data for time-based filtering. The timefilter represents a comparison
// performed on a time range defined by: start ≤ time < end
type TimeFilter struct {
	Precision  time.Duration
	Time       time.Time
	Comparison int
}

// Start is the start of the time range.
func (t *TimeFilter) Start() time.Time {
	return t.Time
}

// End is the end of time range.
func (t *TimeFilter) End() time.Time {
	return t.Time.Add(t.Precision)
}

// IsZero tells if the time contained has the zero time value.
func (t *TimeFilter) IsZero() bool {
	return t.Time.IsZero()
}

// ParseTimeFilter parses a time string for a given query suffix.
//
// The suffix is used to determine the comparison used. For "param", the options are:
// - param     equal
// - param_eq  equal
// - param_le  less than or equal
// - param_ge  greater than or equal
// - param_lt  less than
// - param_gt  greater than
//
// The function accepts RFC3339 dates. In addition, the date can be truncated to
// desired precision and timezone offset is optional. If timezone offset is not specified,
// the local timezone is assumed.
// - year:    2019
// - month:   2019-08
// - date:    2019-08-27
// - hours:   2019-08-27T13
// - minutes: 2019-08-27T13:30
// - seconds: 2019-08-27T13:30:00
// Comparisons will be made with the used precision, e.g. equality for a date
// without hours given will represent a 24 hour range starting from the date.
//
// Timezone offset is either Z for UTC, or +/-hh:mm.
// Both escaped and unscaped query parameters should work, and space can be used instead of +.
func ParseTimeFilter(suffix string, timeString string) TimeFilter {
	filter := TimeFilter{
		Comparison: ComparisonSuffixes[suffix],
	}
	var (
		t   time.Time
		err error
	)
	str := strings.Replace(timeString, " ", "+", -1)

	// use time zone offset if available, otherwise use local timezone
	tz := ""
	if TimeZoneRegex.MatchString(str) {
		tz = "Z07:00"
	}

	if t, err = time.ParseInLocation("2006"+tz, str, time.Local); err == nil { // year
		filter.Precision = t.AddDate(1, 0, 0).Sub(t)
	} else if t, err = time.ParseInLocation("2006-01"+tz, str, time.Local); err == nil { // month
		filter.Precision = t.AddDate(0, 1, 0).Sub(t)
	} else if t, err = time.ParseInLocation("2006-01-02"+tz, str, time.Local); err == nil { // day
		filter.Precision = time.Hour * 24
	} else if t, err = time.ParseInLocation("2006-01-02T15"+tz, str, time.Local); err == nil { // hour
		filter.Precision = time.Hour
	} else if t, err = time.ParseInLocation("2006-01-02T15:04"+tz, str, time.Local); err == nil { // minute
		filter.Precision = time.Minute
	} else if t, err = time.ParseInLocation("2006-01-02T15:04:05"+tz, str, time.Local); err == nil { // second (time.RFC3339)
		filter.Precision = time.Second
	}
	filter.Time = t
	return filter
}

// DatasetFilter is used to specify filtering options for datasets.
type DatasetFilter struct {
	OnlyDrafts    bool         // only drafts are counted
	OnlyPublished bool         // only published are counted
	OnlyAtt       bool         // only datasets in ATT catalog are counted
	OnlyIda       bool         // only datasets in IDA catalog are counted
	DateCreated   []TimeFilter // filters by creation date
	User          string       // filter by user (metadata_provider_user)
	Organization  string       // filter by organization (metadata_provider_org)
	GroupBy       string       // group values, see DatasetFilterGroupByPaths for valid options

	// options for testing, not currently exposed in the stats API
	QvainOwner    string // qvain id of owner
	GroupTimeZone string // time zone used in grouping dates, supported values are "" (local) and "UTC"
}

// DatasetFilterGroupByPaths provides paths/values for grouping datasets. Keys contained
// in this map are the only valid options for group_by.
var DatasetFilterGroupByPaths = map[string]string{
	"schema":       `schema`,
	"organization": `blob->>'metadata_provider_org' as organization`,
	"access_type":  `blob#>>'{"research_dataset","access_rights","access_type","identifier"}' as access_type`,
	"date_created": `date_trunc('day', created$tz) as created`,
}

// GroupByPath returns data path to use in GROUP BY statement.
func (filter *DatasetFilter) GroupByPath() string {
	path := DatasetFilterGroupByPaths[filter.GroupBy]
	if filter.GroupTimeZone == "" {
		path = strings.Replace(path, "$tz", "", 1)
	} else if filter.GroupTimeZone == "UTC" {
		path = strings.Replace(path, "$tz", " at time zone 'UTC'", 1)
	}
	return path
}

// Where returns the WHERE statement for the filter.
func (filter *DatasetFilter) Where() (string, []interface{}) {
	wb := NewWhereBuilder()
	wb.MaybeAdd(filter.OnlyDrafts, `published=false`)
	wb.MaybeAdd(filter.OnlyPublished, `published=true`)
	wb.MaybeAdd(filter.OnlyAtt, `schema='metax-att'`)
	wb.MaybeAdd(filter.OnlyIda, `schema='metax-ida'`)
	wb.MaybeAddString(filter.User, `blob->>'metadata_provider_user'=$`)
	wb.MaybeAddString(filter.Organization, `blob->>'metadata_provider_org'=$`)
	wb.MaybeAddString(filter.QvainOwner, `owner=$`)
	for _, timeFilter := range filter.DateCreated {
		wb.MaybeAddTimeFilter(timeFilter, `created`)
	}
	return wb.Where()
}

// WhereBuilder is a helper object for creating SQL WHERE statements.
type WhereBuilder struct {
	conditions []string
	args       []interface{}
}

// NewWhereBuilder creates a WhereBuilder.
func NewWhereBuilder() *WhereBuilder {
	return &WhereBuilder{}
}

// addArg adds a new argument to args. Any input provided by the users should
// be put in args instead of using them directly in a query to prevent SQL injection.
// Returns the position of the added argument, e.g. $3 for the third argument.
func (w *WhereBuilder) addArg(param interface{}) string {
	w.args = append(w.args, param)
	return "$" + strconv.Itoa(len(w.args))
}

// cprintf passes its arguments to fmt.Sprintf and adds the result as a new condition.
func (w *WhereBuilder) cprintf(format string, a ...interface{}) {
	w.conditions = append(w.conditions, fmt.Sprintf(format, a...))
}

// Where returns the WHERE statement.
func (w *WhereBuilder) Where() (string, []interface{}) {
	if len(w.conditions) == 0 {
		return "", make([]interface{}, 0)
	}
	return "WHERE " + strings.Join(w.conditions, " AND "), w.args
}

// MaybeAdd adds query to conditions if enabled is true.
func (w *WhereBuilder) MaybeAdd(enabled bool, query string) {
	if enabled {
		w.cprintf(query)
	}
}

// MaybeAddString adds query to conditions if s is non-empty. The s argument
// is added as a query parameter and $ in the query string is replaced with its position.
func (w *WhereBuilder) MaybeAddString(s string, query string) {
	if s != "" {
		w.cprintf(strings.Replace(query, "$", w.addArg(s), -1))
	}
}

// MaybeAddTimeFilter adds a time condition if !TimeFilter.IsZero().
// Time comparisons are performed against the query argument.
func (w *WhereBuilder) MaybeAddTimeFilter(filter TimeFilter, query string) {
	if !filter.IsZero() {
		switch filter.Comparison {
		case CompareEq:
			w.cprintf("%s < %s", query, w.addArg(filter.End()))
			w.cprintf("%s >= %s", query, w.addArg(filter.Start()))

		case CompareLe:
			w.cprintf("%s < %s", query, w.addArg(filter.End()))

		case CompareGe:
			w.cprintf("%s >= %s", query, w.addArg(filter.Start()))

		case CompareLt:
			w.cprintf("%s < %s", query, w.addArg(filter.Start()))

		case CompareGt:
			w.cprintf("%s >= %s", query, w.addArg(filter.End()))
		}
	}
}
