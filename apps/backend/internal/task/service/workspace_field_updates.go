package service

import "github.com/kandev/kandev/internal/task/models"

func workspaceFieldUpdate(req *UpdateWorkspaceRequest) models.WorkspaceFieldUpdate {
	update := models.WorkspaceFieldUpdate{
		Name: req.Name, Description: req.Description,
		ACPIdleSuspensionEnabled: req.ACPIdleSuspensionEnabled,
		ACPIdleTimeoutMinutes:    req.ACPIdleTimeoutMinutes,
	}
	update.DefaultExecutorID = workspaceDefaultUpdate(req.DefaultExecutorID)
	update.DefaultEnvironmentID = workspaceDefaultUpdate(req.DefaultEnvironmentID)
	update.DefaultAgentProfileID = workspaceDefaultUpdate(req.DefaultAgentProfileID)
	update.DefaultConfigAgentProfileID = workspaceDefaultUpdate(req.DefaultConfigAgentProfileID)
	return update
}

func workspaceDefaultUpdate(value *string) **string {
	if value == nil {
		return nil
	}
	normalized := normalizeOptionalID(value)
	return &normalized
}
