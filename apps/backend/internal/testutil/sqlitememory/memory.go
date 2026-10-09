//go:build cgo

// Package sqlitememory exposes the production SQLite allocator to isolated tests.
package sqlitememory

/*
#include "memory.h"
*/
import "C"

import _ "github.com/mattn/go-sqlite3"

// Used returns currently allocated native bytes, excluding the Go heap.
func Used() int64 { return int64(C.sqlite3_memory_used()) }

// Peak returns the native high-water mark. Reset only in an isolated subprocess.
func Peak(reset bool) int64 {
	var flag C.int
	if reset {
		flag = 1
	}
	return int64(C.sqlite3_memory_highwater(flag))
}

// Version identifies the same SQLite library used by the repository driver.
func Version() string { return C.GoString(C.sqlite3_libversion()) }
