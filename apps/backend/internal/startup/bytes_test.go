package startup

import "testing"

// TestFormatBytes covers AC-PLATFORM-STARTUP-PROGRESS-003.14. The cases match
// the shared web policy in apps/web/lib/utils/format-bytes.ts: 1024-based
// units, the largest unit selected before rounding, whole B below 1024, one
// fractional digit above it, and a TB cap.
func TestFormatBytes(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{bytes: 0, want: "0 B"},
		{bytes: 1, want: "1 B"},
		{bytes: 1023, want: "1023 B"},
		{bytes: 1024, want: "1.0 KB"},
		{bytes: 1280, want: "1.3 KB"},
		{bytes: 1048575, want: "1024.0 KB"},
		{bytes: 1048576, want: "1.0 MB"},
		{bytes: 15389753344, want: "14.3 GB"},
		{bytes: 1099511627776, want: "1.0 TB"},
		{bytes: 2199023255552, want: "2.0 TB"},
		// A value just below a decimal midpoint: multiplying by ten in floating
		// point lands on the midpoint and rounds up, while toFixed and exact
		// integer rounding keep it down.
		{bytes: 4504644163416883, want: "4096.9 TB"},
	}
	for _, tc := range cases {
		if got := FormatBytes(tc.bytes); got != tc.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

// TestFormatByteRate covers AC-PLATFORM-STARTUP-PROGRESS-003.15: the same unit
// thresholds as byte counts, one fractional digit even for B/s, and a /s
// suffix. The shared numeric cases mirror the web formatter's parity set.
func TestFormatByteRate(t *testing.T) {
	cases := []struct {
		rate float64
		want string
	}{
		{rate: 0, want: "0.0 B/s"},
		{rate: 0.5, want: "0.5 B/s"},
		{rate: 1, want: "1.0 B/s"},
		{rate: 1024, want: "1.0 KB/s"},
		{rate: 1280, want: "1.3 KB/s"},
		{rate: 1048576, want: "1.0 MB/s"},
		{rate: 15389753344, want: "14.3 GB/s"},
	}
	for _, tc := range cases {
		if got := FormatByteRate(tc.rate); got != tc.want {
			t.Errorf("FormatByteRate(%v) = %q, want %q", tc.rate, got, tc.want)
		}
	}
}
