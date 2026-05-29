package worktree

import "time"

// SyncProgressStatus represents the status of a base-branch sync progress event.
type SyncProgressStatus string

const (
	SyncProgressRunning   SyncProgressStatus = "running"
	SyncProgressCompleted SyncProgressStatus = "completed"
)

// SyncProgressEvent reports pre-worktree base-branch synchronization progress.
type SyncProgressEvent struct {
	StepName string
	Status   SyncProgressStatus
	Output   string
	Error    string
}

// SyncProgressCallback is called when base-branch sync status changes.
type SyncProgressCallback func(event SyncProgressEvent)

// Worktree represents a Git worktree associated with a task.
type Worktree struct {
	// ID is the unique identifier for this worktree record.
	ID string `json:"id"`

	// SessionID is the task session associated with this worktree.
	SessionID string `json:"session_id,omitempty"`

	// TaskID is the ID of the task this worktree is associated with.
	// Multiple worktrees can exist for the same task (one per agent session).
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

	// FetchWarning is a non-fatal warning from fetching the checkout branch.
	// Set when fetch from origin failed but a local branch was used as fallback.
	FetchWarning string `json:"fetch_warning,omitempty"`

	// FetchWarningDetail contains the raw git command output for debugging.
	// Shown as collapsible content alongside the user-friendly FetchWarning.
	FetchWarningDetail string `json:"fetch_warning_detail,omitempty"`

	// BaseBranchFallbackWarning is set when the requested BaseBranch did not
	// exist in the repository and the worktree was created from a fallback
	// branch (typically the repository's default_branch) instead. Empty when
	// the original BaseBranch was used.
	BaseBranchFallbackWarning string `json:"base_branch_fallback_warning,omitempty"`

	// BaseBranchFallbackDetail mirrors FetchWarningDetail: a longer message
	// describing why the original branch was not used. Surfaced as collapsible
	// detail alongside BaseBranchFallbackWarning.
	BaseBranchFallbackDetail string `json:"base_branch_fallback_detail,omitempty"`

	// CopiedFiles lists the relative paths of files copied from the source
	// repo into this worktree per the repository's CopyFiles spec. Populated
	// only on the in-memory record returned by Create — not persisted, since
	// it describes a one-shot launch action rather than worktree state. The
	// env preparer reads it to surface a "Copy ignored files" prepare step.
	CopiedFiles []string `json:"-"`

	// CopyFilesWarnings lists non-fatal warnings emitted while copying files
	// (e.g. missing patterns, traversal-rejected paths). Like CopiedFiles,
	// this is in-memory only and read by the env preparer.
	CopyFilesWarnings []string `json:"-"`
}

// CreateRequest contains the parameters for creating a new worktree.
type CreateRequest struct {
	// TaskID is the unique task identifier (required).
	TaskID string

	// SessionID is the task session identifier (required for persistence).
	SessionID string

	// TaskTitle is the human-readable task title (optional).
	// If provided, it will be used to generate semantic worktree/branch names.
	// The title is sanitized and truncated to 20 characters.
	TaskTitle string

	// RepositoryID is the repository identifier (required).
	RepositoryID string

	// RepositoryPath is the local path to the main repository (required).
	RepositoryPath string

	// BaseBranch is the branch to base the worktree on (required).
	// Typically "main" or "master".
	BaseBranch string

	// FallbackBaseBranch is an optional branch to retry with when BaseBranch
	// does not exist in the repository. Typically populated with the
	// repository's default_branch by the caller. When empty, a missing
	// BaseBranch returns ErrInvalidBaseBranch as before.
	FallbackBaseBranch string

	// CheckoutBranch is a branch to fetch from origin and check out directly in the
	// worktree. If the branch is already checked out in another worktree, a unique
	// fallback branch is created using the original name with a random suffix.
	CheckoutBranch string

	// PRNumber, when > 0, identifies the GitHub PR whose head ref should be
	// fetched into the local CheckoutBranch via the refs/pull/<N>/head refspec.
	// Required for fork PRs, whose branches only exist on the contributor's
	// fork — git fetch origin <branch> fails because origin (the base repo)
	// has no such ref. GitHub mirrors every PR head under refs/pull/<N>/head
	// on the base repo, so the refspec form works for same-repo and fork PRs
	// uniformly without needing to add the fork as a remote.
	PRNumber int

	// WorktreeBranchPrefix is the prefix to use for the worktree branch name.
	// If empty, the default prefix is used.
	WorktreeBranchPrefix string

	// PullBeforeWorktree indicates whether to pull from remote before creating the worktree.
	PullBeforeWorktree bool

	// WorktreeID is the ID of an existing worktree to reuse (optional).
	// If provided and valid, the existing worktree is returned instead of creating a new one.
	WorktreeID string

	// TaskDirName is the semantic directory name for the task (e.g. "fix-bug_ab12").
	// When set together with RepoName, the worktree is placed at
	// ~/.kandev/tasks/{TaskDirName}/{RepoName}/ instead of ~/.kandev/worktrees/.
	TaskDirName string

	// RepoName is the repository name used as subdirectory inside the task directory.
	// Only used when TaskDirName is also set.
	RepoName string

	// OnSyncProgress receives progress updates for pre-worktree branch sync.
	OnSyncProgress SyncProgressCallback

	// OnWorktreeCreated, when set, is invoked once the worktree directory has
	// been created and persisted (git worktree add succeeded) but BEFORE the
	// per-repo setup script runs. The env preparer uses it to complete the
	// "Create worktree" UI step so the setup script renders as a distinct,
	// subsequent step instead of overlapping it. The passed worktree already
	// carries any base-branch fallback warning. Called synchronously on the
	// Create goroutine; not invoked when an existing worktree is reused.
	OnWorktreeCreated func(*Worktree)
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
