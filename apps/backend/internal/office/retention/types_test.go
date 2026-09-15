package retention

import "testing"

func TestDefaultSettings_MatchesDocumentedDefaults(t *testing.T) {
	got := DefaultSettings()

	if !got.Enabled {
		t.Fatalf("Enabled = false, want true")
	}
	if got.SweepIntervalHours != 6 {
		t.Fatalf("SweepIntervalHours = %d, want 6", got.SweepIntervalHours)
	}
	if got.BatchLimit != 5000 {
		t.Fatalf("BatchLimit = %d, want 5000", got.BatchLimit)
	}
	wantRoutineRuns := TableSettings{WindowDays: 30, FloorPerOwner: 50, WarnRows: 25000}
	if got.RoutineRuns != wantRoutineRuns {
		t.Fatalf("RoutineRuns = %+v, want %+v", got.RoutineRuns, wantRoutineRuns)
	}
	wantRuns := TableSettings{WindowDays: 30, FloorPerOwner: 50, WarnRows: 25000}
	if got.Runs != wantRuns {
		t.Fatalf("Runs = %+v, want %+v", got.Runs, wantRuns)
	}
	if got.RunEvents.WarnRows != 250000 {
		t.Fatalf("RunEvents.WarnRows = %d, want 250000", got.RunEvents.WarnRows)
	}
}

func TestNormalizeSettings_AcceptsDefaults(t *testing.T) {
	normalized, err := NormalizeSettings(DefaultSettings())
	if err != nil {
		t.Fatalf("NormalizeSettings(defaults): %v", err)
	}
	if normalized != DefaultSettings() {
		t.Fatalf("NormalizeSettings(defaults) = %+v, want unchanged defaults", normalized)
	}
}

func TestNormalizeSettings_RejectsOutOfRangeFields(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Settings)
		wantErr string
	}{
		{"window too low", func(s *Settings) { s.RoutineRuns.WindowDays = 0 }, "routine_runs.window_days"},
		{"window too high", func(s *Settings) { s.Runs.WindowDays = 3651 }, "runs.window_days"},
		{"interval too low", func(s *Settings) { s.SweepIntervalHours = 0 }, "sweep_interval_hours"},
		{"interval too high", func(s *Settings) { s.SweepIntervalHours = 169 }, "sweep_interval_hours"},
		{"floor negative", func(s *Settings) { s.RoutineRuns.FloorPerOwner = -1 }, "routine_runs.floor_per_owner"},
		{"floor too high", func(s *Settings) { s.Runs.FloorPerOwner = 10001 }, "runs.floor_per_owner"},
		{"batch too low", func(s *Settings) { s.BatchLimit = 99 }, "batch_limit"},
		{"batch too high", func(s *Settings) { s.BatchLimit = 100001 }, "batch_limit"},
		{"warn negative", func(s *Settings) { s.RoutineRuns.WarnRows = -1 }, "routine_runs.warn_rows"},
		{"run_events warn negative", func(s *Settings) { s.RunEvents.WarnRows = -1 }, "run_events.warn_rows"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := DefaultSettings()
			tc.mutate(&s)
			_, err := NormalizeSettings(s)
			if err == nil {
				t.Fatalf("NormalizeSettings(%+v): want error, got nil", s)
			}
			if got := err.Error(); !containsField(got, tc.wantErr) {
				t.Fatalf("NormalizeSettings error = %q, want it to name field %q", got, tc.wantErr)
			}
		})
	}
}

func TestNormalizeSettings_ZeroWarnRowsDisablesThreshold(t *testing.T) {
	s := DefaultSettings()
	s.RoutineRuns.WarnRows = 0
	s.Runs.WarnRows = 0
	s.RunEvents.WarnRows = 0
	if _, err := NormalizeSettings(s); err != nil {
		t.Fatalf("NormalizeSettings with zero warn thresholds: %v", err)
	}
}

func containsField(msg, field string) bool {
	return len(msg) >= len(field) && (indexOf(msg, field) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
