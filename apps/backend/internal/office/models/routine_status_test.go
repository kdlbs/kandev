package models

import "testing"

// TestRoutineStatusCanFire covers AC-OFFICE-ROUTINE-STATUS-001.1 through
// -001.4: an allowlist, not a denylist, and a byte-exact comparison with no
// case folding and no trimming.
func TestRoutineStatusCanFire(t *testing.T) {
	tests := []struct {
		name   string
		status RoutineStatus
		want   bool
	}{
		{"active fires", RoutineStatusActive, true},
		{"empty string fires", "", true},
		{"paused suppresses", RoutineStatusPaused, false},
		{"archived suppresses", RoutineStatusArchived, false},
		{"unrecognized value suppresses", "some_future_status", false},
		{"different case does not fire", "Active", false},
		{"leading space does not fire", " active", false},
		{"trailing space does not fire", "active ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.CanFire(); got != tt.want {
				t.Errorf("RoutineStatus(%q).CanFire() = %v, want %v", string(tt.status), got, tt.want)
			}
		})
	}
}
