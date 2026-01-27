// Package process provides background process execution and output streaming for agentctl.
//
// The ProcessRunner manages long-running background processes spawned by agents or users,
// typically shell scripts, build commands, or other tools that need to run independently
// of the agent's main execution flow.
//
// Key Features:
//   - Process isolation via process groups (Setpgid) for clean cleanup
//   - Memory-bounded output buffering via ring buffers (prevents OOM on large outputs)
//   - Real-time output streaming via WebSocket to frontend
//   - Graceful shutdown with SIGTERM → SIGKILL escalation
//   - Per-session process tracking
//
// Lifecycle:
//   1. Start() - Spawns process, creates output pipes, returns immediately
//   2. Background goroutines - Stream stdout/stderr, wait for exit
//   3. Stop() - Sends SIGTERM to process group, escalates to SIGKILL after 2s
//   4. Automatic cleanup - Process removed from tracking after exit
//
// Use Cases:
//   - Agent-triggered builds (npm run build, go test ./...)
//   - Background monitoring scripts (tail -f logs, watch commands)
//   - Interactive shells (if needed, though typically handled separately)
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

// StartProcessRequest contains parameters for starting a new background process.
type StartProcessRequest struct {
	SessionID      string            `json:"session_id"`       // Required: Agent session owning this process
	Kind           types.ProcessKind `json:"kind"`             // Process type (user_command, agent_script, etc.)
	ScriptName     string            `json:"script_name,omitempty"` // Human-readable script identifier
	Command        string            `json:"command"`          // Required: Shell command to execute (passed to sh -lc)
	WorkingDir     string            `json:"working_dir"`      // Working directory (defaults to current dir)
	Env            map[string]string `json:"env,omitempty"`    // Additional environment variables (merged with parent env)
	BufferMaxBytes int64             `json:"buffer_max_bytes,omitempty"` // Max output buffer size (defaults to runner's default)
}

// StopProcessRequest identifies a process to stop.
type StopProcessRequest struct {
	ProcessID string `json:"process_id"` // The process UUID returned by Start()
}

// ProcessInfo represents the complete state of a background process.
type ProcessInfo struct {
	ID         string               `json:"id"`          // Unique process identifier (UUID)
	SessionID  string               `json:"session_id"`  // Agent session that owns this process
	Kind       types.ProcessKind    `json:"kind"`        // Process classification
	ScriptName string               `json:"script_name,omitempty"` // User-friendly name
	Command    string               `json:"command"`     // The shell command being executed
	WorkingDir string               `json:"working_dir"` // Execution directory
	Status     types.ProcessStatus  `json:"status"`      // Current state (starting, running, exited, failed)
	ExitCode   *int                 `json:"exit_code,omitempty"` // Process exit code (nil if still running)
	StartedAt  time.Time            `json:"started_at"`  // When the process was launched
	UpdatedAt  time.Time            `json:"updated_at"`  // Last status change timestamp
	Output     []ProcessOutputChunk `json:"output,omitempty"` // Buffered output (only included if requested)
}

// ProcessOutputChunk represents a single piece of process output (stdout or stderr).
type ProcessOutputChunk struct {
	Stream    string    `json:"stream"`    // "stdout" or "stderr"
	Data      string    `json:"data"`      // Raw output bytes as string
	Timestamp time.Time `json:"timestamp"` // When this chunk was captured
}

// ringBuffer provides memory-bounded FIFO storage for process output.
//
// This prevents out-of-memory errors when processes produce large amounts of output
// (e.g., verbose build logs, long-running monitoring scripts). When the buffer exceeds
// maxBytes, the oldest chunks are automatically evicted.
//
// Thread-safe: All methods use mutex protection.
//
// Example: With maxBytes=2MB, a process that outputs 10MB will only keep the most
// recent ~2MB in memory. Clients can stream output in real-time via WebSocket without
// loading the entire history.
type ringBuffer struct {
	mu       sync.Mutex           // Protects chunks and size
	maxBytes int64                // Maximum total bytes to keep in memory
	size     int64                // Current total bytes stored
	chunks   []ProcessOutputChunk // FIFO queue of output chunks
}

// newRingBuffer creates a ring buffer with the specified size limit.
// Defaults to 2MB if maxBytes <= 0.
func newRingBuffer(maxBytes int64) *ringBuffer {
	if maxBytes <= 0 {
		maxBytes = 2 * 1024 * 1024 // 2MB default
	}
	return &ringBuffer{maxBytes: maxBytes}
}

// append adds a new output chunk, evicting oldest chunks if over the size limit.
func (b *ringBuffer) append(chunk ProcessOutputChunk) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.chunks = append(b.chunks, chunk)
	b.size += int64(len(chunk.Data))

	// Evict oldest chunks until we're back under the limit
	for b.size > b.maxBytes && len(b.chunks) > 0 {
		removed := b.chunks[0]
		b.size -= int64(len(removed.Data))
		b.chunks = b.chunks[1:]
	}
}

// snapshot returns a copy of all buffered chunks at the current moment.
// Safe to call concurrently with append().
func (b *ringBuffer) snapshot() []ProcessOutputChunk {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]ProcessOutputChunk, len(b.chunks))
	copy(out, b.chunks)
	return out
}

// commandProcess represents a single running background process and its state.
type commandProcess struct {
	info       ProcessInfo      // Process metadata and current status
	cmd        *exec.Cmd        // Underlying OS process (nil after completion)
	buffer     *ringBuffer      // Memory-bounded output storage
	stopOnce   sync.Once        // Ensures stopSignal is only closed once
	stopSignal chan struct{}    // Signals output readers to exit before process termination
	mu         sync.Mutex       // Protects info fields during updates
}

// ProcessRunner manages multiple background processes with output streaming.
//
// Thread-safe: All public methods can be called concurrently from multiple goroutines.
//
// Process Lifecycle:
//   1. Start() creates a process in "starting" state
//   2. Transitions to "running" after successful spawn
//   3. Background goroutines stream output and wait for exit
//   4. Final status is "exited" (exit code 0) or "failed" (non-zero exit code)
//   5. Process is automatically removed from tracking after completion
//
// Output Handling:
//   - Stdout and stderr are captured separately and streamed to WebSocket
//   - Output is buffered in memory-bounded ring buffers (prevents OOM)
//   - Get(id, includeOutput=true) retrieves buffered output for a process
//
// Process Groups:
//   - Processes are spawned with Setpgid=true to create new process groups
//   - Stop() kills the entire process group (handles child processes correctly)
//   - This ensures clean cleanup even if the process spawns subprocesses
type ProcessRunner struct {
	logger           *logger.Logger    // Scoped logger for this component
	workspaceTracker *WorkspaceTracker // WebSocket stream coordinator (can be nil)
	bufferMaxBytes   int64             // Default output buffer size for new processes

	mu        sync.RWMutex                  // Protects processes map
	processes map[string]*commandProcess    // Active processes by ID
}

// NewProcessRunner creates a new process runner.
//
// Parameters:
//   - workspaceTracker: Optional coordinator for streaming output to WebSocket clients.
//                       If nil, processes run but output isn't streamed (only buffered).
//   - log: Logger instance for process lifecycle events
//   - bufferMaxBytes: Default output buffer size (typically 2MB). Individual processes
//                     can override via StartProcessRequest.BufferMaxBytes.
func NewProcessRunner(workspaceTracker *WorkspaceTracker, log *logger.Logger, bufferMaxBytes int64) *ProcessRunner {
	return &ProcessRunner{
		logger:           log.WithFields(zap.String("component", "process-runner")),
		workspaceTracker: workspaceTracker,
		bufferMaxBytes:   bufferMaxBytes,
		processes:        make(map[string]*commandProcess),
	}
}

// Start spawns a new background process and returns immediately.
//
// The process is executed via "sh -lc <command>", which:
//   - Loads shell profile (~/.bashrc, ~/.zshrc) for environment setup (-l flag)
//   - Allows complex shell syntax like pipes, redirects, variable expansion (-c flag)
//
// Process Isolation:
//   - Creates new process group (Setpgid=true) for clean shutdown
//   - Children inherit the process group, so Stop() kills the entire tree
//
// Async Operation:
//   - Returns immediately after spawning (doesn't wait for completion)
//   - Three background goroutines handle:
//     1. stdout streaming → readOutput(stdout)
//     2. stderr streaming → readOutput(stderr)
//     3. exit monitoring → wait()
//
// Output Streaming:
//   - stdout/stderr are captured in real-time
//   - Chunks are buffered in memory (ring buffer with size limit)
//   - Chunks are streamed to WebSocket clients (if workspaceTracker set)
//
// State Transitions:
//   - Initial: "starting" (process being spawned)
//   - Success: "running" (process started successfully)
//   - Failure: "failed" (spawn failed - removed from tracking)
//
// Returns:
//   - ProcessInfo with "running" status and unique process ID
//   - Error if validation fails or process spawn fails
func (r *ProcessRunner) Start(ctx context.Context, req StartProcessRequest) (*ProcessInfo, error) {
	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	id := uuid.New().String()
	now := time.Now().UTC()

	// Use "sh -lc" to execute command with login shell environment
	cmd := exec.CommandContext(ctx, "sh", "-lc", req.Command)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}
	cmd.Env = mergeEnv(req.Env)
	// Create new process group for clean shutdown (allows killing entire subprocess tree)
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

// Stop attempts to gracefully terminate a process, escalating to force-kill if needed.
//
// Shutdown Strategy (two-phase):
//
//  1. Graceful Phase (SIGTERM):
//     - Sends SIGTERM to the entire process group (negative PGID)
//     - Allows processes to clean up resources, flush buffers, etc.
//     - Waits up to 2 seconds for graceful exit
//
//  2. Force Phase (SIGKILL):
//     - If process hasn't exited after 2 seconds (or ctx expires), sends SIGKILL
//     - SIGKILL cannot be caught/ignored - guarantees termination
//     - Also sent to entire process group
//
// Process Group Termination:
//   - Kill(-pgid, signal) sends signal to all processes in the group
//   - This ensures child processes are also terminated
//   - Example: "npm run build" spawns webpack, which spawns terser, etc.
//   - Fallback: If process group lookup fails, kills only the main process
//
// Output Reader Cleanup:
//   - Closes stopSignal channel before killing process
//   - Signals readOutput() goroutines to exit cleanly
//   - Prevents goroutine leaks from blocked pipe reads
//
// Behavior:
//   - Returns immediately without waiting for process exit
//   - wait() goroutine detects exit and updates final status
//   - Process is automatically removed from tracking after exit
//
// Returns error only if process not found (already exited or invalid ID).
func (r *ProcessRunner) Stop(ctx context.Context, req StopProcessRequest) error {
	proc, ok := r.get(req.ProcessID)
	if !ok {
		return fmt.Errorf("process not found: %s", req.ProcessID)
	}

	// Signal output readers to exit before attempting to terminate the process.
	// This prevents goroutine leaks from blocked pipe reads.
	proc.stopOnce.Do(func() {
		close(proc.stopSignal)
	})

	// Attempt graceful shutdown (SIGTERM), then escalate to force-kill (SIGKILL)
	if proc.cmd != nil && proc.cmd.Process != nil {
		// Try to get process group ID for killing entire subprocess tree
		pgid, err := syscall.Getpgid(proc.cmd.Process.Pid)

		// Phase 1: Graceful shutdown with SIGTERM
		if err == nil {
			// Kill entire process group (negative PGID)
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
		} else {
			// Fallback: Kill only main process
			_ = proc.cmd.Process.Signal(syscall.SIGTERM)
		}

		// Wait for graceful exit (2 seconds) or context cancellation
		select {
		case <-ctx.Done():
			// Context cancelled - force kill immediately
			if err == nil {
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			} else {
				_ = proc.cmd.Process.Kill()
			}
		case <-time.After(2 * time.Second):
			// Phase 2: Grace period expired - force kill with SIGKILL
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
//
// Iterates through all active processes and calls Stop() on each.
// Errors are collected and returned as a joined error (errors.Join).
//
// Typical usage: Shutdown cleanup when agentctl server is stopping.
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

// Get retrieves process information by ID.
//
// Parameters:
//   - id: Process UUID returned by Start()
//   - includeOutput: If true, includes buffered stdout/stderr in the response.
//                    Set to false for status checks to avoid copying large buffers.
//
// Returns (ProcessInfo, true) if found, or (nil, false) if not found.
//
// Note: Completed processes are automatically removed from tracking, so Get()
// will return false for processes that have finished and been cleaned up.
func (r *ProcessRunner) Get(id string, includeOutput bool) (*ProcessInfo, bool) {
	proc, ok := r.get(id)
	if !ok {
		return nil, false
	}
	info := proc.snapshot(includeOutput)
	return &info, true
}

// List returns all active processes, optionally filtered by session.
//
// Parameters:
//   - sessionID: If non-empty, returns only processes for this session.
//                If empty, returns all processes across all sessions.
//
// Output is always returned without buffered output (includeOutput=false).
// Use Get(id, true) to retrieve output for a specific process.
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

// wait blocks until the process exits, then updates final status and cleans up.
//
// This runs in a background goroutine spawned by Start(). Responsibilities:
//   1. Wait for process termination (blocks on cmd.Wait())
//   2. Extract exit code from process state
//   3. Determine final status: "exited" (code 0) vs "failed" (non-zero)
//   4. Publish final status update to WebSocket clients
//   5. Remove process from tracking map (makes ID unavailable for future lookups)
//
// Exit Code Extraction:
//   - Success (exit code 0): Status = "exited"
//   - Failure (exit code != 0): Status = "failed"
//   - If exit code cannot be determined: Defaults to 1
//
// Cleanup Strategy:
//   - Process is removed from r.processes after exit
//   - This prevents Get() from returning stale/completed processes
//   - Output is lost after removal (not persisted beyond memory)
//   - Clients should stream output in real-time or call Get(id, true) before exit
//
// Goroutine Safety:
//   - Each process has exactly one wait() goroutine
//   - wait() is the sole authority for final status updates
//   - Cleanup happens exactly once (no race with Stop())
func (r *ProcessRunner) wait(proc *commandProcess) {
	err := proc.cmd.Wait()
	exitCode := 0
	status := types.ProcessStatusExited
	if err != nil {
		// Extract exit code from error
		if exitErr, ok := err.(*exec.ExitError); ok {
			if waitStatus, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = waitStatus.ExitStatus()
			} else {
				exitCode = 1 // Fallback if status extraction fails
			}
		} else {
			exitCode = 1 // Non-ExitError (process killed, etc.)
		}
		status = types.ProcessStatusFailed
	}

	r.logger.Debug("process exited",
		zap.String("process_id", proc.info.ID),
		zap.String("session_id", proc.info.SessionID),
		zap.String("status", string(status)),
		zap.Int("exit_code", exitCode),
		zap.Error(err),
	)

	// Update process info with final status
	proc.mu.Lock()
	proc.info.Status = status
	proc.info.ExitCode = &exitCode
	proc.info.UpdatedAt = time.Now().UTC()
	proc.mu.Unlock()

	// Publish final status to WebSocket clients
	r.publishStatus(proc)

	// Remove from tracking map - process is no longer queryable
	// Output buffer is lost after this point (not persisted)
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

// snapshot creates a thread-safe copy of process info at the current moment.
//
// Parameters:
//   - includeOutput: If true, includes buffered output chunks (can be large).
//                    If false, returns info without output (faster, smaller).
//
// Thread-safe: Locks process mutex to prevent races with status updates.
func (p *commandProcess) snapshot(includeOutput bool) ProcessInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	info := p.info
	if includeOutput && p.buffer != nil {
		info.Output = p.buffer.snapshot()
	}
	return info
}

// mergeEnv merges custom environment variables with the parent process environment.
//
// Strategy:
//   1. Start with all environment variables from os.Environ() (parent process)
//   2. Override/add variables from the env map (custom variables)
//   3. Return as []string in "KEY=VALUE" format (required by exec.Cmd.Env)
//
// Example:
//   Parent env: PATH=/usr/bin, HOME=/home/user
//   Custom env: PATH=/custom/bin, FOO=bar
//   Result:     PATH=/custom/bin, HOME=/home/user, FOO=bar
//
// This allows processes to inherit the agentctl server's environment (including PATH,
// HOME, etc.) while also supporting custom variables per process.
func mergeEnv(env map[string]string) []string {
	base := make(map[string]string, len(os.Environ())+len(env))

	// Parse parent environment into map, filtering out npm config variables
	// that cause warnings when passed to npx commands
	for _, entry := range os.Environ() {
		if eq := strings.IndexByte(entry, '='); eq >= 0 {
			key := entry[:eq]
			// Filter out npm-related environment variables that cause warnings
			if isNpmEnvVar(key) {
				continue
			}
			base[key] = entry[eq+1:]
		}
	}

	// Override with custom variables
	for k, v := range env {
		base[k] = v
	}

	// Convert back to []string format
	merged := make([]string, 0, len(base))
	for k, v := range base {
		merged = append(merged, fmt.Sprintf("%s=%s", k, v))
	}
	return merged
}

// isNpmEnvVar returns true if the key is an npm-related environment variable
// that should be filtered out to prevent warnings in npx commands.
func isNpmEnvVar(key string) bool {
	npmPrefixes := []string{
		"npm_config_",
		"npm_package_",
		"npm_lifecycle_",
		"npm_execpath",
		"npm_node_execpath",
	}
	for _, prefix := range npmPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
