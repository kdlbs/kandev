package models

import "time"

// WorkspaceGroup represents a materialized workspace shared by one or more
// tasks. Cleanup defaults are intentionally safe: a freshly-inserted row has
// owned_by_kandev=false and cleanup_policy=never_delete. The materializer
// that actually creates the workspace on disk is the only code path that
// flips those flags via MarkWorkspaceMaterialized.
type WorkspaceGroup struct {
	ID                        string     `json:"id" db:"id"`
	WorkspaceID               string     `json:"workspace_id" db:"workspace_id"`
	OwnerTaskID               string     `json:"owner_task_id" db:"owner_task_id"`
	MaterializedPath          string     `json:"materialized_path,omitempty" db:"materialized_path"`
	MaterializedEnvironmentID string     `json:"materialized_environment_id,omitempty" db:"materialized_environment_id"`
	MaterializedKind          string     `json:"materialized_kind" db:"materialized_kind"`
	OwnedByKandev             bool       `json:"owned_by_kandev" db:"owned_by_kandev"`
	CleanupPolicy             string     `json:"cleanup_policy" db:"cleanup_policy"`
	CleanupStatus             string     `json:"cleanup_status" db:"cleanup_status"`
	CleanedAt                 *time.Time `json:"cleaned_at,omitempty" db:"cleaned_at"`
	CleanupError              string     `json:"cleanup_error,omitempty" db:"cleanup_error"`
	RestoreStatus             string     `json:"restore_status" db:"restore_status"`
	RestoreError              string     `json:"restore_error,omitempty" db:"restore_error"`
	RestoreConfigJSON         string     `json:"restore_config_json,omitempty" db:"restore_config_json"`
	CreatedAt                 time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at" db:"updated_at"`
}

// WorkspaceGroupMember associates a task with a WorkspaceGroup. Released
// memberships keep the row for audit history; ReleasedByCascadeID lets a
// later unarchive scope its restoration to exactly the cascade that
// released the membership.
type WorkspaceGroupMember struct {
	WorkspaceGroupID    string     `json:"workspace_group_id" db:"workspace_group_id"`
	TaskID              string     `json:"task_id" db:"task_id"`
	Role                string     `json:"role" db:"role"`
	ReleasedAt          *time.Time `json:"released_at,omitempty" db:"released_at"`
	ReleaseReason       string     `json:"release_reason,omitempty" db:"release_reason"`
	ReleasedByCascadeID string     `json:"released_by_cascade_id,omitempty" db:"released_by_cascade_id"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
}

// MaterializedWorkspace is the value passed to MarkWorkspaceMaterialized
// when the materializer flips ownership. Path/EnvironmentID are recorded
// verbatim; OwnedByKandev=true is the trigger that switches CleanupPolicy
// from never_delete to delete_when_last_member_archived_or_deleted (the
// repo handles that mapping atomically).
type MaterializedWorkspace struct {
	Path          string
	EnvironmentID string
	Kind          string
	OwnedByKandev bool
	RestoreConfig string
}

// Materialized kinds.
const (
	WorkspaceGroupKindPlainFolder       = "plain_folder"
	WorkspaceGroupKindSingleRepo        = "single_repo"
	WorkspaceGroupKindMultiRepo         = "multi_repo"
	WorkspaceGroupKindRemoteEnvironment = "remote_environment"
)

// Cleanup policy values.
const (
	WorkspaceCleanupPolicyNeverDelete                       = "never_delete"
	WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel = "delete_when_last_member_archived_or_deleted"
)

// Cleanup status values.
const (
	WorkspaceCleanupStatusActive  = "active"
	WorkspaceCleanupStatusPending = "cleanup_pending"
	WorkspaceCleanupStatusCleaned = "cleaned"
	WorkspaceCleanupStatusFailed  = "cleanup_failed"
)

// Restore status values.
const (
	WorkspaceRestoreStatusNotNeeded  = "not_needed"
	WorkspaceRestoreStatusRestorable = "restorable"
	WorkspaceRestoreStatusPending    = "restore_pending"
	WorkspaceRestoreStatusRestored   = "restored"
	WorkspaceRestoreStatusFailed     = "restore_failed"
)

// Workspace group member role values.
const (
	WorkspaceMemberRoleOwner  = "owner"
	WorkspaceMemberRoleMember = "member"
)

// Release reason values used when a membership is released.
const (
	WorkspaceReleaseReasonArchived = "archived"
	WorkspaceReleaseReasonDeleted  = "deleted"
)
