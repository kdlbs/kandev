package forgejo

import (
	"testing"
	"time"
)

func TestShouldPollIssueWatch(t *testing.T) {
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	if !shouldPollIssueWatch(nil, 60, now) {
		t.Fatal("a never-polled watch should run")
	}
	recent := now.Add(-30 * time.Second)
	if shouldPollIssueWatch(&recent, 60, now) {
		t.Fatal("a recent watch should not run")
	}
	due := now.Add(-60 * time.Second)
	if !shouldPollIssueWatch(&due, 60, now) {
		t.Fatal("a due watch should run")
	}
}
