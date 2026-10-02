package service

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/agent/agents"
)

func (s *Service) HandleAgentRuntimeUpdate(ctx context.Context, notice agents.RuntimeUpdateNotice) {
	if notice.AgentID == "" || notice.RuntimeID == "" || notice.Version == "" || notice.OccurrenceID == "" {
		return
	}
	s.handleSemanticOccurrence(ctx, "", "", notice.OccurrenceID, EventSystemUpdateAvailable, map[string]string{
		"agent_name": notice.AgentID, "runtime_id": notice.RuntimeID, "display_name": notice.DisplayName,
		"previous_version": notice.PreviousVersion, "version": notice.Version, "runtime_update_status": notice.Status, "url": notice.URL,
	})
}

func runtimeUpdateMessage(payload map[string]string) (string, string) {
	name, previous, target := payload["display_name"], payload["previous_version"], payload["version"]
	switch payload["runtime_update_status"] {
	case "succeeded":
		return fmt.Sprintf("%s runtime updated", name), fmt.Sprintf("%s runtime updated from %s to %s for future sessions. Open Settings > Agents > Runtime updates to select another version or return to the Kandev default.", name, previous, target)
	case "interrupted":
		return fmt.Sprintf("%s runtime update interrupted", name), fmt.Sprintf("%s runtime update from %s to %s could not be confirmed. Open Settings > Agents > Runtime updates to review the current selection before retrying or returning to the Kandev default.", name, previous, target)
	case "failed":
		return fmt.Sprintf("%s runtime update failed", name), fmt.Sprintf("%s runtime update from %s to %s did not activate. This attempt preserved your selection. Open Settings > Agents > Runtime updates to retry manually or disable automatic updates.", name, previous, target)
	default:
		return fmt.Sprintf("%s runtime update available", name), fmt.Sprintf("%s runtime %s is available (selected version: %s). Open Settings > Agents > Runtime updates to review the update control or manual guidance.", name, target, previous)
	}
}
