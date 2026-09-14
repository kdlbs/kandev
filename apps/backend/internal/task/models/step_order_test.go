package models

import (
	"testing"
	"time"
)

func TestStepOrderLess(t *testing.T) {
	earlier := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := earlier.Add(time.Hour)

	cases := []struct {
		name        string
		left, right *Task
		want        bool
	}{
		{
			name:  "lower position wins",
			left:  &Task{ID: "a", Position: 1, Priority: "low"},
			right: &Task{ID: "b", Position: 2, Priority: "critical"},
			want:  true,
		},
		{
			name:  "critical beats high at equal position",
			left:  &Task{ID: "a", Position: 1, Priority: "critical"},
			right: &Task{ID: "b", Position: 1, Priority: "high"},
			want:  true,
		},
		{
			name:  "known priority beats an unrecognized one",
			left:  &Task{ID: "a", Position: 1, Priority: "low"},
			right: &Task{ID: "b", Position: 1, Priority: "whenever"},
			want:  true,
		},
		{
			name:  "a nil queued_at falls back to created_at instead of skipping the key",
			left:  &Task{ID: "a", Position: 1, Priority: "medium", QueuedAt: nil, CreatedAt: earlier},
			right: &Task{ID: "b", Position: 1, Priority: "medium", QueuedAt: &earlier, CreatedAt: later},
			want:  true,
		},
		{
			// left's fallback (its own created_at, later) loses to right's
			// earlier explicit queued_at, even though right's created_at is
			// later still - discriminates the fallback from two wrong
			// implementations that would both also report true here: (a)
			// treating a nil queued_at as an always-smallest sentinel, and
			// (b) comparing created_at directly instead of queued_at.
			name:  "a nil queued_at's created_at fallback loses to an earlier explicit queued_at",
			left:  &Task{ID: "a", Position: 1, Priority: "medium", QueuedAt: nil, CreatedAt: later},
			right: &Task{ID: "b", Position: 1, Priority: "medium", QueuedAt: &earlier, CreatedAt: later.Add(time.Hour)},
			want:  false,
		},
		{
			// The mirror case: left's fallback (its own created_at, earlier)
			// beats right's later explicit queued_at, even though right's
			// created_at is earlier still - discriminates the fallback from
			// comparing created_at directly instead of queued_at.
			name:  "a nil queued_at's created_at fallback beats a later explicit queued_at",
			left:  &Task{ID: "a", Position: 1, Priority: "medium", QueuedAt: nil, CreatedAt: earlier},
			right: &Task{ID: "b", Position: 1, Priority: "medium", QueuedAt: &later, CreatedAt: earlier.Add(-time.Hour)},
			want:  true,
		},
		{
			name:  "created_at breaks a queued_at tie",
			left:  &Task{ID: "a", Position: 1, Priority: "medium", QueuedAt: &earlier, CreatedAt: earlier},
			right: &Task{ID: "b", Position: 1, Priority: "medium", QueuedAt: &earlier, CreatedAt: later},
			want:  true,
		},
		{
			name:  "id breaks a full tie",
			left:  &Task{ID: "a", Position: 1, Priority: "medium", QueuedAt: &earlier, CreatedAt: earlier},
			right: &Task{ID: "b", Position: 1, Priority: "medium", QueuedAt: &earlier, CreatedAt: earlier},
			want:  true,
		},
		{
			name:  "identical tasks are not before each other",
			left:  &Task{ID: "same", Position: 1, Priority: "medium", QueuedAt: &earlier, CreatedAt: earlier},
			right: &Task{ID: "same", Position: 1, Priority: "medium", QueuedAt: &earlier, CreatedAt: earlier},
			want:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StepOrderLess(tc.left, tc.right); got != tc.want {
				t.Fatalf("StepOrderLess = %v, want %v", got, tc.want)
			}
		})
	}
}
