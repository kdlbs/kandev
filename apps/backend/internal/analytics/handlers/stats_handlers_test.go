package handlers

import (
	"testing"
	"time"
)

func TestParseStatsRange(t *testing.T) {
	tests := []struct {
		name         string
		rangeKey     string
		wantNilStart bool
		wantDays     int
	}{
		{
			name:         "week returns 7 days",
			rangeKey:     "week",
			wantNilStart: false,
			wantDays:     7,
		},
		{
			name:         "month returns 30 days",
			rangeKey:     "month",
			wantNilStart: false,
			wantDays:     30,
		},
		{
			name:         "all returns nil start and allTimeActivityDays",
			rangeKey:     "all",
			wantNilStart: true,
			wantDays:     allTimeActivityDays,
		},
		{
			name:         "empty string defaults to month",
			rangeKey:     "",
			wantNilStart: false,
			wantDays:     30,
		},
		{
			name:         "unknown value defaults to month",
			rangeKey:     "year",
			wantNilStart: false,
			wantDays:     30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, days := parseStatsRange(tt.rangeKey)

			if tt.wantNilStart {
				if start != nil {
					t.Errorf("expected nil start, got %v", start)
				}
			} else {
				if start == nil {
					t.Fatal("expected non-nil start, got nil")
				}
				// Verify start is approximately the right number of days ago
				expectedStart := time.Now().UTC().AddDate(0, 0, -tt.wantDays)
				diff := start.Sub(expectedStart)
				if diff < -time.Second || diff > time.Second {
					t.Errorf("expected start ~%v, got %v (diff: %v)", expectedStart, start, diff)
				}
			}

			if days != tt.wantDays {
				t.Errorf("expected days=%d, got %d", tt.wantDays, days)
			}
		})
	}
}
