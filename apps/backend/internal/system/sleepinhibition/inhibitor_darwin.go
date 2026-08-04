//go:build darwin

package sleepinhibition

import (
	"context"
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
	process   darwinProcess
	waitDone  chan error
	done      chan error
	releaseOn sync.Once
	waitOnce  sync.Once
}

func newDarwinLease(process darwinProcess) *darwinLease {
	lease := &darwinLease{
		process:  process,
		waitDone: make(chan error, 1),
		done:     make(chan error, 1),
	}
	go func() {
		err := process.Wait()
		lease.waitDone <- err
		lease.done <- err
		close(lease.done)
	}()
	return lease
}

func (l *darwinLease) Release() error {
	l.releaseOn.Do(func() {
		_ = l.process.Kill()
	})
	var err error
	l.waitOnce.Do(func() { err = <-l.waitDone })
	return err
}

func (l *darwinLease) Done() <-chan error { return l.done }
