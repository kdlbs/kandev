package lifecycle

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/worktree"
)

// LocalPreparer prepares a local (non-worktree) execution environment.
// Steps: validate workspace → checkout branch (if set) → run setup script (if any).
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

	// Determine effective branch: CheckoutBranch (PR head) takes priority over BaseBranch.
	effectiveBranch := req.CheckoutBranch
	if effectiveBranch == "" {
		effectiveBranch = req.BaseBranch
	}

	totalSteps := 1 // validate workspace
	if effectiveBranch != "" {
		totalSteps++
	}
	if resolvedScript != "" {
		totalSteps++
	}

	stepIdx := 0

	// Step 1: Validate workspace path
	step := beginStep("Validate workspace")
	reportProgress(onProgress, step, stepIdx, totalSteps)
	if req.WorkspacePath == "" && req.RepositoryPath == "" {
		completeStepError(&step, "no workspace or repository path provided")
		steps = append(steps, step)
		return &EnvPrepareResult{Success: false, Steps: steps, ErrorMessage: step.Error, Duration: time.Since(start)}, fmt.Errorf("no workspace path")
	}
	completeStepSuccess(&step)
	steps = append(steps, step)
	reportProgress(onProgress, step, stepIdx, totalSteps)
	stepIdx++

	// Step 2: Checkout branch (if specified)
	if effectiveBranch != "" {
		step = beginStep("Checkout branch")
		step.Command = fmt.Sprintf("git fetch origin %s && git checkout %s", effectiveBranch, effectiveBranch)
		reportProgress(onProgress, step, stepIdx, totalSteps)
		output, err := checkoutBranch(ctx, workspacePath, effectiveBranch)
		if err != nil {
			errMsg := fmt.Sprintf("failed to checkout branch %q: %s", effectiveBranch, output)
			completeStepError(&step, errMsg)
			steps = append(steps, step)
			reportProgress(onProgress, step, stepIdx, totalSteps)
			return &EnvPrepareResult{Success: false, Steps: steps, ErrorMessage: errMsg, Duration: time.Since(start)}, fmt.Errorf("checkout branch: %w", err)
		}
		step.Output = output
		completeStepSuccess(&step)
		steps = append(steps, step)
		reportProgress(onProgress, step, stepIdx, totalSteps)
		stepIdx++
	}

	// Step 3: Run setup script (if provided)
	if resolvedScript != "" {
		steps = runSetupScriptStep(ctx, req, workspacePath, resolvedScript, stepIdx, totalSteps, onProgress, steps, p.logger)
	}

	return &EnvPrepareResult{
		Success:       true,
		Steps:         steps,
		WorkspacePath: workspacePath,
		Duration:      time.Since(start),
	}, nil
}

// checkoutBranch ensures a branch is checked out in the given working directory.
// It first tries to fetch the latest from origin, then checks out the branch.
// If fetch fails (no remote, offline), it falls back to the local branch.
func checkoutBranch(ctx context.Context, workDir, branch string) (string, error) {
	// Try to fetch the latest from origin (best-effort).
	fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", branch)
	fetchCmd.Dir = workDir
	fetchCmd.Run() //nolint:errcheck // fetch failure is non-fatal, we fall back to local

	// Checkout the branch. If the local branch doesn't exist but the remote
	// tracking branch does (from the fetch above), git will create a local
	// branch tracking the remote automatically.
	cmd := exec.CommandContext(ctx, "git", "checkout", branch)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(out))
	if err != nil {
		return outStr, worktree.ClassifyGitError(outStr, err)
	}
	return outStr, nil
}

// setupScriptStreamInterval is the minimum gap between streaming-output callbacks
// while a setup script runs. Chatty scripts (e.g. npm install with progress bars)
// can emit hundreds of writes per second; throttling keeps WS event volume sane
// while still showing live output.
const setupScriptStreamInterval = 100 * time.Millisecond

// runSetupScript executes a setup script in the given working directory,
// streaming combined stdout/stderr to onOutput (if non-nil) as it runs.
// Returns the full accumulated output (trimmed) and any execution error.
func runSetupScript(ctx context.Context, script, workDir string, env map[string]string, onOutput func(current string)) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = buildEnvSlice(env)

	w := newStreamingWriter(onOutput, setupScriptStreamInterval)
	cmd.Stdout = w
	cmd.Stderr = w

	err := cmd.Run()
	return strings.TrimSpace(w.String()), err
}

// streamingWriter accumulates writes into a buffer and invokes onFlush with the
// current snapshot at most once per minGap. It's safe for concurrent Write calls
// (both stdout and stderr write through the same writer).
type streamingWriter struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	onFlush   func(current string)
	lastFlush time.Time
	minGap    time.Duration
}

func newStreamingWriter(onFlush func(current string), minGap time.Duration) *streamingWriter {
	return &streamingWriter{onFlush: onFlush, minGap: minGap}
}

func (w *streamingWriter) Write(p []byte) (int, error) {
	// Hold the lock through the flush so concurrent stdout+stderr writes
	// can't both observe `now-lastFlush >= minGap` and race on the captured
	// `step.Output` shared by the caller's callback.
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(p)
	now := time.Now()
	if w.onFlush != nil && now.Sub(w.lastFlush) >= w.minGap {
		w.lastFlush = now
		w.onFlush(strings.TrimSpace(w.buf.String()))
	}
	return n, err
}

// String returns the full accumulated output.
func (w *streamingWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
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

// setupScriptDisplayCommand returns a short, user-facing command string for the
// "Run setup script" step. Prefers the explicit profile script, then the
// repository-level script. Falls back to empty (the step still shows its name).
func setupScriptDisplayCommand(req *EnvPrepareRequest) string {
	if s := strings.TrimSpace(req.SetupScript); s != "" {
		return s
	}
	if s := strings.TrimSpace(req.RepoSetupScript); s != "" {
		return s
	}
	return ""
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
