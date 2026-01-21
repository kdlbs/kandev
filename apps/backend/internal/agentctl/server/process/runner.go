package process

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

type StartProcessRequest struct {
	SessionID      string            `json:"session_id"`
	Kind           types.ProcessKind `json:"kind"`
	ScriptName     string            `json:"script_name,omitempty"`
	Command        string            `json:"command"`
	WorkingDir     string            `json:"working_dir"`
	Env            map[string]string `json:"env,omitempty"`
	BufferMaxBytes int64             `json:"buffer_max_bytes,omitempty"`
}

type StopProcessRequest struct {
	ProcessID string `json:"process_id"`
}

type ProcessInfo struct {
	ID         string               `json:"id"`
	SessionID  string               `json:"session_id"`
	Kind       types.ProcessKind    `json:"kind"`
	ScriptName string               `json:"script_name,omitempty"`
	Command    string               `json:"command"`
	WorkingDir string               `json:"working_dir"`
	Status     types.ProcessStatus  `json:"status"`
	ExitCode   *int                 `json:"exit_code,omitempty"`
	StartedAt  time.Time            `json:"started_at"`
	UpdatedAt  time.Time            `json:"updated_at"`
	Output     []ProcessOutputChunk `json:"output,omitempty"`
}

type ProcessOutputChunk struct {
	Stream    string    `json:"stream"`
	Data      string    `json:"data"`
	Timestamp time.Time `json:"timestamp"`
}

type ringBuffer struct {
	mu       sync.Mutex
	maxBytes int64
	size     int64
	chunks   []ProcessOutputChunk
}

func newRingBuffer(maxBytes int64) *ringBuffer {
	if maxBytes <= 0 {
		maxBytes = 2 * 1024 * 1024
	}
	return &ringBuffer{maxBytes: maxBytes}
}

func (b *ringBuffer) append(chunk ProcessOutputChunk) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.chunks = append(b.chunks, chunk)
	b.size += int64(len(chunk.Data))

	for b.size > b.maxBytes && len(b.chunks) > 0 {
		removed := b.chunks[0]
		b.size -= int64(len(removed.Data))
		b.chunks = b.chunks[1:]
	}
}

func (b *ringBuffer) snapshot() []ProcessOutputChunk {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]ProcessOutputChunk, len(b.chunks))
	copy(out, b.chunks)
	return out
}

type commandProcess struct {
	info       ProcessInfo
	cmd        *exec.Cmd
	buffer     *ringBuffer
	stopOnce   sync.Once
	stopSignal chan struct{}
	mu         sync.Mutex
}

type ProcessRunner struct {
	logger           *logger.Logger
	workspaceTracker *WorkspaceTracker
	bufferMaxBytes   int64

	mu        sync.RWMutex
	processes map[string]*commandProcess
}

func NewProcessRunner(workspaceTracker *WorkspaceTracker, log *logger.Logger, bufferMaxBytes int64) *ProcessRunner {
	return &ProcessRunner{
		logger:           log.WithFields(zap.String("component", "process-runner")),
		workspaceTracker: workspaceTracker,
		bufferMaxBytes:   bufferMaxBytes,
		processes:        make(map[string]*commandProcess),
	}
}

func (r *ProcessRunner) Start(ctx context.Context, req StartProcessRequest) (*ProcessInfo, error) {
	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	id := uuid.New().String()
	now := time.Now().UTC()

	cmd := exec.CommandContext(ctx, "sh", "-lc", req.Command)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}
	cmd.Env = mergeEnv(req.Env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to attach stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to attach stderr: %w", err)
	}

	bufferMaxBytes := req.BufferMaxBytes
	if bufferMaxBytes <= 0 {
		bufferMaxBytes = r.bufferMaxBytes
	}

	proc := &commandProcess{
		info: ProcessInfo{
			ID:         id,
			SessionID:  req.SessionID,
			Kind:       req.Kind,
			ScriptName: req.ScriptName,
			Command:    req.Command,
			WorkingDir: req.WorkingDir,
			Status:     types.ProcessStatusStarting,
			StartedAt:  now,
			UpdatedAt:  now,
		},
		cmd:        cmd,
		buffer:     newRingBuffer(bufferMaxBytes),
		stopSignal: make(chan struct{}),
	}

	r.mu.Lock()
	r.processes[id] = proc
	r.mu.Unlock()

	r.logger.Debug("process start requested",
		zap.String("process_id", id),
		zap.String("session_id", req.SessionID),
		zap.String("kind", string(req.Kind)),
		zap.String("script_name", req.ScriptName),
		zap.String("working_dir", req.WorkingDir),
	)

	r.publishStatus(proc)

	if err := cmd.Start(); err != nil {
		proc.mu.Lock()
		proc.info.Status = types.ProcessStatusFailed
		proc.info.UpdatedAt = time.Now().UTC()
		proc.mu.Unlock()
		r.publishStatus(proc)
		r.mu.Lock()
		delete(r.processes, id)
		r.mu.Unlock()
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	// Update status to running after successful start so callers can subscribe immediately.
	proc.mu.Lock()
	proc.info.Status = types.ProcessStatusRunning
	proc.info.UpdatedAt = time.Now().UTC()
	proc.mu.Unlock()
	r.publishStatus(proc)

	// Stream output and wait in the background; wait() is responsible for final status + cleanup.
	go r.readOutput(proc, stdout, "stdout")
	go r.readOutput(proc, stderr, "stderr")
	go r.wait(proc)

	info := proc.snapshot(false)
	return &info, nil
}

func (r *ProcessRunner) Stop(ctx context.Context, req StopProcessRequest) error {
	proc, ok := r.get(req.ProcessID)
	if !ok {
		return fmt.Errorf("process not found: %s", req.ProcessID)
	}

	// Signal output readers to exit before attempting to terminate the process.
	proc.stopOnce.Do(func() {
		close(proc.stopSignal)
	})

	// Attempt graceful shutdown, then escalate if the context expires.
	if proc.cmd != nil && proc.cmd.Process != nil {
		pgid, err := syscall.Getpgid(proc.cmd.Process.Pid)
		if err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
		} else {
			_ = proc.cmd.Process.Signal(syscall.SIGTERM)
		}
		select {
		case <-ctx.Done():
			if err == nil {
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			} else {
				_ = proc.cmd.Process.Kill()
			}
		case <-time.After(2 * time.Second):
			if err == nil {
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			} else {
				_ = proc.cmd.Process.Kill()
			}
		}
	}

	return nil
}

// StopAll stops all running processes managed by this runner.
func (r *ProcessRunner) StopAll(ctx context.Context) error {
	r.mu.RLock()
	ids := make([]string, 0, len(r.processes))
	for id := range r.processes {
		ids = append(ids, id)
	}
	r.mu.RUnlock()

	var errs []error
	for _, id := range ids {
		if err := r.Stop(ctx, StopProcessRequest{ProcessID: id}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *ProcessRunner) Get(id string, includeOutput bool) (*ProcessInfo, bool) {
	proc, ok := r.get(id)
	if !ok {
		return nil, false
	}
	info := proc.snapshot(includeOutput)
	return &info, true
}

func (r *ProcessRunner) List(sessionID string) []ProcessInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ProcessInfo, 0)
	for _, proc := range r.processes {
		if sessionID != "" && proc.info.SessionID != sessionID {
			continue
		}
		result = append(result, proc.snapshot(false))
	}
	return result
}

func (r *ProcessRunner) get(id string) (*commandProcess, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	proc, ok := r.processes[id]
	return proc, ok
}

func (r *ProcessRunner) readOutput(proc *commandProcess, reader io.ReadCloser, stream string) {
	defer func() { _ = reader.Close() }()
	buf := bufio.NewReader(reader)
	for {
		select {
		case <-proc.stopSignal:
			return
		default:
		}

		data := make([]byte, 4096)
		n, err := buf.Read(data)
		if n > 0 {
			chunk := ProcessOutputChunk{
				Stream:    stream,
				Data:      string(data[:n]),
				Timestamp: time.Now().UTC(),
			}
			proc.buffer.append(chunk)
			r.publishOutput(proc, chunk)
		}
		if err != nil {
			if err != io.EOF {
				r.logger.Debug("process output read error", zap.Error(err))
			}
			return
		}
	}
}

func (r *ProcessRunner) wait(proc *commandProcess) {
	err := proc.cmd.Wait()
	exitCode := 0
	status := types.ProcessStatusExited
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if waitStatus, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = waitStatus.ExitStatus()
			} else {
				exitCode = 1
			}
		} else {
			exitCode = 1
		}
		status = types.ProcessStatusFailed
	}
	// Final process status is computed here and published once.
	r.logger.Debug("process exited",
		zap.String("process_id", proc.info.ID),
		zap.String("session_id", proc.info.SessionID),
		zap.String("status", string(status)),
		zap.Int("exit_code", exitCode),
		zap.Error(err),
	)

	proc.mu.Lock()
	proc.info.Status = status
	proc.info.ExitCode = &exitCode
	proc.info.UpdatedAt = time.Now().UTC()
	proc.mu.Unlock()
	r.publishStatus(proc)

	// Remove from the runner map after completion so future lookups don't see stale processes.
	// Output is still preserved in the ring buffer returned by GetProcess(includeOutput=true)
	// as long as the caller already has the process ID.
	r.mu.Lock()
	delete(r.processes, proc.info.ID)
	r.mu.Unlock()
}

func (r *ProcessRunner) publishOutput(proc *commandProcess, chunk ProcessOutputChunk) {
	if r.workspaceTracker == nil {
		return
	}
	proc.mu.Lock()
	info := proc.info
	proc.mu.Unlock()

	output := &types.ProcessOutput{
		SessionID: info.SessionID,
		ProcessID: info.ID,
		Kind:      info.Kind,
		Stream:    chunk.Stream,
		Data:      chunk.Data,
		Timestamp: chunk.Timestamp,
	}
	r.workspaceTracker.notifyWorkspaceStreamProcessOutput(output)
}

func (r *ProcessRunner) publishStatus(proc *commandProcess) {
	if r.workspaceTracker == nil {
		return
	}
	proc.mu.Lock()
	info := proc.info
	proc.mu.Unlock()

	update := &types.ProcessStatusUpdate{
		SessionID:  info.SessionID,
		ProcessID:  info.ID,
		Kind:       info.Kind,
		ScriptName: info.ScriptName,
		Command:    info.Command,
		WorkingDir: info.WorkingDir,
		Status:     info.Status,
		ExitCode:   info.ExitCode,
		Timestamp:  time.Now().UTC(),
	}
	r.logger.Debug("process status update",
		zap.String("process_id", info.ID),
		zap.String("session_id", info.SessionID),
		zap.String("status", string(info.Status)),
	)
	r.workspaceTracker.notifyWorkspaceStreamProcessStatus(update)
}

func (p *commandProcess) snapshot(includeOutput bool) ProcessInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	info := p.info
	if includeOutput && p.buffer != nil {
		info.Output = p.buffer.snapshot()
	}
	return info
}

func mergeEnv(env map[string]string) []string {
	base := make(map[string]string, len(os.Environ())+len(env))
	for _, entry := range os.Environ() {
		if eq := strings.IndexByte(entry, '='); eq >= 0 {
			base[entry[:eq]] = entry[eq+1:]
		}
	}
	for k, v := range env {
		base[k] = v
	}
	merged := make([]string, 0, len(base))
	for k, v := range base {
		merged = append(merged, fmt.Sprintf("%s=%s", k, v))
	}
	return merged
}
