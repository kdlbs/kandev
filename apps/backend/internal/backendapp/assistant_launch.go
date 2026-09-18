package backendapp

import (
	"context"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	orchexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
)

func orchestrationLaunchContext(repos *Repositories, launch orchestrationruntime.Launch) orchexecutor.LaunchContext {
	// Every Orchestrator conversation uses the backend-owned broker surface.
	// Workspace coordinators have no user assistant binding, but they still need
	// the same scoped native workspace/task tools. Keeping this profile on the
	// launch prevents the agent from falling back to an ambient CLI/API key.
	value := mcpprofile.New(mcpprofile.SurfaceAssistantBroker, nil, nil)
	profile := &value
	prepared := launch.OnSessionPrepared
	if launch.Authority != nil {
		prepared = func(ctx context.Context, sessionID string) error {
			if err := launch.OnSessionPrepared(ctx, sessionID); err != nil {
				return err
			}
			return repos.Task.SetSessionMetadataKey(ctx, sessionID, orchestrationruntime.AssistantPolicyMetadata, string(mcpprofile.SurfaceAssistantBroker))
		}
	}
	return orchexecutor.LaunchContext{McpProfile: profile, ExecutorProfileID: launch.ExecutorID, Prompt: launch.Prompt, Env: launch.Env, OnSessionPrepared: prepared}
}
