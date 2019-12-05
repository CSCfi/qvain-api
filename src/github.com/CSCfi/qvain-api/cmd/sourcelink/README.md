[![GoDoc](https://godoc.org/github.com/wvh/sourcelink/lib?status.svg)](https://godoc.org/github.com/wvh/sourcelink/lib)
[![Build Status](https://travis-ci.org/wvh/sourcelink.svg?branch=master)](https://travis-ci.org/wvh/sourcelink)
[![Go Report Card](https://goreportcard.com/badge/github.com/wvh/sourcelink)](https://goreportcard.com/report/github.com/wvh/sourcelink)

# sourcelink

## synopsis
```
usage: ./sourcelink <repo url> <hash> [branch]

This program tries to return a link to the source tree for a given hash and base repository path.
It will try to convert git://, ssh:// and scp-style user@host.name:path.git links to http urls.
If it fails to convert the url, it will print the given repo url and return with a non-zero exit code.

Examples:

HTTP upstream
        $ ./sourcelink https://github.com/user/repo abcdef
        https://github.com/user/repo/tree/abcdef

SCP upstream `git@github.com:user/repo.git` taken from git config
        $ ./sourcelink $(git ls-remote --get-url 2>/dev/null) $(shell git rev-parse --short HEAD 2>/dev/null)
        https://github.com/user/repo/tree/abcdef

Cgit upstream
        $ ./sourcelink git://git.zx2c4.com/WireGuard/ 07a03cbc8d186f985bcccede99fc3547f23868d8 jd/no-inline
        https://git.zx2c4.com/WireGuard/tree/?h=jd%2Fno-inline&id=07a03cbc8d186f985bcccede99fc3547f23868d8

```

## library
The code contains a library and a thin command wrapper, so this can be used as a function in Go or as a command line program.

## state
This is a work in progress. If it works for you, vendor it. No API stability guarantees.
