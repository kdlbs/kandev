package hostcli

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
)

// Runner starts host processes. Tests provide a fake; production uses
// ExecRunner.
type Runner interface {
	// Output runs argv to completion and returns its combined stdout and
	// stderr.
	Output(ctx context.Context, argv []string) (string, error)
	// Start launches argv with piped stdin and stdout for a request and
	// response exchange.
	Start(ctx context.Context, argv []string) (Process, error)
}

// Process is a started interactive host process.
type Process interface {
	Stdin() io.Writer
	Stdout() io.Reader
	// Wait blocks until the process exits.
	Wait() error
	// Kill terminates the process. It is safe to call after Wait.
	Kill() error
}

// ErrNotInstalled reports that the executable could not be started.
var ErrNotInstalled = errors.New("host cli executable is not available")

// ExecRunner runs real host processes.
type ExecRunner struct{}

// Output implements Runner.
func (ExecRunner) Output(ctx context.Context, argv []string) (string, error) {
	if len(argv) == 0 {
		return "", errors.New("host cli command is empty")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil && isExecNotFound(err) {
		return string(out), ErrNotInstalled
	}
	return string(out), err
}

type execProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func (p *execProcess) Stdin() io.Writer  { return p.stdin }
func (p *execProcess) Stdout() io.Reader { return p.stdout }
func (p *execProcess) Wait() error       { return p.cmd.Wait() }
func (p *execProcess) Kill() error {
	_ = p.stdin.Close()
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

// Start implements Runner.
func (ExecRunner) Start(ctx context.Context, argv []string) (Process, error) {
	if len(argv) == 0 {
		return nil, errors.New("host cli command is empty")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	// Vendor CLIs log to stderr; it is not part of the protocol stream.
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		if isExecNotFound(err) {
			return nil, ErrNotInstalled
		}
		return nil, err
	}
	return &execProcess{cmd: cmd, stdin: stdin, stdout: stdout}, nil
}

func isExecNotFound(err error) bool {
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "no such file or directory")
}
