// Copyright 2014–2019 Wouter Van Hemel

// Package uuid generates a 128-bit UUID based on Unix time (similar to one of CouchDB's UUID options).
//
// For example:
//
//    0505 da61 3800 49RR RRRR RRRR RRRR RRRR
//    ----------------- 7 bytes, unix time in hex microseconds
//                     ---------------------- 9 bytes, random data
//
// The benefits of this form of UUID are chronological sortability, which is good for database key usage, and a measure of protection against predictability, since it carries at least 72 bits of entropy (9 x 8b). Note that this doesn't necessarily make it a good choice for cryptographic purposes.
//
// This package defaults to the UUID form without dashes, as they're not particularly useful in this format.
//
package uuid

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

// document offsets and lengths
const (
	UUID_BYTES                     = 16
	UUID_RANDOM_BYTES              = 9
	UUID_RANDOM_OFFSET             = 7
	UUID_STRING_LENGTH             = 32
	UUID_STRING_LENGTH_WITH_DASHES = 36

	ByteSize               = 16
	StringLength           = 32
	StringLengthWithDashes = 36
	uuidRandomBytes        = 9
	uuidRandomOffset       = 7
)

var (
	// ErrInvalidUUID means we failed to parse the given uuid.
	ErrInvalidUUID = errors.New("error parsing uuid")
	nilUuid        = UUID{}
	//NullUuid UUID = UUID{}
)

// UUID is an alias for an array of 16 bytes.
type UUID [UUID_BYTES]byte

// NewUUID creates a new unix time stamp based UUID.
func NewUUID() (UUID, error) {
	var ret UUID

	ms := uint64(time.Now().UnixNano() / 1000)

	_, err := rand.Read(ret[UUID_RANDOM_OFFSET:])
	if err != nil {
		return UUID{}, err
	}

	for i := 6; i >= 0; i-- {
		ret[i] = byte(ms & 0xff)
		ms >>= 8
	}

	return ret, nil
}

// MustNewUUID calls NewUUID and panics on error.
func MustNewUUID() UUID {
	uuid, err := NewUUID()
	if err != nil {
		panic(err)
	}
	return uuid
}

// String returns the UUID in string form (without dashes).
func (u UUID) String() string {
	return hex.EncodeToString(u[:])
}

// Array returns a ref to underlying type [16]byte, for modification.
func (u *UUID) Array() *[UUID_BYTES]byte {
	return (*[UUID_BYTES]byte)(u)
}

// Bytes returns the UUID as a byte slice.
func (u UUID) Bytes() []byte {
	return u[:]
}

// ToTime converts the unix time stamp inside the UUID to a time.Time.
func (u UUID) ToTime() time.Time {
	var ms uint64

	for i := uint(0); i <= 6; i++ {
		ms += uint64(u[6-i]) << (8 * i)
	}

	return time.Unix(0, int64(ms)*1000)
}

// MarshalText implements the encoding.TextMarshaler interface (since go 1.2).
func (u UUID) MarshalText() ([]byte, error) {
	return []byte(u.String()), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface (since go 1.2).
func (u *UUID) UnmarshalText(text []byte) error {
	if len(text) == UUID_STRING_LENGTH_WITH_DASHES {
		// strip any dashes
		text = hexOnlyBytes(text)
	}

	if len(text) != UUID_STRING_LENGTH {
		return ErrInvalidUUID
	}

	_, err := hex.Decode(u[:], text)
	if err != nil {
		return ErrInvalidUUID
	}

	return nil
}

// MarshalBinary implements the encoding.BinaryMarshaler interface (since go 1.2).
func (u UUID) MarshalBinary() ([]byte, error) {
	return u.Bytes(), nil
}

// UnmarshalBinary implements the encoding.BinaryUnmarshaler interface (since go 1.2).
func (u *UUID) UnmarshalBinary(b []byte) error {
	if len(b) != UUID_BYTES {
		return ErrInvalidUUID
	}
	copy(u[:], b)
	return nil
}

// Nil returns a UUID with all bytes set to zero.
func Nil() UUID {
	return nilUuid
}

// IsNil returns true if a UUID is unset.
func (u *UUID) IsNil() bool {
	return *u == nilUuid
}

// Equal returns true if two UUIDs are equal.
func Equal(u1 UUID, u2 UUID) bool {
	return bytes.Equal(u1[:], u2[:])
}

// hexOnly filters any non-hexadecimal characters out of a string.
func hexOnly(s string) string {
	b := make([]byte, len(s))
	i := 0
	for _, c := range s {
		if c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f' || c >= '0' && c <= '9' {
			b[i] = byte(c & 0x7f)
			i++
		}
	}
	return string(b[:i])
}

// hexOnlyBytes filters the given slice for valid hex characters.
func hexOnlyBytes(b []byte) []byte {
	// re-use backing array
	nb := b[:0]
	for _, c := range b {
		if c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f' || c >= '0' && c <= '9' {
			nb = append(nb, byte(c&0x7f))
		}
	}
	return nb
}

// FromBytes takes a byte slice and returns a UUID and optionally an error.
func FromBytes(b []byte) (UUID, error) {
	if len(b) != UUID_BYTES {
		return UUID{}, ErrInvalidUUID
	}

	var uuid UUID
	copy(uuid[:], b)
	return uuid, nil
}

// FromString returns a UUID object from a given string and optionally an error.
func FromString(s string) (UUID, error) {
	if len(s) == UUID_STRING_LENGTH_WITH_DASHES {
		// strip any dashes
		s = hexOnly(s)
	}

	if len(s) != UUID_STRING_LENGTH {
		return UUID{}, ErrInvalidUUID
	}

	var uuid UUID
	_, err := hex.Decode(uuid[:], []byte(s))
	if err != nil {
		//return UUID{}, fmt.Errorf("%s: %s", ErrInvalidUUID, err)
		// replace error from encoding/hex to simplify testing
		return UUID{}, ErrInvalidUUID
	}

	return uuid, nil
}

// FromStringUnsafe returns a UUID object from a given string, ignoring any errors.
// (This function just calls FromString() and throws away the error.)
func FromStringUnsafe(s string) UUID {
	u, _ := FromString(s)
	return u
}

// MustFromString returns a UUID object from a given string. It panics if the string can't be parsed.
// (This function just calls FromString() and panics on error.)
func MustFromString(s string) UUID {
	u, err := FromString(s)
	if err != nil {
		panic(err)
	}
	return u
}
