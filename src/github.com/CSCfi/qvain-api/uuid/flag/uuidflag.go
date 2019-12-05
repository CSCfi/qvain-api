// Package uuidflag wraps a UUID so it satisfies the Value and Getter interface of the Go flag package.
package uuidflag

import "github.com/wvh/uuid"

// Uuid wraps UUID.
type Uuid struct {
	uuid.UUID
	set bool
}

// DefaultFromString allows shortcut declaration of a UUID flag with a default value.
// It panics on error; if you don't want that, call the normal Set instance method.
// Note the flag is considered set after this.
func DefaultFromString(val string) Uuid {
	u, err := uuid.FromString(val)
	if err != nil {
		panic(err)
	}
	return Uuid{UUID: u, set: true}
}

// Set function sets the UUID to the given string value or returns an error.
func (u *Uuid) Set(val string) (err error) {
	u.UUID, err = uuid.FromString(val)
	u.set = true
	return err
}

// Get returns the UUID inside the wrapping type.
func (u *Uuid) Get() uuid.UUID {
	return u.UUID
}

// String returns this type's default value for flag help.
// (If no default is set, the value is a zero-filled byte, so return empty string.)
func (u *Uuid) String() string {
	if u.set {
		return u.UUID.String()
	}
	return ""
}

// IsSet indicates whether the UUID value was set, i.e. non-zero byte array.
func (u *Uuid) IsSet() bool {
	return u.set
}
