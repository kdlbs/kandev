package sandbox

import "context"

const (
	ScopeExecutor     = "executor"
	ScopeAgent        = "agent"
	ScopeWorkflowStep = "workflow_step"
	ScopeTask         = "task"
	ModeDisabled      = "disabled"
	ModeRequired      = "required"
)

type ExecutorPolicy struct {
	Mode                string   `json:"mode"`
	InstallationID      string   `json:"installation_id"`
	ProviderKey         string   `json:"provider_key"`
	CeilingRevisionID   string   `json:"ceiling_revision_id"`
	EligibleRevisionIDs []string `json:"eligible_revision_ids"`
	DefaultRevisionID   string   `json:"default_revision_id"`
}

type Binding struct {
	Scope       string          `json:"scope"`
	TargetID    string          `json:"target_id"`
	WorkspaceID string          `json:"workspace_id"`
	Version     int64           `json:"version"`
	RevisionID  string          `json:"revision_id"`
	Executor    *ExecutorPolicy `json:"executor,omitempty"`
}

// SelectionContext contains IDs derived from the persisted launch context.
type SelectionContext struct {
	OrgID          string
	WorkspaceID    string
	ExecutorID     string
	AgentProfileID string
	WorkflowStepID string
	TaskID         string
	ParentTaskID   string
	Prior          *Resolved
}

type CapturedBinding struct {
	Scope    string `json:"scope"`
	TargetID string `json:"target_id"`
	Version  int64  `json:"version"`
}

type Resolved struct {
	Bindings        []CapturedBinding `json:"bindings"`
	Revision        *Revision         `json:"revision"`
	Ceiling         *Revision         `json:"ceiling"`
	SelectionSource string            `json:"selection_source"`
	ExecutorID      string            `json:"executor_id"`
	BindingVersion  int64             `json:"binding_version"`
}

type TargetScope struct {
	OrgID       string
	WorkspaceID string
}

// SetTargetResolver binds target existence and access checks at startup.
func (s *Service) SetTargetResolver(resolve func(context.Context, string, string) (TargetScope, error)) {
	s.resolveTarget = resolve
}
