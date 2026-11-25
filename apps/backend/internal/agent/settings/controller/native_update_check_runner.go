package controller

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/proctree"
)

const (
	nativeUpdateCheckTimeout     = 30 * time.Second
	nativeUpdateCheckOutputLimit = 64 * 1024
	nativeUpdateCheckWaitDelay   = time.Second
)

// UpdateStatusResponseBudget is the write deadline that synchronous status and
// preview routes need. It covers discovery, one wait for a lookup slot held by
// another check, the native check timeout, and process cleanup. This exceeds
// the server's default 30-second write timeout.
const UpdateStatusResponseBudget = 2*nativeUpdateCheckTimeout + 30*time.Second

var (
	errNativeUpdateCheckTimeout     = errors.New("native update check timed out")
	errNativeUpdateCheckOutputLimit = errors.New("native update check output limit exceeded")
	errNativeUpdateCheckEmpty       = errors.New("native update check command is empty")
	errNativeUpdateCheckOffline     = errors.New("native update check execution is disabled")
)

// nativeUpdateCheckRun is one finished check. Output is set only when Reason
// is empty; a stopped or unstarted run never exposes partial output.
// Executable is the launched path as resolved from PATH and RealExecutable
// that path with symlinks evaluated immediately before launch.
type nativeUpdateCheckRun struct {
	Output         agents.NativeUpdateCheckOutput
	Executable     string
	RealExecutable string
	Reason         agents.NativeUpdateCheckReason
}

// nativeUpdateCheckRunner executes trusted native check argv. Resolve and
// Environment expose the inputs used for cache identity without starting a
// process.
type nativeUpdateCheckRunner interface {
	Resolve(command agents.Command) (string, error)
	Environment() []string
	Run(ctx context.Context, command agents.Command) nativeUpdateCheckRun
}

// execNativeUpdateCheckRunner runs a check with the trusted updater's
// environment and executable resolution, contained in a process tree.
type execNativeUpdateCheckRunner struct {
	timeout time.Duration
	// resolveLaunch overrides launch resolution in tests.
	resolveLaunch func(agents.Command) (path, real string, err error)
}

var _ nativeUpdateCheckRunner = execNativeUpdateCheckRunner{}

func (execNativeUpdateCheckRunner) Resolve(command agents.Command) (string, error) {
	argv := command.Args()
	if len(argv) == 0 || argv[0] == "" {
		return "", errNativeUpdateCheckEmpty
	}
	return exec.LookPath(argv[0])
}

func (execNativeUpdateCheckRunner) Environment() []string { return filteredInstallEnv() }

func (r execNativeUpdateCheckRunner) Run(ctx context.Context, command agents.Command) nativeUpdateCheckRun {
	timeout := r.timeout
	if timeout <= 0 {
		timeout = nativeUpdateCheckTimeout
	}
	runCtx, cancelRun := context.WithTimeoutCause(ctx, timeout, errNativeUpdateCheckTimeout)
	defer cancelRun()
	path, real, err, ok := r.launchPaths(runCtx, command)
	if !ok {
		return nativeUpdateCheckRun{Reason: nativeUpdateCheckContextReason(ctx, runCtx)}
	}
	if reason := nativeUpdateCheckContextReason(ctx, runCtx); reason != "" {
		return nativeUpdateCheckRun{Reason: reason}
	}
	if err != nil {
		return nativeUpdateCheckRun{Reason: agents.NativeUpdateCheckReasonNotInstalled}
	}
	limitCtx, cancelLimit := context.WithCancelCause(runCtx)
	defer cancelLimit(nil)

	budget := &nativeCheckOutputBudget{remaining: nativeUpdateCheckOutputLimit, exceed: func() { cancelLimit(errNativeUpdateCheckOutputLimit) }}
	stdout, stderr := &nativeCheckOutputBuffer{budget: budget}, &nativeCheckOutputBuffer{budget: budget}
	cmd := exec.CommandContext(limitCtx, path, command.Args()[1:]...)
	cmd.Env = r.Environment()
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.WaitDelay = nativeUpdateCheckWaitDelay
	exitCode, started, waitErr := runContainedCommand(cmd)
	run := nativeUpdateCheckRun{Executable: path, RealExecutable: real}
	contextReason := nativeUpdateCheckContextReason(ctx, runCtx)
	switch {
	case budget.exceeded():
		run.Reason = agents.NativeUpdateCheckReasonOutputLimit
	case contextReason != "":
		run.Reason = contextReason
	case !started:
		run.Reason = agents.NativeUpdateCheckReasonNotInstalled
	case waitErr != nil:
		// The output was not captured completely (for example, WaitDelay closed
		// pipes a descendant still held), so it must not be parsed.
		run.Reason = agents.NativeUpdateCheckReasonCheckFailed
	default:
		run.Output = agents.NativeUpdateCheckOutput{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode}
	}
	return run
}

func nativeUpdateCheckContextReason(ctx, runCtx context.Context) agents.NativeUpdateCheckReason {
	if errors.Is(context.Cause(runCtx), errNativeUpdateCheckTimeout) || errors.Is(context.Cause(runCtx), context.DeadlineExceeded) {
		return agents.NativeUpdateCheckReasonTimeout
	}
	if ctx.Err() != nil || runCtx.Err() != nil {
		return agents.NativeUpdateCheckReasonCancelled
	}
	return ""
}

// launchPaths resolves the launcher and its real executable off the caller's
// goroutine. Both reads can block on unreachable storage before any process
// exists to cancel, so cancellation returns ok=false at once; the abandoned
// resolution has no spawn authority and its result is discarded.
func (r execNativeUpdateCheckRunner) launchPaths(ctx context.Context, command agents.Command) (path, real string, err error, ok bool) {
	resolve := r.resolveLaunch
	if resolve == nil {
		resolve = r.resolveLaunchPaths
	}
	type resolved struct {
		path, real string
		err        error
	}
	done := make(chan resolved, 1)
	go func() {
		path, real, err := resolve(command)
		done <- resolved{path, real, err}
	}()
	select {
	case got := <-done:
		return got.path, got.real, got.err, true
	case <-ctx.Done():
		return "", "", nil, false
	}
}

func (r execNativeUpdateCheckRunner) resolveLaunchPaths(command agents.Command) (string, string, error) {
	path, err := r.Resolve(command)
	if err != nil {
		return "", "", err
	}
	real := path
	if evaluated, evalErr := filepath.EvalSymlinks(path); evalErr == nil {
		real = evaluated
	}
	return path, real, nil
}

// runContainedCommand starts cmd in its own process tree, terminates the tree
// on context cancellation, and releases it after every exit so descendants
// never outlive the check. waitErr reports a Wait failure other than a
// non-zero exit, which means output capture was incomplete.
func runContainedCommand(cmd *exec.Cmd) (exitCode int, started bool, waitErr error) {
	var (
		treeMu sync.Mutex
		tree   proctree.Tree
	)
	cmd.Cancel = func() error {
		treeMu.Lock()
		current := tree
		treeMu.Unlock()
		current.Kill(cmd)
		return nil
	}
	proctree.Prepare(cmd)
	if err := cmd.Start(); err != nil {
		return -1, false, nil
	}
	captured, err := proctree.Capture(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return -1, false, nil
	}
	treeMu.Lock()
	tree = captured
	treeMu.Unlock()
	err = cmd.Wait()
	treeMu.Lock()
	committed := tree
	treeMu.Unlock()
	committed.Kill(cmd)
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		waitErr = err
	}
	if cmd.ProcessState == nil {
		return -1, true, waitErr
	}
	return cmd.ProcessState.ExitCode(), true, waitErr
}

// nativeCheckOutputBudget is shared by stdout and stderr. The first write
// over the budget records the overflow and stops the run.
type nativeCheckOutputBudget struct {
	mu        sync.Mutex
	remaining int
	over      bool
	exceed    func()
}

func (b *nativeCheckOutputBudget) exceeded() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.over
}

type nativeCheckOutputBuffer struct {
	budget *nativeCheckOutputBudget
	buffer bytes.Buffer
}

func (w *nativeCheckOutputBuffer) Write(data []byte) (int, error) {
	w.budget.mu.Lock()
	if w.budget.over {
		w.budget.mu.Unlock()
		return len(data), nil
	}
	if len(data) > w.budget.remaining {
		w.budget.over = true
		w.budget.mu.Unlock()
		w.budget.exceed()
		return len(data), nil
	}
	w.budget.remaining -= len(data)
	w.buffer.Write(data)
	w.budget.mu.Unlock()
	return len(data), nil
}

func (w *nativeCheckOutputBuffer) String() string {
	w.budget.mu.Lock()
	defer w.budget.mu.Unlock()
	return w.buffer.String()
}
