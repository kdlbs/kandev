package backendapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	settings "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const assistantClaudeAgent = "claude-acp"

type authorityProfiles interface {
	GetAgent(context.Context, string) (*settings.Agent, error)
	GetAgentProfile(context.Context, string) (*settings.AgentProfile, error)
}
type authorityExecutors interface {
	GetExecutor(context.Context, string) (*taskmodels.Executor, error)
	GetExecutorProfile(context.Context, string) (*taskmodels.ExecutorProfile, error)
}
type assistantAuthorityReader struct {
	profiles  authorityProfiles
	executors authorityExecutors
	versions  managedruntime.SelectionReader
	authorize func(context.Context, string) error
}

func (a assistantAuthorityReader) ResolveAssistantAuthority(ctx context.Context, binding models.AssistantBinding, profileID, executorID string) (models.AssistantAuthority, error) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.UserID != binding.OwnerUserID || a.authorize == nil || a.authorize(ctx, binding.WorkspaceID) != nil {
		return models.AssistantAuthority{}, fmt.Errorf("assistant_owner_unavailable")
	}
	profile, agent, err := a.authorityProfile(ctx, binding.WorkspaceID, profileID)
	if err != nil {
		return models.AssistantAuthority{}, err
	}
	executorProfile, executor, err := a.authorityExecutor(ctx, executorID)
	if err != nil {
		return models.AssistantAuthority{}, err
	}
	version, err := a.claudeVersion(ctx)
	if err != nil {
		return models.AssistantAuthority{}, err
	}
	row := models.AssistantAuthority{Restriction: "claude-broker-v1"}
	row.UnsupportedReason = assistantRestrictionCompatibility(agent, profile, executor, executorProfile, version)
	data, err := json.Marshal([]any{binding.OwnerUserID, binding.WorkspaceID, agent, profile, executor, executorProfile, version})
	if err != nil {
		return row, err
	}
	row.Revision = fmt.Sprintf("%x", sha256.Sum256(data))
	return row, nil
}

func (a assistantAuthorityReader) claudeVersion(ctx context.Context) (string, error) {
	spec := agents.NewClaudeACP().ManagedNPMRuntime()
	if a.versions != nil {
		selection, found, err := a.versions.Get(ctx, "claude-acp", spec.Package)
		if err != nil {
			return "", err
		}
		if found {
			return selection.Version, nil
		}
	}
	return spec.DefaultVersionOrPinned(), nil
}

func assistantRestrictionCompatibility(agent *settings.Agent, profile *settings.AgentProfile, executor *taskmodels.Executor, preset *taskmodels.ExecutorProfile, version string) string {
	if agent.Name != assistantClaudeAgent || agent.TUIConfig != nil || profile.CLIPassthrough || version != "0.75.1" {
		return "unsupported_provider_or_version"
	}
	if reason := assistantProfileCompatibility(profile); reason != "" {
		return reason
	}
	if executor.Type != taskmodels.ExecutorTypeLocal || executor.Status != taskmodels.ExecutorStatusActive || len(executor.Config) != 0 {
		return "unsupported_executor"
	}
	if preset.PrepareScript != "" || preset.CleanupScript != "" || len(preset.EnvVars) != 0 || len(preset.Config) != 0 {
		return "unsupported_executor_overrides"
	}
	return ""
}

func (a assistantAuthorityReader) authorityProfile(ctx context.Context, workspace, id string) (*settings.AgentProfile, *settings.Agent, error) {
	profile, err := a.profiles.GetAgentProfile(ctx, id)
	if err != nil || profile == nil || !profile.Enabled || profile.DeletedAt != nil || profile.Role != "" || (profile.WorkspaceID != "" && profile.WorkspaceID != workspace) {
		return nil, nil, fmt.Errorf("assistant_profile_unavailable")
	}
	agent, err := a.profiles.GetAgent(ctx, profile.AgentID)
	if err != nil || agent == nil || (agent.WorkspaceID != nil && *agent.WorkspaceID != workspace) {
		return nil, nil, fmt.Errorf("assistant_agent_unavailable")
	}
	return profile, agent, nil
}

func (a assistantAuthorityReader) authorityExecutor(ctx context.Context, id string) (*taskmodels.ExecutorProfile, *taskmodels.Executor, error) {
	preset, err := a.executors.GetExecutorProfile(ctx, id)
	if err != nil || preset == nil {
		return nil, nil, fmt.Errorf("assistant_executor_unavailable")
	}
	executor, err := a.executors.GetExecutor(ctx, preset.ExecutorID)
	if err != nil || executor == nil || executor.DeletedAt != nil {
		return nil, nil, fmt.Errorf("assistant_executor_unavailable")
	}
	return preset, executor, nil
}

func assistantProfileCompatibility(profile *settings.AgentProfile) string {
	if profile.CommandPrefix != "" || len(profile.ConfigOptions) != 0 || len(profile.EnvVars) != 0 || profile.AutoFallback || profile.FallbackModel != "" {
		return "unsupported_profile_overrides"
	}
	for _, flag := range profile.CLIFlags {
		if flag.Enabled {
			return "unsupported_profile_flags"
		}
	}
	if profile.Mode != "" && profile.Mode != "default" {
		return "unsupported_profile_mode"
	}

	return ""
}
