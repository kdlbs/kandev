package copilotplugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	inventoryTimeout     = 20 * time.Second
	inventoryStdoutLimit = 2 * 1024 * 1024
	inventoryStderrLimit = 32 * 1024
	runnerWaitDelay      = 2 * time.Second
)

// CommandResult is the bounded result of one native CLI invocation. Raw
// buffers never leave this package; callers receive parsed snapshots.
type CommandResult struct {
	Stdout    []byte
	Stderr    []byte
	ExitCode  int
	Truncated bool
}

// Runner executes one native command without a shell.
type Runner interface {
	Run(ctx context.Context, argv []string, env map[string]string, workDir string) (CommandResult, error)
}

// ExecRunner invokes a local executable with a bounded output buffer and
// process-tree cleanup. The profile environment overrides the backend
// environment so the command sees the same home and PATH as the eventual
// agent child.
type ExecRunner struct {
	StdoutLimit int
	StderrLimit int
}

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, argv []string, env map[string]string, workDir string) (CommandResult, error) {
	if len(argv) == 0 || argv[0] == "" {
		return CommandResult{ExitCode: -1}, ErrExecutableUnavailable
	}
	resolved, err := resolveExecutable(argv[0], env)
	if err != nil {
		return CommandResult{ExitCode: -1}, err
	}
	cmd := exec.CommandContext(ctx, resolved, argv[1:]...)
	cmd.Dir = workDir
	cmd.Env = mergeProcessEnvironment(os.Environ(), env)
	cmd.WaitDelay = runnerWaitDelay
	stdoutLimit := r.StdoutLimit
	if stdoutLimit <= 0 || stdoutLimit > inventoryStdoutLimit {
		stdoutLimit = inventoryStdoutLimit
	}
	stderrLimit := r.StderrLimit
	if stderrLimit <= 0 || stderrLimit > inventoryStderrLimit {
		stderrLimit = inventoryStderrLimit
	}
	stdout := &limitedBuffer{limit: stdoutLimit}
	stderr := &limitedBuffer{limit: stderrLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return runCommand(ctx, cmd, stdout, stderr)
}

func runCommand(ctx context.Context, cmd *exec.Cmd, stdout, stderr *limitedBuffer) (CommandResult, error) {
	var (
		treeMu sync.Mutex
		tree   commandTree
	)
	cmd.Cancel = func() error {
		treeMu.Lock()
		current := tree
		treeMu.Unlock()
		current.kill(cmd)
		return nil
	}
	prepareCommandTree(cmd)
	if err := cmd.Start(); err != nil {
		return CommandResult{ExitCode: -1}, fmt.Errorf("start copilot plugin inventory: %w", err)
	}
	captured, captureErr := captureCommandTree(cmd)
	if captureErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return CommandResult{ExitCode: -1}, fmt.Errorf("contain copilot plugin inventory: %w", captureErr)
	}
	treeMu.Lock()
	tree = captured
	treeMu.Unlock()
	runErr := cmd.Wait()
	// The parent may exit while a descendant still owns an inherited pipe.
	// WaitDelay bounds Wait; release the process tree/job on success, nonzero
	// exit, and WaitDelay alike.
	treeMu.Lock()
	committedTree := tree
	treeMu.Unlock()
	committedTree.kill(cmd)
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	result := CommandResult{
		Stdout:    stdout.Bytes(),
		Stderr:    stderr.Bytes(),
		ExitCode:  exitCode,
		Truncated: stdout.Truncated() || stderr.Truncated(),
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return result, context.DeadlineExceeded
		}
		return result, context.Canceled
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return result, nil
		}
		return result, fmt.Errorf("run copilot plugin inventory: %w", runErr)
	}
	return result, nil
}

var (
	// ErrExecutableUnavailable reports that the selected runtime command is not
	// available under the effective environment.
	ErrExecutableUnavailable = errors.New("copilot executable unavailable")
)

func resolveExecutable(name string, env map[string]string) (string, error) {
	windows := runtime.GOOS == "windows"
	if strings.ContainsAny(name, `/\`) || (windows && filepath.VolumeName(name) != "") {
		if path, ok := usableExecutable(name, env, windows); ok {
			return path, nil
		}
		return "", ErrExecutableUnavailable
	}
	searchPath, ok := environmentValue(env, "PATH", windows)
	if !ok {
		searchPath, _ = os.LookupEnv("PATH")
	}
	for _, directory := range filepath.SplitList(searchPath) {
		if directory == "" {
			continue
		}
		if path, ok := usableExecutable(filepath.Join(directory, name), env, windows); ok {
			return path, nil
		}
	}
	return "", ErrExecutableUnavailable
}

func usableExecutable(path string, env map[string]string, windows bool) (string, bool) {
	candidates := []string{path}
	if windows && filepath.Ext(path) == "" {
		extensions, _ := environmentValue(env, "PATHEXT", true)
		if extensions == "" {
			extensions = ".COM;.EXE;.BAT;.CMD"
		}
		for _, extension := range strings.Split(extensions, ";") {
			if extension != "" {
				candidates = append(candidates, path+extension)
			}
		}
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if windows || info.Mode()&0o111 != 0 {
			return candidate, true
		}
	}
	return "", false
}

func environmentValue(env map[string]string, key string, windows bool) (string, bool) {
	if value, ok := env[key]; ok {
		return value, true
	}
	if windows {
		for candidate, value := range env {
			if strings.EqualFold(candidate, key) {
				return value, true
			}
		}
	}
	return "", false
}

func mergeProcessEnvironment(base []string, overlay map[string]string) []string {
	values := make(map[string]string, len(base)+len(overlay))
	for _, item := range base {
		key, value, ok := strings.Cut(item, "=")
		if ok && key != "" {
			values[key] = value
		}
	}
	for key, value := range overlay {
		if key != "" && !strings.ContainsAny(key, "=\x00") && !strings.ContainsRune(value, '\x00') {
			values[key] = value
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

type limitedBuffer struct {
	buffer bytes.Buffer
	limit  int
	large  bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	if len(data) > b.limit-b.buffer.Len() {
		remaining := b.limit - b.buffer.Len()
		if remaining > 0 {
			_, _ = b.buffer.Write(data[:remaining])
		}
		b.large = true
		return len(data), nil
	}
	return b.buffer.Write(data)
}

func (b *limitedBuffer) Bytes() []byte   { return b.buffer.Bytes() }
func (b *limitedBuffer) Truncated() bool { return b.large }
