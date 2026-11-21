package lifecycle

import (
	"context"
	"path/filepath"

	"github.com/kandev/kandev/internal/agent/usage"
	util "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/agentruntime"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/agent"
)

// SetPreparedExecutionUsageReader uses the existing shared local native control
// namespace. It cannot prepare a runtime, create an actor, or fetch provider usage.
func (m *Manager) SetPreparedExecutionUsageReader(reader func(context.Context, *util.UsagePreviewRequest) *usage.ExecutionUsageReport) {
	m.preparedExecutionUsageReader = reader
}
func (m *Manager) CandidateExecutionUsageEnabled() bool {
	return m.providerUsageLimits && m.preparedExecutionUsageReader != nil
}

// PreviewLaunchExecutionUsage consumes a real owner's prepared launch inputs.
// Missing worktree materialization or unsupported executor stays unknown.
func (m *Manager) PreviewLaunchExecutionUsage(ctx context.Context, req *LaunchRequest) *usage.ExecutionUsageReport {
	if !m.CandidateExecutionUsageEnabled() || req == nil || (req.ExecutorType != "" && !localPreparedUsageExecutor(req.ExecutorType)) {
		return nil
	}
	backend, err := m.getExecutorBackend(req.ExecutorType)
	if err != nil || backend.Name() != agentruntime.RuntimeStandalone {
		return nil
	}
	if !m.preparedUsageOwnerKnown(ctx, req) {
		return nil
	}
	input := m.preparedLaunchUsageInput(ctx, req)
	if input == nil {
		return nil
	}
	return m.preparedExecutionUsageReader(ctx, input)
}
func localPreparedUsageExecutor(kind string) bool {
	switch kind {
	case string(taskmodels.ExecutorTypeLocal), "local_pc", string(taskmodels.ExecutorTypeWorktree), "standalone":
		return true
	default:
		return false
	}
}
func (m *Manager) preparedUsageOwnerKnown(ctx context.Context, req *LaunchRequest) bool {
	if req.Owner.Kind == ExecutionOwnerRun {
		if req.OwnerAdmission == nil || req.WorkspaceID != req.Owner.WorkspaceID {
			return false
		}
		if reader, ok := req.OwnerAdmission.(interface {
			CheckExecutionUsageOwner(context.Context, ExecutionOwner) error
		}); ok {
			return reader.CheckExecutionUsageOwner(ctx, req.Owner) == nil
		}
		return req.OwnerAdmission.AdmitExecution(ctx, req.Owner) == nil
	}
	return m.preparedTaskUsageOwnerKnown(ctx, req)
}

func (m *Manager) preparedTaskUsageOwnerKnown(ctx context.Context, req *LaunchRequest) bool {
	if req.SessionID == "" || req.TaskID == "" || m.executorProfileReader == nil {
		return false
	}
	if m.sessionAccessCheck != nil && m.sessionAccessCheck(ctx, req.SessionID) != nil {
		return false
	}
	session, err := m.executorProfileReader.GetTaskSession(ctx, req.SessionID)
	if err != nil || session == nil || session.ID != req.SessionID || session.TaskID != req.TaskID {
		return false
	}
	task, err := m.executorProfileReader.GetTask(ctx, req.TaskID)
	return err == nil && task != nil && task.WorkspaceID == req.WorkspaceID
}
func (m *Manager) preparedLaunchUsageInput(ctx context.Context, request *LaunchRequest) *util.UsagePreviewRequest {
	req := *request
	if req.ExecutorType == "" {
		req.ExecutorType = "standalone"
	}
	name, profile, err := m.resolveAgentProfile(ctx, &req)
	if err != nil || !preparedUsageProfileSupported(name, profile) {
		return nil
	}
	if m.registry == nil {
		return nil
	}
	config, ok := m.registry.Get(name)
	if !ok {
		return nil
	}
	if req.WorkspacePath == "" {
		// Only repo-less scratch paths are deterministic before materialization.
		if req.RepositoryPath != "" || len(req.Repositories) != 0 {
			return nil
		}
		req.WorkspacePath = m.scratchWorkspacePath(&req)
	}
	if !filepath.IsAbs(req.WorkspacePath) {
		return nil
	}
	commands, err := m.buildAgentCommandWithContext(ctx, &req, profile, config, agentruntime.RuntimeStandalone, m.preferNativeBinary(config, agentruntime.RuntimeStandalone, req.Metadata))
	if err != nil {
		return nil
	}
	env, err := m.executionEnvironmentSnapshot(ctx, "", &req, config, profile, false)
	if err != nil {
		return nil
	}
	model := profile.Model
	if req.ModelOverride != "" {
		model = req.ModelOverride
	}
	model, known := m.preparedUsageSessionModel(ctx, &req, model)
	if !known {
		return nil
	}
	return &util.UsagePreviewRequest{AgentID: name, Model: model, Policy: usage.ExecutionUsagePolicy{ProfileID: executionProfileID(&req), AgentID: name, ExecutorType: req.ExecutorType}, InferenceConfig: &util.InferenceConfigDTO{Protocol: agent.ProtocolACP, Command: commands.args, WorkDir: req.WorkspacePath, Env: env, StripEnv: append([]string(nil), config.Runtime().StripEnv...), LaunchWrapper: commands.wrapper}}
}

func preparedUsageProfileSupported(name string, profile *AgentProfileInfo) bool {
	return profile != nil && profile.ProviderKind == "" && (name == codexACPAgentID || name == claudeACPAgentID)
}

// These are the same persisted layers read by GetWorkspaceInfoForSession and
// initializeACPSession, without that provider's delivery-identity writes. The
// candidate profile default is not replaced by a previous profile snapshot.
func (m *Manager) preparedUsageSessionModel(ctx context.Context, req *LaunchRequest, model string) (string, bool) {
	if req.SessionID == "" {
		return model, true
	}
	if m.executorProfileReader == nil {
		return "", false
	}
	session, err := m.executorProfileReader.GetTaskSession(ctx, req.SessionID)
	if err != nil || session == nil || session.TaskID != req.TaskID {
		return "", false
	}
	if config, ok := taskmodels.LoadSessionRuntimeConfig(session.Metadata); ok && config.Model != "" {
		model = config.Model
	}
	if override, ok := taskmodels.LoadSessionRuntimeConfigOverrides(session.Metadata); ok && override.Model != "" {
		model = override.Model
	}
	return model, true
}
