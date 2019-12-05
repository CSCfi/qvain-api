package uuid

// This package is a bit difficult to test as the whole end result is an unpredictable UUID...
//
// The only code path not covered by any test is /dev/random errors in NewUUID(), since those are a bit hard to fake.

import (
	"bytes"
	"encoding/hex"
	"testing"
	"time"
)

func TestNewUUID(t *testing.T) {
	maxSecs := 2                                     // maximum delta seconds when comparing timestamp of UUID with current time (2 secs means 1 sec more or less)
	maxDelta := time.Duration(maxSecs) * time.Second // convert seconds to time.Duration
	now := time.Now()

	uuid, err := NewUUID()
	if err != nil {
		t.Errorf("error making UUID: %s\n", err)
	}

	t.Logf("New UUID: `%v`\n", uuid)

	if len(uuid) != UUID_BYTES {
		t.Errorf("error for len(%s): expected %d, got %d\n", uuid, UUID_BYTES, len(uuid))
	}

	hex := hex.EncodeToString(uuid.Bytes())
	if uuid.String() != hex {
		t.Errorf("error for UUID `%s`: string output `%s` does not match byte output `%v`\n", uuid, uuid.String(), uuid.Bytes())
	}

	ts := uuid.ToTime()
	delta := ts.Sub(now.Truncate(time.Microsecond)) // UUID has microsecond precision: avoid Now() > timestamp because of rounding
	t.Logf("time: %v, now: %v, delta: %v, maxdelta: %v\n", ts, now, delta, maxDelta)
	if delta < 0 {
		// timestamp somehow ended up earlier than Now(): see if this is just a rounding error by switching sign
		t.Logf("warning for UUID `%s`: time stamp delta %v < 0, changing sign\n", uuid, delta)
		delta = -delta
	}
	if delta > maxDelta {
		t.Errorf("error for UUID `%s`: time stamp delta %v > %v\n", uuid, delta, maxDelta)
	}
}

func TestFromString(t *testing.T) {
	var tests = []struct {
		in  string
		out UUID
		err error
	}{
		{"053d02fe137a5c6ce1353443e7c19bd1", [16]byte{0x05, 0x3d, 0x02, 0xfe, 0x13, 0x7a, 0x5c, 0x6c, 0xe1, 0x35, 0x34, 0x43, 0xe7, 0xc1, 0x9b, 0xd1}, nil},
		{"053d02fe-137a-5c6c-e135-3443e7c19bd1", [16]byte{0x05, 0x3d, 0x02, 0xfe, 0x13, 0x7a, 0x5c, 0x6c, 0xe1, 0x35, 0x34, 0x43, 0xe7, 0xc1, 0x9b, 0xd1}, nil},
		{"x53d02fe137a5c6ce1353443e7c19bd1", [16]byte{0x05, 0x3d, 0x02, 0xfe, 0x13, 0x7a, 0x5c, 0x6c, 0xe1, 0x35, 0x34, 0x43, 0xe7, 0xc1, 0x9b, 0xd1}, ErrInvalidUUID},  // 'x' is not a hexadecimal character
		{"053d02fe137a5c6ce1353443e7c19bë", [16]byte{0x05, 0x3d, 0x02, 0xfe, 0x13, 0x7a, 0x5c, 0x6c, 0xe1, 0x35, 0x34, 0x43, 0xe7, 0xc1, 0x9b, 0xd1}, ErrInvalidUUID},   // 'ë' is not a hexadecimal character; ë = 2 bytes, so len = 32 and hexOnly is called
		{"053d02fe137a5c6ce1353443e7c19bd10", [16]byte{0x05, 0x3d, 0x02, 0xfe, 0x13, 0x7a, 0x5c, 0x6c, 0xe1, 0x35, 0x34, 0x43, 0xe7, 0xc1, 0x9b, 0xd1}, ErrInvalidUUID}, // len = 33
		{"", [16]byte{0x05, 0x3d, 0x02, 0xfe, 0x13, 0x7a, 0x5c, 0x6c, 0xe1, 0x35, 0x34, 0x43, 0xe7, 0xc1, 0x9b, 0xd1}, ErrInvalidUUID},                                  // len = 0, str = ""
	}

	for _, test := range tests {
		uuid, err := FromString(test.in)
		if err != nil && err != test.err {
			t.Errorf("Error for `%#v`: %s\n", test.in, err)
		} else if err != nil {
			t.Logf("passed negative test for UUID `%s` (%d)\n", test.in, len(test.in))
			continue
		}
		if uuid != test.out {
			t.Errorf("Expected `%#v`, got `%#v` for string `%s`\n", test.out, uuid, test.in)
		}
	}
}

func TestMarshal(t *testing.T) {
	u, err := NewUUID()
	if err != nil {
		t.Errorf("Error creating uuid")
	}

	// UnmarshalBinary
	uBytes := u.Bytes()
	var newUUID UUID
	err = newUUID.UnmarshalBinary(uBytes)
	if err != nil {
		t.Errorf("Error for UnmarshalBinary: %s", err)
	}
	if u != newUUID {
		t.Errorf("original and UnmarshalBinary not equal: %s != %s", u, newUUID)
	}
	t.Logf("passed UnmarshalBinary test")

	// MarshalBinary
	bin, err := newUUID.MarshalBinary()
	if err != nil {
		t.Errorf("Error for MarshalBinary: %s", err)
	}
	if !bytes.Equal(u.Bytes(), bin) {
		t.Errorf("original and MarshalBinary not equal: %s != %s", u, bin)
	}
	t.Logf("passed MarshalBinary test")

	// MarshalText
	text, err := newUUID.MarshalText()
	if err != nil {
		t.Errorf("Error for MarshalText: %s", err)
	}
	if u.String() != string(text) {
		t.Errorf("original and MarshalText not equal: %s != %s", u.String(), string(text))
	}
	t.Logf("passed MarshalText test")

	var thirdUUID UUID
	err = thirdUUID.UnmarshalText(text)
	if err != nil {
		t.Errorf("Error for UnmarshalText: %s", err)
	}
	if u != thirdUUID {
		t.Errorf("original and UnmarshalText not equal: %s != %s", u.String(), thirdUUID.String())
	}
	t.Logf("passed UnmarshalText test")
}

func BenchmarkNewUUID(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewUUID()
	}
}
