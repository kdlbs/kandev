package toolretention

import (
	"testing"
	"time"
)

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.1, 001.2, 001.3
func TestPolicyDefaultAndAgeValidation(t *testing.T) {
	if got := DefaultPolicy(); got.Enabled || got.Age != (Age{3, "months"}) {
		t.Fatalf("default policy = %+v, want disabled for three months", got)
	}
	for _, a := range []Age{{0, "weeks"}, {521, "weeks"}, {121, "months"}, {1, "days"}, {-1, "months"}} {
		if a.Validate() == nil {
			t.Errorf("accepted invalid age %+v", a)
		}
	}
	for _, a := range []Age{{1, "weeks"}, {520, "weeks"}, {120, "months"}} {
		if err := a.Validate(); err != nil {
			t.Errorf("rejected valid age %+v: %v", a, err)
		}
	}
}

func TestAgeCutoffClampsMonthsAndUsesUTC(t *testing.T) {
	for _, tt := range []struct {
		now  string
		age  Age
		want string
	}{
		{"2024-03-31T12:30:00Z", Age{1, "months"}, "2024-02-29T12:30:00Z"},
		{"2025-03-31T12:30:00Z", Age{1, "months"}, "2025-02-28T12:30:00Z"},
		{"2026-01-14T12:30:00+02:00", Age{1, "weeks"}, "2026-01-07T10:30:00Z"},
	} {
		now, _ := time.Parse(time.RFC3339, tt.now)
		if got := tt.age.Cutoff(now).Format(time.RFC3339); got != tt.want {
			t.Errorf("%s: got %s want %s", tt.now, got, tt.want)
		}
	}
}
