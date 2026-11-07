package service

import (
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

func workspaceFieldUpdate(req *UpdateWorkspaceRequest) models.WorkspaceFieldUpdate {
	update := models.WorkspaceFieldUpdate{
		Name: req.Name, Description: req.Description,
		ACPIdleSuspensionEnabled:           req.ACPIdleSuspensionEnabled,
		ACPIdleTimeoutMinutes:              req.ACPIdleTimeoutMinutes,
		DynamicMidTurnFailoverEnabled:      req.DynamicMidTurnFailoverEnabled,
		DynamicFailedRouteAutoRetryEnabled: req.DynamicFailedRouteAutoRetryEnabled,
		DynamicFailedRouteAutoRetryMax:     req.DynamicFailedRouteAutoRetryMax,

		ClarificationUnansweredInboxEnabled:  req.ClarificationUnansweredInboxEnabled,
		ClarificationUnansweredRetentionDays: req.ClarificationUnansweredRetentionDays,
	}
	update.DefaultExecutorID = workspaceDefaultUpdate(req.DefaultExecutorID)
	update.DefaultEnvironmentID = workspaceDefaultUpdate(req.DefaultEnvironmentID)
	update.DefaultAgentProfileID = workspaceDefaultUpdate(req.DefaultAgentProfileID)
	update.DefaultConfigAgentProfileID = workspaceDefaultUpdate(req.DefaultConfigAgentProfileID)
	return update
}

func validateClarificationUnansweredRetention(days *int) error {
	if days == nil {
		return nil
	}
	if *days < models.MinClarificationUnansweredRetentionDays || *days > models.MaxClarificationUnansweredRetentionDays {
		return fmt.Errorf("%w: got %d", ErrWorkspaceClarificationRetentionInvalid, *days)
	}
	return nil
}

func workspaceDefaultUpdate(value *string) **string {
	if value == nil {
		return nil
	}
	normalized := normalizeOptionalID(value)
	return &normalized
}
