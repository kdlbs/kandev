package process

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

// interactiveProcess represents a running interactive PTY process.
type interactiveProcess struct {
	info   InteractiveProcessInfo
	cmd    *exec.Cmd
	ptmx   PtyHandle // PTY handle (Unix: creack/pty, Windows: ConPTY)
	buffer *ringBuffer

	// Turn detection
	promptPattern *regexp.Regexp
	idleTimeout   time.Duration
	idleTimer     *time.Timer
	idleTimerMu   sync.Mutex

	// Status tracking (vt10x-based TUI detection)
	statusTracker *StatusTracker
	lastState     AgentState

	// User shell flag - when true, process is excluded from session-level lookups
	// (ResizeBySession, GetPtyWriterBySession) to prevent conflicts with passthrough processes
	isUserShell bool

	// Deferred start - process created lazily on first resize
	// This ensures PTY is created at exact frontend dimensions
	started   bool
	startOnce sync.Once
	startCmd  []string
	startDir  string
	startEnv  map[string]string
	startReq  InteractiveStartRequest // Full request for deferred initialization

	// Direct output - when set, raw output goes here instead of event bus
	directOutput   DirectOutputWriter
	directOutputMu sync.RWMutex

	// WebSocket tracking - tracks whether a WebSocket is actively connected
	hasActiveWebSocket bool

	// Lifecycle
	stopOnce   sync.Once
	stopSignal chan struct{}
	waitDone   chan struct{} // closed when wait() returns (cmd.Wait completed)
	mu         sync.Mutex
}

// Start creates an interactive process entry and defers PTY creation until first resize.
// This ensures the PTY is created at exact frontend dimensions, preventing redraw issues.
func (r *InteractiveRunner) Start(ctx context.Context, req InteractiveStartRequest) (*InteractiveProcessInfo, error) {
	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if len(req.Command) == 0 {
		return nil, fmt.Errorf("command is required")
	}

	id := uuid.New().String()
	now := time.Now().UTC()

	bufferMaxBytes := req.BufferMaxBytes
	if bufferMaxBytes <= 0 {
		bufferMaxBytes = r.bufferMaxBytes
	}

	// Compile prompt pattern if provided
	var promptPattern *regexp.Regexp
	if req.PromptPattern != "" {
		var compileErr error
		promptPattern, compileErr = regexp.Compile(req.PromptPattern)
		if compileErr != nil {
			r.logger.Warn("failed to compile prompt pattern, turn detection may not work",
				zap.String("pattern", req.PromptPattern),
				zap.Error(compileErr))
		}
	}

	var idleTimeout time.Duration
	if req.DisableTurnDetection {
		idleTimeout = 0 // No idle timer for user shell terminals
	} else {
		idleTimeout = req.IdleTimeout
		if idleTimeout <= 0 {
			idleTimeout = 5 * time.Second // Default 5 seconds
		}
	}

	// Create process struct WITHOUT spawning PTY yet
	// PTY will be created on first resize when we know the exact dimensions
	proc := &interactiveProcess{
		info: InteractiveProcessInfo{
			ID:         id,
			SessionID:  req.SessionID,
			Command:    req.Command,
			WorkingDir: req.WorkingDir,
			Status:     types.ProcessStatusRunning,
			StartedAt:  now,
			UpdatedAt:  now,
		},
		buffer:        newRingBuffer(bufferMaxBytes),
		promptPattern: promptPattern,
		idleTimeout:   idleTimeout,
		lastState:     StateUnknown,
		isUserShell:   req.IsUserShell,
		stopSignal:    make(chan struct{}),
		waitDone:      make(chan struct{}),
		// Store start parameters for deferred initialization
		started:  false,
		startCmd: req.Command,
		startDir: req.WorkingDir,
		startEnv: req.Env,
		startReq: req,
	}

	r.mu.Lock()
	r.processes[id] = proc
	r.mu.Unlock()

	// If immediate start is requested, start with default dimensions
	if req.ImmediateStart {
		if err := r.immediateStartProcess(req, proc, id); err != nil {
			return nil, err
		}
	} else {
		r.logger.Info("interactive process created (waiting for terminal dimensions)",
			zap.String("process_id", id),
			zap.String("session_id", req.SessionID),
			zap.Strings("command", req.Command),
			zap.String("working_dir", req.WorkingDir),
		)
	}

	r.publishStatus(proc)

	info := proc.snapshot(false)
	return &info, nil
}

// immediateStartProcess starts the PTY process immediately using default or provided dimensions.
func (r *InteractiveRunner) immediateStartProcess(req InteractiveStartRequest, proc *interactiveProcess, id string) error {
	cols := req.DefaultCols
	rows := req.DefaultRows

	// Prefer last known session dimensions from previous resize events.
	// This ensures restarted processes use the correct terminal size
	// instead of the 120x40 defaults.
	r.sessionWsMu.RLock()
	sessWs, exists := r.sessionWs[req.SessionID]
	r.sessionWsMu.RUnlock()
	if exists && sessWs != nil {
		sessWs.mu.RLock()
		if sessWs.lastCols > 0 && sessWs.lastRows > 0 {
			cols = int(sessWs.lastCols)
			rows = int(sessWs.lastRows)
		}
		sessWs.mu.RUnlock()
	}

	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 40
	}
	var startErr error
	proc.startOnce.Do(func() {
		r.logger.Info("immediate start - starting process with default dimensions",
			zap.String("process_id", id),
			zap.String("session_id", req.SessionID),
			zap.Int("cols", cols),
			zap.Int("rows", rows))
		startErr = r.startProcess(proc, cols, rows)
	})
	if startErr != nil {
		r.mu.Lock()
		delete(r.processes, id)
		r.mu.Unlock()
		return fmt.Errorf("failed to start process: %w", startErr)
	}
	r.logger.Info("interactive process started immediately",
		zap.String("process_id", id),
		zap.String("session_id", req.SessionID),
		zap.Strings("command", req.Command),
		zap.String("working_dir", req.WorkingDir),
	)
	return nil
}

// startProcess actually spawns the PTY and process. Called on first resize.
func (r *InteractiveRunner) startProcess(proc *interactiveProcess, cols, rows int) error {
	req := proc.startReq

	// Build command - use Background context so the process lives beyond the request
	// The process lifecycle is managed by Stop() and wait(), not by context cancellation
	cmd := exec.Command(proc.startCmd[0], proc.startCmd[1:]...)
	if proc.startDir != "" {
		cmd.Dir = proc.startDir
	}
	cmd.Env = mergeEnv(proc.startEnv)
	// Note: Do NOT set Setpgid when using PTY - it conflicts with terminal control
	// The PTY session handles process group management

	// Start process in PTY with exact dimensions from frontend
	// Unix: creack/pty, Windows: ConPTY
	ptmx, err := startPTYWithSize(cmd, cols, rows)
	if err != nil {
		return fmt.Errorf("failed to start pty: %w", err)
	}

	// Create status tracker if a detector is configured
	var statusTracker *StatusTracker
	if req.StatusDetector != "" {
		detector := createStatusDetector()
		config := StatusTrackerConfig{
			Rows:            rows,
			Cols:            cols,
			CheckInterval:   req.CheckInterval,
			StabilityWindow: req.StabilityWindow,
		}
		if config.CheckInterval <= 0 {
			config.CheckInterval = 100 * time.Millisecond
		}
		// Create callback that will invoke the runner's state callback
		stateCallback := func(sessionID string, state AgentState) {
			if r.stateCallback != nil {
				r.stateCallback(sessionID, state)
			}
		}
		statusTracker = NewStatusTracker(req.SessionID, detector, stateCallback, config, r.logger)
		r.logger.Debug("status tracker created",
			zap.String("session_id", req.SessionID),
			zap.String("detector", req.StatusDetector))
	}

	proc.mu.Lock()
	proc.ptmx = ptmx
	proc.cmd = cmd
	proc.statusTracker = statusTracker
	proc.started = true
	proc.mu.Unlock()

	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	r.logger.Info("interactive process started at exact dimensions",
		zap.String("process_id", proc.info.ID),
		zap.String("session_id", proc.info.SessionID),
		zap.Int("cols", cols),
		zap.Int("rows", rows),
		zap.Int("pid", pid),
	)

	// Start output reading and process waiting goroutines
	go r.readOutput(proc)
	go r.wait(proc)

	// If an initial command was provided, write it to the PTY after a short delay
	// to allow the shell to initialize and display its prompt
	if req.InitialCommand != "" {
		go func() {
			time.Sleep(100 * time.Millisecond)
			proc.mu.Lock()
			pty := proc.ptmx
			proc.mu.Unlock()
			if pty != nil {
				_, err := pty.Write([]byte(req.InitialCommand + "\n"))
				if err != nil {
					r.logger.Warn("failed to write initial command to PTY",
						zap.String("process_id", proc.info.ID),
						zap.Error(err))
				} else {
					r.logger.Debug("wrote initial command to PTY",
						zap.String("process_id", proc.info.ID),
						zap.String("command", req.InitialCommand))
				}
			}
		}()
	}

	return nil
}

// WriteStdin writes data to the process stdin (through PTY).
func (r *InteractiveRunner) WriteStdin(processID string, data string) error {
	proc, ok := r.get(processID)
	if !ok {
		return fmt.Errorf("process not found: %s", processID)
	}

	proc.mu.Lock()
	started := proc.started
	ptyInstance := proc.ptmx
	proc.mu.Unlock()

	if !started {
		return fmt.Errorf("process not started yet - waiting for terminal dimensions")
	}

	if ptyInstance == nil {
		return fmt.Errorf("process stdin not available")
	}

	_, err := ptyInstance.Write([]byte(data))
	if err != nil {
		return fmt.Errorf("failed to write to stdin: %w", err)
	}

	// Reset idle timer when user sends input
	r.resetIdleTimer(proc)

	return nil
}

// Stop terminates an interactive process.
func (r *InteractiveRunner) Stop(ctx context.Context, processID string) error {
	proc, ok := r.get(processID)
	if !ok {
		return fmt.Errorf("process not found: %s", processID)
	}

	// Signal output reader to exit
	proc.stopOnce.Do(func() {
		close(proc.stopSignal)
	})

	// Stop idle timer
	proc.idleTimerMu.Lock()
	if proc.idleTimer != nil {
		proc.idleTimer.Stop()
	}
	proc.idleTimerMu.Unlock()

	// Close PTY (this will cause the process to receive SIGHUP)
	proc.mu.Lock()
	if proc.ptmx != nil {
		_ = proc.ptmx.Close()
	}
	proc.mu.Unlock()

	// Terminate the process directly (PTY handles its own session management)
	if proc.cmd != nil && proc.cmd.Process != nil {
		_ = terminateProcess(proc.cmd.Process)

		// Wait for the wait() goroutine to finish (it calls cmd.Wait).
		// If it doesn't exit in time, force-kill the process.
		select {
		case <-ctx.Done():
			_ = proc.cmd.Process.Kill()
		case <-time.After(2 * time.Second):
			_ = proc.cmd.Process.Kill()
		case <-proc.waitDone:
			// Process exited cleanly
		}
	}

	return nil
}

// Get retrieves process information by ID.
func (r *InteractiveRunner) Get(id string, includeOutput bool) (*InteractiveProcessInfo, bool) {
	proc, ok := r.get(id)
	if !ok {
		return nil, false
	}
	info := proc.snapshot(includeOutput)
	return &info, true
}

// GetBySession retrieves process information by session ID.
func (r *InteractiveRunner) GetBySession(sessionID string) (*InteractiveProcessInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, proc := range r.processes {
		if proc.info.SessionID == sessionID {
			info := proc.snapshot(false)
			return &info, true
		}
	}
	return nil, false
}

// isProcessAlive checks if the underlying OS process is still running.
// Uses a non-blocking check on waitDone which is closed when cmd.Wait returns.
// Must be called with proc.mu held.
func (r *InteractiveRunner) isProcessAlive(proc *interactiveProcess) bool {
	if proc.cmd == nil || proc.cmd.Process == nil {
		return false
	}
	select {
	case <-proc.waitDone:
		return false
	default:
		return true
	}
}

// IsProcessRunning checks if a process with the given ID exists and is running.
// This is used to detect if a process was killed (e.g., after backend restart).
func (r *InteractiveRunner) IsProcessRunning(processID string) bool {
	proc, ok := r.get(processID)
	if !ok {
		return false
	}

	proc.mu.Lock()
	defer proc.mu.Unlock()

	// Process must be started and alive
	return proc.started && r.isProcessAlive(proc)
}

// IsProcessReadyOrPending checks if a process exists and is either running or pending start.
// This is used by the terminal handler to allow connections to deferred-start processes
// that will start when the terminal sends dimensions.
func (r *InteractiveRunner) IsProcessReadyOrPending(processID string) bool {
	proc, ok := r.get(processID)
	if !ok {
		return false
	}

	proc.mu.Lock()
	defer proc.mu.Unlock()

	// Process exists but hasn't started yet (deferred start) - this is OK
	if !proc.started {
		return true
	}

	// Process started - check if still alive
	return r.isProcessAlive(proc)
}

// GetBuffer returns the buffered output for a process.
func (r *InteractiveRunner) GetBuffer(processID string) ([]ProcessOutputChunk, bool) {
	proc, ok := r.get(processID)
	if !ok {
		return nil, false
	}
	return proc.buffer.snapshot(), true
}

func (r *InteractiveRunner) get(id string) (*interactiveProcess, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	proc, ok := r.processes[id]
	return proc, ok
}

// wait blocks until the process exits and then cleans up.
// Note: cmd.Wait() is intentionally blocking without a timeout. This is the correct
// behavior because:
// 1. Wait() is required to reap the process and prevent zombies
// 2. Stuck processes should be terminated via Stop() which sends SIGTERM/SIGKILL
// 3. Adding a timeout here would leave the process unreachable and create leaks
func (r *InteractiveRunner) wait(proc *interactiveProcess) {
	defer close(proc.waitDone)

	proc.mu.Lock()
	ptyHandle := proc.ptmx
	proc.mu.Unlock()

	exitCode, signalName, err := waitPtyProcess(proc.cmd, ptyHandle)
	status := types.ProcessStatusExited
	if err != nil {
		status = types.ProcessStatusFailed
	}

	r.logger.Info("interactive process exited",
		zap.String("process_id", proc.info.ID),
		zap.String("session_id", proc.info.SessionID),
		zap.String("status", string(status)),
		zap.Int("exit_code", exitCode),
		zap.String("signal", signalName),
		zap.Error(err),
	)

	// Log buffer contents if process exited with error (helps debug startup failures)
	// Use Debug level since non-zero exit is normal for killed processes (e.g., user closing terminal)
	if status == types.ProcessStatusFailed && proc.buffer != nil {
		chunks := proc.buffer.snapshot()
		if len(chunks) > 0 {
			var combinedOutput string
			for _, chunk := range chunks {
				combinedOutput += chunk.Data
			}
			// Truncate for logging (max 2000 chars)
			if len(combinedOutput) > 2000 {
				combinedOutput = combinedOutput[:2000] + "...(truncated)"
			}
			r.logger.Debug("interactive process output before exit",
				zap.String("process_id", proc.info.ID),
				zap.String("session_id", proc.info.SessionID),
				zap.Int("exit_code", exitCode),
				zap.String("output", combinedOutput),
			)
		}
	}

	// Stop idle timer
	proc.idleTimerMu.Lock()
	if proc.idleTimer != nil {
		proc.idleTimer.Stop()
	}
	proc.idleTimerMu.Unlock()

	// Update process info
	proc.mu.Lock()
	proc.info.Status = status
	proc.info.ExitCode = &exitCode
	proc.info.UpdatedAt = time.Now().UTC()
	proc.mu.Unlock()

	// Close PTY
	proc.mu.Lock()
	if proc.ptmx != nil {
		_ = proc.ptmx.Close()
		proc.ptmx = nil
	}
	proc.mu.Unlock()

	r.publishStatus(proc)

	// Remove from tracking
	r.mu.Lock()
	delete(r.processes, proc.info.ID)
	r.mu.Unlock()
}

func (r *InteractiveRunner) publishOutput(proc *interactiveProcess, chunk ProcessOutputChunk) {
	// No gating needed - process starts at exact frontend dimensions via lazy start
	proc.mu.Lock()
	info := proc.info
	proc.mu.Unlock()

	output := &types.ProcessOutput{
		SessionID: info.SessionID,
		ProcessID: info.ID,
		Kind:      types.ProcessKindAgentPassthrough,
		Stream:    chunk.Stream,
		Data:      chunk.Data,
		Timestamp: chunk.Timestamp,
	}

	// Use WorkspaceTracker if available, otherwise use callback
	if r.workspaceTracker != nil {
		r.workspaceTracker.notifyWorkspaceStreamProcessOutput(output)
	} else if r.outputCallback != nil {
		r.outputCallback(output)
	}
}

func (r *InteractiveRunner) publishStatus(proc *interactiveProcess) {
	proc.mu.Lock()
	info := proc.info
	proc.mu.Unlock()

	// Convert []string command to single string for status update
	cmdStr := ""
	if len(info.Command) > 0 {
		cmdStr = info.Command[0]
	}

	update := &types.ProcessStatusUpdate{
		SessionID:  info.SessionID,
		ProcessID:  info.ID,
		Kind:       types.ProcessKindAgentPassthrough,
		Command:    cmdStr,
		WorkingDir: info.WorkingDir,
		Status:     info.Status,
		ExitCode:   info.ExitCode,
		Timestamp:  time.Now().UTC(),
	}

	// Use WorkspaceTracker if available, otherwise use callback
	if r.workspaceTracker != nil {
		r.workspaceTracker.notifyWorkspaceStreamProcessStatus(update)
	} else if r.statusCallback != nil {
		r.statusCallback(update)
	}
}

func (p *interactiveProcess) snapshot(includeOutput bool) InteractiveProcessInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	info := p.info
	if includeOutput && p.buffer != nil {
		info.Output = p.buffer.snapshot()
	}
	return info
}
