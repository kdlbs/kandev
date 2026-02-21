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
	ExecutionID    string
	ExecutorType   executor.Name
	WorkspacePath  string
	RepositoryPath string
	UseWorktree    bool
	SetupScript    string
	Env            map[string]string
}

// PrepareStep represents a single step in the preparation process.
type PrepareStep struct {
	Name      string            `json:"name"`
	Status    PrepareStepStatus `json:"status"`
	Output    string            `json:"output,omitempty"`
	Error     string            `json:"error,omitempty"`
	StartedAt *time.Time        `json:"started_at,omitempty"`
	EndedAt   *time.Time        `json:"ended_at,omitempty"`
}

// EnvPrepareResult contains the result of environment preparation.
type EnvPrepareResult struct {
	Success       bool          `json:"success"`
	Steps         []PrepareStep `json:"steps"`
	WorkspacePath string        `json:"workspace_path,omitempty"`
	ErrorMessage  string        `json:"error_message,omitempty"`
	Duration      time.Duration `json:"duration"`
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
