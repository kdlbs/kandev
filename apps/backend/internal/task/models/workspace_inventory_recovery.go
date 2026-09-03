package models

import (
	"errors"
	"time"
)

const (
	WorkspaceInventoryRecoveryRepaired     = "repaired"
	WorkspaceInventoryRecoveryDeduplicated = "deduplicated"
)

var (
	ErrWorkspaceInventoryRecoveryInvalid             = errors.New("workspace inventory recovery request is invalid")
	ErrWorkspaceInventoryRecoveryConflict            = errors.New("workspace inventory recovery conflicts with current state")
	ErrWorkspaceInventoryRecoveryIdempotencyConflict = errors.New("workspace inventory recovery idempotency conflict")
)

// WorkspaceInventoryPreservation records non-secret evidence captured before
// and verified after an inventory-only repair.
type WorkspaceInventoryPreservation struct {
	ExpectedBranchSlug string `json:"expected_branch_slug"`
	ObservedBranch     string `json:"observed_branch"`
	RefName            string `json:"ref_name"`
	HeadOID            string `json:"head_oid"`
	WorktreeID         string `json:"worktree_id"`
	PathHash           string `json:"path_hash"`
	StatusHash         string `json:"status_hash"`
	ContentHash        string `json:"content_hash"`
	DirtyCount         int    `json:"dirty_count"`
	UntrackedCount     int    `json:"untracked_count"`
	RuntimeState       string `json:"runtime_state"`
}

// WorkspaceInventoryRepair is derived only from server-owned task lifecycle
// records. Transport callers do not provide its identity fields.
type WorkspaceInventoryRepair struct {
	TaskID                        string
	WorkspaceID                   string
	SessionID                     string
	TaskEnvironmentID             string
	TaskRepositoryID              string
	RepositoryID                  string
	EnvironmentRepoID             string
	ExpectedEnvironmentUpdatedAt  time.Time
	ExpectedTaskRepositoryUpdate  time.Time
	ExpectedEnvironmentRepoUpdate time.Time
	BranchSlug                    string
	WorktreeID                    string
	WorktreePath                  string
	WorktreeBranch                string
	Position                      int
	IdempotencyKey                string
	RequestHash                   string
	Preservation                  WorkspaceInventoryPreservation
}

// WorkspaceInventoryRecoveryReceipt is the append-only audit result returned
// without exposing a host checkout path.
type WorkspaceInventoryRecoveryReceipt struct {
	ID                   string                          `json:"id"`
	TaskID               string                          `json:"task_id"`
	WorkspaceID          string                          `json:"workspace_id"`
	SessionID            string                          `json:"session_id"`
	TaskEnvironmentID    string                          `json:"task_environment_id"`
	TaskRepositoryID     string                          `json:"task_repository_id"`
	EnvironmentRepoID    string                          `json:"environment_repo_id"`
	RepositoryID         string                          `json:"repository_id"`
	IdempotencyKey       string                          `json:"idempotency_key"`
	RequestHash          string                          `json:"-"`
	ResultCode           string                          `json:"result_code"`
	Preservation         WorkspaceInventoryPreservation  `json:"preservation"`
	PostRepairEvidence   *WorkspaceInventoryPreservation `json:"post_repair_evidence,omitempty"`
	PostRepairMatched    bool                            `json:"post_repair_matched"`
	PostRepairVerifiedAt *time.Time                      `json:"post_repair_verified_at,omitempty"`
	CreatedAt            time.Time                       `json:"created_at"`
}
