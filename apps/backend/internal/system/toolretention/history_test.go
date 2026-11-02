package toolretention

import (
	"context"
	"testing"
	"time"
)

type failingArchiveHistory struct{}

func (failingArchiveHistory) Analyze(context.Context, time.Time) (HistoryStats, error) {
	return HistoryStats{}, nil
}

func (failingArchiveHistory) HasWork(context.Context, time.Time) (bool, error) { return true, nil }

func (failingArchiveHistory) CompactOnce(context.Context, time.Time, int) (HistoryStats, error) {
	return HistoryStats{ArchiveFailures: 1, Remaining: 1}, nil
}

func TestHistoryCleanupBoundsPersistentArchiveFailures(t *testing.T) {
	service := New(nil, Options{History: failingArchiveHistory{}, Now: time.Now})
	record := &record{}
	record.Operation = &Operation{}
	for attempt := int64(1); attempt <= historyArchiveFailureLimit; attempt++ {
		applied, err := service.stepHistoryCleanup(context.Background(), record)
		if err != nil || !applied {
			t.Fatalf("history cleanup attempt %d: applied=%t err=%v", attempt, applied, err)
		}
	}
	if !record.Progress.HistoryDone {
		t.Fatal("persistently failing archive cleanup did not settle")
	}
	if record.Progress.HistoryArchiveFailures != historyArchiveFailureLimit {
		t.Fatalf("archive failure attempts = %d", record.Progress.HistoryArchiveFailures)
	}
}
