package ownershipperiod

import (
	"testing"
	"time"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name            string
		configured      time.Duration
		idleTimeout     time.Duration
		wantPeriod      time.Duration
		wantAdjustments int
	}{
		{
			name:            "already satisfies ordering, no adjustment",
			configured:      5 * time.Minute,
			idleTimeout:     10 * time.Minute,
			wantPeriod:      5 * time.Minute,
			wantAdjustments: 0,
		},
		{
			name:            "AC-003.4 clamps to half the idle timeout when not shorter",
			configured:      10 * time.Minute,
			idleTimeout:     10 * time.Minute,
			wantPeriod:      5 * time.Minute,
			wantAdjustments: 1,
		},
		{
			name:            "AC-003.4 clamps when configured exceeds idle timeout",
			configured:      20 * time.Minute,
			idleTimeout:     4 * time.Minute,
			wantPeriod:      2 * time.Minute,
			wantAdjustments: 1,
		},
		{
			name:            "AC-003.6 disabled idle reaping (zero) is left unchanged",
			configured:      10 * time.Minute,
			idleTimeout:     0,
			wantPeriod:      10 * time.Minute,
			wantAdjustments: 0,
		},
		{
			name:            "AC-003.6 negative idle timeout treated as disabled",
			configured:      10 * time.Minute,
			idleTimeout:     -1,
			wantPeriod:      10 * time.Minute,
			wantAdjustments: 0,
		},
		{
			name:            "AC-003.7 floor applies when configured value is too small",
			configured:      10 * time.Second,
			idleTimeout:     0,
			wantPeriod:      MinPeriod,
			wantAdjustments: 1,
		},
		{
			name:            "AC-003.7 floor takes precedence over the AC-003.4 clamp",
			configured:      time.Minute,
			idleTimeout:     30 * time.Second,
			wantPeriod:      MinPeriod,
			wantAdjustments: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			period, adjustments := Resolve(tt.configured, tt.idleTimeout)
			if period != tt.wantPeriod {
				t.Fatalf("Resolve(%v, %v) period = %v, want %v",
					tt.configured, tt.idleTimeout, period, tt.wantPeriod)
			}
			if len(adjustments) != tt.wantAdjustments {
				t.Fatalf("Resolve(%v, %v) adjustments = %v (len %d), want len %d",
					tt.configured, tt.idleTimeout, adjustments, len(adjustments), tt.wantAdjustments)
			}
		})
	}
}

// TestRenewalInterval pins AC-EXECUTORS-CONTROL-OWNERSHIP-003.2: three
// consecutive intervals must sum to strictly less than the period, so a
// third renewal attempt after two failures is still due strictly before the
// unowned-period expiry test can fire.
func TestRenewalInterval(t *testing.T) {
	periods := []time.Duration{MinPeriod, 90 * time.Second, 5 * time.Minute, 10 * time.Minute, time.Hour}
	for _, period := range periods {
		interval := RenewalInterval(period)
		if interval <= 0 {
			t.Fatalf("RenewalInterval(%v) = %v, want > 0", period, interval)
		}
		if threeIntervals := 3 * interval; threeIntervals >= period {
			t.Fatalf("RenewalInterval(%v) = %v; 3x = %v, want strictly less than the period",
				period, interval, threeIntervals)
		}
	}
}
