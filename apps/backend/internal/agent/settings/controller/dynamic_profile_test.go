package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestValidateDynamicAgentProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile *dto.DynamicAgentProfileDTO
		wantErr error
	}{
		{
			name:    "missing candidates",
			profile: &dto.DynamicAgentProfileDTO{},
			wantErr: ErrDynamicProfileCandidatesRequired,
		},
		{
			name: "positions must be ordered",
			profile: &dto.DynamicAgentProfileDTO{Candidates: []dto.DynamicAgentCandidateDTO{
				{Position: 0, ExecutionProfileID: "a"},
				{Position: 0, ExecutionProfileID: "b"},
			}},
			wantErr: ErrDynamicProfilePositions,
		},
		{
			name: "explicit fallback model policy is allowed",
			profile: &dto.DynamicAgentProfileDTO{Candidates: []dto.DynamicAgentCandidateDTO{
				{Position: 0, ExecutionProfileID: "a", Rules: map[string]string{"on_provider_error": "try_next"}},
			}},
		},
		{
			name: "unsupported action is rejected",
			profile: &dto.DynamicAgentProfileDTO{Candidates: []dto.DynamicAgentCandidateDTO{
				{Position: 0, ExecutionProfileID: "a", Rules: map[string]string{"on_provider_error": "teleport"}},
			}},
			wantErr: ErrDynamicProfileRule,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDynamicAgentProfile(tt.profile)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("validateDynamicAgentProfile: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDynamicProfileCreateAndUpdatePersistsCandidates(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	if err := ctrl.agentRegistry.Register(agents.NewDynamicAgent()); err != nil {
		t.Fatalf("register dynamic agent: %v", err)
	}
	if err := ctrl.agentRegistry.Register(agents.NewClaudeACP()); err != nil {
		t.Fatalf("register concrete agent: %v", err)
	}
	ctrl.SetDynamicAgentRoutingEnabled(true)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{ID: agents.DynamicAgentID, Name: agents.DynamicAgentID}); err != nil {
		t.Fatalf("create dynamic family: %v", err)
	}
	if err := repo.CreateAgent(ctx, &models.Agent{ID: "claude-family", Name: "claude-acp"}); err != nil {
		t.Fatalf("create concrete family: %v", err)
	}
	candidate := &models.AgentProfile{AgentID: "claude-family", Name: "Claude", AgentDisplayName: "Claude"}
	if err := repo.CreateAgentProfile(ctx, candidate); err != nil {
		t.Fatalf("create candidate: %v", err)
	}

	created, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID: agents.DynamicAgentID,
		Name:    "Balanced",
		Dynamic: &dto.DynamicAgentProfileDTO{Candidates: []dto.DynamicAgentCandidateDTO{{
			Position:           0,
			ExecutionProfileID: candidate.ID,
			Enabled:            true,
			Rules:              map[string]string{"on_provider_error": "try_next"},
		}}},
	})
	if err != nil {
		t.Fatalf("create dynamic profile: %v", err)
	}
	if created.Kind != "dynamic" || created.Dynamic == nil || created.Dynamic.Version != 1 {
		t.Fatalf("created dynamic profile = %#v", created)
	}

	updated, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{
		ID: created.ID,
		Dynamic: &dto.DynamicAgentProfileDTO{
			Version: 1,
			Candidates: []dto.DynamicAgentCandidateDTO{{
				Position:           0,
				ExecutionProfileID: candidate.ID,
				Enabled:            false,
			}},
		},
	})
	if err != nil {
		t.Fatalf("update dynamic profile: %v", err)
	}
	if updated.Dynamic == nil || updated.Dynamic.Version != 2 || updated.Dynamic.Candidates[0].Enabled {
		t.Fatalf("updated dynamic profile = %#v", updated)
	}
	staleName := "Stale overwrite"
	if _, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{
		ID:   created.ID,
		Name: &staleName,
		Dynamic: &dto.DynamicAgentProfileDTO{
			Version:    1,
			Candidates: updated.Dynamic.Candidates,
		},
	}); !errors.Is(err, ErrDynamicProfileVersionConflict) {
		t.Fatalf("stale update error = %v, want version conflict", err)
	}
	stored, err := repo.GetAgentProfile(ctx, created.ID)
	if err != nil {
		t.Fatalf("read dynamic profile after stale update: %v", err)
	}
	if stored.Name != "Balanced" {
		t.Fatalf("stale update changed base profile name to %q", stored.Name)
	}

	ctrl.SetDynamicAgentRoutingEnabled(false)
	disabledName := "Disabled overwrite"
	if _, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: created.ID, Name: &disabledName}); !errors.Is(err, ErrDynamicAgentRoutingDisabled) {
		t.Fatalf("disabled dynamic update error = %v, want %v", err, ErrDynamicAgentRoutingDisabled)
	}
	if _, err := ctrl.DeleteProfile(ctx, created.ID, false); !errors.Is(err, ErrDynamicAgentRoutingDisabled) {
		t.Fatalf("disabled dynamic delete error = %v, want %v", err, ErrDynamicAgentRoutingDisabled)
	}
}
