package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
)

const capturedOutputLimit = 64 * 1024

type processSupervisor struct {
	mu       sync.Mutex
	children []*managedProcess
}

type managedProcess struct {
	cmd      *exec.Cmd
	exitCode int
	exited   bool
	mu       sync.Mutex
	done     chan struct{}
}

func newSupervisor() *processSupervisor {
	return &processSupervisor{}
}

func (s *processSupervisor) add(proc *managedProcess) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.children = append(s.children, proc)
}

func (s *processSupervisor) shutdown(reason string) {
	fmt.Printf("[kandev] shutting down (%s)...\n", reason)
	s.mu.Lock()
	children := append([]*managedProcess(nil), s.children...)
	s.mu.Unlock()
	for _, child := range children {
		child.kill()
	}
}

func (s *processSupervisor) attachSignals() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-ch
		s.shutdown("signal " + sig.String())
		os.Exit(0)
	}()
}

func startProcess(command string, args []string, cwd string, env []string, quiet bool, label string, supervisor *processSupervisor) (*managedProcess, func(), error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = cwd
	cmd.Env = env
	cmd.Stdin = nil
	stdout := newLimitedBuffer(capturedOutputLimit)
	if quiet {
		cmd.Stdout = stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	proc := &managedProcess{cmd: cmd, done: make(chan struct{})}
	supervisor.add(proc)
	go func() {
		err := cmd.Wait()
		code := 0
		if err != nil {
			code = 1
			if exitErr, ok := err.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			}
		}
		proc.mu.Lock()
		proc.exitCode = code
		proc.exited = true
		proc.mu.Unlock()
		close(proc.done)
		if label != "" && code != 0 {
			fmt.Fprintf(os.Stderr, "[kandev] %s exited (code=%d)\n", label, code)
		}
	}()
	return proc, func() {
		snapshot := stdout.Bytes()
		if len(snapshot) == 0 {
			return
		}
		fmt.Fprintln(os.Stderr, "[kandev] --- backend stdout (last captured output) ---")
		_, _ = os.Stderr.Write(snapshot)
		fmt.Fprintln(os.Stderr, "[kandev] --- end backend stdout ---")
	}, nil
}

type limitedBuffer struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.limit {
		b.buf = append([]byte(nil), b.buf[len(b.buf)-b.limit:]...)
	}
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf...)
}

func (p *managedProcess) Exited() (bool, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exited, p.exitCode
}

func (p *managedProcess) kill() {
	if p.cmd.Process == nil {
		return
	}
	_ = p.cmd.Process.Kill()
	<-p.done
}

func waitForAppExit(supervisor *processSupervisor, backend *restartableBackend) int {
	code := <-backend.exitCh
	supervisor.shutdown("backend exit")
	return code
}
