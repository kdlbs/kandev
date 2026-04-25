package lifecycle

import (
	"context"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/common/logger"
)

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
	RepositoryID         string
	RepositoryPath       string
	RepoName             string
	BaseBranch           string
	CheckoutBranch       string
	WorktreeID           string
	WorktreeBranch       string
	WorktreeBranchPrefix string
	PullBeforeWorktree   bool
	RepoSetupScript      string
}

// EnvPrepareRequest contains the parameters for environment preparation.
type EnvPrepareRequest struct {
	TaskID          string
	SessionID       string
	TaskTitle       string
	ExecutionID     string
	ExecutorType    executor.Name
	WorkspacePath   string
	RepositoryPath  string
	RepositoryID    string
	UseWorktree     bool
	SetupScript     string
	RepoSetupScript string // Repository-level setup script (e.g. "make install")
	BaseBranch      string
	CheckoutBranch  string
	WorktreeID      string
	WorktreeBranch  string

	WorktreeBranchPrefix string
	PullBeforeWorktree   bool

	TaskDirName string // Per-task directory name within the workspace (e.g. "task-abc123")
	RepoName    string // Repository slug used with TaskDirName to locate checkouts

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
		RepositoryID:         r.RepositoryID,
		RepositoryPath:       r.RepositoryPath,
		RepoName:             r.RepoName,
		BaseBranch:           r.BaseBranch,
		CheckoutBranch:       r.CheckoutBranch,
		WorktreeID:           r.WorktreeID,
		WorktreeBranch:       r.WorktreeBranch,
		WorktreeBranchPrefix: r.WorktreeBranchPrefix,
		PullBeforeWorktree:   r.PullBeforeWorktree,
		RepoSetupScript:      r.RepoSetupScript,
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
	RepositoryID   string `json:"repository_id"`
	WorktreeID     string `json:"worktree_id,omitempty"`
	WorktreeBranch string `json:"worktree_branch,omitempty"`
	WorktreePath   string `json:"worktree_path,omitempty"`
	MainRepoGitDir string `json:"main_repo_git_dir,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
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
	WorktreeID     string `json:"worktree_id,omitempty"`
	WorktreeBranch string `json:"worktree_branch,omitempty"`
	MainRepoGitDir string `json:"main_repo_git_dir,omitempty"`

	// Worktrees is the per-repository outcome list when the preparer ran in
	// multi-repo mode. Empty for single-repo or repo-less results.
	Worktrees []RepoWorktreeResult `json:"worktrees,omitempty"`
}

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

// PreparerRegistry maps executor types to environment preparers.
type PreparerRegistry struct {
	preparers map[executor.Name]EnvironmentPreparer
	logger    *logger.Logger
}

// NewPreparerRegistry creates a new PreparerRegistry.
func NewPreparerRegistry(log *logger.Logger) *PreparerRegistry {
	return &PreparerRegistry{
		preparers: make(map[executor.Name]EnvironmentPreparer),
		logger:    log,
	}
}

// Register adds a preparer for the given executor type.
func (r *PreparerRegistry) Register(execType executor.Name, preparer EnvironmentPreparer) {
	r.preparers[execType] = preparer
}

// Get returns the preparer for the given executor type, or nil if not found.
func (r *PreparerRegistry) Get(execType executor.Name) EnvironmentPreparer {
	return r.preparers[execType]
}
