package process

import (
	"math"
	"testing"
)

// TestPTYDimension covers the narrowing guard in front of ptyexec.Start.
// InteractiveStartRequest.DefaultCols/DefaultRows arrive as plain ints off the
// wire with nothing upstream bounding them, so an out-of-range value must land
// on the default instead of wrapping into a bogus small terminal.
func TestPTYDimension(t *testing.T) {
	tests := []struct {
		name     string
		in       int
		fallback int
		want     uint16
	}{
		{"typical", 120, defaultPTYCols, 120},
		{"one", 1, defaultPTYCols, 1},
		{"max representable", math.MaxUint16, defaultPTYCols, math.MaxUint16},
		{"zero falls back", 0, defaultPTYCols, defaultPTYCols},
		{"negative falls back", -1, defaultPTYRows, defaultPTYRows},
		// Without the upper guard these would wrap: 65536 -> 0 and 100000 -> 34464.
		{"just over max falls back", math.MaxUint16 + 1, defaultPTYCols, defaultPTYCols},
		{"far over max falls back", 100000, defaultPTYRows, defaultPTYRows},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ptyDimension(tt.in, tt.fallback); got != tt.want {
				t.Errorf("ptyDimension(%d, %d) = %d, want %d", tt.in, tt.fallback, got, tt.want)
			}
		})
	}
}
