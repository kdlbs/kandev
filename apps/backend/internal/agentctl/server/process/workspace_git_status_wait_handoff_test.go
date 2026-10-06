package process

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21
func TestWorkspaceTrackerDetailsWaitAfterObservationEnrichment(t *testing.T) {
	for _, replace := range []bool{false, true} {
		name := "completed enrichment"
		if replace {
			name = "superseding observation"
		}
		t.Run(name, func(t *testing.T) {
			repoDir, cleanup := setupTestRepo(t)
			defer cleanup()
			writeFile(t, repoDir, "README.md", "# Test Repo\nhandoff change\n")
			tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
			t.Cleanup(tracker.Stop)
			started := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			tracker.gitStatusBeforeEnrich = func() {
				close(started)
				<-release
			}
			tracker.gitStatusWaiterJoined = func() {
				waitForSignal(t, started, "enrichment started before basic observation returns")
				tracker.gitStatusEnrichmentMu.Lock()
				job := tracker.gitStatusEnrichmentJob
				tracker.gitStatusEnrichmentMu.Unlock()
				if job == nil {
					t.Fatal("accepted observation has no enrichment job")
				}
				releaseOnce.Do(func() { close(release) })
				waitForSignal(t, job.done, "enrichment completed before basic observation returns")
				if replace {
					status := tracker.currentGitStatus()
					status.HeadCommit = "replacement-head"
					tracker.publishGitStatus(status, tracker.gitStatusObservationID.Add(1), "replacement")
				}
			}

			status, err := tracker.GetGitStatusWithDetails(context.Background(), true)
			if replace {
				if !errors.Is(err, errGitStatusEvidenceChanged) || status.HeadCommit != "replacement-head" {
					t.Fatalf("superseded details = %+v, %v; want evidence-changed error and replacement", status, err)
				}
				return
			}
			if err != nil || status.DetailState != gitStatusDetailReady || !strings.Contains(status.Files["README.md"].Diff, "handoff change") {
				t.Fatalf("completed details = %+v, %v; want ready details from the accepted observation", status, err)
			}
		})
	}
}
