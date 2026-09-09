package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/task/models"
)

// fakeOrphanReapHostSnapshotter never touches the real host: it returns a
// fixed snapshot or error, so phase-level tests can drive
// runOrphanReapPhase's branches deterministically.
type fakeOrphanReapHostSnapshotter struct {
	snap []hostProcess
	err  error
}

func (f fakeOrphanReapHostSnapshotter) Snapshot(context.Context) ([]hostProcess, error) {
	return f.snap, f.err
}

// AC-TASKS-ORPHAN-REAP-001.4: a root that exists again by reap time is
// skipped for this attempt (but stays recorded for a later one).
func TestRunOrphanReapPhaseSkipsRootThatExistsAgain(t *testing.T) {
	svc, _, _ := createTestService(t)
	root := t.TempDir() // still exists on disk
	job := &models.TaskResourceCleanupJob{TaskID: "task-a"}
	snapshot := &taskResourceCleanupSnapshot{}

	errs := svc.runOrphanReapPhase(context.Background(), job, snapshot, []string{root})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(snapshot.OrphanReapRecords) != 0 {
		t.Fatalf("expected no candidate records, got %+v", snapshot.OrphanReapRecords)
	}
	if len(snapshot.OrphanReapSkips) != 1 || snapshot.OrphanReapSkips[0].Root == "" {
		t.Fatalf("expected one root-level skip, got %+v", snapshot.OrphanReapSkips)
	}
	if len(snapshot.OrphanReapRoots) != 1 {
		t.Fatalf("expected the root to remain recorded for a later attempt, got %+v", snapshot.OrphanReapRoots)
	}
}

// AC-TASKS-ORPHAN-REAP-002.6: an unreadable/unavailable host snapshot fails
// the phase closed, not the job.
func TestRunOrphanReapPhaseSkipsPhaseWhenSnapshotUnavailable(t *testing.T) {
	svc, _, _ := createTestService(t)
	root := t.TempDir()
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	svc.orphanReapHostSnapshotter = fakeOrphanReapHostSnapshotter{err: errors.New("lsof unavailable")}
	job := &models.TaskResourceCleanupJob{TaskID: "task-a"}
	snapshot := &taskResourceCleanupSnapshot{}

	errs := svc.runOrphanReapPhase(context.Background(), job, snapshot, []string{root})
	if len(errs) != 0 {
		t.Fatalf("expected no job errors from a phase-wide skip, got %v", errs)
	}
	if len(snapshot.OrphanReapSkips) != 1 || snapshot.OrphanReapSkips[0].Root != "" {
		t.Fatalf("expected one phase-level skip (empty root), got %+v", snapshot.OrphanReapSkips)
	}
}

// AC-TASKS-ORPHAN-REAP-007.4: an unsupported platform is a phase-level skip,
// never a job failure.
func TestRunOrphanReapPhaseSkipsPhaseOnUnsupportedPlatform(t *testing.T) {
	svc, _, _ := createTestService(t)
	root := t.TempDir()
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	svc.orphanReapHostSnapshotter = fakeOrphanReapHostSnapshotter{err: errOrphanReapUnsupportedPlatform}
	job := &models.TaskResourceCleanupJob{TaskID: "task-a"}
	snapshot := &taskResourceCleanupSnapshot{}

	errs := svc.runOrphanReapPhase(context.Background(), job, snapshot, []string{root})
	if len(errs) != 0 {
		t.Fatalf("expected no job errors, got %v", errs)
	}
	if len(snapshot.OrphanReapSkips) != 1 {
		t.Fatalf("expected one phase-level skip, got %+v", snapshot.OrphanReapSkips)
	}
}

// AC-TASKS-ORPHAN-REAP-005.6: a snapshot with no cwd inside any root yields
// an empty result with no warning-level record.
func TestRunOrphanReapPhaseNoRecordsWhenNoCandidatesMatch(t *testing.T) {
	svc, _, _ := createTestService(t)
	root := t.TempDir()
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	svc.orphanReapHostSnapshotter = fakeOrphanReapHostSnapshotter{
		snap: []hostProcess{{PID: 999, PPID: 1, Cwd: "/unrelated/dir", Command: "sh"}},
	}
	job := &models.TaskResourceCleanupJob{TaskID: "task-a"}
	snapshot := &taskResourceCleanupSnapshot{}

	errs := svc.runOrphanReapPhase(context.Background(), job, snapshot, []string{root})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(snapshot.OrphanReapRecords) != 0 || len(snapshot.OrphanReapSkips) != 0 {
		t.Fatalf("expected no records or skips, got records=%+v skips=%+v",
			snapshot.OrphanReapRecords, snapshot.OrphanReapSkips)
	}
}

// AC-TASKS-ORPHAN-REAP-007.5: at most 256 candidates are signalled in one
// attempt; the remainder is deferred, surfaced as a retryable error.
func TestRunOrphanReapPhaseCapsCandidatesAt256(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, _, _ := createTestService(t)
		root := t.TempDir()
		if err := os.RemoveAll(root); err != nil {
			t.Fatalf("RemoveAll: %v", err)
		}

		const total = 300
		snap := make([]hostProcess, 0, total)
		verifier := newFakeOrphanReapVerifier()
		signaler := newFakeOrphanReapSignaler()
		for i := 1; i <= total; i++ {
			pid := i + 1000
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
		snapshot := &taskResourceCleanupSnapshot{}
		errs := svc.runOrphanReapPhase(context.Background(), job, snapshot, []string{root})

		foundCapErr := false
		for _, err := range errs {
			if errors.Is(err, errOrphanReapCandidateBoundReached) {
				foundCapErr = true
			}
		}
		if !foundCapErr {
			t.Fatalf("expected errOrphanReapCandidateBoundReached among %v", errs)
		}
		if len(snapshot.OrphanReapRecords) != orphanReapMaxCandidates {
			t.Fatalf("expected exactly %d candidate records, got %d", orphanReapMaxCandidates, len(snapshot.OrphanReapRecords))
		}
	})
}
