package main

import (
	"testing"
	"time"
)

// TestParseBackgroundDuration pins the /background argument parsing, including
// the regression Copilot flagged on PR #3: a unit-bearing value like "1m" must
// not be mangled into "1ms" by the bare-seconds fallback.
func TestParseBackgroundDuration(t *testing.T) {
	const def = 8 * time.Second
	cases := []struct {
		name string
		cmd  string
		want time.Duration
	}{
		{"no argument uses default", "/background", def},
		{"bare number is seconds", "/background 12", 12 * time.Second},
		{"explicit seconds", "/background 20s", 20 * time.Second},
		{"explicit minutes (regression: not 1ms)", "/background 1m", time.Minute},
		{"explicit hours", "/background 2h", 2 * time.Hour},
		{"explicit milliseconds", "/background 500ms", 500 * time.Millisecond},
		{"unparseable falls back to default", "/background soon", def},
		{"zero falls back to default", "/background 0", def},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseBackgroundDuration(tc.cmd, def); got != tc.want {
				t.Fatalf("parseBackgroundDuration(%q) = %v, want %v", tc.cmd, got, tc.want)
			}
		})
	}
}
