package worktree

import "time"

// Worktree represents a Git worktree associated with a task.
type Worktree struct {
	// ID is the unique identifier for this worktree record.
	ID string `json:"id"`

	// TaskID is the ID of the task this worktree is associated with.
	// This is a 1:1 relationship - each task can have at most one worktree.
	TaskID string `json:"task_id"`

	// RepositoryID is the ID of the repository this worktree belongs to.
	RepositoryID string `json:"repository_id"`

	// RepositoryPath is the local filesystem path to the main repository.
	// Stored for recreation if the worktree directory is lost.
	RepositoryPath string `json:"repository_path"`

	// Path is the absolute filesystem path to the worktree directory.
	Path string `json:"path"`

	// Branch is the Git branch name checked out in this worktree.
	Branch string `json:"branch"`

	// BaseBranch is the branch this worktree was created from.
	BaseBranch string `json:"base_branch"`

	// Status indicates the current state of the worktree.
	// Valid values: active, merged, deleted
	Status string `json:"status"`

	// CreatedAt is when this worktree was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this worktree was last modified.
	UpdatedAt time.Time `json:"updated_at"`

	// MergedAt is when this worktree's branch was merged (if applicable).
	MergedAt *time.Time `json:"merged_at,omitempty"`

	// DeletedAt is when this worktree was deleted (if applicable).
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// CreateRequest contains the parameters for creating a new worktree.
type CreateRequest struct {
	// TaskID is the unique task identifier (required).
	TaskID string

	// RepositoryID is the repository identifier (required).
	RepositoryID string

	// RepositoryPath is the local path to the main repository (required).
	RepositoryPath string

	// BaseBranch is the branch to base the worktree on (required).
	// Typically "main" or "master".
	BaseBranch string

	// BranchName is the name for the new branch (optional).
	// If empty, defaults to "{prefix}{task_id}".
	BranchName string
}

// Validate validates the create request.
func (r *CreateRequest) Validate() error {
	if r.TaskID == "" {
		return ErrWorktreeNotFound
	}
	if r.RepositoryPath == "" {
		return ErrRepoNotGit
	}
	if r.BaseBranch == "" {
		return ErrInvalidBaseBranch
	}
	return nil
}

// MergeRequest contains the parameters for merging a worktree's branch.
type MergeRequest struct {
	// TaskID identifies the worktree to merge.
	TaskID string

	// Method is the merge method: "merge", "squash", or "rebase".
	Method string

	// CleanupAfter indicates whether to delete the worktree after merging.
	CleanupAfter bool
}

// StatusActive is the status for an active, usable worktree.
const StatusActive = "active"

// StatusMerged is the status for a worktree whose branch has been merged.
const StatusMerged = "merged"

// StatusDeleted is the status for a deleted worktree.
const StatusDeleted = "deleted"

