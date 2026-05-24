package controller

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
)

// TestListAgents_ExcludesWorkspaceScopedProfiles regresses the bug where the
// kanban task-create agent picker listed office-managed agent rows (e.g.
// "Claude / CEO"). After ADR 0005 Wave G office AgentInstance and kanban
// AgentProfile share the agent_profiles table — kanban-facing reads must
// strip rows with a non-empty workspace_id.
func TestListAgents_ExcludesWorkspaceScopedProfiles(t *testing.T) {
	st := newFakeStore()
	st.nextAgentID = 1
	agent := &models.Agent{ID: "agent-1", Name: "claude-acp"}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	st.profiles[agent.ID] = []*models.AgentProfile{
		{ID: "p-global", AgentID: agent.ID, Name: "Sonnet", WorkspaceID: ""},
		{ID: "p-office", AgentID: agent.ID, Name: "CEO", WorkspaceID: "ws-1"},
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	ctrl := &Controller{
		repo:          st,
		agentRegistry: registry.NewRegistry(log),
		logger:        log,
	}

	resp, err := ctrl.ListAgents(context.Background())
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(resp.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(resp.Agents))
	}
	got := resp.Agents[0]
	if len(got.Profiles) != 1 {
		t.Fatalf("expected 1 profile after filtering office row, got %d: %+v", len(got.Profiles), got.Profiles)
	}
	if got.Profiles[0].ID != "p-global" {
		t.Errorf("expected p-global, got %q", got.Profiles[0].ID)
	}
}

func TestGetAgent_ExcludesWorkspaceScopedProfiles(t *testing.T) {
	st := newFakeStore()
	agent := &models.Agent{ID: "agent-1", Name: "claude-acp"}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	st.profiles[agent.ID] = []*models.AgentProfile{
		{ID: "p-global", AgentID: agent.ID, Name: "Sonnet", WorkspaceID: ""},
		{ID: "p-office", AgentID: agent.ID, Name: "CEO", WorkspaceID: "ws-1"},
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	ctrl := &Controller{
		repo:          st,
		agentRegistry: registry.NewRegistry(log),
		logger:        log,
	}

	got, err := ctrl.GetAgent(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if len(got.Profiles) != 1 {
		t.Fatalf("expected 1 profile after filtering office row, got %d", len(got.Profiles))
	}
	if got.Profiles[0].ID != "p-global" {
		t.Errorf("expected p-global, got %q", got.Profiles[0].ID)
	}
}

func TestUpdateAgent_ExcludesWorkspaceScopedProfiles(t *testing.T) {
	st := newFakeStore()
	agent := &models.Agent{ID: "agent-1", Name: "claude-acp"}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	st.profiles[agent.ID] = []*models.AgentProfile{
		{ID: "p-global", AgentID: agent.ID, Name: "Sonnet", WorkspaceID: ""},
		{ID: "p-office", AgentID: agent.ID, Name: "CEO", WorkspaceID: "ws-1"},
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	ctrl := &Controller{
		repo:          st,
		agentRegistry: registry.NewRegistry(log),
		logger:        log,
	}

	got, err := ctrl.UpdateAgent(context.Background(), UpdateAgentRequest{ID: "agent-1"})
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if len(got.Profiles) != 1 {
		t.Fatalf("expected 1 profile after filtering office row, got %d", len(got.Profiles))
	}
	if got.Profiles[0].ID != "p-global" {
		t.Errorf("expected p-global, got %q", got.Profiles[0].ID)
	}
}
