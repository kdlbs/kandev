package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// stubExecutionRegistry is the test double for SessionExecutionRegistry: it
// reports a fixed set of session IDs as having a live in-memory execution.
// With switchToLiveAfterFirstCall set, the first LiveSessionIDsForTask call
// reports nothing (classification sees no live execution) and every later
// call reports liveSessions — reproducing a mid-sweep registration race.
type stubExecutionRegistry struct {
	liveSessions               []string
	switchToLiveAfterFirstCall bool
	calls                      int
}

func (r *stubExecutionRegistry) LiveSessionIDsForTask(taskID string) []string {
	r.calls++
	if r.switchToLiveAfterFirstCall && r.calls > 1 {
		return r.liveSessions
	}
	if r.switchToLiveAfterFirstCall {
		return nil
	}
	return r.liveSessions
}

// sweepFixture seeds one workspace, workflow, task, and session, wires the
// stub registry, and returns the fixed sweep clock so tests can place the
// session's UpdatedAt relative to it deterministically.
type sweepFixture struct {
	svc      *Service
	eventBus *MockEventBus
	repo     interface {
		GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
		CreateTaskSession(ctx context.Context, session *models.TaskSession) error
	}
	registry *stubExecutionRegistry
	now      time.Time
	rawRepo  *sqliterepo.Repository
}

// newSweepFixture creates a task holding one RUNNING session whose row went
// quiet silence before the sweep clock.
func newSweepFixture(t *testing.T, silence time.Duration) *sweepFixture {
	t.Helper()
	registry := &stubExecutionRegistry{}
	svc, eventBus, repo := createTestService(t)
	svc.SetSessionExecutionRegistry(registry)
	svc.SetStallDetectionThreshold(2 * time.Hour)

	now := time.Now()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		Title: "Sweep test", Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-1", IsPrimary: true, UpdatedAt: now.Add(-silence),
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	return &sweepFixture{
		svc:      svc,
		eventBus: eventBus,
		repo:     repo,
		registry: registry,
		now:      now,
		rawRepo:  repo,
	}
}

func findTaskStalledEvents(eventBus *MockEventBus) []*bus.Event {
	var found []*bus.Event
	for _, evt := range eventBus.GetPublishedEvents() {
		if evt.Type == events.TaskStalled {
			found = append(found, evt)
		}
	}
	return found
}

// TestService_ActiveSessionSweepEmitsStalledTaskEvent is the #3712
// regression: an unarchived task holding an active session with no live
// execution and no events for longer than the stall threshold must surface a
// task.stalled event instead of stalling silently.
func TestService_ActiveSessionSweepEmitsStalledTaskEvent(t *testing.T) {
	// 3h of silence: past the 2h threshold, still inside the 4h healing
	// grace window.
	fixture := newSweepFixture(t, 3*time.Hour)

	fixture.svc.runActiveSessionSweep(context.Background(), fixture.now)

	stalled := findTaskStalledEvents(fixture.eventBus)
	if len(stalled) != 1 {
		t.Fatalf("task.stalled events = %d, want 1", len(stalled))
	}
	data, ok := stalled[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("task.stalled payload type = %T, want map", stalled[0].Data)
	}
	if got := data["task_id"]; got != "task-1" {
		t.Errorf("task_id = %v, want task-1", got)
	}
	if got := data["workspace_id"]; got != "ws-1" {
		t.Errorf("workspace_id = %v, want ws-1", got)
	}
	ids, _ := data["session_ids"].([]string)
	if len(ids) != 1 || ids[0] != "session-1" {
		t.Errorf("session_ids = %v, want [session-1]", data["session_ids"])
	}

	// Detection only: the session must still be active in the DB.
	session, err := fixture.repo.GetTaskSession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("session state after detection = %q, want RUNNING (detection must not heal)", session.State)
	}
}

// TestService_ActiveSessionSweepSkipsLiveExecution proves the ExecutionStore
// guard: an event-silent session that still has a registered live execution
// is healthy and must not fire a stall event.
func TestService_ActiveSessionSweepSkipsLiveExecution(t *testing.T) {
	fixture := newSweepFixture(t, 3*time.Hour)
	fixture.registry.liveSessions = []string{"session-1"}

	fixture.svc.runActiveSessionSweep(context.Background(), fixture.now)

	if stalled := findTaskStalledEvents(fixture.eventBus); len(stalled) != 0 {
		t.Fatalf("task.stalled events = %d, want 0 for a session with a live execution", len(stalled))
	}
}

// TestService_ActiveSessionSweepSkipsRecentSessions proves the silence
// window: an execution-less session that went quiet inside the threshold is
// not yet stalled (e.g. a launch that has not registered its execution).
func TestService_ActiveSessionSweepSkipsRecentSessions(t *testing.T) {
	// 30 minutes of silence: below the 2h threshold.
	fixture := newSweepFixture(t, 30*time.Minute)

	fixture.svc.runActiveSessionSweep(context.Background(), fixture.now)

	if stalled := findTaskStalledEvents(fixture.eventBus); len(stalled) != 0 {
		t.Fatalf("task.stalled events = %d, want 0 for a session inside the threshold", len(stalled))
	}
}

// TestService_ActiveSessionSweepStallEventEmittedOncePerEpisode proves the
// episode dedupe: a sweep ticking every minute must not re-emit task.stalled
// for the same stalled session, but must report it again after the episode
// ends (a live execution reappears) and a new stall begins.
func TestService_ActiveSessionSweepStallEventEmittedOncePerEpisode(t *testing.T) {
	fixture := newSweepFixture(t, 3*time.Hour)

	fixture.svc.runActiveSessionSweep(context.Background(), fixture.now)
	fixture.svc.runActiveSessionSweep(context.Background(), fixture.now.Add(time.Minute))
	if stalled := findTaskStalledEvents(fixture.eventBus); len(stalled) != 1 {
		t.Fatalf("task.stalled events after two passes = %d, want 1 (once per episode)", len(stalled))
	}

	// Episode ends: the session gains a live execution.
	fixture.registry.liveSessions = []string{"session-1"}
	fixture.svc.runActiveSessionSweep(context.Background(), fixture.now.Add(2*time.Minute))
	fixture.registry.liveSessions = nil

	// A later pass in a new episode reports the stall again.
	fixture.svc.runActiveSessionSweep(context.Background(), fixture.now.Add(3*time.Minute))
	if stalled := findTaskStalledEvents(fixture.eventBus); len(stalled) != 2 {
		t.Fatalf("task.stalled events across two episodes = %d, want 2", len(stalled))
	}
}

// TestService_ActiveSessionSweepHealsOrphanedSessions is the #3711 half of
// the sweep: once an execution-less session has been silent beyond the grace
// window (twice the threshold), the sweep cancels it through the same
// finalizeCancelledSessions transition the archived pass uses, so the DB
// state becomes truthful and the session.state_changed event fires.
func TestService_ActiveSessionSweepHealsOrphanedSessions(t *testing.T) {
	// 5h of silence: past the 4h grace window (2 x 2h threshold).
	fixture := newSweepFixture(t, 5*time.Hour)

	fixture.svc.runActiveSessionSweep(context.Background(), fixture.now)

	session, err := fixture.repo.GetTaskSession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateCancelled {
		t.Fatalf("session state after healing = %q, want CANCELLED", session.State)
	}
	if session.ErrorMessage != models.SessionOrphanedCancelReason {
		t.Fatalf("session error_message = %q, want %q", session.ErrorMessage, models.SessionOrphanedCancelReason)
	}

	var stateChanged *bus.Event
	for _, evt := range fixture.eventBus.GetPublishedEvents() {
		if evt.Type != events.TaskSessionStateChanged {
			continue
		}
		if data, ok := evt.Data.(map[string]interface{}); ok && data["session_id"] == "session-1" {
			stateChanged = evt
		}
	}
	if stateChanged == nil {
		t.Fatal("expected a session.state_changed event for session-1 from the healing pass, got none")
	}
	data := stateChanged.Data.(map[string]interface{})
	if got := data["new_state"]; got != string(models.TaskSessionStateCancelled) {
		t.Errorf("new_state = %v, want CANCELLED", got)
	}
}

// TestService_ActiveSessionSweepDoesNotHealAroundLiveSibling proves the
// task-scoped cancel guard: while any active session of the task still has a
// live execution, the sweep must not cancel the task's other (orphaned)
// sessions, because CancelActiveTaskSessionsByTaskID is task-scoped and would
// kill the live work too.
func TestService_ActiveSessionSweepDoesNotHealAroundLiveSibling(t *testing.T) {
	fixture := newSweepFixture(t, 5*time.Hour)
	ctx := context.Background()
	if err := fixture.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-live", TaskID: "task-1", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-1", UpdatedAt: fixture.now.Add(-5 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	fixture.registry.liveSessions = []string{"session-live"}

	fixture.svc.runActiveSessionSweep(ctx, fixture.now)

	for _, sessionID := range []string{"session-1", "session-live"} {
		session, err := fixture.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			t.Fatalf("GetTaskSession(%s): %v", sessionID, err)
		}
		if session.State != models.TaskSessionStateRunning {
			t.Fatalf("session %s state = %q, want RUNNING (healing must not run while a sibling is live)", sessionID, session.State)
		}
	}
	// The orphaned sibling is still detected.
	if stalled := findTaskStalledEvents(fixture.eventBus); len(stalled) != 1 {
		t.Fatalf("task.stalled events = %d, want 1 for the orphaned sibling", len(stalled))
	}
}

// TestService_ActiveSessionSweepSkipsArchivedTasks proves the pass boundary:
// archived tasks with active sessions belong to the archived pass, not the
// active-task pass, and must not produce task.stalled events.
func TestService_ActiveSessionSweepSkipsArchivedTasks(t *testing.T) {
	fixture := newSweepFixture(t, 5*time.Hour)

	if err := fixture.svc.ArchiveTask(context.Background(), "task-1"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	fixture.svc.runActiveSessionSweep(context.Background(), fixture.now)

	if stalled := findTaskStalledEvents(fixture.eventBus); len(stalled) != 0 {
		t.Fatalf("task.stalled events = %d, want 0 for an archived task (owned by the archived pass)", len(stalled))
	}
}

// TestService_ActiveSessionSweepWithoutRegistryIsSkipped proves the
// fail-closed guard: without the live-execution registry the pass cannot
// prove "no live execution" and must do nothing rather than guess.
func TestService_ActiveSessionSweepWithoutRegistryIsSkipped(t *testing.T) {
	fixture := newSweepFixture(t, 5*time.Hour)
	fixture.svc.sessionExecutionRegistry = nil

	fixture.svc.runActiveSessionSweep(context.Background(), fixture.now)

	if stalled := findTaskStalledEvents(fixture.eventBus); len(stalled) != 0 {
		t.Fatalf("task.stalled events = %d, want 0 without the registry", len(stalled))
	}
}

// TestService_ActiveSessionSweepHealAbortedWhenExecutionRegistersMidSweep is
// the stale-snapshot regression: an execution that registers between the
// sweep's classification reads and the cancellation boundary must abort the
// heal. The cancellation is scoped to the classified session IDs, so the
// late-registered session survives even when the task qualifies for healing.
func TestService_ActiveSessionSweepHealAbortedWhenExecutionRegistersMidSweep(t *testing.T) {
	fixture := newSweepFixture(t, 5*time.Hour)
	ctx := context.Background()
	// A second active session on the same task, equally silent.
	if err := fixture.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-late", TaskID: "task-1", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-1", UpdatedAt: fixture.now.Add(-5 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	// The registry reports no live executions during classification (the
	// fixture's default) but a live execution for session-late by the time
	// the heal boundary re-checks — the exact mid-sweep registration race.
	fixture.registry.liveSessions = []string{"session-late"}
	fixture.registry.switchToLiveAfterFirstCall = true

	fixture.svc.runActiveSessionSweep(ctx, fixture.now)

	for _, sessionID := range []string{"session-1", "session-late"} {
		session, err := fixture.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			t.Fatalf("GetTaskSession(%s): %v", sessionID, err)
		}
		if session.State != models.TaskSessionStateRunning {
			t.Fatalf("session %s state = %q, want RUNNING (heal must abort when a live execution appears)", sessionID, session.State)
		}
	}
}

// TestService_ActiveSessionSweepHealCancelsOnlyClassifiedSessions proves the
// ID-scoped cancellation: when the task genuinely qualifies for healing, the
// cancellation only touches the sessions the sweep classified. A session
// that became active after classification — outside the classified set —
// keeps running.
func TestService_ActiveSessionSweepHealCancelsOnlyClassifiedSessions(t *testing.T) {
	fixture := newSweepFixture(t, 5*time.Hour)
	ctx := context.Background()

	fixture.svc.runActiveSessionSweep(ctx, fixture.now)

	// Both active sessions of the task were classified as healable orphans,
	// so the ID scope covers everything and the heal still completes.
	session, err := fixture.repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateCancelled {
		t.Fatalf("session-1 state = %q, want CANCELLED", session.State)
	}
}

// TestService_ActiveSessionSweepPrunesRecoveredSessionsFromDedupe proves the
// per-session episode semantics: when one of two stalled sessions recovers
// (gains a live execution) while its sibling stays stalled, the recovered
// session's dedupe entry is dropped. If it stalls again, its new episode is
// reported instead of being silently suppressed.
func TestService_ActiveSessionSweepPrunesRecoveredSessionsFromDedupe(t *testing.T) {
	fixture := newSweepFixture(t, 3*time.Hour)
	ctx := context.Background()
	if err := fixture.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-2", TaskID: "task-1", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-1", UpdatedAt: fixture.now.Add(-3 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	// First sweep: both sessions stall; both are reported.
	fixture.svc.runActiveSessionSweep(ctx, fixture.now)
	first := findTaskStalledEvents(fixture.eventBus)
	if len(first) != 1 {
		t.Fatalf("task.stalled events after first sweep = %d, want 1", len(first))
	}
	ids, _ := first[0].Data.(map[string]interface{})["session_ids"].([]string)
	if len(ids) != 2 {
		t.Fatalf("first report session_ids = %v, want both sessions", ids)
	}

	// session-2 recovers; session-1 stays stalled. The next sweep reports
	// nothing new (session-1 was already reported in this episode).
	fixture.registry.liveSessions = []string{"session-2"}
	fixture.svc.runActiveSessionSweep(ctx, fixture.now.Add(time.Minute))
	if stalled := findTaskStalledEvents(fixture.eventBus); len(stalled) != 1 {
		t.Fatalf("task.stalled events after recovery = %d, want 1 (no new report)", len(stalled))
	}

	// session-2 stalls again: its dedupe entry was pruned when it left the
	// stalled set, so the new episode reports it again — alongside nothing
	// else, because session-1's episode is still open.
	fixture.registry.liveSessions = nil
	fixture.svc.runActiveSessionSweep(ctx, fixture.now.Add(2*time.Minute))
	all := findTaskStalledEvents(fixture.eventBus)
	if len(all) != 2 {
		t.Fatalf("task.stalled events across both sweeps = %d, want 2 (recovered session reports again)", len(all))
	}
	ids, _ = all[1].Data.(map[string]interface{})["session_ids"].([]string)
	if len(ids) != 1 || ids[0] != "session-2" {
		t.Fatalf("second report session_ids = %v, want [session-2] only", ids)
	}
}

// TestService_ActiveSessionSweepRetriesFailedStallPublish proves the
// delivery semantics: a session is recorded as notified only after the
// task.stalled publish succeeds, so a transient publish failure is retried
// on the next sweep tick instead of being deduplicated away.
func TestService_ActiveSessionSweepRetriesFailedStallPublish(t *testing.T) {
	fixture := newSweepFixture(t, 3*time.Hour)
	fixture.eventBus.publishErrors = map[string]error{
		events.TaskStalled: errors.New("transient publish failure"),
	}

	// The publish fails; the session must not be recorded as notified.
	fixture.svc.runActiveSessionSweep(context.Background(), fixture.now)

	// The publish succeeds on the retry; the same episode must now be
	// reported exactly once.
	fixture.eventBus.publishErrors = nil
	fixture.svc.runActiveSessionSweep(context.Background(), fixture.now.Add(time.Minute))

	stalled := findTaskStalledEvents(fixture.eventBus)
	if len(stalled) != 1 {
		t.Fatalf("task.stalled events after publish recovery = %d, want 1 (failed publish retried)", len(stalled))
	}
}

// TestService_ActiveSessionSweepStallPayloadTimesNewlyReportedOnly proves
// the payload alignment: when a previously reported session stays stalled
// and a newer, quieter silence qualifies a different session mid-episode,
// the report's timing fields describe the newly reported session, not the
// whole stalled set.
func TestService_ActiveSessionSweepStallPayloadTimesNewlyReportedOnly(t *testing.T) {
	fixture := newSweepFixture(t, 3*time.Hour)
	ctx := context.Background()

	// First sweep reports session-1 (3h of silence).
	fixture.svc.runActiveSessionSweep(ctx, fixture.now)

	// session-2 has been silent much longer (6h) but only exists now.
	if err := fixture.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-old", TaskID: "task-1", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-1", UpdatedAt: fixture.now.Add(-6 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	fixture.svc.runActiveSessionSweep(ctx, fixture.now.Add(time.Minute))

	stalled := findTaskStalledEvents(fixture.eventBus)
	if len(stalled) != 2 {
		t.Fatalf("task.stalled events = %d, want 2", len(stalled))
	}
	data, _ := stalled[1].Data.(map[string]interface{})
	ids, _ := data["session_ids"].([]string)
	if len(ids) != 1 || ids[0] != "session-old" {
		t.Fatalf("second report session_ids = %v, want [session-old]", ids)
	}
	// stalled_for must describe session-old's 6h1m silence (its row went
	// quiet 6h before the original clock; the report tick is 1m later), not
	// session-1's 3h1m.
	if got := data["stalled_for"]; got != (6*time.Hour + time.Minute).String() {
		t.Errorf("stalled_for = %v, want %s (the newly reported session's silence)", got, (6*time.Hour + time.Minute).String())
	}
}

func TestService_ActiveSessionSweepPrunesRemovedTaskNotifications(t *testing.T) {
	fixture := newSweepFixture(t, 30*time.Minute)
	fixture.svc.stallNotifiedSessions = map[string]map[string]struct{}{
		"task-1":    {"session-1": {}},
		"task-gone": {"session-gone": {}},
	}

	fixture.svc.pruneStallNotificationsToCandidates([]*models.Task{{ID: "task-1"}})

	if _, ok := fixture.svc.stallNotifiedSessions["task-gone"]; ok {
		t.Fatal("stale notification entry for a removed task was retained")
	}
	if _, ok := fixture.svc.stallNotifiedSessions["task-1"]; !ok {
		t.Fatal("notification entry for a current task was pruned")
	}
}

func TestService_ActiveSessionSweepClosesOrphanTurnWithoutCompletionEvent(t *testing.T) {
	fixture := newSweepFixture(t, 5*time.Hour)
	ctx := context.Background()
	turn, err := fixture.svc.StartTurn(ctx, "session-1")
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	old := fixture.now.Add(-5 * time.Hour)
	if err := fixture.svc.messages.CreateMessage(ctx, &models.Message{
		ID:            "pending-tool-message",
		TaskSessionID: "session-1",
		TaskID:        "task-1",
		TurnID:        turn.ID,
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypeToolExecute,
		Content:       "tool output",
		Metadata:      map[string]interface{}{"tool_call_id": "call-1", "status": "pending"},
		CreatedAt:     old,
		UpdatedAt:     old,
	}); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if _, err := fixture.rawRepo.DB().ExecContext(ctx,
		`UPDATE task_sessions SET updated_at = ? WHERE id = ?`, old, "session-1"); err != nil {
		t.Fatalf("age session row: %v", err)
	}

	fixture.svc.runActiveSessionSweep(ctx, fixture.now)

	storedTurn, err := fixture.svc.turns.GetTurn(ctx, turn.ID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if storedTurn.CompletedAt == nil || !storedTurn.CompletedAt.Equal(storedTurn.StartedAt) {
		t.Fatalf("orphan turn completion = %v, want zero-duration completion at %v", storedTurn.CompletedAt, storedTurn.StartedAt)
	}
	message, err := fixture.svc.messages.GetMessage(ctx, "pending-tool-message")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if got := message.Metadata["status"]; got != "complete" {
		t.Fatalf("pending tool status = %v, want complete", got)
	}
	for _, event := range fixture.eventBus.GetPublishedEvents() {
		if event.Type != events.TurnCompleted {
			continue
		}
		if data, ok := event.Data.(map[string]interface{}); ok && data["id"] == turn.ID {
			t.Fatal("system orphan cleanup published turn.completed and may advance workflow")
		}
	}
}
