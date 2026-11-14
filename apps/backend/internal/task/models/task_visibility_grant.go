package models

import "encoding/json"

// SessionMetaKeyTaskVisibilityGrant is a backend-owned, session-local grant.
// It is never inferred from a task title, agent profile, or MCP argument.
const SessionMetaKeyTaskVisibilityGrant = "task_visibility_grant"

// TaskVisibilityGrant binds one manual task session to its current workspace.
// The grant is stored only through a trusted backend session metadata write.
type TaskVisibilityGrant struct {
	Version     int    `json:"version"`
	TaskID      string `json:"task_id"`
	SessionID   string `json:"session_id"`
	WorkspaceID string `json:"workspace_id"`
}

func HasTaskVisibilityGrant(task *Task, session *TaskSession) bool {
	if task == nil || session == nil || task.Origin != TaskOriginManual ||
		task.ID == "" || task.WorkspaceID == "" || session.ID == "" ||
		session.TaskID != task.ID {
		return false
	}
	grant, ok := taskVisibilityGrantFromMetadata(session.Metadata)
	return ok && grant.Version == 1 && grant.TaskID == task.ID &&
		grant.SessionID == session.ID && grant.WorkspaceID == task.WorkspaceID
}

func taskVisibilityGrantFromMetadata(metadata map[string]interface{}) (TaskVisibilityGrant, bool) {
	if metadata == nil {
		return TaskVisibilityGrant{}, false
	}
	raw, present := metadata[SessionMetaKeyTaskVisibilityGrant]
	if !present || raw == nil {
		return TaskVisibilityGrant{}, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return TaskVisibilityGrant{}, false
	}
	var grant TaskVisibilityGrant
	if err := json.Unmarshal(encoded, &grant); err != nil {
		return TaskVisibilityGrant{}, false
	}
	return grant, true
}
