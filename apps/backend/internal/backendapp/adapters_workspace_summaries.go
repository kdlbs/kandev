package backendapp

import (
	"sort"

	"github.com/kandev/kandev/internal/task/models"
)

func workspaceTaskDetailSummary(task *models.Task) map[string]any {
	return map[string]any{
		"id": task.ID, "workspace_id": task.WorkspaceID, "title": workspaceExportText(task.Title, 200),
		"description": workspaceExportText(task.Description, 1600), "description_truncated": len(task.Description) > 1600,
		"state": task.State, "workflow_id": task.WorkflowID, "workflow_step_id": task.WorkflowStepID,
		"parent_id": task.ParentID, "priority": task.Priority,
	}
}

func workspaceSessionSummaries(sessions []*models.TaskSession) []map[string]any {
	ordered := append([]*models.TaskSession(nil), sessions...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].UpdatedAt.After(ordered[j].UpdatedAt) })
	if len(ordered) > 8 {
		ordered = ordered[:8]
	}
	rows := make([]map[string]any, 0, len(ordered))
	for _, session := range ordered {
		mode, _ := session.Metadata[models.SessionMetaKeySessionMode].(string)
		rows = append(rows, map[string]any{
			"id": session.ID, "agent_profile_id": session.AgentProfileID, "state": session.State,
			"error_message": workspaceExportText(session.ErrorMessage, 500), "session_mode": workspaceExportText(mode, 80),
			"review_status": session.ReviewStatus, "updated_at": session.UpdatedAt,
		})
	}
	return rows
}
