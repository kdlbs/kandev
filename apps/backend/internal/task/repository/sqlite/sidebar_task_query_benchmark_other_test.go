//go:build !linux || !cgo

package sqlite

import "testing"

func sidebarBenchmarkMemory(_ *testing.B, _ bool) (int64, int64) { return 0, 0 }
