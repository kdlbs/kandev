package models

import (
	"fmt"
	"slices"
	"time"
)

type WorkspaceGrantScope struct {
	Operations     []string `json:"operations"`
	ContextExports []string `json:"context_exports"`
}

type WorkspaceGrantOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type WorkspaceGrantReceiver struct {
	ProfileID         string `json:"profile_id"`
	ProfileName       string `json:"profile_name"`
	ProfileRevision   string `json:"profile_revision"`
	AuthorityRevision string `json:"authority_revision"`
}
type WorkspaceGrantView struct {
	WorkspaceGrant
	WorkspaceName string `json:"workspace_name"`
	Active        bool   `json:"active"`
	Reason        string `json:"reason,omitempty"`
}

type WorkspaceGrant struct {
	ID                      string              `json:"id" db:"id"`
	BindingID               string              `json:"binding_id" db:"binding_id"`
	OwnerUserID             string              `json:"owner_user_id" db:"owner_user_id"`
	WorkspaceID             string              `json:"workspace_id" db:"workspace_id"`
	BindingVersion          int64               `json:"binding_version" db:"binding_version"`
	Revision                int64               `json:"revision" db:"revision"`
	ReceiverProfileID       string              `json:"receiver_profile_id" db:"receiver_profile_id"`
	ReceiverProfileRevision string              `json:"receiver_profile_revision" db:"receiver_profile_revision"`
	AuthorityRevision       string              `json:"authority_revision" db:"authority_revision"`
	Scope                   WorkspaceGrantScope `json:"scope" db:"-"`
	ScopeJSON               string              `json:"-" db:"scope_json"`
	CreatedAt               time.Time           `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time           `json:"updated_at" db:"updated_at"`
	RevokedAt               *time.Time          `json:"revoked_at,omitempty" db:"revoked_at"`
}

func (s WorkspaceGrantScope) Validate() error {
	if !slices.Contains(s.Operations, "observe") || !workspaceGrantValues(s.Operations, []string{"observe", "coordinate"}) {
		return fmt.Errorf("workspace grant requires explicit observation and optional coordination")
	}
	if !workspaceGrantValues(s.ContextExports, []string{"directory", "task_summary", "task_result", "task_input", "handoff"}) {
		return fmt.Errorf("workspace grant requires explicit bounded context fields")
	}
	if (slices.Contains(s.ContextExports, "task_result") || slices.Contains(s.ContextExports, "task_input")) && !slices.Contains(s.ContextExports, "task_summary") {
		return fmt.Errorf("task details require task summary export")
	}
	return nil
}

func workspaceGrantValues(values, allowed []string) bool {
	if len(values) == 0 || len(values) > len(allowed) {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !slices.Contains(allowed, value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

type WorkspaceGrantEvent struct {
	ID           string         `json:"id" db:"id"`
	GrantID      string         `json:"grant_id" db:"grant_id"`
	Revision     int64          `json:"revision" db:"revision"`
	Action       string         `json:"action" db:"action"`
	SnapshotJSON string         `json:"-" db:"snapshot_json"`
	Snapshot     WorkspaceGrant `json:"snapshot" db:"-"`
	CreatedAt    time.Time      `json:"created_at" db:"created_at"`
}

// Export receipts retain the account boundary even after grant revocation or
// cache forgetting. They contain no task bodies, prompts or credential values.
type WorkspaceExport struct {
	ID                      string    `json:"id" db:"id"`
	BindingID               string    `json:"binding_id" db:"binding_id"`
	OwnerUserID             string    `json:"owner_user_id" db:"owner_user_id"`
	ConversationID          string    `json:"conversation_id" db:"conversation_id"`
	WorkspaceID             string    `json:"workspace_id" db:"workspace_id"`
	GrantID                 string    `json:"grant_id" db:"grant_id"`
	GrantRevision           int64     `json:"grant_revision" db:"grant_revision"`
	ReceiverProfileID       string    `json:"receiver_profile_id" db:"receiver_profile_id"`
	ReceiverProfileRevision string    `json:"receiver_profile_revision" db:"receiver_profile_revision"`
	AuthorityRevision       string    `json:"authority_revision" db:"authority_revision"`
	Kind                    string    `json:"kind" db:"kind"`
	CreatedAt               time.Time `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time `json:"updated_at" db:"updated_at"`
}
