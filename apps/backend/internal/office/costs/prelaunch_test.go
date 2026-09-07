package costs

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// TestWindowStart pins AC-OFFICE-BUDGET-002.1-.4/.8: each period computes its
// UTC window start by name, and total is selected by name rather than by
// falling through an unrecognized period to a lifetime window.
func TestWindowStart(t *testing.T) {
	at := time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name      string
		period    models.BudgetPeriod
		wantStart time.Time
		wantOK    bool
	}{
		{
			name:      "daily is most recent UTC midnight",
			period:    models.BudgetPeriodDaily,
			wantStart: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
			wantOK:    true,
		},
		{
			name:      "monthly is the 1st of the UTC month",
			period:    models.BudgetPeriodMonthly,
			wantStart: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			wantOK:    true,
		},
		{
			name:      "yearly is 1 Jan UTC",
			period:    models.BudgetPeriodYearly,
			wantStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			wantOK:    true,
		},
		{
			name:      "total is unbounded below, selected by name",
			period:    models.BudgetPeriodTotal,
			wantStart: time.Time{},
			wantOK:    true,
		},
		{
			name:   "unrecognized period is not ok, never lifetime",
			period: models.BudgetPeriod("weekly"),
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, ok := windowStart(tt.period, at)
			if ok != tt.wantOK {
				t.Fatalf("windowStart(%q) ok = %v, want %v", tt.period, ok, tt.wantOK)
			}
			if ok && !start.Equal(tt.wantStart) {
				t.Errorf("windowStart(%q) start = %v, want %v", tt.period, start, tt.wantStart)
			}
		})
	}
}

// TestWindowStart_DailyConvertsToUTCFirst pins that a non-UTC instant is
// converted to UTC before the midnight boundary is computed, so a run
// evaluated late in a non-UTC day doesn't fall into the wrong UTC day.
func TestWindowStart_DailyConvertsToUTCFirst(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*60*60)
	at := time.Date(2026, 3, 15, 23, 0, 0, 0, loc) // 2026-03-16T04:00:00Z

	start, ok := windowStart(models.BudgetPeriodDaily, at)
	if !ok {
		t.Fatal("windowStart(daily) ok = false, want true")
	}
	want := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	if !start.Equal(want) {
		t.Errorf("windowStart(daily) start = %v, want %v", start, want)
	}
}
