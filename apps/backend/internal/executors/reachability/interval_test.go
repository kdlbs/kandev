package reachability

import "testing"

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.23, AC-EXECUTORS-SSH-REACHABILITY-001.24
func TestClampInterval(t *testing.T) {
	tests := []struct {
		name        string
		in          int
		wantEffect  int
		wantClamped bool
	}{
		{"zero disables and is not a clamp", 0, 0, false},
		{"negative falls back to the default", -5, DefaultIntervalSeconds, true},
		{"below minimum clamps up to 15", 1, MinIntervalSeconds, true},
		{"just below minimum clamps up to 15", 14, MinIntervalSeconds, true},
		{"minimum passes through unchanged", 15, 15, false},
		{"default passes through unchanged", 60, 60, false},
		{"maximum passes through unchanged", 3600, 3600, false},
		{"just above maximum clamps down to 3600", 3601, MaxIntervalSeconds, true},
		{"far above maximum clamps down to 3600", 100000, MaxIntervalSeconds, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effective, clamped := ClampInterval(tt.in)
			if effective != tt.wantEffect || clamped != tt.wantClamped {
				t.Errorf("ClampInterval(%d) = (%d, %v), want (%d, %v)", tt.in, effective, clamped, tt.wantEffect, tt.wantClamped)
			}
		})
	}
}
