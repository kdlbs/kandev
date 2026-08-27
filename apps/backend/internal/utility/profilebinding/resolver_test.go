package profilebinding

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

type fakeProfiles struct {
	byID   map[string]*models.AgentProfile
	byAg   map[string][]*models.AgentProfile
	err    error
	agents []*models.Agent
}

func (f *fakeProfiles) ListAgents(context.Context) ([]*models.Agent, error) {
	if len(f.agents) == 0 {
		return []*models.Agent{{ID: "codex-acp", Name: "codex-acp"}}, f.err
	}
	return f.agents, f.err
}

func (f *fakeProfiles) GetAgent(_ context.Context, id string) (*models.Agent, error) {
	for _, agent := range f.agents {
		if agent.ID == id {
			return agent, f.err
		}
	}
	return &models.Agent{ID: id, Name: id}, f.err
}

func (f *fakeProfiles) GetAgentProfileIncludingDeleted(context.Context, string) (*models.AgentProfile, error) {
	return f.byID["profile-1"], f.err
}

func (f *fakeProfiles) ListAgentProfiles(_ context.Context, agentID string) ([]*models.AgentProfile, error) {
	return f.byAg[agentID], f.err
}

func TestResolverMatchLegacyRequiresExactlyOneEligibleProfile(t *testing.T) {
	profiles := &fakeProfiles{byAg: map[string][]*models.AgentProfile{
		"codex-acp": {
			{ID: "profile-1", AgentID: "codex-acp", Model: "gpt-5", Enabled: true},
			{ID: "profile-2", AgentID: "codex-acp", Model: "gpt-5", Enabled: true},
		},
	}}
	resolver := New(profiles, func(string) bool { return true })

	_, err := resolver.MatchLegacy(context.Background(), "codex-acp", "gpt-5")
	if !errors.Is(err, ErrLegacyBindingAmbiguous) {
		t.Fatalf("MatchLegacy() error = %v, want ambiguous binding", err)
	}
}

func TestResolverMatchLegacyReturnsTheOnlyEligibleProfile(t *testing.T) {
	profiles := &fakeProfiles{byAg: map[string][]*models.AgentProfile{
		"codex-acp": {
			{ID: "disabled", AgentID: "codex-acp", Model: "gpt-5", Enabled: false},
			{ID: "profile-1", AgentID: "codex-acp", Model: "gpt-5", Enabled: true},
		},
	}}
	resolver := New(profiles, func(string) bool { return true })

	got, err := resolver.MatchLegacy(context.Background(), "codex-acp", "gpt-5")
	if err != nil {
		t.Fatalf("MatchLegacy() error = %v", err)
	}
	if got.ID != "profile-1" {
		t.Fatalf("profile ID = %q, want profile-1", got.ID)
	}
}

func TestResolverRejectsWorkspaceCliAndDeletedProfiles(t *testing.T) {
	workspace := "workspace-1"
	profiles := &fakeProfiles{byAg: map[string][]*models.AgentProfile{
		"codex-acp": {
			{ID: "workspace", AgentID: "codex-acp", Model: "gpt-5", Enabled: true, WorkspaceID: workspace},
			{ID: "cli", AgentID: "codex-acp", Model: "gpt-5", Enabled: true, CLIPassthrough: true},
		},
	}}
	resolver := New(profiles, func(string) bool { return true })
	if _, err := resolver.MatchLegacy(context.Background(), "codex-acp", "gpt-5"); !errors.Is(err, ErrLegacyBindingAmbiguous) {
		t.Fatalf("MatchLegacy() error = %v, want no eligible match", err)
	}
}

func TestResolverResolveReturnsRequestedEligibleProfile(t *testing.T) {
	profiles := &fakeProfiles{byID: map[string]*models.AgentProfile{
		"profile-1": {ID: "profile-1", AgentID: "codex-acp", Model: "gpt-5", Enabled: true},
	}}
	resolver := New(profiles, func(string) bool { return true })

	got, err := resolver.Resolve(context.Background(), "profile-1")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.ID != "profile-1" || got.AgentID != "codex-acp" {
		t.Fatalf("Resolve() = %#v, want requested eligible profile", got)
	}
}
