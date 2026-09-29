package controller

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
)

func TestValidateDynamicAgentProfile(t *testing.T) {
	validPolicy := func() *dto.DynamicAgentPolicyDTO {
		return &dto.DynamicAgentPolicyDTO{
			Version: 1,
			Transient: dto.DynamicErrorPolicyDTO{
				Retry:        dto.DynamicRetryPolicyDTO{Enabled: true, MaxRetries: 2, InitialIntervalSeconds: 5},
				WaitForReset: dto.DynamicResetWaitPolicyDTO{Enabled: true, MaxWaitSeconds: 300},
				OnExhausted:  "skip",
			},
			Hard: dto.DynamicErrorPolicyDTO{
				OnExhausted: "stop",
			},
		}
	}
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
		{
			name: "typed policy accepts bounded class settings",
			profile: &dto.DynamicAgentProfileDTO{Candidates: []dto.DynamicAgentCandidateDTO{
				{Position: 0, ExecutionProfileID: "a", Policies: validPolicy()},
			}},
		},
		{
			name: "typed policy rejects an incomplete class document",
			profile: &dto.DynamicAgentProfileDTO{Candidates: []dto.DynamicAgentCandidateDTO{
				{Position: 0, ExecutionProfileID: "a", Policies: &dto.DynamicAgentPolicyDTO{
					Version:   1,
					Transient: dto.DynamicErrorPolicyDTO{OnExhausted: "skip"},
				}},
			}},
			wantErr: ErrDynamicProfileRule,
		},
		{
			name: "typed policy rejects out of range retry",
			profile: &dto.DynamicAgentProfileDTO{Candidates: []dto.DynamicAgentCandidateDTO{
				{Position: 0, ExecutionProfileID: "a", Policies: &dto.DynamicAgentPolicyDTO{
					Version: 1,
					Transient: dto.DynamicErrorPolicyDTO{
						Retry:       dto.DynamicRetryPolicyDTO{Enabled: true, MaxRetries: 11, InitialIntervalSeconds: 5},
						OnExhausted: "skip",
					},
					Hard: dto.DynamicErrorPolicyDTO{OnExhausted: "skip"},
				}},
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

func TestNormalizeDynamicPolicyLegacyRules(t *testing.T) {
	tests := []struct {
		name  string
		rules map[string]string
		check func(t *testing.T, policy dto.DynamicAgentPolicyDTO)
	}{
		{
			name:  "try next defaults both classes to skip",
			rules: map[string]string{"on_provider_error": "try_next"},
			check: func(t *testing.T, policy dto.DynamicAgentPolicyDTO) {
				if policy.Transient.OnExhausted != "skip" || policy.Hard.OnExhausted != "skip" {
					t.Fatalf("policy = %#v", policy)
				}
			},
		},
		{
			name:  "retry same maps to one delayed retry",
			rules: map[string]string{"on_provider_error": "retry_same"},
			check: func(t *testing.T, policy dto.DynamicAgentPolicyDTO) {
				if !policy.Transient.Retry.Enabled || policy.Transient.Retry.MaxRetries != 1 || policy.Transient.Retry.InitialIntervalSeconds != 5 {
					t.Fatalf("policy = %#v", policy.Transient)
				}
				if policy.Hard.Retry != policy.Transient.Retry || policy.Hard.OnExhausted != "stop" {
					t.Fatalf("policy = %#v", policy)
				}
			},
		},
		{
			name:  "specific class rule overrides generic default",
			rules: map[string]string{"on_provider_error": "try_next", "quota_limited": "stop"},
			check: func(t *testing.T, policy dto.DynamicAgentPolicyDTO) {
				if policy.Transient.OnExhausted != "skip" || policy.Hard.OnExhausted != "stop" {
					t.Fatalf("policy = %#v", policy)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &dto.DynamicAgentProfileDTO{Candidates: []dto.DynamicAgentCandidateDTO{{
				Position: 0, ExecutionProfileID: "candidate", Rules: tt.rules,
			}}}
			if err := validateDynamicAgentProfile(profile); err != nil {
				t.Fatalf("validateDynamicAgentProfile: %v", err)
			}
			if profile.Candidates[0].Policies == nil {
				t.Fatal("legacy rules were not normalized")
			}
			tt.check(t, *profile.Candidates[0].Policies)
		})
	}
}

func TestNormalizeDynamicPolicyRejectsConflictingClassRules(t *testing.T) {
	profile := &dto.DynamicAgentProfileDTO{Candidates: []dto.DynamicAgentCandidateDTO{{
		Position:           0,
		ExecutionProfileID: "candidate",
		Rules: map[string]string{
			"rate_limited":        "try_next",
			"network_unavailable": "stop",
		},
	}}}
	if err := validateDynamicAgentProfile(profile); !errors.Is(err, ErrDynamicProfileRule) {
		t.Fatalf("error = %v, want conflicting class rule", err)
	}
}

func TestDynamicPolicyCanonicalRoundTrip(t *testing.T) {
	profile := &dto.DynamicAgentProfileDTO{Candidates: []dto.DynamicAgentCandidateDTO{{
		Position:           0,
		ExecutionProfileID: "candidate",
		Rules:              map[string]string{"on_provider_error": "try_next"},
	}}}
	if err := validateDynamicAgentProfile(profile); err != nil {
		t.Fatalf("validateDynamicAgentProfile: %v", err)
	}
	routes, err := dynamicRoutesFromDTO("dynamic", profile)
	if err != nil {
		t.Fatalf("dynamicRoutesFromDTO: %v", err)
	}
	got, err := dynamicProfileDTO(&models.DynamicAgentProfile{ProfileID: "dynamic", Version: 1}, routes)
	if err != nil {
		t.Fatalf("dynamicProfileDTO: %v", err)
	}
	if got.Candidates[0].Rules != nil {
		t.Fatalf("canonical response retained legacy rules: %#v", got.Candidates[0].Rules)
	}
	if !reflect.DeepEqual(got.Candidates[0].Policies, profile.Candidates[0].Policies) {
		t.Fatalf("policies = %#v, want %#v", got.Candidates[0].Policies, profile.Candidates[0].Policies)
	}
}

func TestUnclassifiedPolicyRoundTrip(t *testing.T) {
	const input = `{"version":1,"candidates":[{"position":0,"execution_profile_id":"candidate","enabled":true,"policies":{"version":1,"transient":{"on_exhausted":"skip"},"hard":{"on_exhausted":"skip"},"unclassified":{"enabled":true,"consecutive_failure_threshold":3}}}]}`
	var profile dto.DynamicAgentProfileDTO
	if err := json.Unmarshal([]byte(input), &profile); err != nil {
		t.Fatalf("unmarshal profile: %v", err)
	}
	if err := validateDynamicAgentProfile(&profile); err != nil {
		t.Fatalf("validateDynamicAgentProfile: %v", err)
	}
	routes, err := dynamicRoutesFromDTO("dynamic", &profile)
	if err != nil {
		t.Fatalf("dynamicRoutesFromDTO: %v", err)
	}
	got, err := dynamicProfileDTO(&models.DynamicAgentProfile{ProfileID: "dynamic", Version: 1}, routes)
	if err != nil {
		t.Fatalf("dynamicProfileDTO: %v", err)
	}
	encoded, err := json.Marshal(got.Candidates[0].Policies)
	if err != nil {
		t.Fatalf("marshal policies: %v", err)
	}
	var policy map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &policy); err != nil {
		t.Fatalf("unmarshal response policies: %v", err)
	}
	var unclassified struct {
		Enabled                     bool  `json:"enabled"`
		ConsecutiveFailureThreshold int64 `json:"consecutive_failure_threshold"`
	}
	if err := json.Unmarshal(policy["unclassified"], &unclassified); err != nil {
		t.Fatalf("unmarshal response unclassified policy: %v", err)
	}
	if !unclassified.Enabled || unclassified.ConsecutiveFailureThreshold != 3 {
		t.Fatalf("unclassified policy = %#v, want enabled threshold 3", unclassified)
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

	policy := &dto.DynamicAgentPolicyDTO{
		Version:   1,
		Transient: dto.DynamicErrorPolicyDTO{OnExhausted: "skip"},
		Hard:      dto.DynamicErrorPolicyDTO{OnExhausted: "skip"},
		Unclassified: &dto.DynamicUnclassifiedPolicyDTO{
			Enabled:                     true,
			ConsecutiveFailureThreshold: 3,
		},
	}
	created, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID: agents.DynamicAgentID,
		Name:    "Balanced",
		Dynamic: &dto.DynamicAgentProfileDTO{Candidates: []dto.DynamicAgentCandidateDTO{{
			Position:           0,
			ExecutionProfileID: candidate.ID,
			Enabled:            true,
			Policies:           policy,
		}}},
	})
	if err != nil {
		t.Fatalf("create dynamic profile: %v", err)
	}
	if created.Kind != "dynamic" || created.Dynamic == nil || created.Dynamic.Version != 1 {
		t.Fatalf("created dynamic profile = %#v", created)
	}
	dynamicRepo, ok := repo.(store.DynamicProfileRepository)
	if !ok {
		t.Fatal("repository does not implement dynamic profile storage")
	}
	assertStoredUnclassifiedPolicy := func(wantVersion int64, wantThreshold int64) {
		t.Helper()
		storedDynamic, storedRoutes, err := dynamicRepo.GetDynamicAgentProfile(ctx, created.ID)
		if err != nil {
			t.Fatalf("get dynamic profile from SQLite: %v", err)
		}
		storedDTO, err := dynamicProfileDTO(storedDynamic, storedRoutes)
		if err != nil {
			t.Fatalf("normalize stored dynamic profile: %v", err)
		}
		gotPolicy := storedDTO.Candidates[0].Policies.Unclassified
		if storedDTO.Version != wantVersion || gotPolicy == nil || !gotPolicy.Enabled || gotPolicy.ConsecutiveFailureThreshold != wantThreshold {
			t.Fatalf("stored policy = %#v, version = %d; want enabled threshold %d at version %d", gotPolicy, storedDTO.Version, wantThreshold, wantVersion)
		}
	}
	assertStoredUnclassifiedPolicy(1, 3)

	invalidPolicy := *policy
	invalidPolicy.Unclassified = &dto.DynamicUnclassifiedPolicyDTO{Enabled: true, ConsecutiveFailureThreshold: 11}
	if _, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{
		ID: created.ID,
		Dynamic: &dto.DynamicAgentProfileDTO{
			Version: 1,
			Candidates: []dto.DynamicAgentCandidateDTO{{
				Position:           0,
				ExecutionProfileID: candidate.ID,
				Enabled:            true,
				Policies:           &invalidPolicy,
			}},
		},
	}); !errors.Is(err, ErrDynamicProfileRule) {
		t.Fatalf("invalid threshold update error = %v, want %v", err, ErrDynamicProfileRule)
	}
	assertStoredUnclassifiedPolicy(1, 3)

	updated, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{
		ID: created.ID,
		Dynamic: &dto.DynamicAgentProfileDTO{
			Version: 1,
			Candidates: []dto.DynamicAgentCandidateDTO{{
				Position:           0,
				ExecutionProfileID: candidate.ID,
				Enabled:            false,
				Policies:           policy,
			}},
		},
	})
	if err != nil {
		t.Fatalf("update dynamic profile: %v", err)
	}
	if updated.Dynamic == nil || updated.Dynamic.Version != 2 || updated.Dynamic.Candidates[0].Enabled {
		t.Fatalf("updated dynamic profile = %#v", updated)
	}
	assertStoredUnclassifiedPolicy(2, 3)
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

func TestDynamicProfileCreateValidatesBeforeCreatingParent(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	if err := ctrl.agentRegistry.Register(agents.NewDynamicAgent()); err != nil {
		t.Fatalf("register dynamic agent: %v", err)
	}
	ctrl.SetDynamicAgentRoutingEnabled(true)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{ID: agents.DynamicAgentID, Name: agents.DynamicAgentID}); err != nil {
		t.Fatalf("create dynamic family: %v", err)
	}

	_, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID: agents.DynamicAgentID,
		Name:    "Invalid",
		Dynamic: &dto.DynamicAgentProfileDTO{Candidates: []dto.DynamicAgentCandidateDTO{{
			Position:           0,
			ExecutionProfileID: "missing-profile",
			Enabled:            true,
		}}},
	})
	if !errors.Is(err, ErrDynamicProfileCandidate) {
		t.Fatalf("create error = %v, want invalid candidate", err)
	}
	profiles, err := repo.ListAgentProfiles(ctx, agents.DynamicAgentID)
	if err != nil {
		t.Fatalf("list dynamic profiles: %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("created %d parent rows after validation failure, want 0", len(profiles))
	}
}
