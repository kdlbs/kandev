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
