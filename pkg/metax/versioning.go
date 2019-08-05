package metax

import (
	"time"

	"github.com/tidwall/gjson"
)

var (
	// IdentifierKey is the key pointing to the primary identifier, if any.
	IdentifierKey = "identifier"

	// NewVersionKey is the key to the value indicating a new version has been created.
	NewVersionKey = "new_version_created"

	// NewVersionId is the key holding the identifier of the new version or an empty string.
	NewVersionIdKey = "new_version_created.identifier"

	// Editor is the key pointing to Qvain's private object inside the dataset.
	EditorKey = "editor"

	// QvainId is the key with the Qvain record id.
	QvainIdKey = "record_id"

	// QvainId is the key with the identifier "qvain".
	QvainIdentifierKey = "identifier"

	// DateCreatedKey is the key for the Metax dataset creation timestamp.
	DateCreatedKey = "date_created"

	// DateModifiedKey is the key for the Metax dataset modification timestamp.
	DateModifiedKey = "date_modified"

	// DateDeprecatedKey is the key for the Metax dataset deprecation timestamp.
	DateDeprecatedKey = "date_deprecated"

	// DateRemovedKey is the key for the Metax dataset removal timestamp.
	DateRemovedKey = "date_removed"
)

func GetIdentifier(blob []byte) string {
	if len(blob) < 1 {
		return ""
	}

	return gjson.GetBytes(blob, IdentifierKey).String()
}

// GetModificationDate returns the date of last modification for a dataset.
func GetModificationDate(blob []byte) time.Time {
	if len(blob) < 1 {
		return time.Time{}
	}

	createdDate := gjson.GetBytes(blob, DateCreatedKey).Time()
	date := createdDate
	modifiedDate := gjson.GetBytes(blob, DateModifiedKey).Time()
	if !modifiedDate.IsZero() && modifiedDate.After(date) {
		date = modifiedDate
	}
	deprecatedDate := gjson.GetBytes(blob, DateDeprecatedKey).Time()
	if !deprecatedDate.IsZero() && deprecatedDate.After(date) {
		date = deprecatedDate
	}
	removedDate := gjson.GetBytes(blob, DateRemovedKey).Time()
	if !removedDate.IsZero() && removedDate.After(date) {
		date = removedDate
	}

	return date
}

func IsPublished(blob []byte) bool {
	if len(blob) < 1 {
		return false
	}

	return gjson.GetBytes(blob, IdentifierKey).Exists()
}

// CreatedNewVersion returns a boolean indicating whether the new version created key exists.
// Note this doesn't check if its value is valid.
func CreatedNewVersion(blob []byte) bool {
	if len(blob) < 1 {
		return false
	}

	result := gjson.GetBytes(blob, NewVersionKey)
	return result.Exists()
}

// MaybeNewVersionId returns the id of a new Metax version if a new version was created, otherwise an empty string.
func MaybeNewVersionId(blob []byte) string {
	if len(blob) < 1 {
		return ""
	}

	return gjson.GetBytes(blob, NewVersionIdKey).String()
}

// GetQvainId returns the Qvain id of the dataset from the editor object or empty string.
func GetQvainId(blob []byte) string {
	result := gjson.GetBytes(blob, EditorKey)
	if result.IsObject() {
		ident, id := result.Get(QvainIdentifierKey).String(), result.Get(QvainIdKey).String()
		if ident == "qvain" || ident == "QVAIN" {
			return id
		}
	}
	return ""
}

// GetIdentifiers returns the Metax identifier, Metax "new version" identifier and Qvain record_id fields.
func GetIdentifiers(blob []byte) (string, string, string) {
	results := gjson.GetManyBytes(blob, IdentifierKey, NewVersionIdKey, EditorKey)
	if results[2].IsObject() {
		ident, qid := results[2].Get(QvainIdentifierKey).String(), results[2].Get(QvainIdKey).String()
		if ident == "qvain" || ident == "QVAIN" {
			return results[0].String(), results[1].String(), qid
		}
	}
	return results[0].String(), results[1].String(), ""
}
