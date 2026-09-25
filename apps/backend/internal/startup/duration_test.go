package startup

import "testing"

func TestEstimateComponents(t *testing.T) {
	cases := []struct {
		name        string
		ms          int64
		wantValue   int64
		wantMinutes bool
	}{
		{"zero floors to one second", 0, 1, false},
		{"sub-second floors to one second", 250, 1, false},
		{"exact second stays whole", 1000, 1, false},
		{"partial second rounds up", 1001, 2, false},
		{"boundary of 60000ms renders in seconds", 60000, 60, false},
		{"just over the boundary renders in minutes", 60001, 2, true},
		{"exact minute stays whole", 120000, 2, true},
		{"partial minute rounds up", 120001, 3, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value, minutes := EstimateComponents(tc.ms)
			if value != tc.wantValue || minutes != tc.wantMinutes {
				t.Fatalf("EstimateComponents(%d) = (%d, %v), want (%d, %v)",
					tc.ms, value, minutes, tc.wantValue, tc.wantMinutes)
			}
		})
	}
}
