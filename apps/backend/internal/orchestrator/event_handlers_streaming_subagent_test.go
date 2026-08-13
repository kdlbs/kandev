package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// fakeSubagentContextRecorder records every RecordSubagentContext call it
// receives, for unit-level assertions on what the orchestrator handlers
// build without going through a real repository.
type fakeSubagentContextRecorder struct {
	mu       sync.Mutex
	requests []taskservice.RecordSubagentContextRequest
}

func (r *fakeSubagentContextRecorder) RecordSubagentContext(_ context.Context, req taskservice.RecordSubagentContextRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req)
}

func (r *fakeSubagentContextRecorder) all() []taskservice.RecordSubagentContextRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]taskservice.RecordSubagentContextRequest, len(r.requests))
	copy(out, r.requests)
	return out
}

// serviceBackedSubagentContextRecorder adapts a real *taskservice.Service to
// orchestrator.SubagentContextRecorder, mirroring
// backendapp.subagentContextAdapter, so the integration test below exercises
// handler -> service -> repository end to end (same pattern as
// serviceBackedMessageCreator / newServiceBackedMessageCreator above).
type serviceBackedSubagentContextRecorder struct {
	svc *taskservice.Service
}

func (r *serviceBackedSubagentContextRecorder) RecordSubagentContext(ctx context.Context, req taskservice.RecordSubagentContextRequest) {
	r.svc.RecordSubagentContext(ctx, req)
}

// seedActiveTurn creates an open turn row and warms the in-memory active-turn
// cache getActiveTurnID reads first, so tests don't race a lazily-started
// turn with a different ID than the one they seeded.
func seedActiveTurn(t *testing.T, svc *Service, repo *sqliterepo.Repository, taskID, sessionID, turnID string) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTurn(context.Background(), &models.Turn{
		ID: turnID, TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}))
	svc.activeTurns.Store(sessionID, turnID)
}

func newSubagentTaskToolCallPayload(taskID, sessionID, toolCallID, parentToolCallID, toolStatus string, normalized *streams.NormalizedPayload) *lifecycle.AgentStreamEventPayload {
	return &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type: "tool_call", ToolCallID: toolCallID, ParentToolCallID: parentToolCallID,
			ToolStatus: toolStatus, ToolTitle: "Task", Normalized: normalized,
		},
	}
}

// TestHandleToolCallEventRecordsSubagentContext covers AC-1: a subagent_task
// frame on handleToolCallEvent records a context with the active turn id.
func TestHandleToolCallEventRecordsSubagentContext(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.messageCreator = newServiceBackedMessageCreator(repo)
	seedActiveTurn(t, svc, repo, "task1", "session1", "turn1")
	recorder := &fakeSubagentContextRecorder{}
	svc.subagentContexts = recorder

	normalized := streams.NewSubagentTask("review the diff", "prompt", "security-reviewer")
	svc.handleToolCallEvent(ctx, newSubagentTaskToolCallPayload("task1", "session1", "tc-1", "", "pending", normalized))

	requests := recorder.all()
	require.Len(t, requests, 1)
	req := requests[0]
	require.Equal(t, "session1", req.TaskSessionID)
	require.Equal(t, "task1", req.TaskID)
	require.Equal(t, "turn1", req.TurnID)
	require.Equal(t, "tc-1", req.ToolCallID)
	require.Equal(t, "pending", req.ToolStatus)
	require.NotNil(t, req.Payload)
	require.Equal(t, "security-reviewer", req.Payload.SubagentType)
}

// TestHandleToolUpdateEventRecordsSubagentContextAgain covers AC-3: a later
// frame for the same tool call on the update path records again, so the
// upsert's merge path runs.
func TestHandleToolUpdateEventRecordsSubagentContextAgain(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.messageCreator = newServiceBackedMessageCreator(repo)
	seedActiveTurn(t, svc, repo, "task1", "session1", "turn1")
	recorder := &fakeSubagentContextRecorder{}
	svc.subagentContexts = recorder

	launchPayload := streams.NewSubagentTask("review the diff", "prompt", "security-reviewer")
	svc.handleToolCallEvent(ctx, newSubagentTaskToolCallPayload("task1", "session1", "tc-1", "", "pending", launchPayload))

	updatePayload := streams.NewSubagentTask("review the diff", "prompt", "security-reviewer")
	updatePayload.SubagentTask().Status = "completed"
	svc.handleToolUpdateEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "task1", SessionID: "session1", ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type: "tool_update", ToolCallID: "tc-1", ToolStatus: "in_progress",
			Normalized: updatePayload,
		},
	})

	requests := recorder.all()
	require.Len(t, requests, 2, "one recorded request per frame")
	require.Equal(t, "turn1", requests[0].TurnID)
	require.Equal(t, "turn1", requests[1].TurnID)
}

// TestHandleToolUpdateEventCarriesTerminalToolStatus covers AC-11: a terminal
// update's ACP tool_status reaches the recorded request.
func TestHandleToolUpdateEventCarriesTerminalToolStatus(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.messageCreator = newServiceBackedMessageCreator(repo)
	seedActiveTurn(t, svc, repo, "task1", "session1", "turn1")
	recorder := &fakeSubagentContextRecorder{}
	svc.subagentContexts = recorder

	svc.handleToolCallEvent(ctx, newSubagentTaskToolCallPayload(
		"task1", "session1", "tc-1", "", "pending", streams.NewSubagentTask("d", "p", "reviewer")))

	terminalPayload := streams.NewSubagentTask("d", "p", "reviewer")
	terminalPayload.SubagentTask().Status = "completed"
	svc.handleToolUpdateEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "task1", SessionID: "session1", ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type: "tool_update", ToolCallID: "tc-1", ToolStatus: agentEventComplete,
			Normalized: terminalPayload,
		},
	})

	requests := recorder.all()
	require.Len(t, requests, 2)
	require.Equal(t, agentEventComplete, requests[1].ToolStatus)
}

// TestHandleToolCallEventIgnoresNonSubagentKind covers "a non-subagent_task
// normalized kind records nothing".
func TestHandleToolCallEventIgnoresNonSubagentKind(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.messageCreator = newServiceBackedMessageCreator(repo)
	seedActiveTurn(t, svc, repo, "task1", "session1", "turn1")
	recorder := &fakeSubagentContextRecorder{}
	svc.subagentContexts = recorder

	svc.handleToolCallEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "task1", SessionID: "session1", ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type: "tool_call", ToolCallID: "tc-read", ToolStatus: "pending",
			Normalized: streams.NewReadFile("/tmp/file.txt", 0, 100),
		},
	})

	require.Empty(t, recorder.all())
}

// TestHandleToolCallEventNilSubagentContextsRecorderIsSafe covers AC-27: a
// nil recorder must not panic, and the message write still happens.
func TestHandleToolCallEventNilSubagentContextsRecorderIsSafe(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	seedActiveTurn(t, svc, repo, "task1", "session1", "turn1")
	require.Nil(t, svc.subagentContexts)

	require.NotPanics(t, func() {
		svc.handleToolCallEvent(ctx, newSubagentTaskToolCallPayload(
			"task1", "session1", "tc-1", "", "pending", streams.NewSubagentTask("d", "p", "reviewer")))
	})

	require.Equal(t, 1, messages.toolCallWrites, "message write must proceed even without a subagent context recorder")
}

// TestHandleToolCallEventRecordsNestedSubagent covers AC-6: a nested frame
// (ParentToolCallID set) is recorded, not dropped.
func TestHandleToolCallEventRecordsNestedSubagent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.messageCreator = newServiceBackedMessageCreator(repo)
	seedActiveTurn(t, svc, repo, "task1", "session1", "turn1")
	recorder := &fakeSubagentContextRecorder{}
	svc.subagentContexts = recorder

	svc.handleToolCallEvent(ctx, newSubagentTaskToolCallPayload(
		"task1", "session1", "tc-nested-child", "tc-nested-parent", "pending",
		streams.NewSubagentTask("d", "p", "reviewer")))

	requests := recorder.all()
	require.Len(t, requests, 1)
	require.Equal(t, "tc-nested-parent", requests[0].ParentToolCallID)
}

// TestSubagentContextRecordedThroughRealServiceAndRepository is the
// integration case: handler -> real taskservice.Service -> real repository,
// mirroring newServiceBackedMessageCreator's pattern for messages. Confirms
// the wiring in backendapp.subagentContextAdapter is exercised end to end and
// that a terminal update settles the row.
func TestSubagentContextRecordedThroughRealServiceAndRepository(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.messageCreator = newServiceBackedMessageCreator(repo)
	seedActiveTurn(t, svc, repo, "task1", "session1", "turn1")

	taskSvc := taskservice.NewService(taskservice.Repos{
		Workspaces:       repo,
		Tasks:            repo,
		TaskRepos:        repo,
		Workflows:        repo,
		Messages:         repo,
		Turns:            repo,
		Sessions:         repo,
		GitSnapshots:     repo,
		RepoEntities:     repo,
		Executors:        repo,
		Environments:     repo,
		TaskEnvironments: repo,
		Reviews:          repo,
		SubagentContexts: repo,
	}, bus.NewMemoryEventBus(testLogger()), testLogger(), taskservice.RepositoryDiscoveryConfig{})
	svc.subagentContexts = &serviceBackedSubagentContextRecorder{svc: taskSvc}

	svc.handleToolCallEvent(ctx, newSubagentTaskToolCallPayload(
		"task1", "session1", "tc-e2e", "", "pending", streams.NewSubagentTask("d", "p", "reviewer")))

	terminalPayload := streams.NewSubagentTask("d", "p", "reviewer")
	terminalPayload.SubagentTask().Status = "completed"
	terminalPayload.SubagentTask().TotalTokens = 500
	svc.handleToolUpdateEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "task1", SessionID: "session1", ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type: "tool_update", ToolCallID: "tc-e2e", ToolStatus: agentEventComplete,
			Normalized: terminalPayload,
		},
	})

	rows, err := repo.ListSubagentContextsBySession(ctx, "session1")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	row := rows[0]
	require.Equal(t, "tc-e2e", row.ToolCallID)
	require.Equal(t, "live", row.Source)
	require.NotNil(t, row.SettledAt)
	require.NotNil(t, row.TotalTokens)
	require.Equal(t, int64(500), *row.TotalTokens)
}
