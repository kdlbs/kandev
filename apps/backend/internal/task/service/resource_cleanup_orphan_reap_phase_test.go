package service

import (
	"context"
	"encoding/json"
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

// A root whose existence cannot be confirmed either way (a stat error
// other than os.ErrNotExist) must not be treated as active — it is recorded
// as a detection-failure skip and never reaches the host snapshot.
func TestRunOrphanReapPhaseFailsClosedOnAmbiguousRootStatError(t *testing.T) {
	svc, _, _ := createTestService(t)
	base := t.TempDir()
	notADir := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ambiguousRoot := filepath.Join(notADir, "child") // Lstat returns ENOTDIR
	svc.orphanReapHostSnapshotter = fakeOrphanReapHostSnapshotter{
		snap: []hostProcess{{PID: 999, PPID: 1, Cwd: ambiguousRoot, Command: "sh"}},
	}
	job := &models.TaskResourceCleanupJob{TaskID: "task-a"}
	snapshot := &taskResourceCleanupSnapshot{}

	errs := svc.runOrphanReapPhase(context.Background(), job, snapshot, []string{ambiguousRoot})
	if len(errs) != 0 {
		t.Fatalf("expected no job errors from a phase-wide skip, got %v", errs)
	}
	if len(snapshot.OrphanReapRecords) != 0 {
		t.Fatalf("expected no candidate records for an ambiguous root, got %+v", snapshot.OrphanReapRecords)
	}
	if len(snapshot.OrphanReapSkips) != 1 {
		t.Fatalf("expected one skip for the ambiguous root, got %+v", snapshot.OrphanReapSkips)
	}
}

// poisonOrphanReapHostSnapshotter fails the test if Snapshot is ever called,
// proving a phase-level early return truly never reads the host.
type poisonOrphanReapHostSnapshotter struct{ t *testing.T }

func (p poisonOrphanReapHostSnapshotter) Snapshot(context.Context) ([]hostProcess, error) {
	p.t.Helper()
	p.t.Fatal("host snapshot must not be read when there are no reap roots")
	return nil, nil
}

// AC-TASKS-ORPHAN-REAP-007.1: zero roots skips the phase without ever
// reading the host process snapshot.
func TestRunOrphanReapPhaseSkipsSnapshotReadWhenNoRoots(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.orphanReapHostSnapshotter = poisonOrphanReapHostSnapshotter{t: t}
	job := &models.TaskResourceCleanupJob{TaskID: "task-a"}
	snapshot := &taskResourceCleanupSnapshot{}

	errs := svc.runOrphanReapPhase(context.Background(), job, snapshot, nil)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(snapshot.OrphanReapRoots) != 0 {
		t.Fatalf("expected no roots recorded, got %+v", snapshot.OrphanReapRoots)
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

// persistOrphanReapProgressBestEffort must actually persist reap
// progress recorded during a failed cleanup attempt, not merely take the
// empty-snapshot no-op path.
func TestPersistOrphanReapProgressBestEffortPersistsSnapshot(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	const taskID = "task-persist-progress"
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "ws-orphan-reap", Title: taskID}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	initial, err := json.Marshal(taskResourceCleanupSnapshot{})
	if err != nil {
		t.Fatalf("marshal initial snapshot: %v", err)
	}
	job := &models.TaskResourceCleanupJob{
		ID: "job-persist-progress", OperationID: "delete:" + taskID, TaskID: taskID,
		Trigger: models.TaskResourceCleanupTriggerDelete,
		State:   models.TaskResourceCleanupStatePending, ResourceSnapshot: string(initial),
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}
	claimed, err := repo.MarkTaskResourceCleanupJobRunning(ctx, job.ID)
	if err != nil || !claimed {
		t.Fatalf("MarkTaskResourceCleanupJobRunning: claimed=%v err=%v", claimed, err)
	}
	running, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}

	snapshot := &taskResourceCleanupSnapshot{
		OrphanReapRoots:   []string{"/tasks/" + taskID},
		OrphanReapRecords: []orphanReapCandidateRecord{{PID: 500, Outcome: orphanReapOutcomeTerminated}},
		OrphanReapSkips:   []orphanReapSkipRecord{{Root: "/tasks/" + taskID, Reason: "reap root exists again at reap time"}},
	}
	svc.persistOrphanReapProgressBestEffort(ctx, running, snapshot)

	persisted, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob after persist: %v", err)
	}
	var decoded taskResourceCleanupSnapshot
	if err := json.Unmarshal([]byte(persisted.ResourceSnapshot), &decoded); err != nil {
		t.Fatalf("unmarshal persisted snapshot: %v", err)
	}
	if len(decoded.OrphanReapRoots) != 1 || decoded.OrphanReapRoots[0] != "/tasks/"+taskID {
		t.Fatalf("expected persisted roots to survive, got %+v", decoded.OrphanReapRoots)
	}
	if len(decoded.OrphanReapRecords) != 1 || decoded.OrphanReapRecords[0].PID != 500 {
		t.Fatalf("expected persisted records to survive, got %+v", decoded.OrphanReapRecords)
	}
	if len(decoded.OrphanReapSkips) != 1 {
		t.Fatalf("expected persisted skips to survive, got %+v", decoded.OrphanReapSkips)
	}
}
