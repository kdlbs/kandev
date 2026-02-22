package lifecycle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// LocalPreparer prepares a local (non-worktree) execution environment.
// Steps: validate workspace → run setup script (if any).
type LocalPreparer struct {
	logger *logger.Logger
}

// NewLocalPreparer creates a new LocalPreparer.
func NewLocalPreparer(log *logger.Logger) *LocalPreparer {
	return &LocalPreparer{
		logger: log.WithFields(zap.String("component", "local-preparer")),
	}
}

func (p *LocalPreparer) Name() string { return "local" }

func (p *LocalPreparer) Prepare(ctx context.Context, req *EnvPrepareRequest, onProgress PrepareProgressCallback) (*EnvPrepareResult, error) {
	start := time.Now()
	var steps []PrepareStep

	workspacePath := req.WorkspacePath
	if workspacePath == "" {
		workspacePath = req.RepositoryPath
	}
	resolvedScript := resolvePreparerSetupScript(req, workspacePath)

	totalSteps := 1 // validate workspace
	if resolvedScript != "" {
		totalSteps++
	}

	// Step 1: Validate workspace path
	step := beginStep("Validate workspace")
	reportProgress(onProgress, step, 0, totalSteps)
	if req.WorkspacePath == "" && req.RepositoryPath == "" {
		completeStepError(&step, "no workspace or repository path provided")
		steps = append(steps, step)
		return &EnvPrepareResult{Success: false, Steps: steps, ErrorMessage: step.Error, Duration: time.Since(start)}, fmt.Errorf("no workspace path")
	}
	completeStepSuccess(&step)
	steps = append(steps, step)
	reportProgress(onProgress, step, 0, totalSteps)

	// Step 2: Run setup script (if provided)
	if resolvedScript != "" {
		step = beginStep("Run setup script")
		reportProgress(onProgress, step, 1, totalSteps)
		output, err := runSetupScript(ctx, resolvedScript, workspacePath, req.Env)
		if err != nil {
			completeStepError(&step, err.Error())
			step.Output = output
			steps = append(steps, step)
			reportProgress(onProgress, step, 1, totalSteps)
			p.logger.Warn("setup script failed", zap.String("task_id", req.TaskID), zap.Error(err))
			// Setup script failure is non-fatal — log and continue
		} else {
			step.Output = output
			completeStepSuccess(&step)
			steps = append(steps, step)
			reportProgress(onProgress, step, 1, totalSteps)
		}
	}

	return &EnvPrepareResult{
		Success:       true,
		Steps:         steps,
		WorkspacePath: workspacePath,
		Duration:      time.Since(start),
	}, nil
}

// runSetupScript executes a setup script in the given working directory.
func runSetupScript(ctx context.Context, script, workDir string, env map[string]string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = buildEnvSlice(env)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// buildEnvSlice converts a map to os.Environ format (KEY=VALUE).
func buildEnvSlice(env map[string]string) []string {
	base := os.Environ()
	if len(env) == 0 {
		return base
	}
	result := make([]string, 0, len(base)+len(env))
	result = append(result, base...)
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}

// Helper functions for step lifecycle

func beginStep(name string) PrepareStep {
	now := time.Now()
	return PrepareStep{
		Name:      name,
		Status:    PrepareStepRunning,
		StartedAt: &now,
	}
}

func completeStepSuccess(step *PrepareStep) {
	now := time.Now()
	step.Status = PrepareStepCompleted
	step.EndedAt = &now
}

func completeStepError(step *PrepareStep, errMsg string) {
	now := time.Now()
	step.Status = PrepareStepFailed
	step.Error = errMsg
	step.EndedAt = &now
}

func completeStepSkipped(step *PrepareStep) {
	now := time.Now()
	step.Status = PrepareStepSkipped
	step.EndedAt = &now
}

func reportProgress(cb PrepareProgressCallback, step PrepareStep, index, total int) {
	if cb != nil {
		cb(step, index, total)
	}
}
