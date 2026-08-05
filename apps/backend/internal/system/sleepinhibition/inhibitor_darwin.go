//go:build darwin

package sleepinhibition

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

type darwinProcess interface {
	Start() error
	Wait() error
	Kill() error
}

type darwinProcessFactory func(name string, args ...string) darwinProcess

type darwinInhibitor struct {
	newProcess darwinProcessFactory
}

func NewPlatformInhibitor() Inhibitor {
	return newDarwinInhibitor(func(name string, args ...string) darwinProcess {
		return &darwinCommand{cmd: exec.Command(name, args...)}
	})
}

func newDarwinInhibitor(newProcess darwinProcessFactory) Inhibitor {
	return &darwinInhibitor{newProcess: newProcess}
}

func (i *darwinInhibitor) Platform() Platform { return PlatformDarwin }
func (i *darwinInhibitor) Supported() bool    { return true }

func (i *darwinInhibitor) Acquire(_ context.Context) (Lease, error) {
	process := i.newProcess("/usr/bin/caffeinate", "-i", "-w", strconv.Itoa(os.Getpid()))
	if err := process.Start(); err != nil {
		return nil, NewIssueError(IssueRequestFailed, err)
	}
	return newDarwinLease(process), nil
}

type darwinCommand struct{ cmd *exec.Cmd }

func (c *darwinCommand) Start() error { return c.cmd.Start() }
func (c *darwinCommand) Wait() error  { return c.cmd.Wait() }
func (c *darwinCommand) Kill() error {
	if c.cmd.Process == nil {
		return nil
	}
	return c.cmd.Process.Kill()
}

type darwinLease struct {
	process     darwinProcess
	waitDone    chan error
	done        chan error
	releaseOnce sync.Once
	waitOnce    sync.Once
	mu          sync.Mutex
	released    bool
	killErr     error
	waitErr     error
}

func newDarwinLease(process darwinProcess) *darwinLease {
	lease := &darwinLease{
		process:  process,
		waitDone: make(chan error, 1),
		done:     make(chan error, 1),
	}
	go func() {
		err := process.Wait()
		lease.mu.Lock()
		if lease.released {
			// A process exit caused by our Release is normal. Keep unexpected
			// exits visible when they happen before release is requested.
			err = nil
		}
		lease.mu.Unlock()
		lease.waitDone <- err
		lease.done <- err
		close(lease.done)
	}()
	return lease
}

func (l *darwinLease) Release() error {
	l.releaseOnce.Do(func() {
		l.mu.Lock()
		l.released = true
		l.mu.Unlock()
		if err := l.process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			l.killErr = err
		}
	})
	l.waitOnce.Do(func() { l.waitErr = <-l.waitDone })
	if l.killErr != nil {
		return l.killErr
	}
	return l.waitErr
}

func (l *darwinLease) Done() <-chan error { return l.done }
