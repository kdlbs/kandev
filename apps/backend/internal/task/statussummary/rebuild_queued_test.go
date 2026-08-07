package statussummary

import (
	"testing"
	"time"
)

func TestBuildFromAuthoritativeIncludesQueuedPromptCount(t *testing.T) {
	now := time.Date(2026, 8, 1, 18, 0, 0, 0, time.UTC)
	got := BuildFromAuthoritative(RebuildInput{
		Now:               now,
		QueuedPromptCount: 7,
	})
	if got.QueuedPromptCount != 7 {
		t.Fatalf("queued prompt count = %d, want 7", got.QueuedPromptCount)
	}
	if got.QueuedPromptCount != 0 && got.PrimarySession != nil {
		t.Fatalf("unexpected session derivation: %+v", got)
	}
}

func TestBuildFromAuthoritativeDefaultsQueuedPromptCountToZero(t *testing.T) {
	got := BuildFromAuthoritative(RebuildInput{Now: time.Now().UTC()})
	if got.QueuedPromptCount != 0 {
		t.Fatalf("queued prompt count = %d, want 0", got.QueuedPromptCount)
	}
}
