package subproc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

const (
	gitPipeWaitDelay = 500 * time.Millisecond
	gitCleanupLimit  = 2 * time.Second
)

// runManagedGit owns the complete Start-to-Wait lifetime of a Git command.
// The command must already have its final environment and output wiring.
func runManagedGit(ctx context.Context, cmd *exec.Cmd) error {
	process, err := startManagedGit(ctx, cmd)
	if err != nil {
		return err
	}
	return process.wait()
}

func runManagedGitOutput(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	if cmd == nil {
		return nil, errors.New("nil Git command")
	}
	if cmd.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}

	var stdout synchronizedBuffer
	cmd.Stdout = &stdout
	var stderr *synchronizedBuffer
	if cmd.Stderr == nil {
		stderr = &synchronizedBuffer{}
		cmd.Stderr = stderr
	}
	process, err := startManagedGit(ctx, cmd)
	if err != nil {
		return nil, err
	}
	runErr := process.wait()
	if runErr != nil && stderr != nil {
		attachGitStderr(runErr, stderr.Bytes())
	}
	return stdout.Bytes(), runErr
}

func runManagedGitCombinedOutput(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	if cmd == nil {
		return nil, errors.New("nil Git command")
	}
	if cmd.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	if cmd.Stderr != nil {
		return nil, errors.New("exec: Stderr already set")
	}

	var output synchronizedBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	process, err := startManagedGit(ctx, cmd)
	if err != nil {
		return nil, err
	}
	runErr := process.wait()
	return output.Bytes(), runErr
}

// GitStream owns a classified Git slot and a streaming stdout pipe. The
// caller must consume Stdout and call Wait exactly once.
type GitStream struct {
	process  *managedGitProcess
	stdout   io.ReadCloser
	execErr  func() error
	cancel   context.CancelFunc
	release  func()
	waitOnce sync.Once
	waitErr  error
	execDone error
}

// Stdout returns the command-owned stdout pipe. It is valid until Wait or
// Close returns.
func (s *GitStream) Stdout() io.ReadCloser {
	if s == nil {
		return nil
	}
	return s.stdout
}

// Close closes the streaming stdout pipe. Wait must still be called to reap
// the process and release the admission slot.
func (s *GitStream) Close() error {
	if s == nil || s.process == nil {
		return nil
	}
	s.process.closePipes()
	s.process.mu.Lock()
	defer s.process.mu.Unlock()
	return s.process.closeErr
}

// Wait reaps the command, closes its lifecycle resources, and releases its
// Git admission slot. The second result is the post-admission execution
// context error, if any.
func (s *GitStream) Wait() (error, error) {
	if s == nil {
		return errors.New("nil Git stream"), nil
	}
	s.waitOnce.Do(func() {
		s.waitErr = s.process.wait()
		s.execDone = s.execErr()
		s.cancel()
		s.release()
	})
	return s.waitErr, s.execDone
}

// StartGitStreamAfterAcquire builds and starts a streaming Git command after
// acquiring its class slot. The slot remains held until GitStream.Wait.
func StartGitStreamAfterAcquire(
	ctx context.Context,
	class GitWorkClass,
	execTimeout time.Duration,
	build func(execCtx context.Context) *exec.Cmd,
) (*GitStream, error, error) {
	release, err := gitThrottle.AcquireClass(ctx, class)
	if err != nil {
		return nil, wrapAdmissionError(err), nil
	}
	execCtx, cancel := withGitExecTimeout(ctx, execTimeout)
	cmd := build(execCtx)
	if cmd == nil {
		cancel()
		release()
		return nil, errors.New("Git command builder returned nil"), execCtx.Err()
	}
	PrepareGitCommand(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		release()
		return nil, fmt.Errorf("attach Git stdout pipe: %w", err), execCtx.Err()
	}
	process, err := startManagedGit(execCtx, cmd, stdout)
	if err != nil {
		_ = stdout.Close()
		cancel()
		release()
		return nil, err, execCtx.Err()
	}
	return &GitStream{
		process: process,
		stdout:  stdout,
		execErr: execCtx.Err,
		cancel:  cancel,
		release: release,
	}, nil, nil
}

type managedGitProcess struct {
	cmd       *exec.Cmd
	lifecycle gitLifecycleHandle
	pipes     []io.Closer

	finished   chan struct{}
	cancelDone chan struct{}
	closeOnce  sync.Once
	waitOnce   sync.Once
	cancelOnce sync.Once

	mu         sync.Mutex
	waitErr    error
	cancelErr  error
	closeErr   error
	releaseErr error
}

func startManagedGit(ctx context.Context, cmd *exec.Cmd, pipes ...io.Closer) (*managedGitProcess, error) {
	if cmd == nil {
		return nil, errors.New("nil Git command")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	PrepareGitCommand(cmd)
	if err := prepareGitLifecycleCommand(cmd); err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = closeGitPipes(pipes...)
		return nil, err
	}
	lifecycle, err := installGitLifecycle(cmd)
	if err != nil {
		closeErr := closeGitPipes(pipes...)
		reapErr := abortStartedGit(cmd)
		return nil, errors.Join(fmt.Errorf("install Git lifecycle: %w", err), closeErr, reapErr)
	}
	process := &managedGitProcess{
		cmd:        cmd,
		lifecycle:  lifecycle,
		pipes:      append([]io.Closer(nil), pipes...),
		finished:   make(chan struct{}),
		cancelDone: make(chan struct{}),
	}
	go process.watchCancellation(ctx)
	return process, nil
}

func (p *managedGitProcess) watchCancellation(ctx context.Context) {
	defer close(p.cancelDone)
	select {
	case <-ctx.Done():
		p.cancelOwnedProcess()
	case <-p.finished:
		// Both channels can be ready when cancellation races with process exit.
		// Check the context after observing process completion so a canceled
		// command still cleans up descendants that outlive its parent.
		if ctx.Err() != nil {
			p.cancelOwnedProcess()
		}
	}
}

func (p *managedGitProcess) cancelOwnedProcess() {
	p.cancelOnce.Do(func() {
		p.mu.Lock()
		p.cancelErr = cancelGitLifecycle(p.lifecycle)
		p.mu.Unlock()
		p.closePipes()
	})
}

func (p *managedGitProcess) wait() error {
	p.waitOnce.Do(func() {
		waitErr := p.cmd.Wait()
		p.mu.Lock()
		p.waitErr = waitErr
		p.mu.Unlock()
		close(p.finished)
		// The cancellation watcher may still be finishing a platform cleanup
		// operation. Waiting here keeps the returned error and admission release
		// ordered after owned cleanup, including when the process exits by signal.
		select {
		case <-p.cancelDone:
		case <-time.After(gitCleanupLimit):
			p.mu.Lock()
			p.cancelErr = errors.Join(p.cancelErr, errors.New("Git cancellation cleanup timed out"))
			p.mu.Unlock()
		}
		p.closePipes()
		p.mu.Lock()
		p.releaseErr = releaseGitLifecycle(p.lifecycle)
		p.mu.Unlock()
	})
	p.mu.Lock()
	defer p.mu.Unlock()
	return errors.Join(p.waitErr, p.cancelErr, p.closeErr, p.releaseErr)
}

func (p *managedGitProcess) closePipes() {
	p.closeOnce.Do(func() {
		var err error
		for _, pipe := range p.pipes {
			if pipe != nil {
				err = errors.Join(err, pipe.Close())
			}
		}
		p.mu.Lock()
		p.closeErr = err
		p.mu.Unlock()
	})
}

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *synchronizedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buffer.Bytes()...)
}

func attachGitStderr(err error, stderr []byte) {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitErr.Stderr = append([]byte(nil), stderr...)
	}
}

func setGitWaitDelay(cmd *exec.Cmd) {
	if cmd.WaitDelay <= 0 || cmd.WaitDelay > gitPipeWaitDelay {
		cmd.WaitDelay = gitPipeWaitDelay
	}
}

func closeGitPipes(pipes ...io.Closer) error {
	var err error
	for _, pipe := range pipes {
		if pipe != nil {
			err = errors.Join(err, pipe.Close())
		}
	}
	return err
}
