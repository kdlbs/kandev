package models

type WorkspaceDirectoryEntry struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	WorkflowID   string `json:"workflow_id,omitempty"`
	EntryAllowed bool   `json:"entry_allowed,omitempty"`
}

type WorkspaceTaskSummary struct {
	ID             string `json:"id"`
	WorkspaceID    string `json:"workspace_id"`
	Title          string `json:"title"`
	State          string `json:"state"`
	WorkflowID     string `json:"workflow_id"`
	WorkflowStepID string `json:"workflow_step_id"`
}

type WorkspaceTaskResult struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	ProfileID string `json:"profile_id"`
	State     string `json:"state"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

type WorkspaceTaskView struct {
	Task    WorkspaceTaskSummary  `json:"task"`
	Results []WorkspaceTaskResult `json:"results,omitempty"`
	HasMore bool                  `json:"has_more"`
}
