package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestHandleAgentCompleted_SubtaskWithoutRequestsInputCollapsesToCompleted
// pins the symptom reported in the v0.88 total-control fix plan:
// "child in a terminal state without requests_input must NOT write
// WAITING_FOR_INPUT". handleAgentCompleted's existing line 1238 writes
// WAITING unconditionally for non-transitioned terminal receipts.
// setSessionWaitingForInputIfRequested (this commit's guard) refuses
// the WAITING write for child tasks and instead collapses the session
// to COMPLETED, so a child agent's clean exit does not leave the
// session in a stuck RUNNING/RUNNING-equivalent state.
//
// The seed task is a subtask (ParentID="parent") in a terminal task state
// and the agent's last message has requests_input=false; the orchestrator
// must therefore finish the session to COMPLETED, not WAITING.
func TestHandleAgentCompleted_SubtaskWithoutRequestsInputCollapsesToCompleted(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	seedSession(t, repo, "child-task", "s-child", "")
	if err := repo.UpdateTask(ctx, &models.Task{
		ID: "child-task", WorkspaceID: "ws1", Title: "child",
		State: v1.TaskStateCompleted, ParentID: "parent",
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("set child-task ParentID: %v", err)
	}
	seedExecutorRunning(t, repo, "s-child", "child-task", "exec-child")

	// No messages: requests_input must be false, so the guard refuses
	// the WAITING write and collapses the session to COMPLETED.

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.handleAgentCompleted(ctx, watcherAgentCompletedData("child-task", "s-child", "exec-child"))
	waitForStopCall(t, agentMgr)

	updated, err := repo.GetTaskSession(ctx, "s-child")
	if err != nil {
		t.Fatalf("load session after subtask terminal: %v", err)
	}
	if updated.State != models.TaskSessionStateCompleted {
		t.Fatalf("subtask terminal without requests_input must collapse to COMPLETED, got %q", updated.State)
	}
}

// TestHandleAgentCompleted_NonTerminalSubtaskWithoutRequestsInputWritesWaiting
// protects the lifecycle boundary: a clean agent turn does not prove that a
// child task reached its terminal workflow state. A non-terminal child must
// remain promptable instead of being collapsed to COMPLETED solely because
// its latest message did not request input.
func TestHandleAgentCompleted_NonTerminalSubtaskWithoutRequestsInputWritesWaiting(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	seedSession(t, repo, "child-task-open", "s-child-open", "")
	if err := repo.UpdateTask(ctx, &models.Task{
		ID: "child-task-open", WorkspaceID: "ws1", Title: "child open",
		State: v1.TaskStateInProgress, ParentID: "parent",
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("set child-task-open ParentID: %v", err)
	}
	seedExecutorRunning(t, repo, "s-child-open", "child-task-open", "exec-child-open")

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.handleAgentCompleted(ctx, watcherAgentCompletedData("child-task-open", "s-child-open", "exec-child-open"))
	waitForStopCall(t, agentMgr)

	updated, err := repo.GetTaskSession(ctx, "s-child-open")
	if err != nil {
		t.Fatalf("load session after non-terminal subtask turn: %v", err)
	}
	if updated.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("non-terminal subtask must remain WAITING_FOR_INPUT, got %q", updated.State)
	}
}

func TestHandleAgentCompleted_SubtaskIgnoresResolvedClarification(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	seedSession(t, repo, "child-resolved", "s-child-resolved", "")
	if err := repo.UpdateTask(ctx, &models.Task{
		ID: "child-resolved", WorkspaceID: "ws1", Title: "child resolved",
		State: v1.TaskStateCompleted, ParentID: "parent",
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("set child-resolved ParentID: %v", err)
	}
	seedExecutorRunning(t, repo, "s-child-resolved", "child-resolved", "exec-child-resolved")
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-child-resolved", TaskID: "child-resolved", TaskSessionID: "s-child-resolved",
		StartedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{
		ID: "m-resolved", TaskSessionID: "s-child-resolved", TaskID: "child-resolved",
		TurnID: "turn-child-resolved", AuthorType: models.MessageAuthorAgent,
		Content: "The earlier question was answered.", Type: models.MessageTypeClarificationRequest,
		RequestsInput: true, Metadata: map[string]interface{}{"status": "answered"},
		CreatedAt: now, UpdatedAt: now,
	}))

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.handleAgentCompleted(ctx, watcherAgentCompletedData("child-resolved", "s-child-resolved", "exec-child-resolved"))
	waitForStopCall(t, agentMgr)

	updated, err := repo.GetTaskSession(ctx, "s-child-resolved")
	if err != nil {
		t.Fatalf("load session after resolved clarification: %v", err)
	}
	if updated.State != models.TaskSessionStateCompleted {
		t.Fatalf("resolved clarification must not keep a terminal child waiting, got %q", updated.State)
	}
}

// TestHandleAgentCompleted_SubtaskWithRequestsInputStillWritesWaiting
// pins the positive half of the guard: a child task whose agent
// actually asked the user for input (clarification request,
// requests_input=true) MUST still flip the session to WAITING so the
// chat UI surfaces the prompt — the guard is supposed to suppress
// false-positive WAITING writes, not legitimate clarification
// surfaces.
func TestHandleAgentCompleted_SubtaskWithRequestsInputStillWritesWaiting(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	seedSession(t, repo, "child-clarify", "s-child-cl", "")
	if err := repo.UpdateTask(ctx, &models.Task{
		ID: "child-clarify", WorkspaceID: "ws1", Title: "child clarify",
		State: v1.TaskStateInProgress, ParentID: "parent",
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("set child-clarify ParentID: %v", err)
	}
	seedExecutorRunning(t, repo, "s-child-cl", "child-clarify", "exec-child-cl")

	// Agent clarification request — the only path that should still
	// surface WAITING for a subtask session. The message row requires a
	// turn_id FK; seed a turn first so CreateMessage succeeds.
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-child-cl", TaskID: "child-clarify", TaskSessionID: "s-child-cl",
		StartedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{
		ID: "m-clarify", TaskSessionID: "s-child-cl", TaskID: "child-clarify",
		TurnID:     "turn-child-cl",
		AuthorType: models.MessageAuthorAgent, Content: "I need X",
		Type: models.MessageTypeClarificationRequest, RequestsInput: true,
		CreatedAt: now, UpdatedAt: now,
	}))

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.handleAgentCompleted(ctx, watcherAgentCompletedData("child-clarify", "s-child-cl", "exec-child-cl"))
	waitForStopCall(t, agentMgr)

	updated, err := repo.GetTaskSession(ctx, "s-child-cl")
	if err != nil {
		t.Fatalf("load session after subtask clarification: %v", err)
	}
	if updated.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("subtask with requests_input=true must still flip to WAITING, got %q", updated.State)
	}
}

// TestHandleAgentCompleted_SiblingSessionWithoutRequestsInputStillWritesWaiting
// pins the negative half of the parent-side guard. A root task with
// multiple sessions has each session finish independently: the finishing
// session must still flip to WAITING (its siblings can keep working),
// even though its last message did not request input. The guard
// therefore matches subtasks only; root tasks keep the original
// affordance.
func TestHandleAgentCompleted_SiblingSessionWithoutRequestsInputStillWritesWaiting(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	// Root task (no ParentID) with two sessions: one finishing, one
	// still running. The finishing session's agent exit has no
	// requests_input — the guard must NOT collapse it, so the multi-
	// session task can carry a per-session WAITING signal.
	seedSession(t, repo, "t1", "s-finishing", "")
	seedExecutorRunning(t, repo, "s-finishing", "t1", "exec-finishing")
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "s-running", TaskID: "t1",
		State:     models.TaskSessionStateRunning,
		StartedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
	}))

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.handleAgentCompleted(ctx, watcherAgentCompletedData("t1", "s-finishing", "exec-finishing"))
	waitForStopCall(t, agentMgr)

	updated, err := repo.GetTaskSession(ctx, "s-finishing")
	if err != nil {
		t.Fatalf("load finishing session: %v", err)
	}
	if updated.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("sibling session on root task must still flip to WAITING, got %q", updated.State)
	}
}

func watcherAgentCompletedData(taskID, sessionID, execID string) watcher.AgentEventData {
	return watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: execID,
	}
}
