//go:build linux && cgo

package sqlite

import (
	"github.com/kandev/kandev/internal/testutil/sqlitememory"
	"testing"
)

func sidebarBenchmarkMemory(b *testing.B, reset bool) (int64, int64) {
	b.Helper()
	if reset {
		sqlitememory.Peak(true)
		return sqlitememory.Used(), sidebarProcessRSS(b)
	}
	return sqlitememory.Peak(false), sidebarProcessRSS(b)
}
