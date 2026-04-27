package service

import (
	"github.com/kandev/kandev/internal/orchestrate/models"
)

// buildEnvVars constructs the environment variable map injected into agent
// sessions before launch. The map includes identity, API access, and wake
// context variables described in the agent-context spec.
func (si *SchedulerIntegration) buildEnvVars(
	wakeup *models.WakeupRequest,
	agent *models.AgentInstance,
	jwt, workspaceID string,
) map[string]string {
	env := map[string]string{
		"KANDEV_API_URL":      si.svc.apiBaseURL,
		"KANDEV_API_KEY":      jwt,
		"KANDEV_AGENT_ID":     agent.ID,
		"KANDEV_AGENT_NAME":   agent.Name,
		"KANDEV_WORKSPACE_ID": workspaceID,
		"KANDEV_RUN_ID":       wakeup.ID,
		"KANDEV_WAKE_REASON":  wakeup.Reason,
	}
	if taskID := extractField(wakeup.Payload, "task_id"); taskID != "" {
		env["KANDEV_TASK_ID"] = taskID
	}
	if commentID := extractField(wakeup.Payload, "comment_id"); commentID != "" {
		env["KANDEV_WAKE_COMMENT_ID"] = commentID
	}
	return env
}

// extractField parses a single key from a JSON payload string.
func extractField(payloadJSON, key string) string {
	return ParseWakeupPayload(payloadJSON)[key]
}

// SetAPIBaseURL sets the base URL used for KANDEV_API_URL in agent env vars.
func (s *Service) SetAPIBaseURL(url string) {
	s.apiBaseURL = url
}
