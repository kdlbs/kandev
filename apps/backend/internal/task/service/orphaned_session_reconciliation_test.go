package service

import (
	"context"
	"sync"
	"testing"
	"time"

	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

// fakeExecutionLiveness is the test double for the orphan sweep's
// execution-liveness seam: only session IDs in live map as backed by a live
// in-memory execution.
type fakeExecutionLiveness struct {
	live map[string]bool
}

func (f *fakeExecutionLiveness) HasLiveExecution(sessionID string) bool {
	return f.live[sessionID]
}

// seedOrphanSweepFixtures creates the one workspace and workflow every
// orphan-sweep test task hangs off.
func seedOrphanSweepFixtures(t *testing.T, repo *sqliterepo.Repository) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-orphan", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-orphan", WorkspaceID: "ws-orphan", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
}

func seedOrphanSweepTask(t *testing.T, repo *sqliterepo.Repository, taskID string, archived bool) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws-orphan", WorkflowID: "wf-orphan", WorkflowStepID: "step-1",
		Title: "Test " + taskID, Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask(%s): %v", taskID, err)
	}
	if archived {
		if _, err := repo.DB().ExecContext(ctx,
			`UPDATE tasks SET archived_at = ? WHERE id = ?`, time.Now().UTC(), taskID); err != nil {
			t.Fatalf("archive %s via SQL: %v", taskID, err)
		}
	}
}

func TestService_OrphanedSessionReconciliationTerminalizesUnbackedSessions(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedOrphanSweepFixtures(t, repo)

	stale := time.Now().UTC().Add(-30 * time.Minute)
	fresh := time.Now().UTC().Add(-1 * time.Minute)

	seedOrphanSweepTask(t, repo, "task-orphaned", false)
	seedOrphanSweepTask(t, repo, "task-live-exec", false)
	seedOrphanSweepTask(t, repo, "task-inflight-launch", false)
	seedOrphanSweepTask(t, repo, "task-primary", false)
	seedOrphanSweepTask(t, repo, "task-archived", true)

	seedSession := func(id, taskID string, state models.TaskSessionState, updatedAt time.Time) {
		t.Helper()
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: id, TaskID: taskID, State: state,
			AgentProfileID: "agent-1", IsPrimary: true, UpdatedAt: updatedAt, StartedAt: updatedAt,
		}); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", id, err)
		}
	}
	// The orphan this sweep exists for: unarchived task, stale RUNNING row,
	// nothing in the execution store backs it (backend restart mid-turn).
	seedSession("session-orphaned-running", "task-orphaned", models.TaskSessionStateRunning, stale)
	// Same orphan class in STARTING: the launch died with the process.
	seedSession("session-orphaned-starting", "task-orphaned", models.TaskSessionStateStarting, stale)
	// Healthy sibling of the orphaned task: waiting for user input, must
	// survive the sweep even though the task lost its RUNNING session.
	seedSession("session-healthy-sibling", "task-orphaned", models.TaskSessionStateWaitingForInput, stale)
	// Stale RUNNING row that a live execution still backs (e.g. re-tracked by
	// startup recovery): must never be terminalized.
	seedSession("session-live-execution", "task-live-exec", models.TaskSessionStateRunning, stale)
	// Fresh RUNNING row: an in-flight launch may not have reached the
	// in-memory store yet, so the grace window must protect it.
	seedSession("session-inflight-launch", "task-inflight-launch", models.TaskSessionStateRunning, fresh)
	// Archived task with a stale RUNNING session: the archived pass owns it,
	// the orphan pass must not touch it.
	seedSession("session-archived-task", "task-archived", models.TaskSessionStateRunning, stale)
	// Primary session on the orphaned task: the CANCELLED event must carry
	// is_primary=true so the status-summary projection keeps the durable
	// primary assignment.
	seedSession("session-primary", "task-primary", models.TaskSessionStateRunning, stale)

	svc.SetExecutionLivenessChecker(&fakeExecutionLiveness{
		live: map[string]bool{"session-live-execution": true},
	})
	svc.runOrphanedSessionReconciliation(ctx)

	assertState := func(id string, want models.TaskSessionState) {
		t.Helper()
		session, err := repo.GetTaskSession(ctx, id)
		if err != nil {
			t.Fatalf("GetTaskSession(%s): %v", id, err)
		}
		if session.State != want {
			t.Errorf("session %s state = %q, want %q", id, session.State, want)
		}
	}
	assertState("session-orphaned-running", models.TaskSessionStateCancelled)
	assertState("session-orphaned-starting", models.TaskSessionStateCancelled)
	assertState("session-healthy-sibling", models.TaskSessionStateWaitingForInput)
	assertState("session-live-execution", models.TaskSessionStateRunning)
	assertState("session-inflight-launch", models.TaskSessionStateRunning)
	assertState("session-archived-task", models.TaskSessionStateRunning)
	assertState("session-primary", models.TaskSessionStateCancelled)

	orphaned, err := repo.GetTaskSession(ctx, "session-orphaned-running")
	if err != nil {
		t.Fatalf("GetTaskSession(orphaned): %v", err)
	}
	if orphaned.ErrorMessage != models.SessionOrphanedCancelReason {
		t.Errorf("orphaned session error_message = %q, want %q",
			orphaned.ErrorMessage, models.SessionOrphanedCancelReason)
	}

	// The event must carry the durable is_primary flag: the primary session's
	// cancellation reports is_primary=true rather than the zero value.
	if data := sessionCancelledEvent(eventBus, "session-primary"); data != nil {
		if isPrimary, ok := data["is_primary"].(bool); !ok || !isPrimary {
			t.Errorf("primary session cancellation event is_primary = %v, want true", data["is_primary"])
		}
	} else {
		t.Error("expected a cancellation event for session-primary")
	}

	for _, id := range []string{"session-orphaned-running", "session-orphaned-starting", "session-primary"} {
		if !sessionCancelledEventPublished(eventBus, id) {
			t.Errorf("expected a session.state_changed event for %s from the orphan sweep, got none", id)
		}
	}
	for _, id := range []string{"session-healthy-sibling", "session-live-execution", "session-inflight-launch", "session-archived-task"} {
		if sessionCancelledEventPublished(eventBus, id) {
			t.Errorf("unexpected session.state_changed event for %s", id)
		}
	}
}

func sessionCancelledEventPublished(eventBus *MockEventBus, sessionID string) bool {
	for _, evt := range eventBus.GetPublishedEvents() {
		if evt.Type != events.TaskSessionStateChanged {
			continue
		}
		data, ok := evt.Data.(map[string]interface{})
		if ok && data["session_id"] == sessionID && data["new_state"] == string(models.TaskSessionStateCancelled) {
			return true
		}
	}
	return false
}

// sessionCancelledEvent returns the last session.state_changed payload for
// sessionID, or nil when none published. The orphan sweep's event must carry
// the durable is_primary flag — the status-summary projector demotes the
// session when a cancellation event reports is_primary: false.
func sessionCancelledEvent(eventBus *MockEventBus, sessionID string) map[string]interface{} {
	var found map[string]interface{}
	for _, evt := range eventBus.GetPublishedEvents() {
		if evt.Type != events.TaskSessionStateChanged {
			continue
		}
		data, ok := evt.Data.(map[string]interface{})
		if ok && data["session_id"] == sessionID && data["new_state"] == string(models.TaskSessionStateCancelled) {
			found = data
		}
	}
	return found
}

// TestService_OrphanedSessionReconciliationRequiresLivenessChecker pins the
// fail-safe: with no execution-liveness seam wired, the sweep must be inert —
// absence-from-store is its only dead signal, and a nil checker can never
// prove a session unbacked.
func TestService_OrphanedSessionReconciliationRequiresLivenessChecker(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedOrphanSweepFixtures(t, repo)
	seedOrphanSweepTask(t, repo, "task-orphaned", false)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-orphaned", TaskID: "task-orphaned", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-1", IsPrimary: true,
		UpdatedAt: time.Now().UTC().Add(-30 * time.Minute), StartedAt: time.Now().UTC().Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	// No SetExecutionLivenessChecker call.
	svc.runOrphanedSessionReconciliation(ctx)

	session, err := repo.GetTaskSession(ctx, "session-orphaned")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q, want RUNNING (sweep must stay inert without the liveness seam)", session.State)
	}
}

func TestService_OrphanedSessionReconciliationStopsAtAdmissionDeadline(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	setupCtx := context.Background()
	seedOrphanSweepFixtures(t, repo)
	seedOrphanSweepTask(t, repo, "task-expired-pass", false)
	stale := time.Now().UTC().Add(-30 * time.Minute)
	if err := repo.CreateTaskSession(setupCtx, &models.TaskSession{
		ID: "session-expired-pass", TaskID: "task-expired-pass", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-1", IsPrimary: true, UpdatedAt: stale, StartedAt: stale,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	candidate, err := repo.GetTaskSession(setupCtx, "session-expired-pass")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	svc.SetExecutionLivenessChecker(&fakeExecutionLiveness{})

	expiredCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	svc.reconcileOrphanedSessions(expiredCtx, []*models.TaskSession{candidate}, stale)

	session, err := repo.GetTaskSession(setupCtx, "session-expired-pass")
	if err != nil {
		t.Fatalf("GetTaskSession after expired pass: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q, want RUNNING after an expired pass", session.State)
	}
	if sessionCancelledEventPublished(eventBus, "session-expired-pass") {
		t.Fatal("expired pass must not publish a cancellation event")
	}
}

type delayedOrphanSessionRepository struct {
	*sqliterepo.Repository
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *delayedOrphanSessionRepository) CancelRunningTaskSessionByID(
	ctx context.Context,
	sessionID, reason string,
	staleBefore time.Time,
) (*models.TaskSession, error) {
	r.once.Do(func() { close(r.entered) })
	<-r.release
	return r.Repository.CancelRunningTaskSessionByID(ctx, sessionID, reason, staleBefore)
}

func TestService_OrphanedSessionReconciliationUsesFreshEffectsContextAfterDeadline(t *testing.T) {
	delayed := &delayedOrphanSessionRepository{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	svc, eventBus, repo := createTestServiceWithSessionsRepo(t, func(repo *sqliterepo.Repository) repository.SessionRepository {
		delayed.Repository = repo
		return delayed
	})
	setupCtx := context.Background()
	seedOrphanSweepFixtures(t, repo)
	seedOrphanSweepTask(t, repo, "task-late-effects", false)
	stale := time.Now().UTC().Add(-30 * time.Minute)
	if err := repo.CreateTaskSession(setupCtx, &models.TaskSession{
		ID: "session-late-effects", TaskID: "task-late-effects", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-1", IsPrimary: true, UpdatedAt: stale, StartedAt: stale,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	candidate, err := repo.GetTaskSession(setupCtx, "session-late-effects")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	clarifications := &recordingTaskClarificationCanceller{}
	svc.SetClarificationCanceller(clarifications)
	svc.SetExecutionLivenessChecker(&fakeExecutionLiveness{})

	passCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		svc.reconcileOrphanedSessions(passCtx, []*models.TaskSession{candidate}, stale.Add(time.Second))
		close(done)
	}()
	select {
	case <-delayed.entered:
	case <-time.After(time.Second):
		t.Fatal("orphan cancellation did not start")
	}
	select {
	case <-passCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("reconciliation pass did not reach its deadline")
	}
	close(delayed.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reconciliation did not finish after the delayed write was released")
	}

	if len(clarifications.contextErrs) != 1 || clarifications.contextErrs[0] != nil {
		t.Fatalf("clarification cleanup context errors = %v, want one nil error after a late commit", clarifications.contextErrs)
	}
	if !sessionCancelledEventPublished(eventBus, "session-late-effects") {
		t.Fatal("expected cancellation event after the delayed write committed")
	}
}

// TestService_OrphanedSessionReconciliationSparesRowRefreshedSinceCandidateRead
// pins the read-then-write guard: a launch CAS-writes its session to STARTING
// (bumping updated_at) before it registers an execution in the in-memory
// store, so the row can be refreshed between the sweep's candidate read and
// its cancellation write. The cancel's staleBefore predicate must fail to
// match that refreshed row instead of cancelling a launch in progress.
func TestService_OrphanedSessionReconciliationSparesRowRefreshedSinceCandidateRead(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedOrphanSweepFixtures(t, repo)
	seedOrphanSweepTask(t, repo, "task-relaunch", false)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-relaunch", TaskID: "task-relaunch", State: models.TaskSessionStateStarting,
		AgentProfileID: "agent-1", IsPrimary: true,
		UpdatedAt: time.Now().UTC().Add(-30 * time.Minute), StartedAt: time.Now().UTC().Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	// Simulate the race: the candidate read sees the stale row, then an
	// in-flight launch refreshes it before the sweep's write lands.
	intercept := &livenessInterceptChecker{delegate: &fakeExecutionLiveness{}}
	svc.SetExecutionLivenessChecker(intercept)
	intercept.onCheck = func(sessionID string) {
		if sessionID != "session-relaunch" || intercept.refreshed {
			return
		}
		intercept.refreshed = true
		now := time.Now().UTC()
		if _, err := repo.DB().ExecContext(ctx,
			`UPDATE task_sessions SET state = ?, updated_at = ? WHERE id = ?`,
			string(models.TaskSessionStateStarting), now, sessionID); err != nil {
			t.Errorf("refresh session during liveness check: %v", err)
		}
	}

	svc.runOrphanedSessionReconciliation(ctx)

	session, err := repo.GetTaskSession(ctx, "session-relaunch")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateStarting {
		t.Fatalf("session state = %q, want STARTING (a row refreshed since the candidate read must not be reaped)", session.State)
	}
	if sessionCancelledEventPublished(eventBus, "session-relaunch") {
		t.Error("unexpected session.state_changed event for the refreshed session")
	}
}

// livenessInterceptChecker delegates to a fake liveness checker and lets a
// test mutate the DB between the sweep's liveness check and its write,
// reproducing the in-flight-launch race window.
type livenessInterceptChecker struct {
	delegate  *fakeExecutionLiveness
	refreshed bool
	onCheck   func(sessionID string)
}

func (l *livenessInterceptChecker) HasLiveExecution(sessionID string) bool {
	if l.onCheck != nil {
		l.onCheck(sessionID)
	}
	return l.delegate.HasLiveExecution(sessionID)
}

// TestSessionOrphanedCancelReasonIsNotArchiveReason pins the reason taxonomy:
// resume paths branch on IsArchiveCancelReason, so the orphan reason must
// stay distinct from the archive reasons.
func TestSessionOrphanedCancelReasonIsNotArchiveReason(t *testing.T) {
	if models.IsArchiveCancelReason(models.SessionOrphanedCancelReason) {
		t.Fatal("SessionOrphanedCancelReason must not be an archive cancel reason")
	}
}
