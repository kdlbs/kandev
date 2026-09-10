package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"testing/synctest"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agentruntime"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
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
	snapshot := &taskResourceCleanupSnapshot{OrphanReapRoots: []string{root}}

	errs := svc.runOrphanReapPhase(context.Background(), job, snapshot)
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
	snapshot := &taskResourceCleanupSnapshot{OrphanReapRoots: []string{ambiguousRoot}}

	errs := svc.runOrphanReapPhase(context.Background(), job, snapshot)
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

	errs := svc.runOrphanReapPhase(context.Background(), job, snapshot)
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
	snapshot := &taskResourceCleanupSnapshot{OrphanReapRoots: []string{root}}

	errs := svc.runOrphanReapPhase(context.Background(), job, snapshot)
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
	snapshot := &taskResourceCleanupSnapshot{OrphanReapRoots: []string{root}}

	errs := svc.runOrphanReapPhase(context.Background(), job, snapshot)
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
	snapshot := &taskResourceCleanupSnapshot{OrphanReapRoots: []string{root}}

	errs := svc.runOrphanReapPhase(context.Background(), job, snapshot)
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
		snapshot := &taskResourceCleanupSnapshot{OrphanReapRoots: []string{root}}
		errs := svc.runOrphanReapPhase(context.Background(), job, snapshot)

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

// AC-TASKS-ORPHAN-REAP-006.4 + AC-TASKS-ORPHAN-REAP-007.5: candidates are
// processed in ascending PID order, and when the 256 cap truncates the list
// it is the 256 LOWEST PIDs that get signalled, in ascending order -- not an
// arbitrary or reversed 256. The host snapshot is built in descending PID
// order so a broken or missing sort would signal the wrong 256, in the wrong
// order, and this test would catch it (the sibling cap test above builds its
// snapshot in already-ascending order, which cannot tell the two apart).
func TestRunOrphanReapPhaseSignalsLowestPIDsInAscendingOrderWhenCapped(t *testing.T) {
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
		for i := total; i >= 1; i-- {
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
		snapshot := &taskResourceCleanupSnapshot{OrphanReapRoots: []string{root}}
		_ = svc.runOrphanReapPhase(context.Background(), job, snapshot)

		var sigtermPIDs []int
		for _, s := range signaler.sentSignals() {
			if s.sig == orphanReapSigterm {
				sigtermPIDs = append(sigtermPIDs, s.pid)
			}
		}
		if len(sigtermPIDs) != orphanReapMaxCandidates {
			t.Fatalf("expected exactly %d SIGTERMs sent, got %d: %v", orphanReapMaxCandidates, len(sigtermPIDs), sigtermPIDs)
		}
		for i, pid := range sigtermPIDs {
			wantPID := 1001 + i // the 256 lowest PIDs (1001..1256), ascending
			if pid != wantPID {
				t.Fatalf("expected SIGTERMs in ascending order over the 256 lowest PIDs starting at 1001, "+
					"got %v at index %d (want %d)", sigtermPIDs, i, wantPID)
			}
		}
	})
}

// blockedPIDPoisonSignaler wraps a fakeOrphanReapSignaler and fails the test
// the instant the ownership-blocked PID is signalled or liveness-checked, so
// the assertion holds even if the phase's ownership filter is the thing that
// silently regresses (rather than the signaler happening to no-op for it).
type blockedPIDPoisonSignaler struct {
	t       *testing.T
	blocked int
	*fakeOrphanReapSignaler
}

func (s blockedPIDPoisonSignaler) Signal(pid int, sig orphanReapSignal) error {
	if pid == s.blocked {
		s.t.Fatalf("Signal must never be called for the ownership-blocked pid %d", pid)
	}
	return s.fakeOrphanReapSignaler.Signal(pid, sig)
}

func (s blockedPIDPoisonSignaler) Alive(pid int) (alive, known bool) {
	if pid == s.blocked {
		s.t.Fatalf("Alive must never be called for the ownership-blocked pid %d", pid)
	}
	return s.fakeOrphanReapSignaler.Alive(pid)
}

// AC-TASKS-ORPHAN-REAP-003.3: a candidate excluded by the ownership filter
// must never reach the signaler at all, not merely end up unsignalled by
// coincidence. Deleting applyOrphanReapOwnership's ancestry check would let
// this candidate flow into signalOrphanReapCandidates alongside the unrelated
// one, and the poison signaler above would fail the test the moment that
// happened -- a gap where every prior test asserted only the final count.
func TestRunOrphanReapPhaseNeverSignalsOwnershipBlockedCandidate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, _, repo := createTestService(t)
		ctx := context.Background()
		mustCreateOrphanReapTask(t, repo, "task-a")
		mustCreateOrphanReapTask(t, repo, "task-other")
		if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "exec-other", SessionID: "sess-other", TaskID: "task-other", ExecutorID: "executor-1",
			Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
			LocalPID: 400,
		}); err != nil {
			t.Fatalf("UpsertExecutorRunning: %v", err)
		}

		root := t.TempDir()
		if err := os.RemoveAll(root); err != nil {
			t.Fatalf("RemoveAll: %v", err)
		}
		const blockedPID = 500
		const allowedPID = 600
		blockedCwd := filepath.Join(root, "blocked")
		allowedCwd := filepath.Join(root, "allowed")

		// Both PIDs get a valid reverify entry: the point of this test is that
		// the ownership filter itself excludes the blocked candidate before
		// signalling, not that some unrelated downstream check happens to
		// filter it out first.
		verifier := newFakeOrphanReapVerifier()
		verifier.set(blockedPID, blockedCwd)
		verifier.set(allowedPID, allowedCwd)
		fake := newFakeOrphanReapSignaler()
		fake.setAlive(allowedPID, true)
		fake.onSignal = func(pid int, sig orphanReapSignal) {
			if sig == orphanReapSigterm {
				fake.setAlive(pid, false)
			}
		}

		svc.orphanReapHostSnapshotter = fakeOrphanReapHostSnapshotter{snap: []hostProcess{
			{PID: 400, PPID: 1, Cwd: "/other", Command: "sh"},            // another task's local_pid
			{PID: blockedPID, PPID: 400, Cwd: blockedCwd, Command: "sh"}, // child of it: blocked
			{PID: allowedPID, PPID: 1, Cwd: allowedCwd, Command: "sh"},   // unrelated ancestry: allowed
		}}
		svc.orphanReapVerifier = verifier
		svc.orphanReapSignaler = blockedPIDPoisonSignaler{t: t, blocked: blockedPID, fakeOrphanReapSignaler: fake}

		job := &models.TaskResourceCleanupJob{TaskID: "task-a"}
		snapshot := &taskResourceCleanupSnapshot{OrphanReapRoots: []string{root}}
		_ = svc.runOrphanReapPhase(ctx, job, snapshot)

		foundBlockedSkip := false
		for _, rec := range snapshot.OrphanReapRecords {
			if rec.PID == blockedPID {
				if rec.Outcome != orphanReapOutcomeSkipped {
					t.Fatalf("expected blocked pid %d to be skipped, got outcome %q", blockedPID, rec.Outcome)
				}
				foundBlockedSkip = true
			}
		}
		if !foundBlockedSkip {
			t.Fatalf("expected a skip record for the ownership-blocked pid %d, got %+v", blockedPID, snapshot.OrphanReapRecords)
		}

		foundAllowedSignal := false
		for _, sent := range fake.sentSignals() {
			if sent.pid == allowedPID && sent.sig == orphanReapSigterm {
				foundAllowedSignal = true
			}
		}
		if !foundAllowedSignal {
			t.Fatalf("expected the unrelated candidate %d to still be signalled, got %+v", allowedPID, fake.sentSignals())
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
		OrphanReapRoots: []string{"/tasks/" + taskID},
		OrphanReapRecords: []orphanReapCandidateRecord{{
			PID: 500, Cwd: "/tasks/" + taskID + "/child", Root: "/tasks/" + taskID,
			Command: "sh", Outcome: orphanReapOutcomeTerminated,
		}},
		OrphanReapSkips: []orphanReapSkipRecord{{Root: "/tasks/" + taskID, Reason: "reap root exists again at reap time"}},
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
	if len(decoded.OrphanReapRecords) != 1 {
		t.Fatalf("expected persisted records to survive, got %+v", decoded.OrphanReapRecords)
	}
	gotRecord := decoded.OrphanReapRecords[0]
	wantRecord := snapshot.OrphanReapRecords[0]
	if gotRecord != wantRecord {
		t.Fatalf("persisted record = %+v, want every field to survive the DB round trip: %+v", gotRecord, wantRecord)
	}
	if len(decoded.OrphanReapSkips) != 1 {
		t.Fatalf("expected persisted skips to survive, got %+v", decoded.OrphanReapSkips)
	}
}

// AC-TASKS-ORPHAN-REAP-005.3: when the phase terminates or kills at least
// one candidate, it emits one additional aggregate warn log naming the
// task, the count, and each affected pid with its cwd, on top of the
// per-candidate logs recordOrphanReapCandidate already emits.
func TestRunOrphanReapPhaseEmitsAggregateWarnLogWhenCandidatesAreReaped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, _, _ := createTestService(t)
		core, logs := observer.New(zapcore.WarnLevel)
		log, logErr := commonlogger.NewFromZap(zap.New(core))
		if logErr != nil {
			t.Fatalf("create logger: %v", logErr)
		}
		svc.logger = log

		root := t.TempDir()
		if err := os.RemoveAll(root); err != nil {
			t.Fatalf("RemoveAll: %v", err)
		}
		const reapedPID = 700
		cwd := filepath.Join(root, "child")

		verifier := newFakeOrphanReapVerifier()
		verifier.set(reapedPID, cwd)
		fake := newFakeOrphanReapSignaler()
		fake.setAlive(reapedPID, true)
		fake.onSignal = func(pid int, sig orphanReapSignal) {
			if sig == orphanReapSigterm {
				fake.setAlive(pid, false)
			}
		}
		svc.orphanReapHostSnapshotter = fakeOrphanReapHostSnapshotter{snap: []hostProcess{
			{PID: reapedPID, PPID: 1, Cwd: cwd, Command: "sh"},
		}}
		svc.orphanReapVerifier = verifier
		svc.orphanReapSignaler = fake

		job := &models.TaskResourceCleanupJob{TaskID: "task-aggregate"}
		snapshot := &taskResourceCleanupSnapshot{OrphanReapRoots: []string{root}}
		_ = svc.runOrphanReapPhase(context.Background(), job, snapshot)

		var found []observer.LoggedEntry
		for _, entry := range logs.All() {
			if entry.Message == "orphan reap: phase reaped candidates" {
				found = append(found, entry)
			}
		}
		if len(found) != 1 {
			t.Fatalf("expected exactly one aggregate reap warn log, got %d: %+v", len(found), logs.All())
		}
		fields := found[0].ContextMap()
		if fields["task_id"] != "task-aggregate" {
			t.Fatalf("expected task_id field task-aggregate, got %+v", fields)
		}
		if fields["count"] != int64(1) {
			t.Fatalf("expected count=1, got %+v", fields["count"])
		}
		wantProcesses := fmt.Sprintf("%d:%s", reapedPID, cwd)
		if fields["processes"] != wantProcesses {
			t.Fatalf("expected processes=%q, got %+v", wantProcesses, fields["processes"])
		}
	})
}

// AC-TASKS-ORPHAN-REAP-005.1: the persisted record must carry the process
// identifier, resolved working directory, matched reap root, command name,
// and outcome. Drives a real candidate through runOrphanReapPhase end to
// end rather than hand-building a record, so a producer that stopped
// setting one of these fields would fail this test.
func TestRunOrphanReapPhaseRecordsAllFieldsOnATerminatedCandidate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, _, _ := createTestService(t)
		root := t.TempDir()
		if err := os.RemoveAll(root); err != nil {
			t.Fatalf("RemoveAll: %v", err)
		}
		const reapedPID = 800
		cwd := filepath.Join(root, "child")

		verifier := newFakeOrphanReapVerifier()
		verifier.set(reapedPID, cwd)
		fake := newFakeOrphanReapSignaler()
		fake.setAlive(reapedPID, true)
		fake.onSignal = func(pid int, sig orphanReapSignal) {
			if sig == orphanReapSigterm {
				fake.setAlive(pid, false)
			}
		}
		svc.orphanReapHostSnapshotter = fakeOrphanReapHostSnapshotter{snap: []hostProcess{
			{PID: reapedPID, PPID: 1, Cwd: cwd, Command: "sh"},
		}}
		svc.orphanReapVerifier = verifier
		svc.orphanReapSignaler = fake

		job := &models.TaskResourceCleanupJob{TaskID: "task-all-fields"}
		snapshot := &taskResourceCleanupSnapshot{OrphanReapRoots: []string{root}}
		if errs := svc.runOrphanReapPhase(context.Background(), job, snapshot); len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		rec, ok := findOrphanReapRecord(snapshot, reapedPID)
		if !ok {
			t.Fatalf("expected a record for pid %d, snapshot=%+v", reapedPID, snapshot.OrphanReapRecords)
		}
		if rec.Cwd != cwd {
			t.Fatalf("record.Cwd = %q, want %q", rec.Cwd, cwd)
		}
		if rec.Root != root {
			t.Fatalf("record.Root = %q, want %q", rec.Root, root)
		}
		if rec.Command != "sh" {
			t.Fatalf("record.Command = %q, want %q", rec.Command, "sh")
		}
		if rec.Outcome != orphanReapOutcomeTerminated {
			t.Fatalf("record.Outcome = %q, want %q", rec.Outcome, orphanReapOutcomeTerminated)
		}
	})
}
