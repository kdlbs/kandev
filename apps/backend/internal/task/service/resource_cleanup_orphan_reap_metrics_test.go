package service

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/task/models"
)

// orphan_reap_total previously had zero test coverage. These assert the
// counters actually increment for each recording path, using a before/after
// delta since the expvar.Map is process-global and shared across the whole
// test binary run.
func orphanReapCounterValue(key string) int64 {
	v := orphanReapCounters.Get(key)
	if v == nil {
		return 0
	}
	stringer, ok := v.(interface{ String() string })
	if !ok {
		return 0
	}
	n, err := strconv.ParseInt(stringer.String(), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func TestOrphanReapCountersIncrementOnCandidateOutcomes(t *testing.T) {
	beforeSeen := orphanReapCounterValue(orphanReapCounterSeen)
	beforeTerminated := orphanReapCounterValue(orphanReapCounterTerminated)
	beforeKilled := orphanReapCounterValue(orphanReapCounterKilled)
	beforeSurvived := orphanReapCounterValue(orphanReapCounterSurvived)
	beforeSkippedCandidate := orphanReapCounterValue(orphanReapCounterSkippedCandidate)

	svc := newOrphanReapSignalTestService()
	snapshot := &taskResourceCleanupSnapshot{}
	svc.recordOrphanReapCandidate(snapshot, "task-a", orphanReapCandidateRecord{PID: 500, Outcome: orphanReapOutcomeTerminated})
	svc.recordOrphanReapCandidate(snapshot, "task-a", orphanReapCandidateRecord{PID: 600, Outcome: orphanReapOutcomeKilled})
	svc.recordOrphanReapCandidate(snapshot, "task-a", orphanReapCandidateRecord{PID: 700, Outcome: orphanReapOutcomeSurvived})
	svc.recordOrphanReapCandidate(snapshot, "task-a", orphanReapCandidateRecord{PID: 800, Outcome: orphanReapOutcomeSkipped})

	if got := orphanReapCounterValue(orphanReapCounterSeen) - beforeSeen; got != 4 {
		t.Fatalf("expected seen to increment by 4, got %d", got)
	}
	if got := orphanReapCounterValue(orphanReapCounterTerminated) - beforeTerminated; got != 1 {
		t.Fatalf("expected terminated to increment by 1, got %d", got)
	}
	if got := orphanReapCounterValue(orphanReapCounterKilled) - beforeKilled; got != 1 {
		t.Fatalf("expected killed to increment by 1, got %d", got)
	}
	if got := orphanReapCounterValue(orphanReapCounterSurvived) - beforeSurvived; got != 1 {
		t.Fatalf("expected survived to increment by 1, got %d", got)
	}
	if got := orphanReapCounterValue(orphanReapCounterSkippedCandidate) - beforeSkippedCandidate; got != 1 {
		t.Fatalf("expected skipped_candidate to increment by 1, got %d", got)
	}
}

func TestOrphanReapCountersIncrementOnRootAndPhaseSkips(t *testing.T) {
	beforeRoot := orphanReapCounterValue(orphanReapCounterSkippedRoot)
	beforePhase := orphanReapCounterValue(orphanReapCounterSkippedPhase)

	svc := newOrphanReapSignalTestService()
	snapshot := &taskResourceCleanupSnapshot{}
	svc.recordOrphanReapRootSkip(snapshot, "/tasks/task-a", "reap root exists again at reap time")
	svc.recordOrphanReapRootSkipDetectionFailure(snapshot, "/tasks/task-b", "cannot confirm reap root state")
	svc.recordOrphanReapPhaseSkip(snapshot, "unsupported platform")
	svc.recordOrphanReapPhaseSkipDetectionFailure(snapshot, "host process snapshot unavailable")

	if got := orphanReapCounterValue(orphanReapCounterSkippedRoot) - beforeRoot; got != 2 {
		t.Fatalf("expected skipped_root to increment by 2, got %d", got)
	}
	if got := orphanReapCounterValue(orphanReapCounterSkippedPhase) - beforePhase; got != 2 {
		t.Fatalf("expected skipped_phase to increment by 2, got %d", got)
	}
}

// AC-TASKS-ORPHAN-REAP-007.5: the candidate bound counter increments when a
// phase attempt defers a remainder past orphanReapMaxCandidates.
func TestOrphanReapCountersIncrementOnCapReached(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		before := orphanReapCounterValue(orphanReapCounterCapReached)

		svc, _, _ := createTestService(t)
		root := t.TempDir()
		if err := os.RemoveAll(root); err != nil {
			t.Fatalf("RemoveAll: %v", err)
		}

		const total = orphanReapMaxCandidates + 5
		snap := make([]hostProcess, 0, total)
		verifier := newFakeOrphanReapVerifier()
		signaler := newFakeOrphanReapSignaler()
		for i := 1; i <= total; i++ {
			pid := i + 2000
			cwd := filepath.Join(root, strconv.Itoa(i))
			snap = append(snap, hostProcess{PID: pid, PPID: 1, Cwd: cwd, Command: "sh"})
			verifier.set(pid, cwd)
			signaler.setAlive(pid, true)
		}
		signaler.onSignal = func(pid int, sig orphanReapSignal) {
			if sig == orphanReapSigterm {
				signaler.setAlive(pid, false)
			}
		}
		svc.orphanReapHostSnapshotter = fakeOrphanReapHostSnapshotter{snap: snap}
		svc.orphanReapVerifier = verifier
		svc.orphanReapSignaler = signaler

		job := &models.TaskResourceCleanupJob{TaskID: "task-a"}
		snapshot := &taskResourceCleanupSnapshot{OrphanReapRoots: []string{root}}
		_ = svc.runOrphanReapPhase(context.Background(), job, snapshot)

		if got := orphanReapCounterValue(orphanReapCounterCapReached) - before; got != 1 {
			t.Fatalf("expected cap_reached to increment by 1, got %d", got)
		}
	})
}
