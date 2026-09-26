package codexdbg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/pkg/codexappserver"
)

const (
	defaultStderrLimit = 1024 * 1024
	processWaitLimit   = 5 * time.Second
)

// RunnerConfig defines one app-server subprocess and its local capture.
type RunnerConfig struct {
	Executable        string
	Args              []string
	Environment       []string
	Workdir           string
	CapturePath       string
	ExecutableVersion string
	Operation         string
	CaptureStderr     bool
	MaxStderrBytes    int64
	MaxFrameBytes     int
}

// Runner owns one debugger-started app-server process, its pipe tree, and its
// capture file. It never attaches to a running application process.
type Runner struct {
	client      *codexappserver.Client
	recorder    *Recorder
	command     *exec.Cmd
	tree        processTree
	stdin       *os.File
	stdout      *os.File
	stderr      *os.File
	tempWorkdir string
	workdir     string
	processDone chan struct{}
	stderrDone  chan struct{}
	watchStop   chan struct{}
	closeOnce   sync.Once
	watchOnce   sync.Once
	closeErr    error
	maxStderr   int64
}

// StartRunner creates a private capture and starts the requested executable.
// executableVersion must come from that exact executable's --version output.
func StartRunner(ctx context.Context, config RunnerConfig) (*Runner, error) {
	config, workdir, tempWorkdir, err := prepareRunner(ctx, config)
	if err != nil {
		return nil, err
	}
	recorder, err := NewRecorder(config.CapturePath, config.ExecutableVersion)
	if err != nil {
		if tempWorkdir != "" {
			_ = os.RemoveAll(tempWorkdir)
		}
		return nil, err
	}
	if err := recorder.RecordMeta("operation", map[string]any{"name": config.Operation, "workdir": workdir}); err != nil {
		_ = recorder.Close("capture_error")
		if tempWorkdir != "" {
			_ = os.RemoveAll(tempWorkdir)
		}
		return nil, err
	}

	command, pipes, err := startRunnerCommand(config, workdir)
	if err != nil {
		_ = recorder.Close("start_failed")
		if tempWorkdir != "" {
			_ = os.RemoveAll(tempWorkdir)
		}
		return nil, err
	}
	tree, err := captureProcessTree(command)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		pipes.close()
		_ = recorder.Close("containment_failed")
		if tempWorkdir != "" {
			_ = os.RemoveAll(tempWorkdir)
		}
		return nil, fmt.Errorf("contain app-server process tree: %w", err)
	}

	runner := &Runner{
		recorder:    recorder,
		command:     command,
		tree:        tree,
		stdin:       pipes.stdin,
		stdout:      pipes.stdout,
		stderr:      pipes.stderr,
		tempWorkdir: tempWorkdir,
		workdir:     workdir,
		processDone: make(chan struct{}),
		stderrDone:  make(chan struct{}),
		watchStop:   make(chan struct{}),
		maxStderr:   config.MaxStderrBytes,
	}
	runner.client = codexappserver.NewClient(pipes.stdin, pipes.stdout, codexappserver.Options{MaxFrameBytes: config.MaxFrameBytes})
	runner.client.SetFrameObserver(recorder.RecordFrame)
	go runner.waitProcess()
	go runner.drainStderr(config.CaptureStderr)
	go runner.watchContext(ctx)
	_ = recorder.RecordMeta("process_started", map[string]any{"pid": command.Process.Pid})
	return runner, nil
}

func prepareRunner(ctx context.Context, config RunnerConfig) (RunnerConfig, string, string, error) {
	if ctx == nil {
		return config, "", "", errors.New("runner context is required")
	}
	if config.Executable == "" {
		return config, "", "", errors.New("app-server executable is required")
	}
	if config.ExecutableVersion == "" {
		return config, "", "", errors.New("app-server executable version is required")
	}
	if config.CapturePath == "" {
		return config, "", "", errors.New("capture path is required")
	}
	if config.Operation == "" {
		config.Operation = "debug"
	}
	if config.MaxStderrBytes <= 0 {
		config.MaxStderrBytes = defaultStderrLimit
	}
	if err := ctx.Err(); err != nil {
		return config, "", "", err
	}

	workdir := config.Workdir
	tempWorkdir := ""
	if workdir == "" {
		created, err := privateTempWorkdir()
		if err != nil {
			return config, "", "", fmt.Errorf("create temporary workdir: %w", err)
		}
		workdir = created
		tempWorkdir = created
	} else {
		info, err := os.Stat(workdir)
		if err != nil {
			return config, "", "", fmt.Errorf("inspect workdir: %w", err)
		}
		if !info.IsDir() {
			return config, "", "", errors.New("workdir must be a directory")
		}
	}
	return config, workdir, tempWorkdir, nil
}

type runnerCommandPipes struct {
	stdin, stdout, stderr *os.File
	childStdin            *os.File
	childStdout           *os.File
	childStderr           *os.File
}

func startRunnerCommand(config RunnerConfig, workdir string) (*exec.Cmd, *runnerCommandPipes, error) {
	//nolint:gosec // The developer explicitly selects the executable and argument vector.
	command := exec.Command(config.Executable, config.Args...)
	command.Dir = workdir
	if len(config.Environment) > 0 {
		command.Env = mergeEnvironment(os.Environ(), config.Environment)
	}
	configureProcessTree(command)
	pipes := &runnerCommandPipes{}
	var err error
	pipes.childStdin, pipes.stdin, err = os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("create stdin pipe: %w", err)
	}
	pipes.stdout, pipes.childStdout, err = os.Pipe()
	if err != nil {
		pipes.close()
		return nil, nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	pipes.stderr, pipes.childStderr, err = os.Pipe()
	if err != nil {
		pipes.close()
		return nil, nil, fmt.Errorf("create stderr pipe: %w", err)
	}
	command.Stdin, command.Stdout, command.Stderr = pipes.childStdin, pipes.childStdout, pipes.childStderr
	if err := command.Start(); err != nil {
		pipes.close()
		return nil, nil, fmt.Errorf("start app-server executable: %w", err)
	}
	closeRunnerFile(pipes.childStdin)
	closeRunnerFile(pipes.childStdout)
	closeRunnerFile(pipes.childStderr)
	return command, pipes, nil
}

func (p *runnerCommandPipes) close() {
	closeRunnerFile(p.stdin)
	closeRunnerFile(p.stdout)
	closeRunnerFile(p.stderr)
	closeRunnerFile(p.childStdin)
	closeRunnerFile(p.childStdout)
	closeRunnerFile(p.childStderr)
}

func closeRunnerFile(file *os.File) {
	if file != nil {
		_ = file.Close()
	}
}

// ResolveExecutable finds an executable using PATH when needed.
func ResolveExecutable(path string) (string, error) {
	if path == "" {
		path = "codex"
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("find app-server executable: %w", err)
	}
	return resolved, nil
}

// ReadExecutableVersion asks the selected binary for its version without
// starting app-server or making an account request.
func ReadExecutableVersion(ctx context.Context, executable string) (string, error) {
	if ctx == nil {
		return "", errors.New("version context is required")
	}
	versionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(versionCtx, executable, "--version")
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read executable version: %w", err)
	}
	version := strings.TrimSpace(string(output))
	if version == "" || strings.Contains(version, "\n") {
		return "", errors.New("app-server executable returned an invalid version")
	}
	return version, nil
}

// Client returns the protocol client attached to this subprocess.
func (r *Runner) Client() *codexappserver.Client { return r.client }

// CapturePath returns the private JSONL capture path.
func (r *Runner) CapturePath() string { return r.recorder.Path() }

// Workdir returns the directory used by the subprocess.
func (r *Runner) Workdir() string { return r.workdir }

// Close kills the owned process tree, drains explicit stderr capture, closes
// protocol streams, flushes the capture, and removes an allocated workdir.
func (r *Runner) Close(reason string) error {
	if reason == "" {
		reason = "closed"
	}
	r.watchOnce.Do(func() { close(r.watchStop) })
	r.closeOnce.Do(func() {
		r.tree.kill(r.command)
		_ = r.client.Close()
		select {
		case <-r.processDone:
		case <-time.After(processWaitLimit):
			r.tree.kill(r.command)
			if r.command.Process != nil {
				_ = r.command.Process.Kill()
			}
			select {
			case <-r.processDone:
			case <-time.After(processWaitLimit):
				r.closeErr = errors.New("app-server process did not exit after termination")
			}
		}
		select {
		case <-r.stderrDone:
		case <-time.After(time.Second):
			r.closeErr = errors.Join(r.closeErr, errors.New("app-server stderr did not close"))
		}
		_ = r.stderr.Close()
		_ = r.stdin.Close()
		_ = r.stdout.Close()
		r.closeErr = errors.Join(r.closeErr, r.recorder.Close(reason))
		if r.tempWorkdir != "" {
			r.closeErr = errors.Join(r.closeErr, os.RemoveAll(r.tempWorkdir))
		}
	})
	return r.closeErr
}

func (r *Runner) watchContext(ctx context.Context) {
	select {
	case <-ctx.Done():
		reason := "cancelled"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			reason = "timeout"
		}
		_ = r.Close(reason)
	case <-r.watchStop:
	}
}

func (r *Runner) waitProcess() {
	_ = r.command.Wait()
	close(r.processDone)
}

func (r *Runner) drainStderr(capture bool) {
	defer close(r.stderrDone)
	buffer := make([]byte, 32*1024)
	var captured int64
	truncated := false
	for {
		n, err := r.stderr.Read(buffer)
		if n > 0 {
			r.captureStderrChunk(buffer[:n], capture, &captured, &truncated)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				_ = r.recorder.RecordMeta("stderr_read_failed", map[string]any{"error": err.Error()})
			}
			break
		}
	}
	if truncated {
		_ = r.recorder.RecordMeta("stderr_truncated", map[string]any{"limit_bytes": r.maxStderr})
	}
}

func (r *Runner) captureStderrChunk(data []byte, capture bool, captured *int64, truncated *bool) {
	if !capture || len(data) == 0 {
		return
	}
	remaining := r.maxStderr - *captured
	if remaining <= 0 {
		*truncated = true
		return
	}
	writeBytes := data
	if int64(len(writeBytes)) > remaining {
		writeBytes = writeBytes[:remaining]
		*truncated = true
	}
	if err := r.recorder.RecordStderr(string(writeBytes)); err != nil {
		*truncated = true
	}
	*captured += int64(len(writeBytes))
}

func mergeEnvironment(base, overrides []string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	for _, item := range base {
		name, value, ok := strings.Cut(item, "=")
		if ok {
			values[name] = value
		}
	}
	for _, item := range overrides {
		name, value, ok := strings.Cut(item, "=")
		if ok {
			values[name] = value
		}
	}
	out := make([]string, 0, len(values))
	for name, value := range values {
		out = append(out, name+"="+value)
	}
	return out
}

func privateTempWorkdir() (string, error) {
	path, err := os.MkdirTemp("", "kandev-codexdbg-*")
	if err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o700); err != nil {
		_ = os.RemoveAll(path)
		return "", err
	}
	return path, nil
}
