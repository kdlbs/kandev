package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestClaudeExactModelAliasConflict(t *testing.T) {
	cases := []struct {
		name       string
		agentName  string
		model      string
		exact      bool
		envVars    []dto.ProfileEnvVarDTO
		wantReject bool
	}{
		{name: "sonnet alias", agentName: "claude-acp", model: claudeModelAliasSonnet, exact: true, envVars: []dto.ProfileEnvVarDTO{{Key: "ANTHROPIC_MODEL", Value: "claude-sonnet-5-5"}}, wantReject: true},
		{name: "opus alias", agentName: "claude-acp", model: claudeModelAliasOpus, exact: true, envVars: []dto.ProfileEnvVarDTO{{Key: "ANTHROPIC_MODEL", Value: "claude-opus-5-5"}}, wantReject: true},
		{name: "haiku alias", agentName: "claude-acp", model: claudeModelAliasHaiku, exact: true, envVars: []dto.ProfileEnvVarDTO{{Key: "ANTHROPIC_MODEL", Value: "claude-haiku-5-5"}}, wantReject: true},
		{name: "fable alias", agentName: "claude-acp", model: claudeModelAliasFable, exact: true, envVars: []dto.ProfileEnvVarDTO{{Key: "ANTHROPIC_MODEL", Value: "claude-fable-5-5"}}, wantReject: true},
		{name: "context variant", agentName: "claude-acp", model: claudeModelAliasSonnet + "[1m]", exact: true, envVars: []dto.ProfileEnvVarDTO{{Key: "ANTHROPIC_MODEL", Value: "claude-sonnet-5-5"}}},
		{name: "full model ID", agentName: "claude-acp", model: "claude-sonnet-5-5", exact: true, envVars: []dto.ProfileEnvVarDTO{{Key: "ANTHROPIC_MODEL", Value: "claude-opus-5-5"}}},
		{name: "default selector", agentName: "claude-acp", model: "default", exact: true, envVars: []dto.ProfileEnvVarDTO{{Key: "ANTHROPIC_MODEL", Value: "claude-sonnet-5-5"}}},
		{name: "opusplan selector", agentName: "claude-acp", model: "opusplan", exact: true, envVars: []dto.ProfileEnvVarDTO{{Key: "ANTHROPIC_MODEL", Value: "claude-opus-5-5"}}},
		{name: "other agent", agentName: "codex-acp", model: claudeModelAliasSonnet, exact: true, envVars: []dto.ProfileEnvVarDTO{{Key: "ANTHROPIC_MODEL", Value: "claude-sonnet-5-5"}}},
		{name: "compatible profile", agentName: "claude-acp", model: claudeModelAliasSonnet, envVars: []dto.ProfileEnvVarDTO{{Key: "ANTHROPIC_MODEL", Value: "claude-sonnet-5-5"}}},
		{name: "matching literal", agentName: "claude-acp", model: claudeModelAliasSonnet, exact: true, envVars: []dto.ProfileEnvVarDTO{{Key: "ANTHROPIC_MODEL", Value: " sonnet "}}},
		{name: "omitted value", agentName: "claude-acp", model: claudeModelAliasSonnet, exact: true},
		{name: "blank value", agentName: "claude-acp", model: claudeModelAliasSonnet, exact: true, envVars: []dto.ProfileEnvVarDTO{{Key: "ANTHROPIC_MODEL", Value: "   "}}},
		{name: "different key case", agentName: "claude-acp", model: claudeModelAliasSonnet, exact: true, envVars: []dto.ProfileEnvVarDTO{{Key: "anthropic_model", Value: "claude-sonnet-5-5"}}},
		{name: "secret reference", agentName: "claude-acp", model: claudeModelAliasSonnet, exact: true, envVars: []dto.ProfileEnvVarDTO{{Key: "ANTHROPIC_MODEL", SecretID: "secret-1"}}},
		{name: "trimmed different value", agentName: "claude-acp", model: claudeModelAliasSonnet, exact: true, envVars: []dto.ProfileEnvVarDTO{{Key: "ANTHROPIC_MODEL", Value: " claude-sonnet-5-5 "}}, wantReject: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl, repo := newClaudeProfileController(tc.agentName)
			_, err := ctrl.CreateProfile(context.Background(), CreateProfileRequest{
				AgentID:           "agent-1",
				Name:              "Default",
				Model:             tc.model,
				RequireExactModel: tc.exact,
				EnvVars:           tc.envVars,
			})
			if tc.wantReject {
				if !errors.Is(err, ErrClaudeExactModelAliasConflict) {
					t.Fatalf("CreateProfile error = %v, want Claude alias conflict", err)
				}
				if len(repo.created) != 0 {
					t.Fatalf("rejected profile was stored: %#v", repo.created)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateProfile: %v", err)
			}
			if len(repo.created) != 1 {
				t.Fatalf("stored %d profiles, want 1", len(repo.created))
			}
		})
	}
}

func TestClaudeExactModelAliasConflictTransitions(t *testing.T) {
	t.Run("unchanged legacy conflict permits unrelated and identical edits", func(t *testing.T) {
		for _, fullForm := range []bool{false, true} {
			t.Run(map[bool]string{false: "unrelated patch", true: "identical full form"}[fullForm], func(t *testing.T) {
				ctrl, repo, profileID := newLegacyClaudeConflict(t)
				name := "Renamed"
				req := UpdateProfileRequest{ID: profileID, Name: &name}
				if fullForm {
					model := claudeModelAliasSonnet
					requireExact := true
					envVars := []dto.ProfileEnvVarDTO{{Key: "ANTHROPIC_MODEL", Value: "claude-sonnet-5-5"}}
					req.Model = &model
					req.RequireExactModel = &requireExact
					req.EnvVars = &envVars
				}
				if _, err := ctrl.UpdateProfile(context.Background(), req); err != nil {
					t.Fatalf("UpdateProfile rejected unchanged conflict: %v", err)
				}
				stored, err := repo.GetAgentProfile(context.Background(), profileID)
				if err != nil {
					t.Fatal(err)
				}
				if !stored.RequireExactModel || stored.FallbackModel != claudeModelAliasHaiku || !stored.AutoFallback {
					t.Fatalf("saved model policy = exact:%v fallback:%q auto:%v", stored.RequireExactModel, stored.FallbackModel, stored.AutoFallback)
				}
			})
		}
	})

	t.Run("changed pair is rejected and leaves the stored profile unchanged", func(t *testing.T) {
		ctrl, repo, profileID := newLegacyClaudeConflict(t)
		model := claudeModelAliasOpus
		if _, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{ID: profileID, Model: &model}); !errors.Is(err, ErrClaudeExactModelAliasConflict) {
			t.Fatalf("UpdateProfile error = %v, want Claude alias conflict", err)
		}
		stored, err := repo.GetAgentProfile(context.Background(), profileID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Model != claudeModelAliasSonnet || stored.EnvVars[0].Value != "claude-sonnet-5-5" {
			t.Fatalf("rejected pair changed stored selection: model=%q env=%#v", stored.Model, stored.EnvVars)
		}
	})

	t.Run("selecting a full model ID repairs the pair and preserves policy", func(t *testing.T) {
		ctrl, repo, profileID := newLegacyClaudeConflict(t)
		model := "claude-sonnet-5-5"
		if _, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{ID: profileID, Model: &model}); err != nil {
			t.Fatalf("UpdateProfile repair: %v", err)
		}
		assertClaudeProfilePolicyPreserved(t, repo, profileID)
	})

	t.Run("removing the environment entry repairs the pair and preserves policy", func(t *testing.T) {
		ctrl, repo, profileID := newLegacyClaudeConflict(t)
		envVars := []dto.ProfileEnvVarDTO{}
		if _, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{ID: profileID, EnvVars: &envVars}); err != nil {
			t.Fatalf("UpdateProfile repair: %v", err)
		}
		assertClaudeProfilePolicyPreserved(t, repo, profileID)
	})

	t.Run("enabling strictness on a legacy pair is rejected", func(t *testing.T) {
		ctrl, repo, profileID := newLegacyClaudeConflict(t)
		profile, err := repo.GetAgentProfile(context.Background(), profileID)
		if err != nil {
			t.Fatal(err)
		}
		profile.RequireExactModel = false
		if err := repo.UpdateAgentProfileWithEnabledIntent(context.Background(), profile, nil); err != nil {
			t.Fatal(err)
		}
		requireExact := true
		if _, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{ID: profileID, RequireExactModel: &requireExact}); !errors.Is(err, ErrClaudeExactModelAliasConflict) {
			t.Fatalf("UpdateProfile error = %v, want Claude alias conflict", err)
		}
	})
}

func TestUpdateProfileClaudeAliasGuardResolvesAgentOnlyForNewConflicts(t *testing.T) {
	tests := []struct {
		name         string
		request      func(*testing.T, *fakeStore, string) UpdateProfileRequest
		wantLookups  int
		wantConflict bool
	}{
		{
			name: "full model ID",
			request: func(_ *testing.T, _ *fakeStore, profileID string) UpdateProfileRequest {
				model := "claude-sonnet-5-5"
				return UpdateProfileRequest{ID: profileID, Model: &model}
			},
			wantLookups: 1,
		},
		{
			name: "compatible profile",
			request: func(t *testing.T, repo *fakeStore, profileID string) UpdateProfileRequest {
				profile, err := repo.GetAgentProfile(context.Background(), profileID)
				if err != nil {
					t.Fatal(err)
				}
				profile.RequireExactModel = false
				if err := repo.UpdateAgentProfileWithEnabledIntent(context.Background(), profile, nil); err != nil {
					t.Fatal(err)
				}
				model := claudeModelAliasOpus
				return UpdateProfileRequest{ID: profileID, Model: &model}
			},
			wantLookups: 1,
		},
		{
			name: "unchanged legacy pair with an environment edit",
			request: func(_ *testing.T, _ *fakeStore, profileID string) UpdateProfileRequest {
				envVars := []dto.ProfileEnvVarDTO{
					{Key: claudeModelEnvVar, Value: "claude-sonnet-5-5"},
					{Key: "EXTRA", Value: "value"},
				}
				return UpdateProfileRequest{ID: profileID, EnvVars: &envVars}
			},
			wantLookups: 2,
		},
		{
			name: "new conflicting alias",
			request: func(_ *testing.T, _ *fakeStore, profileID string) UpdateProfileRequest {
				model := claudeModelAliasOpus
				return UpdateProfileRequest{ID: profileID, Model: &model}
			},
			wantLookups:  1,
			wantConflict: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl, repo, profileID := newLegacyClaudeConflict(t)
			request := tc.request(t, repo, profileID)
			countedRepo := &agentLookupCountingStore{fakeStore: repo}
			ctrl.repo = countedRepo

			_, err := ctrl.UpdateProfile(context.Background(), request)
			if tc.wantConflict {
				if !errors.Is(err, ErrClaudeExactModelAliasConflict) {
					t.Fatalf("UpdateProfile error = %v, want Claude alias conflict", err)
				}
			} else if err != nil {
				t.Fatalf("UpdateProfile: %v", err)
			}
			if countedRepo.getAgentCalls != tc.wantLookups {
				t.Fatalf("GetAgent calls = %d, want %d", countedRepo.getAgentCalls, tc.wantLookups)
			}
		})
	}
}

type agentLookupCountingStore struct {
	*fakeStore
	getAgentCalls int
}

func (s *agentLookupCountingStore) GetAgent(ctx context.Context, agentID string) (*models.Agent, error) {
	s.getAgentCalls++
	return s.fakeStore.GetAgent(ctx, agentID)
}

func newClaudeProfileController(agentName string) (*Controller, *fakeStore) {
	claude := agents.NewClaudeACP()
	registered := map[string]agents.Agent{claude.ID(): claude}
	if agentName != claude.ID() {
		registered[agentName] = agents.NewCodexACP()
	}
	ctrl := newTestController(registered)
	repo := newFakeStore()
	agent := &models.Agent{ID: "agent-1", Name: agentName}
	repo.agents[agent.ID] = agent
	repo.byName[agent.Name] = agent
	ctrl.repo = repo
	return ctrl, repo
}

func newLegacyClaudeConflict(t *testing.T) (*Controller, *fakeStore, string) {
	t.Helper()
	ctrl, repo := newClaudeProfileController(agents.NewClaudeACP().ID())
	profile := &models.AgentProfile{
		AgentID:           "agent-1",
		Name:              "Default",
		Model:             claudeModelAliasSonnet,
		RequireExactModel: true,
		FallbackModel:     claudeModelAliasHaiku,
		AutoFallback:      true,
		EnvVars:           []models.ProfileEnvVar{{Key: "ANTHROPIC_MODEL", Value: "claude-sonnet-5-5"}},
		Enabled:           true,
	}
	if err := repo.CreateAgentProfile(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	return ctrl, repo, profile.ID
}

func assertClaudeProfilePolicyPreserved(t *testing.T, repo *fakeStore, profileID string) {
	t.Helper()
	stored, err := repo.GetAgentProfile(context.Background(), profileID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.RequireExactModel || stored.FallbackModel != claudeModelAliasHaiku || !stored.AutoFallback {
		t.Fatalf("saved model policy = exact:%v fallback:%q auto:%v", stored.RequireExactModel, stored.FallbackModel, stored.AutoFallback)
	}
}
