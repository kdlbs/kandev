package handlers

import (
	"context"
	"encoding/json"
	"expvar"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// readSignalReceivedCounterExact reads the exact-match entry in the ADR 0015
// workflow_step_completion_signal_received_total expvar map for the given
// key, verifying the label the handler actually wrote rather than a prefix
// that would also match a wrong or empty value. Callers seed a distinct
// agent_id per test (see seedAgentProfileSnapshot) so the exact key is also
// unique across this file's process-wide expvar state.
func readSignalReceivedCounterExact(t *testing.T, key string) int64 {
	t.Helper()
	v := expvar.Get("workflow_step_completion_signal_received_total")
	require.NotNil(t, v, "workflow_step_completion_signal_received_total must be published")
	m, ok := v.(*expvar.Map)
	require.True(t, ok, "workflow_step_completion_signal_received_total must be an expvar.Map")
	var total int64
	m.Do(func(kv expvar.KeyValue) {
		if kv.Key != key {
			return
		}
		n, err := strconv.ParseInt(kv.Value.String(), 10, 64)
		require.NoError(t, err, "counter %q value not int: %s", kv.Key, kv.Value.String())
		total += n
	})
	return total
}

// seedAgentProfileSnapshot writes an AgentProfileSnapshot onto the session
// with distinct agent_id and agent_name values, matching the shape
// executor.resolveAgentProfileSnapshot produces in production: agent_id is
// the store's auto-generated UUID for the agent row
// (internal/agent/settings/store/sqlite.go CreateAgent), agent_name is the
// registry-facing type like "claude"/"codex"
// (internal/agent/settings/controller/reconciler.go ensureDBAgent). Seeding
// them with different values means a test asserting on agentName fails if
// the handler is ever switched back to reading agent_id.
func seedAgentProfileSnapshot(t *testing.T, repo *sqliterepo.Repository, sessionID, agentName string) {
	t.Helper()
	require.NoError(t, repo.UpdateTaskSessionAgentProfileSnapshot(context.Background(), sessionID, map[string]interface{}{
		"agent_id":   "decoy-uuid-" + agentName,
		"agent_name": agentName,
	}))
}

// seedStepCompleteTarget seeds a workspace, task (with WorkflowStepID), and
// session in the requested state. Used by every TestHandleStepComplete_* case
// that needs the precondition chain in `resolveStepCompleteTarget` to succeed.
func seedStepCompleteTarget(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID, stepID string, state models.TaskSessionState) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID:        "ws-step-complete",
		Name:      "Step Complete",
		CreatedAt: now,
		UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:             taskID,
		WorkspaceID:    "ws-step-complete",
		WorkflowStepID: stepID,
		Title:          "Step Complete Task",
		State:          v1.TaskStateInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        sessionID,
		TaskID:    taskID,
		State:     state,
		StartedAt: now,
		UpdatedAt: now,
	}))
}

func newStepCompleteHandler(t *testing.T, taskSvc *service.Service, repo *sqliterepo.Repository, bus *mcpRecordingEventBus) *Handlers {
	t.Helper()
	return &Handlers{
		taskSvc:     taskSvc,
		sessionRepo: repo,
		eventBus:    bus,
		logger:      testLogger(t).WithFields(),
	}
}

type stepCompleteSessionReadBarrier struct {
	*sqliterepo.Repository
	ready   chan<- struct{}
	release <-chan struct{}
	reads   atomic.Int32
}

func (r *stepCompleteSessionReadBarrier) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	// Capture the session before signaling readiness. Otherwise one caller can
	// pass the barrier, claim the signal, and let the other caller read the
	// already-populated bag after release, turning the concurrency test into a
	// publish-retry test.
	session, err := r.Repository.GetTaskSession(ctx, id)
	if r.reads.Add(1) <= 2 {
		r.ready <- struct{}{}
		<-r.release
	}
	return session, err
}

type concurrentStepCompleteEventBus struct {
	mu     sync.Mutex
	events []*bus.Event
}

func (b *concurrentStepCompleteEventBus) Publish(_ context.Context, _ string, event *bus.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event)
	return nil
}

func (b *concurrentStepCompleteEventBus) eventCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

// TestHandleStepComplete_MissingFields covers the input-validation branches
// that fail before any DB lookup. All three return ErrorCodeValidation.
func TestHandleStepComplete_MissingFields(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]interface{}
		want    string
	}{
		{
			name:    "missing task_id",
			payload: map[string]interface{}{"session_id": "s1", "summary": "done"},
			want:    ws.ErrorCodeValidation,
		},
		{
			name:    "missing session_id",
			payload: map[string]interface{}{"task_id": "t1", "summary": "done"},
			want:    ws.ErrorCodeValidation,
		},
		{
			name:    "blank summary",
			payload: map[string]interface{}{"task_id": "t1", "session_id": "s1", "summary": "   "},
			want:    ws.ErrorCodeValidation,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handlers{logger: testLogger(t).WithFields()}
			msg := makeWSMessage(t, ws.ActionMCPStepComplete, tc.payload)
			resp, err := h.handleStepComplete(context.Background(), msg)
			require.NoError(t, err)
			assertWSError(t, resp, tc.want)
		})
	}
}

// TestHandleStepComplete_SessionDoesNotBelongToTask verifies the ownership
// guard rejects requests where the session.TaskID doesn't match the request's
// task_id.
func TestHandleStepComplete_SessionDoesNotBelongToTask(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-owner", "session-owner", "step-1", models.TaskSessionStateRunning)
	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)

	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-OTHER",
		"session_id": "session-owner",
		"summary":    "wrong task",
	})
	resp, err := h.handleStepComplete(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	assert.Empty(t, bus.events, "ownership rejection must not publish an event")
}

// TestHandleStepComplete_TerminalSessionRejected covers the
// Completed/Failed/Cancelled guard: writing a signal to a terminal session
// would never be consumed (subscriber short-circuits on non-WAITING state,
// no future turn-end fires), so we reject up front instead of returning
// accepted=true followed by silent no-op.
func TestHandleStepComplete_TerminalSessionRejected(t *testing.T) {
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled,
	} {
		t.Run(string(state), func(t *testing.T) {
			svc, repo := newTestTaskService(t)
			seedStepCompleteTarget(t, repo, "task-term", "session-term", "step-1", state)
			bus := &mcpRecordingEventBus{}
			h := newStepCompleteHandler(t, svc, repo, bus)

			msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
				"task_id":    "task-term",
				"session_id": "session-term",
				"summary":    "too late",
			})
			resp, err := h.handleStepComplete(context.Background(), msg)
			require.NoError(t, err)
			assertWSError(t, resp, ws.ErrorCodeValidation)
			assert.Empty(t, bus.events, "terminal-session rejection must not publish")
		})
	}
}

// TestHandleStepComplete_RejectsSignalFromMovedTurn prevents a stale reviewer
// turn from satisfying the successor Work step after a rejection moves the
// task back. The completion signal must belong to the step stamped on the
// turn that called the tool, not only to the task's current step.
func TestHandleStepComplete_RejectsSignalFromMovedTurn(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-moved", "session-moved", "step-review", models.TaskSessionStateRunning)

	turn := &models.Turn{
		ID:            "turn-review",
		TaskSessionID: "session-moved",
		TaskID:        "task-moved",
		StartedAt:     time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	stamped, err := repo.CreateTurnWithStepStamp(ctx, turn)
	require.NoError(t, err)
	require.True(t, stamped, "review turn must carry its launch step stamp")

	task, err := repo.GetTask(ctx, "task-moved")
	require.NoError(t, err)
	task.WorkflowStepID = "step-work"
	require.NoError(t, repo.UpdateTask(ctx, task))

	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)
	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-moved",
		"session_id": "session-moved",
		"summary":    "late reviewer signal",
	})

	resp, err := h.handleStepComplete(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	assert.Empty(t, bus.events, "a signal from a moved turn must not publish")

	session, err := repo.GetTaskSession(ctx, "session-moved")
	require.NoError(t, err)
	_, hasSignal := models.LoadPendingStepSignal(session.Metadata)
	assert.False(t, hasSignal, "a stale reviewer signal must not be persisted")
}

// TestHandleStepComplete_FirstCallAccepted covers the happy path: bag is
// written, event is published with the documented payload shape, and the
// response reports accepted=true with the persisted step_id + signaled_at.
func TestHandleStepComplete_FirstCallAccepted(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-first", "session-first", "step-1", models.TaskSessionStateRunning)
	seedAgentProfileSnapshot(t, repo, "session-first", "claude-first-call")
	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)

	const counterKey = "source=agent;agent_type=claude-first-call"
	before := readSignalReceivedCounterExact(t, counterKey)

	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-first",
		"session_id": "session-first",
		"summary":    "implementation finished",
		"handoff":    "tests next",
	})
	resp, err := h.handleStepComplete(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, true, payload["accepted"])
	assert.Equal(t, "step-1", payload["step_id"])
	// signaled_at is part of the documented response contract — pin its
	// presence + RFC3339Nano shape so a future refactor can't silently
	// drop or rename the field.
	signaledAt, ok := payload["signaled_at"].(string)
	require.True(t, ok, "expected signaled_at string in response payload")
	_, parseErr := time.Parse(time.RFC3339Nano, signaledAt)
	require.NoError(t, parseErr, "signaled_at must be RFC3339Nano")

	// Bag written under the canonical key.
	session, err := repo.GetTaskSession(context.Background(), "session-first")
	require.NoError(t, err)
	bag, ok := models.LoadPendingStepSignal(session.Metadata)
	require.True(t, ok, "expected bag entry to be persisted")
	assert.Equal(t, "step-1", bag.StepID)
	assert.Equal(t, models.StepCompletionSourceAgent, bag.Source)
	assert.Equal(t, "implementation finished", bag.Summary)
	assert.Equal(t, "tests next", bag.Handoff)

	// Bus event published with the public payload shape (no handoff/blockers
	// on the wire — those live in the bag only).
	require.Len(t, bus.events, 1, "expected one bus publish")
	assert.Equal(t, events.WorkflowStepCompletionSignaled, bus.events[0].Type)
	data, ok := bus.events[0].Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-first", data["task_id"])
	assert.Equal(t, "session-first", data["session_id"])
	assert.Equal(t, "step-1", data["step_id"])
	assert.Equal(t, "implementation finished", data["summary"])
	_, hasHandoff := data["handoff"]
	assert.False(t, hasHandoff, "handoff is bag-only, not on the wire")

	after := readSignalReceivedCounterExact(t, counterKey)
	assert.Equal(t, int64(1), after-before, "accepted signal must increment workflow_step_completion_signal_received_total")
}

// TestHandleStepComplete_DedupRunningNoRepublish covers the
// `already_signaled` short-circuit while the session is still RUNNING. The
// inline turn-end path will pick up the bag — no re-publish is needed and
// none should fire (avoids a spurious second event for the subscriber).
func TestHandleStepComplete_DedupRunningNoRepublish(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-dup", "session-dup", "step-1", models.TaskSessionStateRunning)
	// Pre-write the bag to simulate "first call already happened".
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session-dup", models.SessionMetaKeyPendingStepCompletion, models.PendingStepCompletionSignal{
		StepID:     "step-1",
		Source:     models.StepCompletionSourceAgent,
		Summary:    "first call",
		SignaledAt: time.Now().UTC(),
	}))
	seedAgentProfileSnapshot(t, repo, "session-dup", "claude-dup-call")
	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)

	const counterKey = "source=agent;agent_type=claude-dup-call"
	before := readSignalReceivedCounterExact(t, counterKey)

	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-dup",
		"session_id": "session-dup",
		"summary":    "second call (same step)",
	})
	resp, err := h.handleStepComplete(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, false, payload["accepted"])
	assert.Equal(t, "already_signaled", payload["reason"])

	assert.Empty(t, bus.events, "RUNNING dedup path must not re-publish (inline turn-end will consume the bag)")

	after := readSignalReceivedCounterExact(t, counterKey)
	assert.Equal(t, before, after, "already_signaled dedup must not increment workflow_step_completion_signal_received_total")
}

// TestHandleStepComplete_DedupWaitingRepublishes covers the retry-after-
// publish-failure path. When the session is WAITING_FOR_INPUT, the bag is
// already persisted but the orchestrator subscriber may have never fired
// (e.g., the first call's publish failed after the bag write). A retry
// MUST re-publish so the subscriber gets a chance to drive the transition,
// otherwise the session stays stuck until the user replies.
func TestHandleStepComplete_DedupWaitingRepublishes(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-retry", "session-retry", "step-1", models.TaskSessionStateWaitingForInput)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session-retry", models.SessionMetaKeyPendingStepCompletion, models.PendingStepCompletionSignal{
		StepID:     "step-1",
		Source:     models.StepCompletionSourceAgent,
		Summary:    "first call",
		SignaledAt: time.Now().UTC(),
	}))
	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)

	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-retry",
		"session_id": "session-retry",
		"summary":    "retry after publish failure",
	})
	resp, err := h.handleStepComplete(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, false, payload["accepted"])
	assert.Equal(t, "already_signaled", payload["reason"])

	require.Len(t, bus.events, 1, "WAITING dedup must re-publish the bus event so the subscriber can drive the transition")
	assert.Equal(t, events.WorkflowStepCompletionSignaled, bus.events[0].Type)
}

// TestHandleStepComplete_ConcurrentCallsClaimOneSignal verifies that two
// requests which read the empty bag at the same time still produce one
// accepted signal, one event, and one telemetry increment.
func TestHandleStepComplete_ConcurrentCallsClaimOneSignal(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-concurrent", "session-concurrent", "step-1", models.TaskSessionStateWaitingForInput)
	seedAgentProfileSnapshot(t, repo, "session-concurrent", "claude-concurrent")

	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	gatedRepo := &stepCompleteSessionReadBarrier{
		Repository: repo,
		ready:      ready,
		release:    release,
	}
	eventBus := &concurrentStepCompleteEventBus{}
	h := &Handlers{
		taskSvc:     svc,
		sessionRepo: gatedRepo,
		eventBus:    eventBus,
		logger:      testLogger(t).WithFields(),
	}

	const counterKey = "source=agent;agent_type=claude-concurrent"
	before := readSignalReceivedCounterExact(t, counterKey)
	messages := []*ws.Message{
		makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
			"task_id":    "task-concurrent",
			"session_id": "session-concurrent",
			"summary":    "first concurrent signal",
		}),
		makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
			"task_id":    "task-concurrent",
			"session_id": "session-concurrent",
			"summary":    "second concurrent signal",
		}),
	}
	type result struct {
		response *ws.Message
		err      error
	}
	results := make(chan result, len(messages))
	for _, msg := range messages {
		go func(msg *ws.Message) {
			response, err := h.handleStepComplete(ctx, msg)
			results <- result{response: response, err: err}
		}(msg)
	}
	<-ready
	<-ready
	close(release)

	accepted := 0
	for range messages {
		outcome := <-results
		require.NoError(t, outcome.err)
		require.NotNil(t, outcome.response)
		var payload map[string]interface{}
		require.NoError(t, json.Unmarshal(outcome.response.Payload, &payload))
		if payload["accepted"] == true {
			accepted++
		}
	}

	assert.Equal(t, 1, accepted, "only one concurrent request can claim the current workflow step")
	assert.Equal(t, 1, eventBus.eventCount(), "only the winning request can publish the completion event")
	after := readSignalReceivedCounterExact(t, counterKey)
	assert.Equal(t, int64(1), after-before, "only the winning request can increment received-signal telemetry")

	session, err := repo.GetTaskSession(ctx, "session-concurrent")
	require.NoError(t, err)
	signal, ok := models.LoadPendingStepSignal(session.Metadata)
	require.True(t, ok, "the winning request must persist the completion signal")
	assert.Equal(t, "step-1", signal.StepID)
	assert.Contains(t, []string{"first concurrent signal", "second concurrent signal"}, signal.Summary)
}
