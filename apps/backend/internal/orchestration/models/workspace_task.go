package models

// WorkspaceTaskSpec selects existing Kanban resources for delegated work.
// WorkspaceID is supplied by the authenticated runtime, never by the agent payload.
type WorkspaceTaskSpec struct {
	DelegationReference
	DirectProfile  bool
	WorkspaceID    string
	WorkflowID     string
	WorkflowStepID string
	ExecutionMode  string
	RepositoryID   string
	ParentID       string
	ChiefID        string
	AssigneeID     string
	Title          string
	Description    string
	ExternalID     string
}

// WorkspaceTaskCommand manages an existing delivery task under a signed workspace.
type WorkspaceTaskCommand struct {
	OperationRequest
	DelegationReference
	DirectProfile bool   `json:"-"`
	SessionID     string `json:"session_id"`
	Prompt        string `json:"prompt"`
	WorkspaceID   string
	ChiefID       string
	TaskID        string
	Action        string `json:"action"`
	AssigneeID    string `json:"assignee"`
}

// DelegationReference follows an objective without changing the assigned profile
// or treating stored context as permission to act.
type DelegationReference struct {
	Packet              *ContextPacket `json:"-"`
	ObjectiveID         string         `json:"objective_id,omitempty"`
	ContextRef          string         `json:"context_ref,omitempty"`
	AcceptanceRevision  int64          `json:"acceptance_revision,omitempty"`
	SourceCommentID     string         `json:"source_comment_id,omitempty"`
	Acceptance          []Criterion    `json:"-"`
	DispatchOperationID string         `json:"-"`
}
