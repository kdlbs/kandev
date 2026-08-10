package service

import (
	"context"
	"database/sql"
	"testing"

	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/utility/models"
	"github.com/kandev/kandev/internal/utility/template"
)

type fakeProfileResolver struct {
	profile *agentsettingsmodels.AgentProfile
	err     error
}

func (r fakeProfileResolver) Resolve(context.Context, string) (*agentsettingsmodels.AgentProfile, error) {
	return r.profile, r.err
}
func (r fakeProfileResolver) MatchLegacy(context.Context, string, string) (*agentsettingsmodels.AgentProfile, error) {
	return r.profile, r.err
}

func TestMigrateLegacyBindingsUpdatesOnlyUnambiguousRows(t *testing.T) {
	repo := &fakeRepository{agents: map[string]*models.UtilityAgent{
		"legacy":  {ID: "legacy", AgentID: "codex-acp", Model: "gpt-5", ProfileBindingState: models.ProfileBindingExplicit},
		"inherit": {ID: "inherit", ProfileBindingState: models.ProfileBindingInherit},
	}}
	svc := NewService(repo)
	svc.SetProfileResolver(fakeProfileResolver{profile: &agentsettingsmodels.AgentProfile{ID: "profile-1"}})
	updated, err := svc.MigrateLegacyBindings(context.Background())
	if err != nil || updated != 1 {
		t.Fatalf("MigrateLegacyBindings() = (%d, %v), want (1, nil)", updated, err)
	}
	if repo.agents["legacy"].AgentProfileID != "profile-1" || repo.agents["legacy"].ProfileBindingState != models.ProfileBindingExplicit {
		t.Fatalf("legacy row was not migrated: %#v", repo.agents["legacy"])
	}
	if repo.agents["inherit"].ProfileBindingState != models.ProfileBindingInherit {
		t.Fatalf("inherit row changed: %#v", repo.agents["inherit"])
	}
}

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

func TestPreparePromptRequest_UsesDefaultAgentWhenAgentIDIsUnset(t *testing.T) {
	t.Parallel()

	svc := NewService(&fakeRepository{agents: map[string]*models.UtilityAgent{
		"custom": {
			ID:      "custom",
			Prompt:  "Do {{UserPrompt}}",
			AgentID: "",
			Model:   "custom-model",
		},
	}})

	req, err := svc.PreparePromptRequest(
		context.Background(),
		"custom",
		&template.Context{UserPrompt: "fix the bug"},
		&DefaultUtilitySettings{
			AgentID: "default-acp",
			Model:   "default-model",
		},
		false,
	)
	if err != nil {
		t.Fatalf("PreparePromptRequest() error = %v", err)
	}

	if req.AgentCLI != "default-acp" {
		t.Fatalf("AgentCLI = %q, want %q", req.AgentCLI, "default-acp")
	}
	if req.Model != "custom-model" {
		t.Fatalf("Model = %q, want %q", req.Model, "custom-model")
	}
}

func TestPreparePromptRequest_PreservesConfiguredAgentAndModelWithoutDefaults(t *testing.T) {
	t.Parallel()

	svc := NewService(&fakeRepository{agents: map[string]*models.UtilityAgent{
		"custom": {
			ID:      "custom",
			Prompt:  "Do {{UserPrompt}}",
			AgentID: "custom-acp",
			Model:   "custom-model",
		},
	}})

	req, err := svc.PreparePromptRequest(
		context.Background(),
		"custom",
		&template.Context{UserPrompt: "fix the bug"},
		nil,
		false,
	)
	if err != nil {
		t.Fatalf("PreparePromptRequest() error = %v", err)
	}

	if req.AgentCLI != "custom-acp" {
		t.Fatalf("AgentCLI = %q, want %q", req.AgentCLI, "custom-acp")
	}
	if req.Model != "custom-model" {
		t.Fatalf("Model = %q, want %q", req.Model, "custom-model")
	}
}

func TestPreparePromptRequest_IgnoresDefaultsWhenAgentAndModelAreConfigured(t *testing.T) {
	t.Parallel()

	svc := NewService(&fakeRepository{agents: map[string]*models.UtilityAgent{
		"custom": {
			ID:      "custom",
			Prompt:  "Do {{UserPrompt}}",
			AgentID: "custom-acp",
			Model:   "custom-model",
		},
	}})

	req, err := svc.PreparePromptRequest(
		context.Background(),
		"custom",
		&template.Context{UserPrompt: "fix the bug"},
		&DefaultUtilitySettings{
			AgentID: "default-acp",
			Model:   "default-model",
		},
		false,
	)
	if err != nil {
		t.Fatalf("PreparePromptRequest() error = %v", err)
	}

	if req.AgentCLI != "custom-acp" {
		t.Fatalf("AgentCLI = %q, want %q", req.AgentCLI, "custom-acp")
	}
	if req.Model != "custom-model" {
		t.Fatalf("Model = %q, want %q", req.Model, "custom-model")
	}
}
