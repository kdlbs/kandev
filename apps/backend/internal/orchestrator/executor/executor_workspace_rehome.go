package executor

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	workspaceMetadataSSHRemoteTaskDir = "ssh_remote_task_dir"
	workspaceMetadataContainerID      = "container_id"
)

type taskEnvironmentRehomer interface {
	ClaimTaskEnvironmentRehome(context.Context, string, string, string, bool) (bool, error)
}

// WorkspaceRehomeError preserves both failures for the durable session error
// projection. It deliberately keeps the original typed cause in the chain.
type WorkspaceRehomeError struct {
	Original error
	Recovery error
}

func (e *WorkspaceRehomeError) Error() string {
	return fmt.Sprintf("workspace rehome failed after %v: %v", e.Original, e.Recovery)
}

func (e *WorkspaceRehomeError) Unwrap() []error { return []error{e.Original, e.Recovery} }

func (e *Executor) retryLaunchAfterMissingWorkspace(
	ctx context.Context,
	taskID, sessionID string,
	env *models.TaskEnvironment,
	req *LaunchAgentRequest,
	original error,
	allowPossibleDataLoss bool,
) (*LaunchAgentResponse, error) {
	if !models.IsMissingTaskWorkspace(original) || env == nil || req == nil {
		return nil, original
	}
	rehomer, ok := e.repo.(taskEnvironmentRehomer)
	if !ok {
		return nil, original
	}
	claimed, err := rehomer.ClaimTaskEnvironmentRehome(ctx, taskID, env.ID, sessionID, allowPossibleDataLoss)
	if err != nil {
		return nil, &WorkspaceRehomeError{Original: original, Recovery: err}
	}
	if !claimed {
		return nil, &WorkspaceRehomeError{Original: original, Recovery: models.ErrWorkspacePreparing}
	}

	env.Status = models.TaskEnvironmentStatusCreating
	env.MaterializationSessionID = sessionID
	env.WorkspacePath = ""
	env.ControlPort = 0
	env.ContainerID = ""
	env.SandboxID = ""
	env.Repos = nil
	req.WorkspaceReuseRequired = false
	req.PreviousExecutionID = ""
	req.WorktreeID = ""
	for index := range req.Repositories {
		req.Repositories[index].WorktreeID = ""
	}
	if req.Metadata != nil {
		delete(req.Metadata, workspaceMetadataSSHRemoteTaskDir)
		delete(req.Metadata, workspaceMetadataContainerID)
		delete(req.Metadata, "sprite_name")
	}
	resp, retryErr := e.agentManager.LaunchAgent(ctx, req)
	if retryErr != nil {
		e.markTaskEnvironmentMaterializationFailed(ctx, env, sessionID)
		return nil, &WorkspaceRehomeError{Original: original, Recovery: retryErr}
	}
	if resp == nil {
		e.markTaskEnvironmentMaterializationFailed(ctx, env, sessionID)
		return nil, &WorkspaceRehomeError{Original: original, Recovery: fmt.Errorf("replacement launch returned no response")}
	}
	return resp, nil
}
