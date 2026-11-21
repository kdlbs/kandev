package executor

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/task/models"
)

type candidateUsagePreviewer interface {
	CandidateExecutionUsageEnabled() bool
	PreviewLaunchExecutionUsage(context.Context, *LaunchAgentRequest) *usage.ExecutionUsageReport
}

// WithCandidateExecutionUsage snapshots only durable source definitions and the
// current launch's overrides. It never invokes checkout, credential brokers,
// delivery identity creation, workspace preparation, or a provider probe.
func (e *Executor) WithCandidateExecutionUsage(ctx context.Context, session *models.TaskSession, opts LaunchOptions) context.Context {
	preview, ok := e.agentManager.(candidateUsagePreviewer)
	if !ok || !preview.CandidateExecutionUsageEnabled() || session == nil {
		return ctx
	}
	snapshot := *session
	env := cloneStringMap(opts.Env)
	return usage.WithCandidateUsageReader(ctx, func(ctx context.Context, profileID string) *usage.ExecutionUsageReport {
		req := e.candidateUsageLaunchRequest(ctx, &snapshot, profileID, opts.ExecutorID, env)
		if req == nil {
			return nil
		}
		return preview.PreviewLaunchExecutionUsage(ctx, req)
	})
}

func (e *Executor) candidateUsageLaunchRequest(ctx context.Context, session *models.TaskSession, profileID, executorID string, env map[string]string) *LaunchAgentRequest {
	task, err := e.repo.GetTask(ctx, session.TaskID)
	if err != nil || task == nil {
		return nil
	}
	cfg, known := e.candidateUsageExecutorConfig(ctx, task, session, executorID)
	if !known {
		return nil
	}
	req := &LaunchAgentRequest{TaskID: task.ID, SessionID: session.ID, WorkspaceID: task.WorkspaceID, TaskTitle: task.Title, IsEphemeral: task.IsEphemeral, AgentProfileID: profileID, OfficeAgentProfileID: session.AgentProfileID, WorkspacePath: session.WorkspacePath, ExecutorType: cfg.ExecutorType, ExecutorConfig: cfg.ExecutorCfg, Metadata: cfg.Metadata, Env: cloneStringMap(env)}
	repos, err := e.candidateUsageRepositories(ctx, task.ID, task.WorkspaceID)
	if err != nil {
		return nil
	}
	if !e.candidateUsageWorkspace(ctx, req, session.TaskEnvironmentID, len(repos) != 0) {
		return nil
	}
	if e.resolveLaunchEnvironment(ctx, req, cfg.ProfileEnvVars, repos) != nil {
		return nil
	}
	return req
}

func candidateUsageLocalExecutor(kind string) bool {
	switch kind {
	case "local", "local_pc", "worktree", "standalone":
		return true
	default:
		return false
	}
}

func (e *Executor) candidateUsageRepositories(ctx context.Context, taskID, workspaceID string) ([]*repoInfo, error) {
	bindings, err := e.repo.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, err
	}
	result := make([]*repoInfo, 0, len(bindings))
	for _, binding := range bindings {
		if binding == nil || binding.RepositoryID == "" {
			continue
		}
		repository, readErr := e.repo.GetRepository(ctx, binding.RepositoryID)
		if readErr != nil {
			return nil, readErr
		}
		if repository == nil || repository.WorkspaceID != workspaceID {
			return nil, fmt.Errorf("candidate repository workspace is unavailable")
		}
		result = append(result, &repoInfo{Repository: repository})
	}
	return result, nil
}

func (e *Executor) candidateUsageExecutorConfig(ctx context.Context, task *models.Task, session *models.TaskSession, executorID string) (executorConfig, bool) {
	metadata := cloneMetadata(task.Metadata)
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	if session.ExecutorProfileID != "" {
		metadata["executor_profile_id"] = session.ExecutorProfileID
	}
	if executorID == "" {
		executorID = session.ExecutorID
	}
	cfg := e.resolveExecutorConfig(ctx, executorID, task.WorkspaceID, metadata)
	if !candidateUsageLocalExecutor(cfg.ExecutorType) {
		return executorConfig{}, false
	}
	// A failed profile lookup must not silently lend the default account.
	if session.ExecutorProfileID != "" {
		profile, profileErr := e.repo.GetExecutorProfile(ctx, session.ExecutorProfileID)
		if profileErr != nil || profile == nil || (profile.ExecutorID != "" && profile.ExecutorID != cfg.ExecutorID) {
			return executorConfig{}, false
		}
	}
	return cfg, true
}

func (e *Executor) candidateUsageWorkspace(ctx context.Context, req *LaunchAgentRequest, environmentID string, hasRepositories bool) bool {
	if req.WorkspacePath == "" && environmentID != "" {
		existing, envErr := e.repo.GetTaskEnvironment(ctx, environmentID)
		if envErr != nil || existing == nil || existing.TaskID != req.TaskID || existing.ExecutorType != req.ExecutorType {
			return false
		}
		req.WorkspacePath = existing.WorkspacePath
	}
	if req.WorkspacePath == "" {
		folders, folderErr := e.repo.ListTaskWorkspaceFolders(ctx, req.TaskID)
		if folderErr != nil || len(folders) != 0 || hasRepositories {
			return false
		}
	}
	return true
}
