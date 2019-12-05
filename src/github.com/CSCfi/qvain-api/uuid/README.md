[![GoDoc](https://godoc.org/github.com/wvh/uuid?status.svg)](https://godoc.org/github.com/wvh/uuid)
[![Build Status](https://travis-ci.org/wvh/uuid.svg?branch=master)](https://travis-ci.org/wvh/uuid)
[![Go Report Card](https://goreportcard.com/badge/github.com/wvh/uuid)](https://goreportcard.com/report/github.com/wvh/uuid)

# uuid

This package generates a 128-bit UUID based on Unix time (similar to one of CouchDB's UUID options).

The first 7 bytes indicate the time since the unix epoch in hex microseconds;
the last 9 bytes are cryptographically secure random bits.

The benefits of this form of UUID are chronological sortability, which is good for database key usage, and a measure of protection against predictability, since it carries at least 72 bits of entropy (9 x 8b). Note that this doesn't necessarily make it a good choice for cryptographic purposes.

This package defaults to the UUID form without dashes, as they're not particularly useful in this format.

Licensed under MIT/ISC license. Use freely. Fixes and improvements welcome.
