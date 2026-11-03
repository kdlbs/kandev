package controller

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/profileconfig"
	"github.com/kandev/kandev/internal/agent/settings/store"
)

func newDevinProfileTestController(t *testing.T) (*Controller, store.Repository, context.Context, string) {
	t.Helper()
	ctrl, repo, ctx, _, _ := newProviderTestController(t)
	devin := &models.Agent{Name: agentsDevinForTest}
	if err := repo.CreateAgent(ctx, devin); err != nil {
		t.Fatalf("seed Devin agent: %v", err)
	}
	return ctrl, repo, ctx, devin.ID
}

const agentsDevinForTest = "devin-acp"

func sandboxValue(t *testing.T, profile *models.AgentProfile) (string, bool) {
	t.Helper()
	for _, envVar := range profile.EnvVars {
		if envVar.Key == profileconfig.DevinSandboxEnvKey {
			return envVar.Value, true
		}
	}
	return "", false
}

// @covers AC-AGENTS-DEVIN-PERMISSIONS-001.2
// @covers AC-AGENTS-DEVIN-PERMISSIONS-001.6
func TestDevinProfileDefaults(t *testing.T) {
	t.Run("new profile defaults to Smart and an explicit disabled sandbox", func(t *testing.T) {
		ctrl, repo, ctx, agentID := newDevinProfileTestController(t)
		created, err := ctrl.CreateProfile(ctx, CreateProfileRequest{AgentID: agentID, Name: "New"})
		if err != nil {
			t.Fatalf("CreateProfile: %v", err)
		}
		profile, err := repo.GetAgentProfile(ctx, created.ID)
		if err != nil {
			t.Fatalf("get profile: %v", err)
		}
		if profile.Mode != profileconfig.DevinSmartMode {
			t.Fatalf("mode = %q, want %q", profile.Mode, profileconfig.DevinSmartMode)
		}
		if value, ok := sandboxValue(t, profile); !ok || value != "false" {
			t.Fatalf("sandbox = %q/%t, want an explicit false override", value, ok)
		}
		if profile.AutoApprove {
			t.Fatal("auto-approve must keep its existing false default")
		}
	})

	t.Run("explicit mode and sandbox values are preserved", func(t *testing.T) {
		ctrl, repo, ctx, agentID := newDevinProfileTestController(t)
		created, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
			AgentID: agentID, Name: "Bypass", Mode: profileconfig.DevinBypassMode,
			EnvVars: []dto.ProfileEnvVarDTO{
				{Key: "OTHER", Value: "kept"},
				{Key: profileconfig.DevinSandboxEnvKey, Value: "false"},
			},
			AutoApprove: true,
		})
		if err != nil {
			t.Fatalf("CreateProfile: %v", err)
		}
		profile, err := repo.GetAgentProfile(ctx, created.ID)
		if err != nil {
			t.Fatalf("get profile: %v", err)
		}
		if profile.Mode != profileconfig.DevinBypassMode {
			t.Fatalf("mode = %q", profile.Mode)
		}
		if value, _ := sandboxValue(t, profile); value != "false" {
			t.Fatalf("sandbox = %q", value)
		}
		if len(profile.EnvVars) != 2 || profile.EnvVars[0].Key != "OTHER" {
			t.Fatalf("environment entries = %#v", profile.EnvVars)
		}
		if !profile.AutoApprove {
			t.Fatal("explicit auto-approve was not preserved")
		}
	})

	t.Run("the defaulted sandbox entry respects the environment bound", func(t *testing.T) {
		ctrl, _, ctx, agentID := newDevinProfileTestController(t)
		envVars := make([]dto.ProfileEnvVarDTO, 100)
		for i := range envVars {
			envVars[i] = dto.ProfileEnvVarDTO{Key: fmt.Sprintf("KEY_%d", i), Value: "value"}
		}
		_, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
			AgentID: agentID, Name: "Too many entries", EnvVars: envVars,
		})
		if !errors.Is(err, ErrInvalidProfileEnvVars) {
			t.Fatalf("CreateProfile error = %v, want ErrInvalidProfileEnvVars", err)
		}
	})

	t.Run("explicit inheritance keeps the provider setting", func(t *testing.T) {
		ctrl, repo, ctx, agentID := newDevinProfileTestController(t)
		created, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
			AgentID: agentID, Name: "Inherited", DevinSandboxInherited: true,
		})
		if err != nil {
			t.Fatalf("CreateProfile: %v", err)
		}
		profile, err := repo.GetAgentProfile(ctx, created.ID)
		if err != nil {
			t.Fatalf("get profile: %v", err)
		}
		if profile.Mode != profileconfig.DevinSmartMode {
			t.Fatalf("mode = %q, want Smart", profile.Mode)
		}
		if _, ok := sandboxValue(t, profile); ok {
			t.Fatalf("explicit inheritance gained a sandbox override: %#v", profile.EnvVars)
		}
	})

	t.Run("enabled sandbox without a mode selects Autonomous", func(t *testing.T) {
		ctrl, repo, ctx, agentID := newDevinProfileTestController(t)
		created, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
			AgentID: agentID, Name: "Sandboxed",
			EnvVars: []dto.ProfileEnvVarDTO{{Key: profileconfig.DevinSandboxEnvKey, Value: "true"}},
		})
		if err != nil {
			t.Fatalf("CreateProfile: %v", err)
		}
		profile, err := repo.GetAgentProfile(ctx, created.ID)
		if err != nil {
			t.Fatalf("get profile: %v", err)
		}
		if profile.Mode != profileconfig.DevinAutonomousMode {
			t.Fatalf("mode = %q, want autonomous", profile.Mode)
		}
	})

	t.Run("explicit enabled sandbox with Smart is rejected before storage", func(t *testing.T) {
		ctrl, repo, ctx, agentID := newDevinProfileTestController(t)
		_, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
			AgentID: agentID, Name: "Conflicting", Mode: profileconfig.DevinSmartMode,
			EnvVars: []dto.ProfileEnvVarDTO{{Key: profileconfig.DevinSandboxEnvKey, Value: "true"}},
		})
		if !errors.Is(err, ErrInvalidDevinPermissionIntent) {
			t.Fatalf("CreateProfile error = %v, want ErrInvalidDevinPermissionIntent", err)
		}
		profiles, listErr := repo.ListAgentProfiles(ctx, agentID)
		if listErr != nil {
			t.Fatalf("list profiles: %v", listErr)
		}
		if len(profiles) != 0 {
			t.Fatalf("rejected create persisted a profile: %#v", profiles)
		}
	})

	t.Run("unrelated existing profile updates and duplication keep their values", func(t *testing.T) {
		ctrl, repo, ctx, agentID := newDevinProfileTestController(t)
		legacy := &models.AgentProfile{AgentID: agentID, Name: "Legacy", AutoApprove: true}
		if err := repo.CreateAgentProfile(ctx, legacy); err != nil {
			t.Fatalf("seed legacy profile: %v", err)
		}
		name := "Renamed"
		updated, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: legacy.ID, Name: &name})
		if err != nil {
			t.Fatalf("UpdateProfile: %v", err)
		}
		if updated.Mode != "" {
			t.Fatalf("name-only update applied a default mode: %q", updated.Mode)
		}
		duplicated, err := ctrl.DuplicateProfile(ctx, DuplicateProfileRequest{ID: legacy.ID})
		if err != nil {
			t.Fatalf("DuplicateProfile: %v", err)
		}
		copied, err := repo.GetAgentProfile(ctx, duplicated.ID)
		if err != nil {
			t.Fatalf("get duplicate: %v", err)
		}
		if copied.Mode != "" {
			t.Fatalf("duplicate mode = %q, want the empty source mode", copied.Mode)
		}
		if _, ok := sandboxValue(t, copied); ok {
			t.Fatalf("duplicate gained a sandbox override: %#v", copied.EnvVars)
		}
	})

	t.Run("explicit empty mode patch stays empty", func(t *testing.T) {
		ctrl, repo, ctx, agentID := newDevinProfileTestController(t)
		created, err := ctrl.CreateProfile(ctx, CreateProfileRequest{AgentID: agentID, Name: "Smart"})
		if err != nil {
			t.Fatalf("CreateProfile: %v", err)
		}
		empty := ""
		updated, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: created.ID, Mode: &empty})
		if err != nil {
			t.Fatalf("UpdateProfile: %v", err)
		}
		if updated.Mode != "" {
			t.Fatalf("mode = %q, want the explicit empty value", updated.Mode)
		}
		profile, err := repo.GetAgentProfile(ctx, created.ID)
		if err != nil {
			t.Fatalf("get profile: %v", err)
		}
		if profile.Mode != "" {
			t.Fatalf("stored mode = %q", profile.Mode)
		}
	})

	t.Run("conflicting update is rejected", func(t *testing.T) {
		ctrl, _, ctx, agentID := newDevinProfileTestController(t)
		created, err := ctrl.CreateProfile(ctx, CreateProfileRequest{AgentID: agentID, Name: "Smart"})
		if err != nil {
			t.Fatalf("CreateProfile: %v", err)
		}
		sandbox := []dto.ProfileEnvVarDTO{{Key: profileconfig.DevinSandboxEnvKey, Value: "true"}}
		_, err = ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: created.ID, EnvVars: &sandbox})
		if !errors.Is(err, ErrInvalidDevinPermissionIntent) {
			t.Fatalf("UpdateProfile error = %v, want ErrInvalidDevinPermissionIntent", err)
		}
	})
}

// @covers AC-AGENTS-DEVIN-PERMISSIONS-001.4
func TestDevinSandboxProfileContext(t *testing.T) {
	ctrl, repo, ctx, agentID := newDevinProfileTestController(t)
	profile := &models.AgentProfile{
		AgentID: agentID, Name: "Sandboxed", Mode: profileconfig.DevinAutonomousMode,
		EnvVars: []models.ProfileEnvVar{{Key: profileconfig.DevinSandboxEnvKey, Value: "true"}},
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	utility := &draftDiscoveryUtility{}
	ctrl.hostUtility = utility

	if _, err := ctrl.FetchProfileDynamicModels(ctx, agentsDevinForTest, dto.ProfileCapabilityRequest{
		ProfileID: profile.ID, AuthorizationScope: "user-1",
	}); err != nil {
		t.Fatalf("FetchProfileDynamicModels: %v", err)
	}
	if got := utility.request.Context.Env[profileconfig.DevinSandboxEnvKey]; got != "true" {
		t.Fatalf("sandbox env = %q, want the literal true override", got)
	}
	if _, err := ctrl.ResolveAgentModelConfig(ctx, agentsDevinForTest, dto.ResolveAgentModelConfigRequest{
		Model: "model-a", ProfileID: profile.ID, AuthorizationScope: "user-1",
	}); err != nil {
		t.Fatalf("ResolveAgentModelConfig: %v", err)
	}
	if len(utility.modelRequests) != 1 || utility.modelRequests[0].ProfileContext == nil ||
		utility.modelRequests[0].ProfileContext.Env[profileconfig.DevinSandboxEnvKey] != "true" {
		t.Fatalf("model-option sandbox context = %#v", utility.modelRequests)
	}

	draft := completeDraftLaunchSettings()
	draftMode := profileconfig.DevinSmartMode
	draft.Mode = &draftMode
	sandboxEnv := []dto.ProfileEnvVarDTO{{Key: profileconfig.DevinSandboxEnvKey, Value: "false"}}
	draft.EnvVars = &sandboxEnv
	if _, err := ctrl.FetchProfileDynamicModels(ctx, agentsDevinForTest, dto.ProfileCapabilityRequest{
		AuthorizationScope: "user-1", LaunchSettings: draft,
	}); err != nil {
		t.Fatalf("draft FetchProfileDynamicModels: %v", err)
	}
	if got := utility.request.Context.Env[profileconfig.DevinSandboxEnvKey]; got != "false" {
		t.Fatalf("draft sandbox env = %q, want the edited draft value", got)
	}
}

// @covers AC-AGENTS-DEVIN-PERMISSIONS-001.3
func TestDevinUnavailableSmartKeepsRequestedValue(t *testing.T) {
	ctrl, repo, ctx, agentID := newDevinProfileTestController(t)
	profile := &models.AgentProfile{AgentID: agentID, Name: "Smart", Mode: profileconfig.DevinSmartMode}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	utility := &draftDiscoveryUtility{
		recordingProfileDiscoveryUtility: recordingProfileDiscoveryUtility{
			capabilityResult: &hostutility.ProfileCapabilityResult{
				ContextRevision: "revision-1",
				Capabilities: hostutility.AgentCapabilities{
					Status:         hostutility.StatusOK,
					CurrentModeID:  "code",
					CurrentModelID: "model-a",
					Models:         []hostutility.Model{{ID: "model-a", Name: "Model A"}},
					Modes:          []hostutility.Mode{{ID: "code", Name: "Code"}, {ID: "ask", Name: "Ask"}, {ID: "plan", Name: "Plan"}},
				},
			},
		},
	}
	ctrl.hostUtility = utility
	response, err := ctrl.FetchProfileDynamicModels(ctx, agentsDevinForTest, dto.ProfileCapabilityRequest{
		ProfileID: profile.ID, AuthorizationScope: "user-1",
	})
	if err != nil {
		t.Fatalf("FetchProfileDynamicModels: %v", err)
	}
	if len(response.Modes) != 3 || response.Modes[0].ID != "code" {
		t.Fatalf("modes = %#v, want only the advertised provider modes", response.Modes)
	}
	for _, mode := range response.Modes {
		if mode.ID == profileconfig.DevinSmartMode {
			t.Fatalf("unavailable Smart was synthesized: %#v", response.Modes)
		}
	}
	stored, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if stored.Mode != profileconfig.DevinSmartMode {
		t.Fatalf("saved mode = %q, want the requested Smart value to remain", stored.Mode)
	}
}

func draftPermissionSettings(mode string, sandbox string) *dto.ProfileLaunchSettingsRequest {
	envVars := []dto.ProfileEnvVarDTO{{Key: profileconfig.DevinSandboxEnvKey, Value: sandbox}}
	cliFlags := []dto.CLIFlagDTO{}
	commandPrefix := ""
	return &dto.ProfileLaunchSettingsRequest{
		Mode: &mode, EnvVars: &envVars, CLIFlags: &cliFlags, CommandPrefix: &commandPrefix,
	}
}

func seedDevinPermissionProfile(
	t *testing.T,
	repo store.Repository,
	ctx context.Context,
	agentID, mode, sandbox string,
) *models.AgentProfile {
	t.Helper()
	profile := &models.AgentProfile{
		AgentID: agentID, Name: mode + "-" + sandbox, Mode: mode,
		EnvVars: []models.ProfileEnvVar{{Key: profileconfig.DevinSandboxEnvKey, Value: sandbox}},
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	return profile
}

// @covers AC-AGENTS-DEVIN-PERMISSIONS-001.3
// @covers AC-AGENTS-DEVIN-PERMISSIONS-001.4
func TestSavedProfileDiscoveryUsesEditedPermissionIntent(t *testing.T) {
	tests := []struct {
		name         string
		savedMode    string
		savedSandbox string
		draftMode    string
		draftSandbox string
	}{
		{
			name: "enable sandbox on a saved Smart profile", savedMode: profileconfig.DevinSmartMode,
			savedSandbox: "false", draftMode: profileconfig.DevinAutonomousMode, draftSandbox: "true",
		},
		{
			name: "disable sandbox on a saved Autonomous profile", savedMode: profileconfig.DevinAutonomousMode,
			savedSandbox: "true", draftMode: profileconfig.DevinSmartMode, draftSandbox: "false",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, repo, ctx, agentID := newDevinProfileTestController(t)
			profile := seedDevinPermissionProfile(t, repo, ctx, agentID, tt.savedMode, tt.savedSandbox)
			utility := &draftDiscoveryUtility{}
			ctrl.hostUtility = utility
			draft := draftPermissionSettings(tt.draftMode, tt.draftSandbox)

			if _, err := ctrl.FetchProfileDynamicModels(ctx, agentsDevinForTest, dto.ProfileCapabilityRequest{
				ProfileID: profile.ID, AuthorizationScope: "user-1", LaunchSettings: draft,
			}); err != nil {
				t.Fatalf("baseline discovery rejected the edited intent: %v", err)
			}
			if !utility.called {
				t.Fatal("baseline discovery did not reach the utility")
			}
			if _, err := ctrl.ResolveAgentModelConfig(ctx, agentsDevinForTest, dto.ResolveAgentModelConfigRequest{
				Model: "model-a", Mode: tt.draftMode, ProfileID: profile.ID,
				AuthorizationScope: "user-1", LaunchSettings: draft,
			}); err != nil {
				t.Fatalf("dependent option resolution rejected the edited intent: %v", err)
			}
			if len(utility.modelRequests) != 1 {
				t.Fatalf("option requests = %d, want 1", len(utility.modelRequests))
			}
			stored, err := repo.GetAgentProfile(ctx, profile.ID)
			if err != nil {
				t.Fatalf("get profile: %v", err)
			}
			if stored.Mode != tt.savedMode {
				t.Fatalf("discovery mutated the saved mode: %q", stored.Mode)
			}
		})
	}
}

// @covers AC-AGENTS-DEVIN-PERMISSIONS-001.3
func TestSavedProfileDiscoveryRejectsConflictingPermissionIntent(t *testing.T) {
	ctrl, repo, ctx, agentID := newDevinProfileTestController(t)
	profile := seedDevinPermissionProfile(t, repo, ctx, agentID, profileconfig.DevinSmartMode, "false")
	utility := &draftDiscoveryUtility{}
	ctrl.hostUtility = utility
	conflicting := draftPermissionSettings(profileconfig.DevinSmartMode, "true")

	if _, err := ctrl.FetchProfileDynamicModels(ctx, agentsDevinForTest, dto.ProfileCapabilityRequest{
		ProfileID: profile.ID, AuthorizationScope: "user-1", LaunchSettings: conflicting,
	}); !errors.Is(err, ErrInvalidDevinPermissionIntent) {
		t.Fatalf("baseline error = %v, want ErrInvalidDevinPermissionIntent", err)
	}
	if utility.called {
		t.Fatal("conflicting baseline intent reached the utility")
	}
	if _, err := ctrl.ResolveAgentModelConfig(ctx, agentsDevinForTest, dto.ResolveAgentModelConfigRequest{
		Model: "model-a", Mode: profileconfig.DevinSmartMode, ProfileID: profile.ID,
		AuthorizationScope: "user-1", LaunchSettings: conflicting,
	}); !errors.Is(err, ErrInvalidDevinPermissionIntent) {
		t.Fatalf("resolve error = %v, want ErrInvalidDevinPermissionIntent", err)
	}
	if len(utility.modelRequests) != 0 {
		t.Fatal("conflicting option intent reached the utility")
	}
}

// @covers AC-AGENTS-DEVIN-PERMISSIONS-001.6
func TestSavedProfileDiscoveryKeepsSavedIntentWithoutDraft(t *testing.T) {
	ctrl, repo, ctx, agentID := newDevinProfileTestController(t)
	profile := seedDevinPermissionProfile(t, repo, ctx, agentID, profileconfig.DevinAutonomousMode, "true")
	utility := &draftDiscoveryUtility{}
	ctrl.hostUtility = utility

	if _, err := ctrl.FetchProfileDynamicModels(ctx, agentsDevinForTest, dto.ProfileCapabilityRequest{
		ProfileID: profile.ID, AuthorizationScope: "user-1",
	}); err != nil {
		t.Fatalf("saved-only discovery: %v", err)
	}
	if !utility.called {
		t.Fatal("saved-only discovery did not reach the utility")
	}
}
