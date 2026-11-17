package models

import (
	p "github.com/kandev/kandev/internal/mcp/profile"
	"time"
)

// ProviderMCPBindingSnapshot is immutable provider authority and resume evidence.
// Credentials stay in the helper's protected receipt journal.
type ProviderMCPBindingSnapshot struct {
	BindingID       string     `db:"binding_id"`
	SessionID       string     `db:"session_id"`
	TaskID          string     `db:"task_id"`
	WorkspaceID     string     `db:"workspace_id"`
	ExecutionID     string     `db:"execution_id"`
	OperationID     string     `db:"operation_id"`
	BindingJSON     string     `db:"binding_json"`
	CatalogJSON     string     `db:"catalog_json"`
	TargetRevision  int64      `db:"target_revision"`
	PolicyRevision  int64      `db:"policy_revision"`
	NativeSessionID string     `db:"native_session_id"`
	CreatedAt       time.Time  `db:"created_at"`
	AcceptedAt      *time.Time `db:"accepted_at"`
}

// BuiltinMCPAcceptance carries verified helper evidence into the SQL CAS.
type BuiltinMCPAcceptance struct {
	SessionID             string
	OperationID           string
	RowVersion            int64
	Binding               p.ProviderMCPBinding
	Catalog               map[string][]string
	NativeSessionID       string
	NativeResumeConfirmed bool
}
