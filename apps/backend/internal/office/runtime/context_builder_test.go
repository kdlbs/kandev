package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

var errBoom = errors.New("boom")

func TestContextBuilderBuildsAndPersistsRuntimeSnapshot(t *testing.T) {
	agents := &recordingAgentReader{
		agent: &models.AgentInstance{
			ID:          "agent-1",
			WorkspaceID: "ws-1",
			Role:        models.AgentRoleCEO,
		},
	}
	store := &recordingRunSnapshotStore{}
	builder := ContextBuilder{Agents: agents, Runs: store}
	run := &models.Run{
		ID:             "run-1",
		AgentProfileID: "agent-1",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"task-1","session_id":"session-1"}`,
	}

	runCtx, err := builder.BuildAndPersist(context.Background(), run)
	if err != nil {
		t.Fatalf("BuildAndPersist: %v", err)
	}

	if runCtx.WorkspaceID != "ws-1" || runCtx.AgentID != "agent-1" {
		t.Fatalf("unexpected identity: %+v", runCtx)
	}
	if runCtx.TaskID != "task-1" || runCtx.SessionID != "session-1" {
		t.Fatalf("unexpected task/session: %+v", runCtx)
	}
	if !runCtx.Capabilities.Allows(CapabilityCreateAgent) {
		t.Fatal("expected CEO runtime context to allow create_agent")
	}
	if len(store.calls) != 1 {
		t.Fatalf("expected 1 snapshot write, got %d", len(store.calls))
	}
	var caps Capabilities
	if err := json.Unmarshal([]byte(store.calls[0].Capabilities), &caps); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}
	if !caps.Allows(CapabilityCreateAgent) {
		t.Fatal("persisted capabilities should include create_agent")
	}
}

type recordingAgentReader struct {
	agent *models.AgentInstance
}

func (r *recordingAgentReader) GetAgentInstance(_ context.Context, _ string) (*models.AgentInstance, error) {
	return r.agent, nil
}

func (r *recordingAgentReader) ListAgentInstances(_ context.Context, _ string) ([]*models.AgentInstance, error) {
	return []*models.AgentInstance{r.agent}, nil
}

func (r *recordingAgentReader) ListAgentInstancesByIDs(
	_ context.Context,
	_ []string,
) ([]*models.AgentInstance, error) {
	return []*models.AgentInstance{r.agent}, nil
}

type recordingRunSnapshotStore struct {
	calls []snapshotCall
}

type snapshotCall struct {
	RunID         string
	Capabilities  string
	InputSnapshot string
	SessionID     string
}

func (s *recordingRunSnapshotStore) UpdateRunRuntimeSnapshot(
	_ context.Context,
	id string,
	capabilities string,
	inputSnapshot string,
	sessionID string,
) error {
	s.calls = append(s.calls, snapshotCall{
		RunID:         id,
		Capabilities:  capabilities,
		InputSnapshot: inputSnapshot,
		SessionID:     sessionID,
	})
	return nil
}

type stubSeatResolver struct {
	held    bool
	err     error
	taskID  string
	agentID string
}

func (r *stubSeatResolver) HoldsDecisionSeat(_ context.Context, taskID, agentID string) (bool, error) {
	r.taskID = taskID
	r.agentID = agentID
	return r.held, r.err
}

func buildTestRunContext(t *testing.T, seats DecisionSeatResolver) RunContext {
	t.Helper()
	agents := &recordingAgentReader{
		agent: &models.AgentInstance{ID: "agent-1", WorkspaceID: "ws-1", Role: models.AgentRoleWorker},
	}
	builder := ContextBuilder{Agents: agents, Runs: &recordingRunSnapshotStore{}, Seats: seats}
	run := &models.Run{
		ID:             "run-1",
		AgentProfileID: "agent-1",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"task-1"}`,
	}
	runCtx, err := builder.Build(context.Background(), run)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return runCtx
}

func hasAvailableAction(runCtx RunContext, want string) bool {
	for _, action := range runCtx.AvailableActions {
		if action == want {
			return true
		}
	}
	return false
}

func TestContextBuilderGrantsRecordStepDecisionForSeatHolder(t *testing.T) {
	runCtx := buildTestRunContext(t, &stubSeatResolver{held: true})
	if !hasAvailableAction(runCtx, AvailableActionRecordStepDecision) {
		t.Fatal("expected seat holder to be granted record_step_decision")
	}
}

func TestContextBuilderDeniesRecordStepDecisionForNonHolder(t *testing.T) {
	runCtx := buildTestRunContext(t, &stubSeatResolver{held: false})
	if hasAvailableAction(runCtx, AvailableActionRecordStepDecision) {
		t.Fatal("expected non-holder to be denied record_step_decision")
	}
}

func TestContextBuilderDeniesRecordStepDecisionWhenResolverNil(t *testing.T) {
	runCtx := buildTestRunContext(t, nil)
	if hasAvailableAction(runCtx, AvailableActionRecordStepDecision) {
		t.Fatal("expected nil resolver to deny record_step_decision")
	}
}

func TestContextBuilderReturnsResolverError(t *testing.T) {
	seats := &stubSeatResolver{held: true, err: errBoom}
	builder := ContextBuilder{
		Agents: &recordingAgentReader{
			agent: &models.AgentInstance{ID: "agent-1", WorkspaceID: "ws-1", Role: models.AgentRoleWorker},
		},
		Seats: seats,
	}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{"task_id":"task-1"}`}
	if _, err := builder.Build(context.Background(), run); !errors.Is(err, errBoom) {
		t.Fatalf("expected resolver error to abort context build, got %v", err)
	}
}

func TestContextBuilderDeniesRecordStepDecisionForTasklessRun(t *testing.T) {
	agents := &recordingAgentReader{
		agent: &models.AgentInstance{ID: "agent-1", WorkspaceID: "ws-1", Role: models.AgentRoleWorker},
	}
	builder := ContextBuilder{
		Agents: agents,
		Runs:   &recordingRunSnapshotStore{},
		Seats:  &stubSeatResolver{held: true},
	}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Reason: "heartbeat", Payload: "{}"}
	runCtx, err := builder.Build(context.Background(), run)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if hasAvailableAction(runCtx, AvailableActionRecordStepDecision) {
		t.Fatal("expected taskless run to deny record_step_decision")
	}
}

func TestContextBuilderPersistsSeatActionAsAdvisoryMetadata(t *testing.T) {
	store := &recordingRunSnapshotStore{}
	seats := &stubSeatResolver{held: true}
	builder := ContextBuilder{
		Agents: &recordingAgentReader{
			agent: &models.AgentInstance{
				ID:          "agent-1",
				WorkspaceID: "ws-1",
				Role:        models.AgentRoleWorker,
			},
		},
		Runs:  store,
		Seats: seats,
	}
	run := &models.Run{
		ID:             "run-1",
		AgentProfileID: "agent-1",
		Reason:         "task_review_requested",
		Payload:        `{"task_id":"task-1"}`,
	}

	runCtx, err := builder.BuildAndPersist(context.Background(), run)
	if err != nil {
		t.Fatalf("BuildAndPersist: %v", err)
	}
	if seats.taskID != "task-1" || seats.agentID != "agent-1" {
		t.Fatalf("seat resolver received task=%q agent=%q", seats.taskID, seats.agentID)
	}

	rawContext, err := json.Marshal(runCtx)
	if err != nil {
		t.Fatalf("marshal run context: %v", err)
	}
	var contextSnapshot map[string]any
	if err := json.Unmarshal(rawContext, &contextSnapshot); err != nil {
		t.Fatalf("decode run context: %v", err)
	}
	actions, ok := contextSnapshot["available_actions"].([]any)
	if !ok || len(actions) != 1 || actions[0] != "record_step_decision" {
		t.Fatalf("expected advisory record_step_decision action, got %#v", contextSnapshot["available_actions"])
	}

	var capabilities map[string]any
	if err := json.Unmarshal([]byte(store.calls[0].Capabilities), &capabilities); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}
	if _, ok := capabilities["record_step_decision"]; ok {
		t.Fatalf("record_step_decision must not be serialized as a runtime capability: %s", store.calls[0].Capabilities)
	}
}
