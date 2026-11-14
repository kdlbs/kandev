//go:build !windows && !linux

package hostmemory

import "context"

// unsupportedReader is used on platforms without an implemented memory
// reading (currently darwin and everything else). It reports an incomplete
// reading, so the gate fails open there and never stalls launches on a host it
// cannot measure. The product target for this guard is Windows, where the
// measured outage occurred.
type unsupportedReader struct{}

// NewReader returns a reader that reports an incomplete reading.
func NewReader() Reader { return unsupportedReader{} }

func (unsupportedReader) Read(ctx context.Context) (Reading, error) {
	if err := ctx.Err(); err != nil {
		return Reading{}, err
	}
	return Reading{Complete: false}, nil
}
