package models

import "time"

// CoordinatorPrincipal is the durable, opaque subject approved by an
// operator. Backing task/session bindings are replaceable implementation
// details and never the principal's identity.
type WorkspaceAgentPrincipal struct {
	ID                   string     `json:"id"`
	WorkspaceID          string     `json:"workspace_id"`
	PluginInstallationID string     `json:"plugin_installation_id"`
	LogicalKey           string     `json:"logical_key"`
	BackingTaskID        string     `json:"backing_task_id"`
	BackingSessionID     string     `json:"backing_session_id"`
	RevokedAt            *time.Time `json:"revoked_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// CoordinatorGrant is an explicit operator grant for a task. Capabilities are
// persisted as a normalized comma-separated list; only the coordinator
// authority package interprets their values.
type CoordinatorGrant struct {
	ID                string `json:"id"`
	CoordinatorTaskID string `json:"coordinator_task_id"`
	// PrincipalID is the durable subject for new grants. A non-empty legacy
	// CoordinatorTaskID with empty PrincipalID is non-transferable and requires
	// explicit operator re-consent before principal migration.
	PrincipalID     string     `json:"principal_id,omitempty"`
	WorkspaceID     string     `json:"workspace_id"`
	ScopeKind       string     `json:"scope_kind"`
	ScopeID         string     `json:"scope_id"`
	Capabilities    string     `json:"capabilities"`
	Note            string     `json:"note"`
	GrantedByUserID string     `json:"granted_by_user_id"`
	GrantedAt       time.Time  `json:"granted_at"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	RevokedByUserID string     `json:"revoked_by_user_id,omitempty"`
}

// CoordinatorAuditEvent records a privileged coordination attempt. Detail is
// intentionally restricted to a reason code, never task content or paths.
type CoordinatorAuditEvent struct {
	ID         string    `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
	// PrincipalID is the durable subject. Actor task/session identify only the
	// replaceable execution resource that performed this attempt.
	PrincipalID    string `json:"principal_id"`
	ActorTaskID    string `json:"actor_task_id"`
	ActorSessionID string `json:"actor_session_id"`
	TargetTaskID   string `json:"target_task_id"`
	WorkspaceID    string `json:"workspace_id"`
	Action         string `json:"action"`
	Capability     string `json:"capability"`
	Decision       string `json:"decision"`
	DenyReason     string `json:"deny_reason"`
	GrantID        string `json:"grant_id"`
	Result         string `json:"result"`
	Detail         string `json:"detail"`
}
