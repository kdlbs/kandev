package lifecycle

import (
	"context"
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

// EnvPrepareRequest contains the parameters for environment preparation.
type EnvPrepareRequest struct {
	TaskID         string
	SessionID      string
	TaskTitle      string
	ExecutionID    string
	ExecutorType   executor.Name
	WorkspacePath  string
	RepositoryPath string
	RepositoryID   string
	UseWorktree    bool
	SetupScript    string
	BaseBranch     string
	CheckoutBranch string
	WorktreeID     string
	WorktreeBranch string

	WorktreeBranchPrefix string
	PullBeforeWorktree   bool

	TaskDirName string // Per-task directory name within the workspace (e.g. "task-abc123")
	RepoName    string // Repository slug used with TaskDirName to locate checkouts

	Env map[string]string
}

// PrepareStep represents a single step in the preparation process.
type PrepareStep struct {
	Name          string            `json:"name"`
	Status        PrepareStepStatus `json:"status"`
	Output        string            `json:"output,omitempty"`
	Error         string            `json:"error,omitempty"`
	Warning       string            `json:"warning,omitempty"`
	WarningDetail string            `json:"warning_detail,omitempty"`
	StartedAt     *time.Time        `json:"started_at,omitempty"`
	EndedAt       *time.Time        `json:"ended_at,omitempty"`
}

// EnvPrepareResult contains the result of environment preparation.
type EnvPrepareResult struct {
	Success       bool          `json:"success"`
	Steps         []PrepareStep `json:"steps"`
	WorkspacePath string        `json:"workspace_path,omitempty"`
	ErrorMessage  string        `json:"error_message,omitempty"`
	Duration      time.Duration `json:"duration"`

	// Worktree fields (populated when worktree preparer runs)
	WorktreeID     string `json:"worktree_id,omitempty"`
	WorktreeBranch string `json:"worktree_branch,omitempty"`
	MainRepoGitDir string `json:"main_repo_git_dir,omitempty"`
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
