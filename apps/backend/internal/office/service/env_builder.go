package service

import (
	"github.com/kandev/kandev/internal/office/models"
)

// buildEnvVars constructs the environment variable map injected into agent
// sessions before launch. The map includes identity, API access, and wake
// context variables described in the agent-context spec.
func (si *SchedulerIntegration) buildEnvVars(
	run *models.Run,
	agent *models.AgentInstance,
	jwt, workspaceID string,
) map[string]string {
	env := map[string]string{
		"KANDEV_API_URL":      si.svc.apiBaseURL,
		"KANDEV_API_KEY":      jwt,
		"KANDEV_RUN_TOKEN":    jwt,
		"KANDEV_AGENT_ID":     agent.ID,
		"KANDEV_AGENT_NAME":   agent.Name,
		"KANDEV_WORKSPACE_ID": workspaceID,
		"KANDEV_RUN_ID":       run.ID,
		"KANDEV_WAKE_REASON":  run.Reason,
	}
	if taskID := extractField(run.Payload, "task_id"); taskID != "" {
		env["KANDEV_TASK_ID"] = taskID
	}
	if commentID := extractField(run.Payload, "comment_id"); commentID != "" {
		env["KANDEV_WAKE_COMMENT_ID"] = commentID
	}
	// KANDEV_CLI - path to agentctl binary for CLI operations.
	// Default to host binary path; overridden per executor type by injectKandevCLI.
	if si.svc.agentctlBinaryPath != "" {
		env["KANDEV_CLI"] = si.svc.agentctlBinaryPath
	}
	return env
}

// injectKandevCLI overrides KANDEV_CLI for remote executor types where the
// host binary path does not apply. For Docker and Sprites, the agentctl
// binary is baked into the image or uploaded, so we use the container-side path.
func (si *SchedulerIntegration) injectKandevCLI(env map[string]string, executorType string) {
	switch executorType {
	case "local_docker", "sprites":
		env["KANDEV_CLI"] = "/usr/local/bin/agentctl"
	}
}

// extractField parses a single key from a JSON payload string.
func extractField(payloadJSON, key string) string {
	return ParseRunPayload(payloadJSON)[key]
}
