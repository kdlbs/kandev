package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
)

// RepositoryPreparationError identifies the repository whose preparation
// prevented a multi-repository launch. Its wrapped cause remains available for
// errors.Is/errors.As, while Error() exposes only bounded, credential-safe
// detail to launch and task error surfaces.
type RepositoryPreparationError struct {
	RepositoryID     string
	TaskRepositoryID string
	RepositoryName   string
	Cause            error
}

func (e *RepositoryPreparationError) Error() string {
	if e == nil {
		return "repository preparation failed"
	}
	name := strings.TrimSpace(e.RepositoryName)
	identity := strings.TrimSpace(e.RepositoryID)
	label := identity
	if name != "" && identity != "" {
		label = fmt.Sprintf("%s (%s)", name, identity)
	} else if name != "" {
		label = name
	}
	if label == "" {
		label = "unknown repository"
	}
	if e.Cause == nil {
		return fmt.Sprintf("repository %s preparation failed", label)
	}
	return fmt.Sprintf("repository %s preparation failed: %s", label, routingerr.Sanitize(e.Cause.Error()))
}

func (e *RepositoryPreparationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// PrepareStepStatus represents the status of a preparation step.
type PrepareStepStatus string

const (
	PrepareStepPending   PrepareStepStatus = "pending"
	PrepareStepRunning   PrepareStepStatus = "running"
	PrepareStepCompleted PrepareStepStatus = "completed"
	PrepareStepFailed    PrepareStepStatus = "failed"
	PrepareStepSkipped   PrepareStepStatus = "skipped"
)

// RepoPrepareSpec describes one repository for multi-repo environment preparation.
// Mirrors the per-repo prepare fields that EnvPrepareRequest historically
// carried at the top level. When EnvPrepareRequest.Repositories is non-empty,
// each entry produces one prepared worktree under the shared TaskDirName.
type RepoPrepareSpec struct {
	TaskRepositoryID   string
	RepositoryID       string
	RepositoryPath     string
	RepoName           string
	BaseBranch         string
	DefaultBranch      string // Repository's default_branch, used as fallback when BaseBranch is missing
	CheckoutBranch     string
	PRNumber           int // GitHub PR number when CheckoutBranch is a PR head; enables refs/pull/<N>/head fetch for fork PRs.
	RemoteContribution *models.RemoteContribution
	WorktreeID         string
	// WorkspaceReuseRequired makes preparation attach to the exact canonical
	// environment. It forbids worktree creation/recreation and all repository
	// mutating setup paths.
	WorkspaceReuseRequired bool
	WorktreeBranch         string
	WorktreeBranchPrefix   string
	WorktreeBranchTemplate string
	WorktreeBranchTicket   string
	PullBeforeWorktree     bool
	RemoteSyncHandled      bool
	// RefreshRepository is an optional provider-authenticated refresh deferred
	// until worktree materialization. A valid reusable worktree bypasses it.
	RefreshRepository          func(context.Context) error
	RefreshRepositoryWithState func(context.Context) (repoclone.RemoteRefState, error)
	RemoteRefState             repoclone.RemoteRefState
	RepoSetupScript            string
	ContributionDestination    *models.ContributionDestination
	// BranchSlug, when set, suffixes the worktree path as
	// {RepoName}-{BranchSlug} so two specs sharing a RepositoryID don't collide
	// on disk.
	BranchSlug string
	// BranchIdentitySlug is the stable branch key used for worktree reuse and
	// persisted environment metadata. It may be non-empty even when BranchSlug
	// is empty so primary branches can keep the flat legacy path.
	BranchIdentitySlug string
}

// EnvPrepareRequest contains the parameters for environment preparation.
type EnvPrepareRequest struct {
	TaskID                  string
	WorkspaceID             string
	SessionID               string
	TaskEnvironmentID       string
	TaskTitle               string
	ExecutionID             string
	ExecutorType            executor.Name
	WorkspacePath           string
	RepositoryPath          string
	RepositoryID            string
	TaskRepositoryID        string
	UseWorktree             bool
	SetupScript             string
	RepoSetupScript         string // Repository-level setup script (e.g. "make install")
	BaseBranch              string
	DefaultBranch           string // Repository's default_branch, used as fallback when BaseBranch is missing
	CheckoutBranch          string
	PRNumber                int // GitHub PR number when CheckoutBranch is a PR head; enables refs/pull/<N>/head fetch for fork PRs.
	RemoteContribution      *models.RemoteContribution
	ContributionDestination *models.ContributionDestination
	WorktreeID              string
	WorktreeBranch          string
	// WorkspaceReuseRequired applies attach-only semantics to every repository
	// in this request. RepoPrepareSpec carries the same value so multi-repo
	// callers can retain it while requests are split.
	WorkspaceReuseRequired bool

	WorktreeBranchPrefix   string
	WorktreeBranchTemplate string
	WorktreeBranchTicket   string
	PullBeforeWorktree     bool
	RemoteSyncHandled      bool
	// RefreshRepository is an optional provider-authenticated refresh deferred
	// until worktree materialization. A valid reusable worktree bypasses it.
	RefreshRepository          func(context.Context) error
	RefreshRepositoryWithState func(context.Context) (repoclone.RemoteRefState, error)
	RemoteRefState             repoclone.RemoteRefState

	TaskDirName string // Per-task directory name within the workspace (e.g. "task-abc123")
	RepoName    string // Repository slug used with TaskDirName to locate checkouts
	BranchSlug  string // Optional branch directory suffix for multi-branch tasks
	// BranchIdentitySlug is the stable branch key for worktree cache/persistence.
	// It may be non-empty when BranchSlug is empty to preserve a flat path.
	// Empty leaves the synthesized spec identity empty; worktree code falls back.
	BranchIdentitySlug string

	// Repositories carries one entry per repository when the request is
	// multi-repo. When non-empty it is the source of truth; the legacy
	// single-repo top-level fields above are populated from Repositories[0]
	// for callers that have not yet been updated.
	Repositories []RepoPrepareSpec

	Env map[string]string
}

// RepoSpecs returns the per-repo prepare specs for this request. When
// Repositories is set it is returned verbatim; otherwise a length-1 list is
// synthesized from the legacy top-level single-repo fields. Returns an empty
// slice for repo-less requests.
func (r *EnvPrepareRequest) RepoSpecs() []RepoPrepareSpec {
	if len(r.Repositories) > 0 {
		return r.Repositories
	}
	if r.RepositoryID == "" && r.RepositoryPath == "" {
		return nil
	}
	return []RepoPrepareSpec{{
		TaskRepositoryID:           r.TaskRepositoryID,
		RepositoryID:               r.RepositoryID,
		RepositoryPath:             r.RepositoryPath,
		RepoName:                   r.RepoName,
		BaseBranch:                 r.BaseBranch,
		DefaultBranch:              r.DefaultBranch,
		CheckoutBranch:             r.CheckoutBranch,
		PRNumber:                   r.PRNumber,
		RemoteContribution:         r.RemoteContribution,
		WorktreeID:                 r.WorktreeID,
		WorkspaceReuseRequired:     r.WorkspaceReuseRequired,
		WorktreeBranch:             r.WorktreeBranch,
		WorktreeBranchPrefix:       r.WorktreeBranchPrefix,
		WorktreeBranchTemplate:     r.WorktreeBranchTemplate,
		WorktreeBranchTicket:       r.WorktreeBranchTicket,
		PullBeforeWorktree:         r.PullBeforeWorktree,
		RemoteSyncHandled:          r.RemoteSyncHandled,
		RefreshRepository:          r.RefreshRepository,
		RefreshRepositoryWithState: r.RefreshRepositoryWithState,
		RemoteRefState:             r.RemoteRefState,
		RepoSetupScript:            r.RepoSetupScript,
		BranchSlug:                 r.BranchSlug,
		BranchIdentitySlug:         r.BranchIdentitySlug,
		ContributionDestination:    r.ContributionDestination,
	}}
}

// PrepareStep represents a single step in the preparation process.
type PrepareStep struct {
	Name          string            `json:"name"`
	Command       string            `json:"command,omitempty"`
	Status        PrepareStepStatus `json:"status"`
	Output        string            `json:"output,omitempty"`
	Error         string            `json:"error,omitempty"`
	Warning       string            `json:"warning,omitempty"`
	WarningDetail string            `json:"warning_detail,omitempty"`
	StartedAt     *time.Time        `json:"started_at,omitempty"`
	EndedAt       *time.Time        `json:"ended_at,omitempty"`
}

// RepoWorktreeResult is the per-repository outcome of environment preparation.
// Populated by preparers that handle multi-repo launches; each entry corresponds
// to one RepoPrepareSpec from the request.
type RepoWorktreeResult struct {
	TaskRepositoryID          string `json:"task_repository_id,omitempty"`
	RepositoryID              string `json:"repository_id"`
	BranchSlug                string `json:"branch_slug,omitempty"`
	WorktreeID                string `json:"worktree_id,omitempty"`
	WorktreeBranch            string `json:"worktree_branch,omitempty"`
	WorktreePath              string `json:"worktree_path,omitempty"`
	MainRepoGitDir            string `json:"main_repo_git_dir,omitempty"`
	RequestedBaseBranch       string `json:"requested_base_branch,omitempty"`
	BaseBranch                string `json:"base_branch,omitempty"`
	BaseBranchFallbackWarning string `json:"base_branch_fallback_warning,omitempty"`
	ErrorMessage              string `json:"error_message,omitempty"`
}

// EnvPrepareResult contains the result of environment preparation.
type EnvPrepareResult struct {
	Success       bool          `json:"success"`
	Steps         []PrepareStep `json:"steps"`
	WorkspacePath string        `json:"workspace_path,omitempty"`
	ErrorMessage  string        `json:"error_message,omitempty"`
	Duration      time.Duration `json:"duration"`

	// Worktree fields (populated when worktree preparer runs).
	// Legacy single-worktree fields; for multi-repo results they mirror Worktrees[0].
	WorktreeID                string `json:"worktree_id,omitempty"`
	WorktreeBranch            string `json:"worktree_branch,omitempty"`
	MainRepoGitDir            string `json:"main_repo_git_dir,omitempty"`
	RequestedBaseBranch       string `json:"requested_base_branch,omitempty"`
	BaseBranch                string `json:"base_branch,omitempty"`
	BaseBranchFallbackWarning string `json:"base_branch_fallback_warning,omitempty"`

	// Worktrees is the per-repository outcome list when the preparer ran in
	// multi-repo mode. Empty for single-repo or repo-less results.
	Worktrees []RepoWorktreeResult `json:"worktrees,omitempty"`

	// Error carries the underlying typed error chain when Success is false.
	// Populated alongside ErrorMessage so that callers wrapping the result
	// can use %w to preserve errors.Is/errors.As reachability for sentinels
	// like worktree.ErrBranchCheckedOut. ErrorMessage remains the canonical
	// human-readable string; Error is the typed chain for programmatic use.
	Error error `json:"-"`
}

// prepareResultError keeps the display message separate from the typed cause.
// The display message can be sanitized while errors.Is/errors.As still reach
// the original cause through Unwrap.
type prepareResultError struct {
	message string
	cause   error
}

func (e *prepareResultError) Error() string { return e.message }
func (e *prepareResultError) Unwrap() error { return e.cause }

// PrepareProgressCallback is called when a preparation step changes status.
type PrepareProgressCallback func(step PrepareStep, stepIndex int, totalSteps int)

// EnvironmentPreparer prepares the execution environment before an agent is launched.
type EnvironmentPreparer interface {
	// Name returns the name of this preparer (e.g. "local", "worktree", "docker").
	Name() string

	// Prepare executes the environment preparation steps.
	Prepare(ctx context.Context, req *EnvPrepareRequest, onProgress PrepareProgressCallback) (*EnvPrepareResult, error)
}

// MaxStepOutputBytes is the maximum size of a single step's Output field
// when persisting to session metadata.
const MaxStepOutputBytes = 10 * 1024

// SerializePrepareResult converts a prepare result into a map suitable for
// storage in session metadata. Used by both the synchronous persistence path
// (persistLaunchState) and the async event handler (handlePrepareCompleted).
func SerializePrepareResult(result *EnvPrepareResult) map[string]interface{} {
	status := "completed"
	if !result.Success {
		status = "failed"
	}
	steps := make([]map[string]interface{}, 0, len(result.Steps))
	for _, step := range result.Steps {
		output := step.Output
		if len(output) > MaxStepOutputBytes {
			// Truncate at a valid UTF-8 boundary to avoid splitting multi-byte runes.
			output = strings.ToValidUTF8(output[:MaxStepOutputBytes], "") + "\n... (truncated)"
		}
		entry := map[string]interface{}{
			"name": step.Name, "status": string(step.Status),
			"output": output, "command": step.Command,
		}
		if step.Error != "" {
			entry["error"] = step.Error
		}
		if step.Warning != "" {
			entry["warning"] = step.Warning
		}
		if step.WarningDetail != "" {
			entry["warning_detail"] = step.WarningDetail
		}
		if step.StartedAt != nil {
			entry["started_at"] = step.StartedAt.Format(time.RFC3339Nano)
		}
		if step.EndedAt != nil {
			entry["ended_at"] = step.EndedAt.Format(time.RFC3339Nano)
		}
		steps = append(steps, entry)
	}
	return map[string]interface{}{
		"status": status, "steps": steps,
		"error_message": result.ErrorMessage,
		"duration_ms":   result.Duration.Milliseconds(),
	}
}

// PreparerRegistry maps executor types (models.ExecutorType — the "local",
// "worktree", "local_docker", "sprites" taxonomy) to environment preparers.
//
// Keyed by ExecutorType, not Runtime: preparers do per-executor-type filesystem
// setup (e.g. worktree creation for the worktree type), and different
// ExecutorTypes that share a Runtime (local + worktree both run on standalone)
// can still get distinct preparation logic.
type PreparerRegistry struct {
	preparers map[models.ExecutorType]EnvironmentPreparer
	logger    *logger.Logger
}

// NewPreparerRegistry creates a new PreparerRegistry.
func NewPreparerRegistry(log *logger.Logger) *PreparerRegistry {
	return &PreparerRegistry{
		preparers: make(map[models.ExecutorType]EnvironmentPreparer),
		logger:    log,
	}
}

// Register adds a preparer for the given executor type.
func (r *PreparerRegistry) Register(execType models.ExecutorType, preparer EnvironmentPreparer) {
	r.preparers[execType] = preparer
}

// Get returns the preparer for the given executor type, or nil if not found.
func (r *PreparerRegistry) Get(execType models.ExecutorType) EnvironmentPreparer {
	return r.preparers[execType]
}
