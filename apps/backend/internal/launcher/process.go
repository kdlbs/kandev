package launcher

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const capturedOutputLimit = 64 * 1024
const managedProcessShutdownGrace = 40 * time.Second

var launcherShutdownDebug atomic.Bool
var launcherStatusOutput io.Writer = os.Stderr

type processSupervisor struct {
	mu           sync.Mutex
	children     []*managedProcess
	shutdownOnce sync.Once
}

type managedProcess struct {
	label    string
	cmd      *exec.Cmd
	exitCode int
	exited   bool
	mu       sync.Mutex
	done     chan struct{}
}

type managedProcessShutdownResult struct {
	label       string
	pid         int
	duration    time.Duration
	graceful    bool
	forceKilled bool
	err         error
}

type shutdownSummary struct {
	graceful    int
	forceKilled int
	failed      int
}

func newSupervisor() *processSupervisor {
	return &processSupervisor{}
}

func setLauncherShutdownDebug(enabled bool) {
	launcherShutdownDebug.Store(enabled)
}

func shutdownDebugf(format string, args ...interface{}) {
	if !launcherShutdownDebug.Load() {
		return
	}
	fmt.Fprintf(os.Stderr, "[kandev] [SHUTDOWN-DEBUG] "+format+"\n", args...)
}

func launcherInfof(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(launcherStatusOutput, "[kandev] "+format+"\n", args...)
}

func (s *processSupervisor) add(proc *managedProcess) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.children = append(s.children, proc)
	shutdownDebugf("supervisor add child pid=%d total_children=%d", proc.cmd.Process.Pid, len(s.children))
}

func (s *processSupervisor) shutdown(reason string) {
	s.shutdownOnce.Do(func() {
		s.runShutdown(reason)
	})
}

func (s *processSupervisor) runShutdown(reason string) {
	s.mu.Lock()
	children := append([]*managedProcess(nil), s.children...)
	s.mu.Unlock()
	start := time.Now()
	launcherInfof("graceful shutdown started (reason=%s, timeout=%s, processes=%d)",
		reason, managedProcessShutdownGrace, len(children))
	shutdownDebugf("launcher shutdown begin reason=%q launcher_pid=%d children=%d", reason, os.Getpid(), len(children))
	results := make([]managedProcessShutdownResult, 0, len(children))
	for _, child := range children {
		results = append(results, child.kill())
	}
	logShutdownComplete(time.Since(start), results)
	shutdownDebugf("launcher shutdown complete reason=%q", reason)
}

func (s *processSupervisor) attachSignals() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	shutdownDebugf("launcher signal handler armed launcher_pid=%d", os.Getpid())
	go func() {
		sig := <-ch
		shutdownDebugf("launcher received signal=%s launcher_pid=%d", sig.String(), os.Getpid())
		s.shutdown("signal " + sig.String())
		os.Exit(0)
	}()
}

func startProcess(command string, args []string, cwd string, env []string, quiet bool, label string, supervisor *processSupervisor) (*managedProcess, func(), error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = cwd
	cmd.Env = env
	cmd.Stdin = nil
	configureManagedProcess(cmd)
	stdout := newLimitedBuffer(capturedOutputLimit)
	var stdoutSink io.Writer
	if !quiet {
		stdoutSink = os.Stdout
	}
	cmd.Stdout = newProcessOutput(stdout, stdoutSink, os.Stderr, label+".stdout")
	cmd.Stderr = newProcessOutput(nil, os.Stderr, nil, label+".stderr")
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	shutdownDebugf("managed process started label=%q pid=%d command=%q args=%q cwd=%q", label, cmd.Process.Pid, command, args, cwd)
	proc := &managedProcess{label: label, cmd: cmd, done: make(chan struct{})}
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
		state := ""
		if cmd.ProcessState != nil {
			state = cmd.ProcessState.String()
		}
		shutdownDebugf("managed process wait complete label=%q pid=%d code=%d state=%q err=%v", label, cmd.Process.Pid, code, state, err)
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

type processOutput struct {
	mu           sync.Mutex
	buffer       *limitedBuffer
	sink         io.Writer
	fallbackSink io.Writer
	label        string
	sinkDisabled bool
}

func newProcessOutput(buffer *limitedBuffer, sink io.Writer, fallbackSink io.Writer, label string) *processOutput {
	return &processOutput{buffer: buffer, sink: sink, fallbackSink: fallbackSink, label: label}
}

func (w *processOutput) Write(p []byte) (int, error) {
	if w.buffer != nil {
		_, _ = w.buffer.Write(p)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.sink == nil || w.sinkDisabled {
		return len(p), nil
	}
	if _, err := w.sink.Write(p); err != nil {
		if w.fallbackSink != nil && w.fallbackSink != w.sink {
			shutdownDebugf("switching broken output sink to fallback label=%q err=%v", w.label, err)
			w.sink = w.fallbackSink
			w.fallbackSink = nil
			var fallbackErr error
			if _, fallbackErr = w.sink.Write(p); fallbackErr == nil {
				return len(p), nil
			}
			err = fallbackErr
		}
		w.sinkDisabled = true
		shutdownDebugf("disabling broken output sink label=%q err=%v", w.label, err)
	}
	return len(p), nil
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

func (p *managedProcess) kill() managedProcessShutdownResult {
	start := time.Now()
	result := managedProcessShutdownResult{label: p.label}
	if p.cmd.Process == nil {
		shutdownDebugf("managed process kill skipped: process nil")
		result.err = fmt.Errorf("process is nil")
		return result
	}
	pid := p.cmd.Process.Pid
	result.pid = pid
	shutdownDebugf("managed process kill begin label=%q pid=%d grace=%s", p.label, pid, managedProcessShutdownGrace)
	shutdownDebugf("managed process group SIGTERM requested label=%q pgid=%d", p.label, pid)
	if err := terminateManagedProcessGroup(pid); err != nil {
		shutdownDebugf("managed process group SIGTERM failed pid=%d err=%v; killing process", pid, err)
		shutdownDebugf("managed process SIGKILL requested label=%q pid=%d reason=%q", p.label, pid, "sigterm_failed")
		_ = p.cmd.Process.Kill()
		<-p.done
		result.duration = time.Since(start)
		result.forceKilled = true
		result.err = err
		shutdownDebugf("managed process killed after SIGTERM failure pid=%d", pid)
		return result
	}
	shutdownDebugf("managed process group SIGTERM sent pgid=%d", pid)
	select {
	case <-p.done:
		result.duration = time.Since(start)
		result.graceful = true
		shutdownDebugf("managed process exited within grace pid=%d", pid)
		return result
	case <-time.After(managedProcessShutdownGrace):
	}
	shutdownDebugf("managed process grace expired; sending SIGKILL pgid=%d", pid)
	result.forceKilled = true
	if err := killManagedProcessGroup(pid); err != nil {
		shutdownDebugf("managed process group SIGKILL failed pid=%d err=%v; killing process", pid, err)
		shutdownDebugf("managed process SIGKILL requested label=%q pid=%d reason=%q", p.label, pid, "process_group_kill_failed")
		_ = p.cmd.Process.Kill()
		result.err = err
	}
	<-p.done
	result.duration = time.Since(start)
	shutdownDebugf("managed process killed after grace pid=%d", pid)
	return result
}

func waitForAppExit(supervisor *processSupervisor, backend *restartableBackend) int {
	code := <-backend.exitCh
	supervisor.shutdown("backend exit")
	return code
}

func logShutdownComplete(duration time.Duration, results []managedProcessShutdownResult) {
	summary := summarizeShutdown(results)
	launcherInfof("graceful shutdown complete (duration=%s, graceful=%d, force_killed=%d, failed=%d)",
		duration.Round(time.Millisecond), summary.graceful, summary.forceKilled, summary.failed)
	for _, result := range results {
		if result.forceKilled || result.err != nil {
			label := result.label
			if label == "" {
				label = "process"
			}
			if result.err != nil {
				launcherInfof("shutdown detail: %s pid=%d required force cleanup after %s: %v",
					label, result.pid, result.duration.Round(time.Millisecond), result.err)
				continue
			}
			launcherInfof("shutdown detail: %s pid=%d required SIGKILL after %s",
				label, result.pid, result.duration.Round(time.Millisecond))
		}
	}
}

func summarizeShutdown(results []managedProcessShutdownResult) shutdownSummary {
	var summary shutdownSummary
	for _, result := range results {
		if result.forceKilled {
			summary.forceKilled++
		}
		if result.err != nil {
			summary.failed++
		}
		if result.graceful {
			summary.graceful++
		}
	}
	return summary
}
