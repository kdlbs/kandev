package models

// TaskCoverage describes the exact active collection represented by a workflow read.
type TaskCoverage struct {
	WorkspaceID     string `json:"workspace_id"`
	WorkflowID      string `json:"workflow_id"`
	Membership      string `json:"membership"`
	Total           int    `json:"total"`
	Complete        bool   `json:"complete"`
	OrderingProfile string `json:"ordering_profile"`
}

// TaskWorkflowCoverage includes hidden, omitted, and unassigned task scopes.
type TaskWorkflowCoverage struct {
	WorkspaceID string   `json:"workspace_id"`
	WorkflowIDs []string `json:"workflow_ids"`
	Complete    bool     `json:"complete"`
}
