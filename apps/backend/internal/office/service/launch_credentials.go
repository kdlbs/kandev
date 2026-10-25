package service

import (
	"context"
	"fmt"
)

// RemintLaunchCredentials binds Office credentials to the concrete launch session,
// falling back to the run session for early replay. Missing identity or a nil
// minter preserves the environment; required refresh failures return no environment.
func (si *SchedulerIntegration) RemintLaunchCredentials(
	ctx context.Context, taskID, sessionID string, env map[string]string,
) (map[string]string, error) {
	runID := env["KANDEV_RUN_ID"]
	agentID := env["KANDEV_AGENT_ID"]
	workspaceID := env["KANDEV_WORKSPACE_ID"]
	if runID == "" || agentID == "" || workspaceID == "" || si.svc.agentTokenMinter == nil {
		return env, nil
	}
	run, err := si.svc.repo.GetRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("launch credential re-mint: load run %s: %w", runID, err)
	}
	resolvedSessionID := sessionID
	if resolvedSessionID == "" {
		resolvedSessionID = run.SessionID
	}
	jwt, err := si.svc.agentTokenMinter.MintRuntimeJWT(
		agentID, taskID, workspaceID, runID, resolvedSessionID, run.Capabilities,
	)
	if err != nil {
		return nil, fmt.Errorf("launch credential re-mint: mint runtime jwt: %w", err)
	}
	refreshed := make(map[string]string, len(env))
	for k, v := range env {
		refreshed[k] = v
	}
	refreshed["KANDEV_API_KEY"] = jwt
	refreshed["KANDEV_RUN_TOKEN"] = jwt
	return refreshed, nil
}
