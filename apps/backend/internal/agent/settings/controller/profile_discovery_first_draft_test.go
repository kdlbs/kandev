package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/secrets"
)

// draftDiscoveryUtility records both capability and model-option requests so a
// draft regression can compare the two probe contexts.
type draftDiscoveryUtility struct {
	recordingProfileDiscoveryUtility
	modelRequests []hostutility.ModelConfigResolutionRequest
}

func (f *draftDiscoveryUtility) ResolveModelConfig(
	ctx context.Context,
	agentName string,
	request hostutility.ModelConfigResolutionRequest,
) (hostutility.ModelConfigResolution, error) {
	f.modelRequests = append(f.modelRequests, request)
	return f.recordingProfileDiscoveryUtility.ResolveModelConfig(ctx, agentName, request)
}

func completeDraftLaunchSettings() *dto.ProfileLaunchSettingsRequest {
	envVars := []dto.ProfileEnvVarDTO{{Key: "CODEX_PATH", Value: "/draft/codex"}}
	cliFlags := []dto.CLIFlagDTO{{Flag: "--profile-catalog=draft", Enabled: true}}
	commandPrefix := "sandbox --"
	return &dto.ProfileLaunchSettingsRequest{
		EnvVars:       &envVars,
		CLIFlags:      &cliFlags,
		CommandPrefix: &commandPrefix,
	}
}

func requireNoAgentRow(t *testing.T, repo store.Repository, ctx context.Context, agentName string) {
	t.Helper()
	agent, err := repo.GetAgentByName(ctx, agentName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("get agent %q: %v", agentName, err)
	}
	if agent != nil {
		t.Fatalf("agent %q unexpectedly has a persisted parent row: %+v", agentName, agent)
	}
}

// @covers AC-AGENTS-PROFILE-DISCOVERY-001.2
// @covers AC-AGENTS-PROFILE-DISCOVERY-001.3
// @covers AC-AGENTS-PROFILE-DISCOVERY-001.5
func TestFirstDraftProfileDiscoveryWithoutAgentRow(t *testing.T) {
	for _, agentName := range []string{"devin-acp", "qwen-acp"} {
		for _, refresh := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/refresh=%t", agentName, refresh), func(t *testing.T) {
				ctrl, repo, ctx, _, _ := newProviderTestController(t)
				requireNoAgentRow(t, repo, ctx, agentName)
				utility := &draftDiscoveryUtility{
					recordingProfileDiscoveryUtility: recordingProfileDiscoveryUtility{
						modelResolution: hostutility.ModelConfigResolution{
							Status:          hostutility.StatusOK,
							ContextRevision: "revision-2",
							ConfigOptions: []hostutility.ConfigOption{{
								ID: "thinking", Name: "Thinking", Type: "select", CurrentValue: "high",
								Options: []hostutility.ConfigOptionChoice{{Value: "high", Name: "High"}},
							}},
						},
					},
				}
				ctrl.hostUtility = utility
				launch := completeDraftLaunchSettings()

				capabilities, err := ctrl.FetchProfileDynamicModels(ctx, agentName, dto.ProfileCapabilityRequest{
					AuthorizationScope: "user-1", LaunchSettings: launch, Refresh: refresh,
				})
				if err != nil {
					t.Fatalf("FetchProfileDynamicModels: %v", err)
				}
				if !utility.called {
					t.Fatal("draft discovery never reached the host utility")
				}
				if utility.request.Refresh != refresh {
					t.Fatalf("forwarded refresh = %t, want %t", utility.request.Refresh, refresh)
				}
				wantScope := "user-1:draft:" + agentName
				if utility.request.Context.Scope != wantScope {
					t.Fatalf("draft scope = %q, want %q", utility.request.Context.Scope, wantScope)
				}
				if capabilities.Status != string(hostutility.StatusOK) || capabilities.ContextRevision != "revision-1" {
					t.Fatalf("capability status/revision = %q/%q", capabilities.Status, capabilities.ContextRevision)
				}
				if len(capabilities.Models) != 1 || capabilities.Models[0].ID != "model-a" ||
					len(capabilities.Modes) != 1 || capabilities.Modes[0].ID != "build" ||
					len(capabilities.Commands) != 1 || capabilities.Commands[0].Name != "init" {
					t.Fatalf("draft capabilities omitted provider data: %#v", capabilities)
				}
				requireNoAgentRow(t, repo, ctx, agentName)

				options, err := ctrl.ResolveAgentModelConfig(ctx, agentName, dto.ResolveAgentModelConfigRequest{
					Model: "model-a", ProfileID: "", LaunchSettings: launch,
					AuthorizationScope: "user-1", Refresh: refresh,
				})
				if err != nil {
					t.Fatalf("ResolveAgentModelConfig: %v", err)
				}
				if len(utility.modelRequests) != 1 {
					t.Fatalf("model-option requests = %d, want 1", len(utility.modelRequests))
				}
				request := utility.modelRequests[0]
				if request.ProfileContext == nil || request.ProfileContext.Scope != wantScope {
					t.Fatalf("model-option profile context = %#v, want draft scope %q", request.ProfileContext, wantScope)
				}
				if request.Refresh != refresh || request.Model != "model-a" {
					t.Fatalf("model-option request = %#v", request)
				}
				if options.Status != string(hostutility.StatusOK) || options.ContextRevision != "revision-2" {
					t.Fatalf("option status/revision = %q/%q", options.Status, options.ContextRevision)
				}
				if len(options.ConfigOptions) != 1 || options.ConfigOptions[0].ID != "thinking" ||
					options.ConfigOptions[0].CurrentValue != "high" {
					t.Fatalf("draft option snapshot = %#v", options.ConfigOptions)
				}
				requireNoAgentRow(t, repo, ctx, agentName)
			})
		}
	}
}

// @covers AC-AGENTS-PROFILE-DISCOVERY-001.6
// @covers AC-AGENTS-PROFILE-DISCOVERY-002.1
// @covers AC-AGENTS-PROFILE-DISCOVERY-002.2
func TestFirstDraftProfileDiscoveryScope(t *testing.T) {
	ctrl, repo, ctx, codexID, _ := newProviderTestController(t)
	utility := &draftDiscoveryUtility{}
	ctrl.hostUtility = utility
	launch := completeDraftLaunchSettings()

	draftScope := func(agentName, scope string) string {
		t.Helper()
		utility.request = hostutility.ProfileCapabilityRequest{}
		if _, err := ctrl.FetchProfileDynamicModels(ctx, agentName, dto.ProfileCapabilityRequest{
			AuthorizationScope: scope, LaunchSettings: launch,
		}); err != nil {
			t.Fatalf("FetchProfileDynamicModels(%s): %v", agentName, err)
		}
		return utility.request.Context.Scope
	}
	draftOptionScope := func(agentName, scope string) string {
		t.Helper()
		utility.modelRequests = nil
		if _, err := ctrl.ResolveAgentModelConfig(ctx, agentName, dto.ResolveAgentModelConfigRequest{
			Model: "model-a", LaunchSettings: launch, AuthorizationScope: scope,
		}); err != nil {
			t.Fatalf("ResolveAgentModelConfig(%s): %v", agentName, err)
		}
		if len(utility.modelRequests) != 1 || utility.modelRequests[0].ProfileContext == nil {
			t.Fatalf("model-option request was not recorded: %#v", utility.modelRequests)
		}
		return utility.modelRequests[0].ProfileContext.Scope
	}

	baseline := draftScope("qwen-acp", "user-1")
	if want := "user-1:draft:qwen-acp"; baseline != want {
		t.Fatalf("draft scope = %q, want %q", baseline, want)
	}
	if optionScope := draftOptionScope("qwen-acp", "user-1"); optionScope != baseline {
		t.Fatalf("option scope = %q, want the baseline draft scope %q", optionScope, baseline)
	}

	if err := repo.CreateAgent(ctx, &models.Agent{Name: "qwen-acp"}); err != nil {
		t.Fatalf("seed qwen parent: %v", err)
	}
	if afterParent := draftScope("qwen-acp", "user-1"); afterParent != baseline {
		t.Fatalf("draft scope after parent creation = %q, want stable %q", afterParent, baseline)
	}

	if other := draftScope("devin-acp", "user-1"); other == baseline {
		t.Fatalf("another registered agent reused the draft scope %q", other)
	}
	if other := draftScope("qwen-acp", "user-2"); other == baseline {
		t.Fatalf("another authorization scope reused the draft scope %q", other)
	}

	profile := &models.AgentProfile{AgentID: codexID, Name: "Saved"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("seed saved profile: %v", err)
	}
	utility.request = hostutility.ProfileCapabilityRequest{}
	if _, err := ctrl.FetchProfileDynamicModels(ctx, "codex-acp", dto.ProfileCapabilityRequest{
		AuthorizationScope: "user-1", ProfileID: profile.ID,
	}); err != nil {
		t.Fatalf("FetchProfileDynamicModels(saved): %v", err)
	}
	if got, want := utility.request.Context.Scope, "user-1:"+profile.ID; got != want {
		t.Fatalf("saved scope = %q, want %q", got, want)
	}
}

// @covers AC-AGENTS-PROFILE-DISCOVERY-001.1
// @covers AC-AGENTS-PROFILE-DISCOVERY-001.4
func TestFirstDraftProfileDiscoveryValidation(t *testing.T) {
	tests := []struct {
		name      string
		agentName string
		scope     string
		launch    *dto.ProfileLaunchSettingsRequest
		secret    *profileSecretStore
		seed      func(t *testing.T, ctrl *Controller, repo store.Repository, ctx context.Context, reg *registry.Registry)
		wantErr   error
	}{
		{
			name: "unknown agent", agentName: "missing-agent", scope: "user-1",
			launch: completeDraftLaunchSettings(), wantErr: ErrAgentNotFound,
		},
		{
			name: "disabled optional agent", agentName: "codex-app-server", scope: "user-1",
			launch: completeDraftLaunchSettings(),
			seed: func(t *testing.T, _ *Controller, repo store.Repository, ctx context.Context, _ *registry.Registry) {
				if err := repo.CreateAgent(ctx, &models.Agent{Name: "codex-app-server"}); err != nil {
					t.Fatalf("seed disabled parent: %v", err)
				}
			},
			wantErr: ErrAgentNotFound,
		},
		{
			name: "non-inference agent", agentName: "terminal-agent", scope: "user-1",
			launch: completeDraftLaunchSettings(),
			seed: func(t *testing.T, _ *Controller, _ store.Repository, _ context.Context, reg *registry.Registry) {
				if err := reg.Register(agents.NewTUIAgent(agents.TUIAgentConfig{
					AgentID: "terminal-agent", AgentName: "Terminal", Command: "terminal-agent",
				})); err != nil {
					t.Fatalf("register terminal agent: %v", err)
				}
			},
			wantErr: ErrInvalidProfileDiscovery,
		},
		{
			name: "empty authority", agentName: "devin-acp", scope: "",
			launch: completeDraftLaunchSettings(), wantErr: ErrInvalidProfileDiscovery,
		},
		{
			name: "omitted draft snapshot", agentName: "devin-acp", scope: "user-1",
			launch: nil, wantErr: ErrInvalidProfileDiscovery,
		},
		{
			name: "incomplete draft snapshot", agentName: "devin-acp", scope: "user-1",
			launch: func() *dto.ProfileLaunchSettingsRequest {
				envVars := []dto.ProfileEnvVarDTO{}
				commandPrefix := ""
				return &dto.ProfileLaunchSettingsRequest{EnvVars: &envVars, CommandPrefix: &commandPrefix}
			}(),
			wantErr: ErrInvalidProfileDiscovery,
		},
		{
			name: "deleted global secret", agentName: "devin-acp", scope: "user-1",
			launch: func() *dto.ProfileLaunchSettingsRequest {
				envVars := []dto.ProfileEnvVarDTO{{Key: "API_TOKEN", SecretID: "secret-1"}}
				cliFlags := []dto.CLIFlagDTO{}
				commandPrefix := ""
				return &dto.ProfileLaunchSettingsRequest{EnvVars: &envVars, CLIFlags: &cliFlags, CommandPrefix: &commandPrefix}
			}(),
			secret:  &profileSecretStore{},
			wantErr: ErrInvalidProfileDiscovery,
		},
		{
			name: "workspace secret", agentName: "devin-acp", scope: "user-1",
			launch: func() *dto.ProfileLaunchSettingsRequest {
				envVars := []dto.ProfileEnvVarDTO{{Key: "API_TOKEN", SecretID: "secret-1"}}
				cliFlags := []dto.CLIFlagDTO{}
				commandPrefix := ""
				return &dto.ProfileLaunchSettingsRequest{EnvVars: &envVars, CLIFlags: &cliFlags, CommandPrefix: &commandPrefix}
			}(),
			secret: &profileSecretStore{secret: &secrets.Secret{
				ID: "secret-1", Scope: secrets.ScopeWorkspace, WorkspaceID: "workspace-1",
			}},
			wantErr: ErrInvalidProfileDiscovery,
		},
		{
			name: "explicit empty launch settings", agentName: "devin-acp", scope: "user-1",
			launch: explicitEmptyDraftLaunchSettings(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, repo, ctx, _, _ := newProviderTestController(t)
			if tt.seed != nil {
				tt.seed(t, ctrl, repo, ctx, ctrl.agentRegistry)
			}
			if tt.secret != nil {
				ctrl.secretStore = tt.secret
			}
			utility := &draftDiscoveryUtility{}
			ctrl.hostUtility = utility

			_, err := ctrl.FetchProfileDynamicModels(ctx, tt.agentName, dto.ProfileCapabilityRequest{
				AuthorizationScope: tt.scope, LaunchSettings: tt.launch,
			})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("FetchProfileDynamicModels error = %v, want %v", err, tt.wantErr)
				}
				if utility.called {
					t.Fatal("refused draft still reached the host utility")
				}
				return
			}
			if err != nil {
				t.Fatalf("FetchProfileDynamicModels: %v", err)
			}
			if !utility.called {
				t.Fatal("valid draft never reached the host utility")
			}
		})
	}
}

func explicitEmptyDraftLaunchSettings() *dto.ProfileLaunchSettingsRequest {
	envVars := []dto.ProfileEnvVarDTO{}
	cliFlags := []dto.CLIFlagDTO{}
	commandPrefix := ""
	return &dto.ProfileLaunchSettingsRequest{
		EnvVars: &envVars, CLIFlags: &cliFlags, CommandPrefix: &commandPrefix,
	}
}

// failingAgentLookupRepo propagates a repository failure from the saved-parent
// lookup so the draft path cannot swallow it.
type failingAgentLookupRepo struct {
	store.Repository
	err error
}

func (r failingAgentLookupRepo) GetAgentByName(context.Context, string) (*models.Agent, error) {
	return nil, r.err
}

// @covers AC-AGENTS-PROFILE-DISCOVERY-001.4
func TestSavedProfileDiscoveryRequiresParentOwnership(t *testing.T) {
	tests := []struct {
		name     string
		profile  func(codexID, claudeID string) *models.AgentProfile
		agent    string
		skipSeed bool
		wantErr  error
	}{
		{
			name:     "missing profile",
			agent:    "codex-acp",
			profile:  func(string, string) *models.AgentProfile { return &models.AgentProfile{ID: "missing-profile"} },
			skipSeed: true,
			wantErr:  ErrProfileDiscoveryMissing,
		},
		{
			name:  "another agent's profile",
			agent: "claude-acp",
			profile: func(codexID, _ string) *models.AgentProfile {
				return &models.AgentProfile{AgentID: codexID, Name: "Codex profile"}
			},
			wantErr: ErrProfileDiscoveryMissing,
		},
		{
			name:  "workspace profile",
			agent: "codex-acp",
			profile: func(codexID, _ string) *models.AgentProfile {
				return &models.AgentProfile{AgentID: codexID, Name: "Workspace", WorkspaceID: "workspace-1"}
			},
			wantErr: ErrProfileDiscoveryMissing,
		},
		{
			name:  "role profile",
			agent: "codex-acp",
			profile: func(codexID, _ string) *models.AgentProfile {
				return &models.AgentProfile{AgentID: codexID, Name: "Role", Role: "worker"}
			},
			wantErr: ErrProfileDiscoveryMissing,
		},
		{
			name:  "dynamic profile",
			agent: "codex-acp",
			profile: func(_, _ string) *models.AgentProfile {
				return &models.AgentProfile{AgentID: agents.DynamicAgentID, Name: "Dynamic"}
			},
			wantErr: ErrProfileDiscoveryMissing,
		},
		{
			name:  "orphan profile without a parent row",
			agent: "qwen-acp",
			profile: func(_, _ string) *models.AgentProfile {
				return &models.AgentProfile{AgentID: "deleted-parent-row", Name: "Orphan"}
			},
			wantErr: ErrAgentNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, repo, ctx, codexID, claudeID := newProviderTestController(t)
			profile := tt.profile(codexID, claudeID)
			if !tt.skipSeed {
				if err := repo.CreateAgentProfile(ctx, profile); err != nil {
					t.Fatalf("seed profile: %v", err)
				}
			}
			utility := &draftDiscoveryUtility{}
			ctrl.hostUtility = utility

			_, err := ctrl.FetchProfileDynamicModels(ctx, tt.agent, dto.ProfileCapabilityRequest{
				AuthorizationScope: "user-1", ProfileID: profile.ID,
				LaunchSettings: completeDraftLaunchSettings(),
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("FetchProfileDynamicModels error = %v, want %v", err, tt.wantErr)
			}
			if utility.called {
				t.Fatal("refused saved profile still reached the host utility")
			}
		})
	}

	t.Run("repository failure", func(t *testing.T) {
		ctrl, repo, ctx, codexID, _ := newProviderTestController(t)
		profile := &models.AgentProfile{AgentID: codexID, Name: "Saved"}
		if err := repo.CreateAgentProfile(ctx, profile); err != nil {
			t.Fatalf("seed profile: %v", err)
		}
		boom := errors.New("repository lookup failed")
		ctrl.repo = failingAgentLookupRepo{Repository: repo, err: boom}
		ctrl.hostUtility = &draftDiscoveryUtility{}

		_, err := ctrl.FetchProfileDynamicModels(ctx, "codex-acp", dto.ProfileCapabilityRequest{
			AuthorizationScope: "user-1", ProfileID: profile.ID,
		})
		if !errors.Is(err, boom) {
			t.Fatalf("FetchProfileDynamicModels error = %v, want the repository failure", err)
		}
	})
}

// TestFirstDraftProfileDiscoveryScopeIsCanonical pins the registry identity in
// the draft scope even when the seeded row UUID differs from the registry ID.
func TestFirstDraftProfileDiscoveryScopeIsCanonical(t *testing.T) {
	ctrl, _, ctx, codexID, _ := newProviderTestController(t)
	utility := &draftDiscoveryUtility{}
	ctrl.hostUtility = utility
	if codexID == "codex-acp" {
		t.Fatalf("fixture parent row ID unexpectedly equals the registry ID")
	}

	if _, err := ctrl.FetchProfileDynamicModels(ctx, "codex-acp", dto.ProfileCapabilityRequest{
		AuthorizationScope: "user-5", LaunchSettings: completeDraftLaunchSettings(),
	}); err != nil {
		t.Fatalf("FetchProfileDynamicModels: %v", err)
	}
	if got, want := utility.request.Context.Scope, "user-5:draft:codex-acp"; got != want {
		t.Fatalf("draft scope = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(utility.request.Context.CLIFlags, []string{"--profile-catalog=draft"}) {
		t.Fatalf("draft launch context lost CLI flags: %#v", utility.request.Context)
	}
}
