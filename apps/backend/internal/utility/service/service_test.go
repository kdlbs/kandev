package service

import (
	"context"
	"database/sql"
	"testing"

	"github.com/kandev/kandev/internal/utility/models"
	"github.com/kandev/kandev/internal/utility/template"
)

type fakeRepository struct {
	agents map[string]*models.UtilityAgent
}

func (r *fakeRepository) ListAgents(context.Context) ([]*models.UtilityAgent, error) {
	out := make([]*models.UtilityAgent, 0, len(r.agents))
	for _, agent := range r.agents {
		out = append(out, agent)
	}
	return out, nil
}

func (r *fakeRepository) GetAgentByID(_ context.Context, id string) (*models.UtilityAgent, error) {
	agent, ok := r.agents[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return agent, nil
}

func (r *fakeRepository) GetAgentByName(_ context.Context, name string) (*models.UtilityAgent, error) {
	for _, agent := range r.agents {
		if agent.Name == name {
			return agent, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (r *fakeRepository) CreateAgent(_ context.Context, agent *models.UtilityAgent) error {
	r.agents[agent.ID] = agent
	return nil
}

func (r *fakeRepository) UpdateAgent(_ context.Context, agent *models.UtilityAgent) error {
	r.agents[agent.ID] = agent
	return nil
}

func (r *fakeRepository) DeleteAgent(_ context.Context, id string) error {
	delete(r.agents, id)
	return nil
}

func (r *fakeRepository) ListCalls(context.Context, string, int) ([]*models.UtilityAgentCall, error) {
	return nil, nil
}

func (r *fakeRepository) GetCallByID(context.Context, string) (*models.UtilityAgentCall, error) {
	return nil, sql.ErrNoRows
}

func (r *fakeRepository) CreateCall(context.Context, *models.UtilityAgentCall) error {
	return nil
}

func (r *fakeRepository) UpdateCall(context.Context, *models.UtilityAgentCall) error {
	return nil
}

func TestPreparePromptRequest_UsesDefaultAgentAndModelAsPairWhenModelIsUnset(t *testing.T) {
	t.Parallel()

	svc := NewService(&fakeRepository{agents: map[string]*models.UtilityAgent{
		"builtin-enhance-prompt": {
			ID:      "builtin-enhance-prompt",
			Prompt:  "Improve {{UserPrompt}}",
			AgentID: "claude-acp",
			Model:   "",
			Builtin: true,
		},
	}})

	req, err := svc.PreparePromptRequest(
		context.Background(),
		"builtin-enhance-prompt",
		&template.Context{UserPrompt: "fix the bug"},
		&DefaultUtilitySettings{
			AgentID: "opencode-acp",
			Model:   "opencode-go/deepseek-v4-flash",
		},
		false,
	)
	if err != nil {
		t.Fatalf("PreparePromptRequest() error = %v", err)
	}

	if req.AgentCLI != "opencode-acp" {
		t.Fatalf("AgentCLI = %q, want %q", req.AgentCLI, "opencode-acp")
	}
	if req.Model != "opencode-go/deepseek-v4-flash" {
		t.Fatalf("Model = %q, want %q", req.Model, "opencode-go/deepseek-v4-flash")
	}
}
