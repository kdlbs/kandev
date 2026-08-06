//go:build darwin

package sleepinhibition

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestDarwinInhibitorStartsCaffeinateWithSystemSleepFlags(t *testing.T) {
	process := newFakeDarwinProcess(errors.New("signal: killed"))
	var name string
	var args []string
	inhibitor := newDarwinInhibitor(func(gotName string, gotArgs ...string) darwinProcess {
		name = gotName
		args = append([]string(nil), gotArgs...)
		return process
	})

	lease, err := inhibitor.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if name != "/usr/bin/caffeinate" {
		t.Fatalf("command = %q", name)
	}
	wantArgs := []string{"-i", "-w", strconv.Itoa(os.Getpid())}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("repeated release: %v", err)
	}
	if got := process.killCount(); got != 1 {
		t.Fatalf("kill count = %d, want 1", got)
	}
	select {
	case err := <-lease.Done():
		if err != nil {
			t.Fatalf("intentional release reported error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("lease did not complete after release")
	}
}

func TestDarwinInhibitorMapsStartFailure(t *testing.T) {
	inhibitor := newDarwinInhibitor(func(string, ...string) darwinProcess {
		process := newFakeDarwinProcess(nil)
		process.startErr = errors.New("start failed")
		return process
	})
	_, err := inhibitor.Acquire(context.Background())
	if err == nil || IssueFromError(err) != IssueRequestFailed {
		t.Fatalf("acquire error = %v, want request_failed", err)
	}
}

func TestDarwinLeaseReportsUnexpectedExit(t *testing.T) {
	process := newFakeDarwinProcess(errors.New("caffeinate exited"))
	lease, err := newDarwinInhibitor(func(string, ...string) darwinProcess { return process }).Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	process.exit()

	select {
	case err := <-lease.Done():
		if err == nil || !errors.Is(err, process.waitErr) {
			t.Fatalf("unexpected exit error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("lease did not report unexpected exit")
	}
	if err := lease.Release(); err == nil {
		t.Fatal("release after unexpected exit returned nil")
	}
}

type fakeDarwinProcess struct {
	mu       sync.Mutex
	startErr error
	waitErr  error
	killErr  error
	waitCh   chan struct{}
	exited   bool
	kills    int
	once     sync.Once
}

func newFakeDarwinProcess(waitErr error) *fakeDarwinProcess {
	return &fakeDarwinProcess{waitErr: waitErr, waitCh: make(chan struct{})}
}

func (p *fakeDarwinProcess) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.startErr
}

func (p *fakeDarwinProcess) Wait() error {
	<-p.waitCh
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.waitErr
}

func (p *fakeDarwinProcess) Kill() error {
	p.mu.Lock()
	p.kills++
	if p.exited {
		p.mu.Unlock()
		return os.ErrProcessDone
	}
	p.exited = true
	killErr := p.killErr
	p.mu.Unlock()
	p.once.Do(func() { close(p.waitCh) })
	return killErr
}

func (p *fakeDarwinProcess) exit() {
	p.mu.Lock()
	p.exited = true
	p.mu.Unlock()
	p.once.Do(func() { close(p.waitCh) })
}

func (p *fakeDarwinProcess) killCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.kills
}
