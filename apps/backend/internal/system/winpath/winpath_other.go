//go:build !windows

// Package winpath opens Windows filesystem paths without following reparse
// points. The package is Windows-only; this file keeps it buildable on other
// platforms for `go build ./...`.
package winpath
