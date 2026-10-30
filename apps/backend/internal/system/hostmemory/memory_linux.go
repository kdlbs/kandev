//go:build linux

package hostmemory

import (
	"context"
	"os"
)

type linuxReader struct{}

// NewReader returns the Linux host-memory reader.
func NewReader() Reader { return linuxReader{} }

func (linuxReader) Read(ctx context.Context) (Reading, error) {
	if err := ctx.Err(); err != nil {
		return Reading{}, err
	}
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return Reading{Complete: false}, err
	}
	// An unreadable overcommit mode leaves the commit pair unjudged; available
	// memory alone still decides.
	overcommit, _ := os.ReadFile("/proc/sys/vm/overcommit_memory")
	return readingFromProc(string(data), string(overcommit))
}
