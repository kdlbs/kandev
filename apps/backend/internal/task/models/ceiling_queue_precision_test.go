package models

import (
	"testing"
	"time"
)

func TestCeilingQueuePreservesSubsecondArrival(t *testing.T) {
	original := time.Date(2026, 10, 16, 0, 0, 0, 123456789, time.UTC)
	keys := CeilingRecordKeys(CeilingDeferral{Kind: CeilingLaunchStart, QueuedAt: original})
	restored, err := ReadCeilingDeferral(keys)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.QueuedAt.Equal(original) {
		t.Fatalf("arrival order precision lost: stored=%v restored=%s want=%s", keys[CeilingQueuedAtKey], restored.QueuedAt.Format(time.RFC3339Nano), original.Format(time.RFC3339Nano))
	}
}
