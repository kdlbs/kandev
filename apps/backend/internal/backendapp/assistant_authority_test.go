package backendapp

import (
	"context"
	"encoding/json"
	"testing"

	settings "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type authorityTestProfiles struct{ profile *settings.AgentProfile }

func (p *authorityTestProfiles) GetAgent(context.Context, string) (*settings.Agent, error) {
	return &settings.Agent{ID: "claude-acp"}, nil
}
func (p *authorityTestProfiles) GetAgentProfile(context.Context, string) (*settings.AgentProfile, error) {
	return p.profile, nil
}

type authorityTestExecutors struct {
	preset   *taskmodels.ExecutorProfile
	executor *taskmodels.Executor
}

func (e *authorityTestExecutors) GetExecutorProfile(context.Context, string) (*taskmodels.ExecutorProfile, error) {
	return e.preset, nil
}
func (e *authorityTestExecutors) GetExecutor(context.Context, string) (*taskmodels.Executor, error) {
	return e.executor, nil
}

func TestAssistantAuthorityOwnerProfileAndExecutor(t *testing.T) {
	_, tasks := newOfficeTaskAdapterHarness(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner", Role: authn.RoleMember})
	p := &authorityTestProfiles{profile: &settings.AgentProfile{ID: "profile", AgentID: "claude-acp", Enabled: true}}
	e := &authorityTestExecutors{preset: &taskmodels.ExecutorProfile{ID: "local", ExecutorID: "machine"}, executor: &taskmodels.Executor{ID: "machine", Type: taskmodels.ExecutorTypeLocal, Status: taskmodels.ExecutorStatusActive}}
	reader := assistantAuthorityReader{profiles: p, executors: e, authorize: tasks.AuthorizeWorkspaceAccess}
	binding := models.AssistantBinding{OwnerUserID: "owner", WorkspaceID: "ws-1"}
	first, err := reader.ResolveAssistantAuthority(ctx, binding, "profile", "local")
	require.NoError(t, err)
	require.Empty(t, first.UnsupportedReason)
	_, err = reader.ResolveAssistantAuthority(context.Background(), binding, "profile", "local")
	require.Error(t, err)
	p.profile.EnvVars = []settings.ProfileEnvVar{{Key: "TOKEN", Value: "SYNTHETIC_ENV_CANARY"}}
	changed, err := reader.ResolveAssistantAuthority(ctx, binding, "profile", "local")
	require.NoError(t, err)
	require.NotEqual(t, first.Revision, changed.Revision)
	require.NotEmpty(t, changed.UnsupportedReason)
	data, err := json.Marshal(changed)
	require.NoError(t, err)
	require.NotContains(t, string(data), "SYNTHETIC_ENV_CANARY")
	p.profile.Enabled = false
	_, err = reader.ResolveAssistantAuthority(ctx, binding, "profile", "local")
	require.Error(t, err)
}

func TestAssistantReadOnlySupportedMatrix(t *testing.T) {
	for _, change := range []string{"codex", "version", "shell", "setup", "remote", "flags", "passthrough"} {
		t.Run(change, func(t *testing.T) {
			agent := &settings.Agent{ID: "claude-acp"}
			profile := &settings.AgentProfile{}
			executor := &taskmodels.Executor{Type: taskmodels.ExecutorTypeLocal, Status: taskmodels.ExecutorStatusActive}
			preset := &taskmodels.ExecutorProfile{}
			version := "0.75.1"
			switch change {
			case "codex":
				agent.ID = "codex"
			case "version":
				version = "0.75.2"
			case "shell":
				profile.CommandPrefix = "sh"
			case "setup":
				preset.PrepareScript = "mutate"
			case "remote":
				executor.Type = taskmodels.ExecutorTypeSSH
			case "flags":
				profile.CLIFlags = []settings.CLIFlag{{Flag: "--tools=Bash", Enabled: true}}
			case "passthrough":
				profile.CLIPassthrough = true
			}
			require.NotEmpty(t, assistantRestrictionCompatibility(agent, profile, executor, preset, version))
		})
	}
}
