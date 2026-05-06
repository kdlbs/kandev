package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

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
