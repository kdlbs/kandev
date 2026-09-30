package controller

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/secrets"
)

type recordingProfileDiscoveryUtility struct {
	request hostutility.ProfileCapabilityRequest
	called  bool
}

func (f *recordingProfileDiscoveryUtility) Get(string) (hostutility.AgentCapabilities, bool) {
	return hostutility.AgentCapabilities{Status: hostutility.StatusOK}, true
}

func (f *recordingProfileDiscoveryUtility) Refresh(context.Context, string) (hostutility.AgentCapabilities, error) {
	return hostutility.AgentCapabilities{Status: hostutility.StatusOK}, nil
}

func (f *recordingProfileDiscoveryUtility) ResolveModelConfig(
	context.Context,
	string,
	hostutility.ModelConfigResolutionRequest,
) (hostutility.ModelConfigResolution, error) {
	return hostutility.ModelConfigResolution{Status: hostutility.StatusOK}, nil
}

func (f *recordingProfileDiscoveryUtility) ProbeProfileCapabilities(
	_ context.Context,
	_ string,
	request hostutility.ProfileCapabilityRequest,
) (hostutility.ProfileCapabilityResult, error) {
	f.called = true
	f.request = request
	return hostutility.ProfileCapabilityResult{
		ContextRevision: "revision-1",
		Capabilities: hostutility.AgentCapabilities{
			Status:         hostutility.StatusOK,
			CurrentModelID: "model-a",
			CurrentModeID:  "build",
			Models:         []hostutility.Model{{ID: "model-a", Name: "Model A"}},
			Modes:          []hostutility.Mode{{ID: "build", Name: "Build"}},
			Commands:       []hostutility.Command{{Name: "init", Description: "Initialize"}},
		},
	}, nil
}

func TestFetchProfileDynamicModelsUsesSavedLaunchSettings(t *testing.T) {
	ctrl, repo, ctx, agentID, _ := newProviderTestController(t)
	profile := &models.AgentProfile{
		AgentID:       agentID,
		Name:          "Profile",
		Model:         "model-a",
		EnvVars:       []models.ProfileEnvVar{{Key: "CODEX_PATH", Value: "/profile/codex"}},
		CLIFlags:      []models.CLIFlag{{Flag: "--profile-catalog=saved", Enabled: true}},
		CommandPrefix: "npx --",
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	utility := &recordingProfileDiscoveryUtility{}
	ctrl.hostUtility = utility

	response, err := ctrl.FetchProfileDynamicModels(ctx, "codex-acp", dto.ProfileCapabilityRequest{
		ProfileID: profile.ID, AuthorizationScope: "user-1", Refresh: true,
	})
	if err != nil {
		t.Fatalf("FetchProfileDynamicModels: %v", err)
	}
	if utility.request.Refresh != true {
		t.Fatal("profile refresh flag was not forwarded")
	}
	want := hostutility.ProfileProbeContext{
		Scope:         "user-1:" + profile.ID,
		Env:           map[string]string{"CODEX_PATH": "/profile/codex"},
		CLIFlags:      []string{"--profile-catalog=saved"},
		CommandPrefix: []string{"npx", "--"},
	}
	if !reflect.DeepEqual(utility.request.Context, want) {
		t.Fatalf("resolved profile context = %#v, want %#v", utility.request.Context, want)
	}
	if response.Status != string(hostutility.StatusOK) || response.ContextRevision != "revision-1" {
		t.Fatalf("response status/revision = %q/%q", response.Status, response.ContextRevision)
	}
	if len(response.Models) != 1 || response.Models[0].ID != "model-a" || response.Models[0].Name != "Model A" ||
		len(response.Modes) != 1 || response.Modes[0].ID != "build" ||
		len(response.Commands) != 1 || response.Commands[0].Name != "init" {
		t.Fatalf("profile capability response omitted provider data: %#v", response)
	}
}

func TestFetchProfileDynamicModelsUsesCompleteDraftSnapshot(t *testing.T) {
	ctrl, _, ctx, agentID, _ := newProviderTestController(t)
	utility := &recordingProfileDiscoveryUtility{}
	ctrl.hostUtility = utility
	envVars := []dto.ProfileEnvVarDTO{{Key: "CODEX_PATH", Value: "/draft/codex"}}
	cliFlags := []dto.CLIFlagDTO{{Flag: "--profile-catalog=draft", Enabled: true}}
	commandPrefix := "sandbox --"

	_, err := ctrl.FetchProfileDynamicModels(ctx, "codex-acp", dto.ProfileCapabilityRequest{
		AuthorizationScope: "user-2",
		LaunchSettings: &dto.ProfileLaunchSettingsRequest{
			EnvVars:       &envVars,
			CLIFlags:      &cliFlags,
			CommandPrefix: &commandPrefix,
		},
	})
	if err != nil {
		t.Fatalf("FetchProfileDynamicModels draft: %v", err)
	}
	want := hostutility.ProfileProbeContext{
		Scope:         "user-2:draft:" + agentID,
		Env:           map[string]string{"CODEX_PATH": "/draft/codex"},
		CLIFlags:      []string{"--profile-catalog=draft"},
		CommandPrefix: []string{"sandbox", "--"},
	}
	if !reflect.DeepEqual(utility.request.Context, want) {
		t.Fatalf("resolved draft context = %#v, want %#v", utility.request.Context, want)
	}
}

func TestFetchProfileDynamicModelsRejectsUnavailableSecretsBeforeProbe(t *testing.T) {
	cases := []struct {
		name        string
		secretStore *profileSecretStore
	}{
		{name: "deleted global secret", secretStore: &profileSecretStore{}},
		{
			name: "workspace secret",
			secretStore: &profileSecretStore{secret: &secrets.Secret{
				ID: "secret-1", Scope: secrets.ScopeWorkspace, WorkspaceID: "workspace-1",
			}},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, repo, ctx, agentID, _ := newProviderTestController(t)
			profile := &models.AgentProfile{
				AgentID: agentID,
				Name:    "Profile with secret",
				EnvVars: []models.ProfileEnvVar{{Key: "API_TOKEN", SecretID: "secret-1"}},
			}
			if err := repo.CreateAgentProfile(ctx, profile); err != nil {
				t.Fatalf("seed profile: %v", err)
			}
			ctrl.secretStore = tt.secretStore
			utility := &recordingProfileDiscoveryUtility{}
			ctrl.hostUtility = utility

			_, err := ctrl.FetchProfileDynamicModels(ctx, "codex-acp", dto.ProfileCapabilityRequest{
				ProfileID: profile.ID, AuthorizationScope: "user-1",
			})
			if !errors.Is(err, ErrInvalidProfileDiscovery) {
				t.Fatalf("FetchProfileDynamicModels error = %v, want invalid discovery context", err)
			}
			if utility.called {
				t.Fatal("provider probe started with an unavailable secret")
			}
		})
	}
}

func TestFetchProfileDynamicModelsDraftCanExplicitlyClearSavedLaunchSettings(t *testing.T) {
	ctrl, repo, ctx, agentID, _ := newProviderTestController(t)
	profile := &models.AgentProfile{
		AgentID:       agentID,
		Name:          "Profile",
		EnvVars:       []models.ProfileEnvVar{{Key: "CODEX_PATH", Value: "/saved/codex"}},
		CLIFlags:      []models.CLIFlag{{Flag: "--profile-catalog=saved", Enabled: true}},
		CommandPrefix: "sandbox --",
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	utility := &recordingProfileDiscoveryUtility{}
	ctrl.hostUtility = utility
	emptyEnv := []dto.ProfileEnvVarDTO{}
	emptyFlags := []dto.CLIFlagDTO{}
	emptyPrefix := ""

	_, err := ctrl.FetchProfileDynamicModels(ctx, "codex-acp", dto.ProfileCapabilityRequest{
		ProfileID: profile.ID, AuthorizationScope: "user-1",
		LaunchSettings: &dto.ProfileLaunchSettingsRequest{
			EnvVars: &emptyEnv, CLIFlags: &emptyFlags, CommandPrefix: &emptyPrefix,
		},
	})
	if err != nil {
		t.Fatalf("FetchProfileDynamicModels with explicit clears: %v", err)
	}
	if len(utility.request.Context.Env) != 0 || len(utility.request.Context.CLIFlags) != 0 || len(utility.request.Context.CommandPrefix) != 0 {
		t.Fatalf("launch context retained saved values after clear: %#v", utility.request.Context)
	}
}
