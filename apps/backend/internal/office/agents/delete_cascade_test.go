package agents

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// recordingTerminator captures TerminateAllForAgent calls so the deletion test
// can assert the cascade fired with the right agent ID.
type recordingTerminator struct {
	calls   []string
	failOn  string
	failErr error
}

func (r *recordingTerminator) TerminateAllForAgent(_ context.Context, agentID, _ string) error {
	r.calls = append(r.calls, agentID)
	if r.failOn != "" && r.failOn == agentID {
		return r.failErr
	}
	return nil
}

func TestDeleteAgentInstance_CascadesSessions(t *testing.T) {
	svc, repo := newTestAgentService(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID: "ws-del",
		Name:        "QA",
		Role:        models.AgentRoleQA,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := repo.GetAgentInstance(ctx, agent.ID); err != nil {
		t.Fatalf("seed get: %v", err)
	}

	rt := &recordingTerminator{}
	svc.SetSessionTerminator(rt)

	if err := svc.DeleteAgentInstance(ctx, agent.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if len(rt.calls) != 1 || rt.calls[0] != agent.ID {
		t.Errorf("cascade calls = %+v, want [%q]", rt.calls, agent.ID)
	}
}

// TestDeleteAgentInstance_CascadeFailureSwallowed confirms the cascade error
// does not surface to the caller — deletion has already succeeded at the DB
// layer and a failed cleanup is logged but not propagated.
func TestDeleteAgentInstance_CascadeFailureSwallowed(t *testing.T) {
	svc, _ := newTestAgentService(t)
	ctx := context.Background()
	agent := &models.AgentInstance{WorkspaceID: "ws-del2", Name: "QA2", Role: models.AgentRoleQA}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}
	rt := &recordingTerminator{failOn: agent.ID, failErr: errors.New("cascade boom")}
	svc.SetSessionTerminator(rt)

	if err := svc.DeleteAgentInstance(ctx, agent.ID); err != nil {
		t.Fatalf("delete should swallow cascade error, got %v", err)
	}
}
