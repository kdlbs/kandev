package backendapp

import (
	"context"
	"testing"
)

func TestPhase31Effective_SixteenCombinations(t *testing.T) {
	for mask := 0; mask < 16; mask++ {
		on, p2, p3, p31 := mask&1 != 0, mask&2 != 0, mask&4 != 0, mask&8 != 0
		if got, want := phase31Effective(on, p2, p3, p31), on && p2 && p3 && p31; got != want {
			t.Fatalf("phase31Effective(%v,%v,%v,%v) = %v, want %v", on, p2, p3, p31, got, want)
		}
	}
}

func TestInitCoordinatorWiring_Phase31FollowsAllFourFlags(t *testing.T) {
	for mask := 0; mask < 16; mask++ {
		on, p2, p3, p31 := mask&1 != 0, mask&2 != 0, mask&4 != 0, mask&8 != 0
		svc, err := initCoordinatorWiring(context.Background(), newCoordinatorTestPool(t), newCoordinatorTestTracker(t), nil, nil, nil, on, p2, p3, p31, newTestLogger())
		if err != nil {
			t.Fatalf("initCoordinatorWiring: %v", err)
		}
		if svc == nil {
			continue
		}
		if want := on && p2 && p3 && p31; svc.Phase31Enabled() != want {
			t.Fatalf("mask %d: Phase31Enabled = %v, want %v", mask, svc.Phase31Enabled(), want)
		}
	}
}
