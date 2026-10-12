package service

import (
	"context"
	"fmt"
)

// RemintCeilingLaunchCredentials implements orchestrator.CeilingLaunchCredentialReminter,
// re-minting KANDEV_API_KEY / KANDEV_RUN_TOKEN for a ceiling-deferred launch
// replay from the durable identity fields (agent/workspace/run id) that
// remain in env. The original env is returned unchanged when it carries no
// Office run identity (a non-Office launch, or an Office env this build's
// buildEnvVars shape has since dropped a field from) — there is nothing to
// re-mint, and startTask should proceed rather than fail the replay outright.
func (si *SchedulerIntegration) RemintCeilingLaunchCredentials(
	ctx context.Context, taskID string, env map[string]string,
) (map[string]string, error) {
	runID := env["KANDEV_RUN_ID"]
	agentID := env["KANDEV_AGENT_ID"]
	workspaceID := env["KANDEV_WORKSPACE_ID"]
	if runID == "" || agentID == "" || workspaceID == "" || si.svc.agentTokenMinter == nil {
		return env, nil
	}
	run, err := si.svc.repo.GetRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("ceiling credential re-mint: load run %s: %w", runID, err)
	}
	jwt, err := si.svc.agentTokenMinter.MintRuntimeJWT(agentID, taskID, workspaceID, runID, run.SessionID, run.Capabilities)
	if err != nil {
		return nil, fmt.Errorf("ceiling credential re-mint: mint runtime jwt: %w", err)
	}
	refreshed := make(map[string]string, len(env))
	for k, v := range env {
		refreshed[k] = v
	}
	refreshed["KANDEV_API_KEY"] = jwt
	refreshed["KANDEV_RUN_TOKEN"] = jwt
	return refreshed, nil
}
