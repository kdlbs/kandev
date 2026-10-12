package handlers

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// recordingEventBus captures every published event for assertions, without
// pulling in the full MemoryEventBus dispatch machinery this handler-level
// test does not need.
type recordingEventBus struct {
	mu     sync.Mutex
	topics []string
	events []*bus.Event
}

func (b *recordingEventBus) Publish(_ context.Context, topic string, event *bus.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.topics = append(b.topics, topic)
	b.events = append(b.events, event)
	return nil
}

func (b *recordingEventBus) last() (string, *bus.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.topics) == 0 {
		return "", nil
	}
	return b.topics[len(b.topics)-1], b.events[len(b.events)-1]
}

// fakeProposalWorkflowReader/fakeProposalStepReader are minimal WorkflowReader
// and WorkflowStepReader test doubles for the propose_task success path,
// keyed maps mirroring internal/coordinator/proposals_test.go's fakes (those
// are unexported and package-scoped, so this package needs its own).
type fakeProposalWorkflowReader struct {
	workflows map[string]*taskmodels.Workflow
}

func (f *fakeProposalWorkflowReader) GetWorkflow(_ context.Context, id string) (*taskmodels.Workflow, error) {
	return f.workflows[id], nil
}

type fakeProposalStepReader struct {
	stepsByWorkflow map[string][]*wfmodels.WorkflowStep
}

func (f *fakeProposalStepReader) ListStepsByWorkflow(_ context.Context, workflowID string) ([]*wfmodels.WorkflowStep, error) {
	return f.stepsByWorkflow[workflowID], nil
}

// newProposeTaskTestHandlers builds a *Handlers with a real *coordinator.Service
// (backed by an in-memory SQLite store) plus a recordingEventBus, seeds one
// coordinator row, and wires SetProposalDeps with a workflow/start-step pair
// so the success path can run end to end.
func newProposeTaskTestHandlers(t *testing.T) (*Handlers, *coordinator.Coordinator, *recordingEventBus) {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	store, err := coordinator.NewStore(conn, conn)
	require.NoError(t, err)
	validator := coordinator.NewValidator(nil, nil)
	svc := coordinator.NewService(store, validator, nil, testLogger(t))

	c := &coordinator.Coordinator{
		WorkspaceID:       "ws-propose",
		Name:              "Ops",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "executor-1",
	}
	require.NoError(t, store.CreateCoordinator(context.Background(), c))

	workflowReader := &fakeProposalWorkflowReader{workflows: map[string]*taskmodels.Workflow{
		"wf-1": {ID: "wf-1", WorkspaceID: c.WorkspaceID, Name: "Workflow"},
	}}
	stepReader := &fakeProposalStepReader{stepsByWorkflow: map[string][]*wfmodels.WorkflowStep{
		"wf-1": {{ID: "start", IsStartStep: true}},
	}}
	svc.SetProposalDeps(workflowReader, nil, nil, stepReader)

	eventBus := &recordingEventBus{}
	h := &Handlers{logger: testLogger(t).WithFields(), eventBus: eventBus}
	h.SetCoordinatorService(svc)
	return h, c, eventBus
}

func coordinatorPrincipalContext(coordinatorID string) context.Context {
	return mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		CoordinatorID:   coordinatorID,
		WorkspaceID:     "ws-propose",
		CallerTaskID:    "coordinator-conversation-task",
		CallerSessionID: "coordinator-session",
		Surface:         mcpprofile.SurfaceCoordinator,
	})
}

func proposeTaskPayload() map[string]interface{} {
	return map[string]interface{}{
		"title":       "Fix flaky test",
		"description": "The retry loop in X is racy.",
		"rationale":   "Observed 3 CI failures this week.",
		"workflow_id": "wf-1",
	}
}

// TestHandleProposeTask_Success proves a valid request from a coordinator
// principal creates a proposal, publishes coordinator.updated, and returns
// the proposal id and status (AC-COORDINATOR-PROPOSALS-001.1, .5).
func TestHandleProposeTask_Success(t *testing.T) {
	h, c, eventBus := newProposeTaskTestHandlers(t)
	ctx := coordinatorPrincipalContext(c.ID)
	msg := makeWSMessage(t, coordinator.ActionProposeTask, proposeTaskPayload())

	resp, err := h.handleProposeTask(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, "expected success, got: %s", resp.Payload)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.NotEmpty(t, payload["proposal_id"])
	require.Equal(t, string(coordinator.ProposalStatusPending), payload["status"])

	topic, event := eventBus.last()
	require.Equal(t, events.CoordinatorUpdated, topic)
	require.NotNil(t, event)
}

// TestHandleProposeTask_NonCoordinatorPrincipalForbidden proves the handler
// itself (not just the guard) refuses a caller without a coordinator
// principal, defense in depth against a guard bypass.
func TestHandleProposeTask_NonCoordinatorPrincipalForbidden(t *testing.T) {
	h, _, _ := newProposeTaskTestHandlers(t)
	msg := makeWSMessage(t, coordinator.ActionProposeTask, proposeTaskPayload())

	resp, err := h.handleProposeTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
}

// TestHandleProposeTask_CoordinatorServiceUnavailable proves a nil
// coordinatorSvc (features.coordinator off, or a session that never wired
// it) fails closed with a distinct error rather than panicking.
func TestHandleProposeTask_CoordinatorServiceUnavailable(t *testing.T) {
	h := &Handlers{logger: testLogger(t).WithFields()}
	ctx := coordinatorPrincipalContext("coordinator-1")
	msg := makeWSMessage(t, coordinator.ActionProposeTask, proposeTaskPayload())

	resp, err := h.handleProposeTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeUnavailable)
}

// TestHandleProposeTask_FieldErrorMapsToValidation proves a *coordinator.FieldError
// (AC-COORDINATOR-PROPOSALS-001.3) maps to ErrorCodeValidation and names the
// offending field, rather than leaking as an internal error.
func TestHandleProposeTask_FieldErrorMapsToValidation(t *testing.T) {
	h, c, _ := newProposeTaskTestHandlers(t)
	ctx := coordinatorPrincipalContext(c.ID)
	payload := proposeTaskPayload()
	payload["title"] = ""
	msg := makeWSMessage(t, coordinator.ActionProposeTask, payload)

	resp, err := h.handleProposeTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	var errPayload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &errPayload))
	require.Equal(t, "title", errPayload.Details["field"])
}

// TestHandleProposeTask_CapReachedMapsToValidation proves the 25-open-
// proposal cap (AC-COORDINATOR-PROPOSALS-001.4) surfaces as a validation
// error rather than an internal error.
func TestHandleProposeTask_CapReachedMapsToValidation(t *testing.T) {
	h, c, _ := newProposeTaskTestHandlers(t)
	ctx := coordinatorPrincipalContext(c.ID)

	for i := 0; i < 25; i++ {
		msg := makeWSMessage(t, coordinator.ActionProposeTask, proposeTaskPayload())
		resp, err := h.handleProposeTask(ctx, msg)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type, "seed proposal %d unexpectedly failed: %s", i, resp.Payload)
	}

	msg := makeWSMessage(t, coordinator.ActionProposeTask, proposeTaskPayload())
	resp, err := h.handleProposeTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleProposeKind_PhaseTwoOffIsUnknownAction(t *testing.T) {
	h, c, _ := newProposeTaskTestHandlers(t)
	ctx := coordinatorPrincipalContext(c.ID)
	for action, kind := range map[string]string{
		coordinator.ActionProposeResume:  coordinator.ProposalKindResume,
		coordinator.ActionProposeMessage: coordinator.ProposalKindMessage,
		coordinator.ActionProposeMove:    coordinator.ProposalKindMove,
	} {
		resp, err := h.proposeKindHandler(kind)(ctx, makeWSMessage(t, action, map[string]interface{}{"task_id": "t"}))
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeUnknownAction)
	}
}

func TestHandleProposeKind_NonCoordinatorPrincipalForbidden(t *testing.T) {
	h, _, _ := newProposeTaskTestHandlers(t)
	resp, err := h.proposeKindHandler(coordinator.ProposalKindMove)(context.Background(),
		makeWSMessage(t, coordinator.ActionProposeMove, map[string]interface{}{"task_id": "t"}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
}
