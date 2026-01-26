// Package process provides background process execution and output streaming for agentctl.
//
// InteractiveRunner extends the pattern from ProcessRunner to support interactive
// CLI passthrough sessions where users interact directly with agent CLIs through
// a PTY-backed terminal.

package process

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// InteractiveStartRequest contains parameters for starting an interactive passthrough process.
type InteractiveStartRequest struct {
	SessionID         string            `json:"session_id"`                    // Required: Agent session owning this process
	Command           []string          `json:"command"`                       // Required: Command and args to execute
	WorkingDir        string            `json:"working_dir"`                   // Working directory
	Env               map[string]string `json:"env,omitempty"`                 // Additional environment variables
	PromptPattern     string            `json:"prompt_pattern,omitempty"`      // Regex pattern to detect agent prompt for turn completion
	IdleTimeoutMs     int               `json:"idle_timeout_ms,omitempty"`     // Idle timeout in ms for turn detection
	BufferMaxBytes    int64             `json:"buffer_max_bytes,omitempty"`    // Max output buffer size
	StatusDetector    string            `json:"status_detector,omitempty"`     // Status detector type: "claude_code", "codex", ""
	CheckIntervalMs   int               `json:"check_interval_ms,omitempty"`   // How often to check state (default 100ms)
	StabilityWindowMs int               `json:"stability_window_ms,omitempty"` // State stability window (default 0)
	ImmediateStart    bool              `json:"immediate_start,omitempty"`     // Start immediately with default dimensions (don't wait for resize)
	DefaultCols       int               `json:"default_cols,omitempty"`        // Default columns if ImmediateStart (default 120)
	DefaultRows       int               `json:"default_rows,omitempty"`        // Default rows if ImmediateStart (default 40)
}

// InteractiveProcessInfo represents the state of an interactive process.
type InteractiveProcessInfo struct {
	ID         string               `json:"id"`
	SessionID  string               `json:"session_id"`
	Command    []string             `json:"command"`
	WorkingDir string               `json:"working_dir"`
	Status     types.ProcessStatus  `json:"status"`
	ExitCode   *int                 `json:"exit_code,omitempty"`
	StartedAt  time.Time            `json:"started_at"`
	UpdatedAt  time.Time            `json:"updated_at"`
	Output     []ProcessOutputChunk `json:"output,omitempty"`
}

// DirectOutputWriter is a writer that receives raw PTY output.
// When set, output bypasses the event bus and goes directly to this writer.
type DirectOutputWriter interface {
	io.Writer
	io.Closer
}

// interactiveProcess represents a running interactive PTY process.
type interactiveProcess struct {
	info   InteractiveProcessInfo
	cmd    *exec.Cmd
	ptmx   *os.File // PTY master file descriptor (using creack/pty)
	buffer *ringBuffer

	// Turn detection
	promptPattern *regexp.Regexp
	idleTimeout   time.Duration
	idleTimer     *time.Timer
	idleTimerMu   sync.Mutex

	// Status tracking (vt10x-based TUI detection)
	statusTracker *StatusTracker
	lastState     AgentState

	// Deferred start - process created lazily on first resize
	// This ensures PTY is created at exact frontend dimensions
	started       bool
	startOnce     sync.Once
	startCmd      []string
	startDir      string
	startEnv      map[string]string
	startReq      InteractiveStartRequest // Full request for deferred initialization

	// Direct output - when set, raw output goes here instead of event bus
	directOutput   DirectOutputWriter
	directOutputMu sync.RWMutex

	// WebSocket tracking - tracks whether a WebSocket is actively connected
	hasActiveWebSocket bool

	// Lifecycle
	stopOnce   sync.Once
	stopSignal chan struct{}
	mu         sync.Mutex
}

// TurnCompleteCallback is called when turn detection determines the agent is waiting for input.
type TurnCompleteCallback func(sessionID string)

// OutputCallback is called when process output is received.
// Used when running without a WorkspaceTracker (e.g., standalone passthrough mode).
type OutputCallback func(output *types.ProcessOutput)

// StatusCallback is called when process status changes.
// Used when running without a WorkspaceTracker (e.g., standalone passthrough mode).
type StatusCallback func(status *types.ProcessStatusUpdate)

// AgentStateCallback is called when agent TUI state changes (working, waiting, etc.).
type AgentStateCallback func(sessionID string, state AgentState)

// sessionWebSocket tracks a WebSocket connection at the session level.
// This allows the WebSocket to survive process restarts.
type sessionWebSocket struct {
	writer DirectOutputWriter
	mu     sync.RWMutex
}

// InteractiveRunner manages interactive PTY-based processes with stdin support.
type InteractiveRunner struct {
	logger           *logger.Logger
	workspaceTracker *WorkspaceTracker
	bufferMaxBytes   int64
	turnCompleteCallback TurnCompleteCallback
	outputCallback       OutputCallback
	statusCallback       StatusCallback
	stateCallback        AgentStateCallback

	mu        sync.RWMutex
	processes map[string]*interactiveProcess

	// Session-level WebSocket tracking - survives process restarts
	sessionWsMu sync.RWMutex
	sessionWs   map[string]*sessionWebSocket
}

// NewInteractiveRunner creates a new interactive process runner.
func NewInteractiveRunner(workspaceTracker *WorkspaceTracker, log *logger.Logger, bufferMaxBytes int64) *InteractiveRunner {
	return &InteractiveRunner{
		logger:           log.WithFields(zap.String("component", "interactive-runner")),
		workspaceTracker: workspaceTracker,
		bufferMaxBytes:   bufferMaxBytes,
		processes:        make(map[string]*interactiveProcess),
		sessionWs:        make(map[string]*sessionWebSocket),
	}
}

// SetTurnCompleteCallback sets the callback to invoke when turn detection fires.
func (r *InteractiveRunner) SetTurnCompleteCallback(cb TurnCompleteCallback) {
	r.turnCompleteCallback = cb
}

// SetOutputCallback sets the callback to invoke when process output is received.
// This is used when running without a WorkspaceTracker.
func (r *InteractiveRunner) SetOutputCallback(cb OutputCallback) {
	r.outputCallback = cb
}

// SetStatusCallback sets the callback to invoke when process status changes.
// This is used when running without a WorkspaceTracker.
func (r *InteractiveRunner) SetStatusCallback(cb StatusCallback) {
	r.statusCallback = cb
}

// SetStateCallback sets the callback to invoke when agent TUI state changes.
func (r *InteractiveRunner) SetStateCallback(cb AgentStateCallback) {
	r.stateCallback = cb
}

// createStatusDetector creates the appropriate detector based on the detector type.
// The idle detector is the default - it relies on the idle timer mechanism for turn detection.
func createStatusDetector(detectorType string) StatusDetector {
	return NewIdleDetector()
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

	idleTimeout := time.Duration(req.IdleTimeoutMs) * time.Millisecond
	if idleTimeout <= 0 {
		idleTimeout = 5 * time.Second // Default 5 seconds
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
		stopSignal:    make(chan struct{}),
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
		cols := req.DefaultCols
		rows := req.DefaultRows
		if cols <= 0 {
			cols = 120 // Default width
		}
		if rows <= 0 {
			rows = 40 // Default height
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
			return nil, fmt.Errorf("failed to start process: %w", startErr)
		}

		r.logger.Info("interactive process started immediately",
			zap.String("process_id", id),
			zap.String("session_id", req.SessionID),
			zap.Strings("command", req.Command),
			zap.String("working_dir", req.WorkingDir),
		)
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

	// Start process in PTY with exact dimensions from frontend using creack/pty
	// This is the battle-tested PTY library that properly handles resize/SIGWINCH
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		return fmt.Errorf("failed to start pty: %w", err)
	}

	// Create status tracker if a detector is configured
	var statusTracker *StatusTracker
	if req.StatusDetector != "" {
		detector := createStatusDetector(req.StatusDetector)
		config := StatusTrackerConfig{
			Rows:            rows,
			Cols:            cols,
			CheckInterval:   time.Duration(req.CheckIntervalMs) * time.Millisecond,
			StabilityWindow: time.Duration(req.StabilityWindowMs) * time.Millisecond,
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
		_ = proc.cmd.Process.Signal(syscall.SIGTERM)

		// Wait for graceful exit, then force kill
		done := make(chan struct{})
		go func() {
			_ = proc.cmd.Wait()
			close(done)
		}()

		select {
		case <-ctx.Done():
			_ = proc.cmd.Process.Kill()
		case <-time.After(2 * time.Second):
			_ = proc.cmd.Process.Kill()
		case <-done:
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
// Must be called with proc.mu held.
func (r *InteractiveRunner) isProcessAlive(proc *interactiveProcess) bool {
	if proc.cmd != nil && proc.cmd.Process != nil {
		// Signal 0 checks if process exists without sending a signal
		err := proc.cmd.Process.Signal(syscall.Signal(0))
		return err == nil
	}
	return false
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

// ResizeBySession resizes the PTY for a process by session ID.
// On first resize, this triggers lazy process start at the exact frontend dimensions.
func (r *InteractiveRunner) ResizeBySession(sessionID string, cols, rows uint16) error {
	r.mu.RLock()
	var proc *interactiveProcess
	for _, p := range r.processes {
		if p.info.SessionID == sessionID {
			proc = p
			break
		}
	}
	r.mu.RUnlock()

	if proc == nil {
		return fmt.Errorf("no process found for session %s", sessionID)
	}

	// Lazy start: spawn process on first resize when we have exact dimensions
	var startErr error
	proc.startOnce.Do(func() {
		r.logger.Info("first resize received - starting process",
			zap.String("session_id", sessionID),
			zap.Uint16("cols", cols),
			zap.Uint16("rows", rows))
		startErr = r.startProcess(proc, int(cols), int(rows))
	})
	if startErr != nil {
		return fmt.Errorf("failed to start process on first resize: %w", startErr)
	}

	proc.mu.Lock()
	ptyInstance := proc.ptmx
	statusTracker := proc.statusTracker
	proc.mu.Unlock()

	// If process started, resize the PTY using creack/pty's Setsize
	// This properly sets the window size and sends SIGWINCH to the process
	if ptyInstance != nil {
		if err := pty.Setsize(ptyInstance, &pty.Winsize{
			Cols: cols,
			Rows: rows,
		}); err != nil {
			return fmt.Errorf("failed to resize PTY: %w", err)
		}
	}

	// Also resize the status tracker's virtual terminal
	if statusTracker != nil {
		statusTracker.Resize(int(cols), int(rows))
	}

	r.logger.Debug("resized PTY",
		zap.String("session_id", sessionID),
		zap.Uint16("cols", cols),
		zap.Uint16("rows", rows))

	return nil
}

func (r *InteractiveRunner) get(id string) (*interactiveProcess, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	proc, ok := r.processes[id]
	return proc, ok
}

func (r *InteractiveRunner) readOutput(proc *interactiveProcess) {
	buf := make([]byte, 32768) // 32KB buffer for better throughput
	recentOutput := ""         // Keep recent output for prompt pattern matching
	firstRead := true

	for {
		select {
		case <-proc.stopSignal:
			r.logger.Debug("readOutput: stop signal received",
				zap.String("process_id", proc.info.ID))
			return
		default:
		}

		proc.mu.Lock()
		ptyInstance := proc.ptmx
		proc.mu.Unlock()

		if ptyInstance == nil {
			r.logger.Debug("readOutput: pty is nil, exiting",
				zap.String("process_id", proc.info.ID))
			return
		}

		n, err := ptyInstance.Read(buf)
		if firstRead {
			r.logger.Info("readOutput: first read attempt",
				zap.String("process_id", proc.info.ID),
				zap.Int("bytes", n),
				zap.Error(err))
			firstRead = false
		}
		if n > 0 {
			data := buf[:n]
			dataStr := string(data)

			// Respond to cursor position queries (DSR) if no terminal is connected yet.
			// Some CLI tools (like Codex) query cursor position on startup with \e[6n
			// and expect a response \e[row;colR. Without this, they timeout and exit.
			// Only respond if no direct output writer is connected (no real terminal yet).
			proc.directOutputMu.RLock()
			hasDirectWriter := proc.directOutput != nil
			proc.directOutputMu.RUnlock()

			if !hasDirectWriter {
				// Check for cursor position query (DSR): ESC [ 6 n or ESC [ ? 6 n
				if containsDSRQuery(data) {
					// Respond with cursor at position 1,1 (top-left)
					response := "\x1b[1;1R"
					if _, err := ptyInstance.Write([]byte(response)); err != nil {
						r.logger.Debug("failed to respond to cursor position query",
							zap.String("process_id", proc.info.ID),
							zap.Error(err))
					} else {
						r.logger.Debug("responded to cursor position query",
							zap.String("process_id", proc.info.ID))
					}
				}
				// Check for primary device attributes query (DA1): ESC [ c or ESC [ 0 c
				if containsDA1Query(data) {
					// Respond as VT100 terminal with advanced video option
					response := "\x1b[?1;2c"
					if _, err := ptyInstance.Write([]byte(response)); err != nil {
						r.logger.Debug("failed to respond to device attributes query",
							zap.String("process_id", proc.info.ID),
							zap.Error(err))
					}
				}
			}

			// Feed to status tracker for TUI state detection
			if proc.statusTracker != nil {
				proc.statusTracker.Write(data)

				// Periodically check state (debounced by ShouldCheck)
				if proc.statusTracker.ShouldCheck() {
					newState := proc.statusTracker.CheckAndUpdate()
					if newState != proc.lastState {
						proc.lastState = newState
						r.handleStateChange(proc, newState)
					}
				}
			}

			// Always buffer output for scrollback restoration on reconnect
			chunk := ProcessOutputChunk{
				Stream:    "stdout",
				Data:      dataStr,
				Timestamp: time.Now().UTC(),
			}
			proc.buffer.append(chunk)

			// Check if we have a direct output writer (binary WebSocket mode)
			proc.directOutputMu.RLock()
			directWriter := proc.directOutput
			proc.directOutputMu.RUnlock()

			if directWriter != nil {
				// Direct mode: write raw bytes to the WebSocket
				if _, writeErr := directWriter.Write(data); writeErr != nil {
					r.logger.Debug("direct output write error",
						zap.String("process_id", proc.info.ID),
						zap.Error(writeErr))
					// Don't return - the writer might have closed but process continues
				}
			} else {
				// No WebSocket connected: also publish via event bus
				r.publishOutput(proc, chunk)
			}

			// Update recent output for prompt pattern matching (keep last 1KB)
			// Trim before appending to prevent temporary memory spikes with large outputs
			maxSize := 1024
			if len(recentOutput)+len(dataStr) > maxSize {
				// Calculate how much to keep from existing buffer
				keepFromExisting := maxSize - len(dataStr)
				if keepFromExisting < 0 {
					keepFromExisting = 0
				}
				if keepFromExisting > 0 && len(recentOutput) > keepFromExisting {
					recentOutput = recentOutput[len(recentOutput)-keepFromExisting:]
				} else if keepFromExisting == 0 {
					recentOutput = ""
				}
			}
			recentOutput += dataStr
			// Final trim in case dataStr itself was larger than maxSize
			if len(recentOutput) > maxSize {
				recentOutput = recentOutput[len(recentOutput)-maxSize:]
			}

			// Check prompt pattern for turn completion
			if proc.promptPattern != nil && proc.promptPattern.MatchString(recentOutput) {
				r.emitTurnComplete(proc)
				recentOutput = "" // Reset after match
			}

			// Reset idle timer on any output
			r.resetIdleTimer(proc)
		}
		if err != nil {
			r.logger.Debug("interactive process output read ended",
				zap.String("process_id", proc.info.ID),
				zap.Error(err))
			return
		}
	}
}

func (r *InteractiveRunner) resetIdleTimer(proc *interactiveProcess) {
	proc.idleTimerMu.Lock()
	defer proc.idleTimerMu.Unlock()

	if proc.idleTimer != nil {
		proc.idleTimer.Stop()
	}

	if proc.idleTimeout > 0 {
		proc.idleTimer = time.AfterFunc(proc.idleTimeout, func() {
			r.emitTurnComplete(proc)
		})
	}
}

func (r *InteractiveRunner) emitTurnComplete(proc *interactiveProcess) {
	if r.turnCompleteCallback != nil {
		r.turnCompleteCallback(proc.info.SessionID)
	}
	r.logger.Debug("turn complete detected",
		zap.String("process_id", proc.info.ID),
		zap.String("session_id", proc.info.SessionID))
}

// handleStateChange processes agent state changes detected by the status tracker.
func (r *InteractiveRunner) handleStateChange(proc *interactiveProcess, state AgentState) {
	r.logger.Debug("agent state changed",
		zap.String("process_id", proc.info.ID),
		zap.String("session_id", proc.info.SessionID),
		zap.String("state", string(state)))

	// WaitingInput state triggers turn complete
	if state == StateWaitingInput {
		r.emitTurnComplete(proc)
	}

	// Invoke the state callback for external handling
	if r.stateCallback != nil {
		r.stateCallback(proc.info.SessionID, state)
	}
}

// wait blocks until the process exits and then cleans up.
// Note: cmd.Wait() is intentionally blocking without a timeout. This is the correct
// behavior because:
// 1. Wait() is required to reap the process and prevent zombies
// 2. Stuck processes should be terminated via Stop() which sends SIGTERM/SIGKILL
// 3. Adding a timeout here would leave the process unreachable and create leaks
func (r *InteractiveRunner) wait(proc *interactiveProcess) {
	err := proc.cmd.Wait()
	exitCode := 0
	status := types.ProcessStatusExited
	var signalName string
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if waitStatus, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if waitStatus.Signaled() {
					signalName = waitStatus.Signal().String()
					exitCode = 128 + int(waitStatus.Signal())
				} else {
					exitCode = waitStatus.ExitStatus()
				}
			} else {
				exitCode = 1
			}
		} else {
			exitCode = 1
		}
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
			r.logger.Error("interactive process output before exit",
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

// SetDirectOutput sets a direct output writer for a process.
// When set, PTY output bypasses the event bus and goes directly to this writer.
// This is used for the dedicated binary WebSocket in passthrough mode.
// Also tracks the WebSocket at the session level to survive process restarts.
// Returns error if process not found.
func (r *InteractiveRunner) SetDirectOutput(processID string, writer DirectOutputWriter) error {
	proc, ok := r.get(processID)
	if !ok {
		return fmt.Errorf("process not found: %s", processID)
	}

	sessionID := proc.info.SessionID

	proc.directOutputMu.Lock()
	proc.directOutput = writer
	proc.hasActiveWebSocket = true
	proc.directOutputMu.Unlock()

	// Also track at session level for restart support
	r.sessionWsMu.Lock()
	r.sessionWs[sessionID] = &sessionWebSocket{writer: writer}
	r.sessionWsMu.Unlock()

	r.logger.Info("direct output set for process",
		zap.String("process_id", processID),
		zap.String("session_id", sessionID))

	return nil
}

// ClearDirectOutput clears the direct output writer for a process.
// Output will return to the normal event bus path.
// Also clears the session-level WebSocket tracking.
func (r *InteractiveRunner) ClearDirectOutput(processID string) error {
	proc, ok := r.get(processID)
	if !ok {
		// Process may have been deleted - still try to clear session tracking
		r.logger.Debug("process not found during ClearDirectOutput, trying to clear by session",
			zap.String("process_id", processID))
		return nil
	}

	sessionID := proc.info.SessionID

	proc.directOutputMu.Lock()
	proc.directOutput = nil
	proc.hasActiveWebSocket = false
	proc.directOutputMu.Unlock()

	// Also clear session-level tracking
	r.sessionWsMu.Lock()
	delete(r.sessionWs, sessionID)
	r.sessionWsMu.Unlock()

	r.logger.Info("direct output cleared for process",
		zap.String("process_id", processID),
		zap.String("session_id", sessionID))

	return nil
}

// ClearDirectOutputBySession clears the direct output for a session.
// This is used when the terminal WebSocket disconnects.
func (r *InteractiveRunner) ClearDirectOutputBySession(sessionID string) {
	// Clear session-level tracking
	r.sessionWsMu.Lock()
	delete(r.sessionWs, sessionID)
	r.sessionWsMu.Unlock()

	// Also clear from any process with this session
	r.mu.RLock()
	for _, proc := range r.processes {
		if proc.info.SessionID == sessionID {
			proc.directOutputMu.Lock()
			proc.directOutput = nil
			proc.hasActiveWebSocket = false
			proc.directOutputMu.Unlock()
		}
	}
	r.mu.RUnlock()

	r.logger.Info("direct output cleared for session",
		zap.String("session_id", sessionID))
}

// ConnectSessionWebSocket connects an existing session WebSocket to a process.
// This is called when a new process starts for a session that already has an active WebSocket.
// Returns true if a WebSocket was connected.
func (r *InteractiveRunner) ConnectSessionWebSocket(processID string) bool {
	proc, ok := r.get(processID)
	if !ok {
		return false
	}

	sessionID := proc.info.SessionID

	r.sessionWsMu.RLock()
	sessWs, exists := r.sessionWs[sessionID]
	r.sessionWsMu.RUnlock()

	if !exists || sessWs == nil {
		return false
	}

	sessWs.mu.RLock()
	writer := sessWs.writer
	sessWs.mu.RUnlock()

	if writer == nil {
		return false
	}

	proc.directOutputMu.Lock()
	proc.directOutput = writer
	proc.hasActiveWebSocket = true
	proc.directOutputMu.Unlock()

	r.logger.Info("connected existing session WebSocket to new process",
		zap.String("process_id", processID),
		zap.String("session_id", sessionID))

	return true
}

// GetPtyWriter returns a writer for sending input to the PTY.
// This is used for the dedicated binary WebSocket in passthrough mode.
func (r *InteractiveRunner) GetPtyWriter(processID string) (io.Writer, error) {
	proc, ok := r.get(processID)
	if !ok {
		return nil, fmt.Errorf("process not found: %s", processID)
	}

	proc.mu.Lock()
	defer proc.mu.Unlock()

	if !proc.started {
		return nil, fmt.Errorf("process not started yet - waiting for terminal dimensions")
	}

	if proc.ptmx == nil {
		return nil, fmt.Errorf("PTY not available")
	}

	return proc.ptmx, nil
}

// GetPtyWriterBySession returns a writer for sending input to the PTY for a session.
// This is used to reconnect after a process restart.
func (r *InteractiveRunner) GetPtyWriterBySession(sessionID string) (io.Writer, string, error) {
	r.mu.RLock()
	var proc *interactiveProcess
	for _, p := range r.processes {
		if p.info.SessionID == sessionID {
			proc = p
			break
		}
	}
	r.mu.RUnlock()

	if proc == nil {
		return nil, "", fmt.Errorf("no process found for session: %s", sessionID)
	}

	proc.mu.Lock()
	defer proc.mu.Unlock()

	if !proc.started {
		return nil, proc.info.ID, fmt.Errorf("process not started yet - waiting for terminal dimensions")
	}

	if proc.ptmx == nil {
		return nil, proc.info.ID, fmt.Errorf("PTY not available")
	}

	return proc.ptmx, proc.info.ID, nil
}

// HasActiveWebSocket checks if a process has an active WebSocket connection.
// This is used to determine if auto-restart should be attempted on process exit.
func (r *InteractiveRunner) HasActiveWebSocket(processID string) bool {
	proc, ok := r.get(processID)
	if !ok {
		return false
	}

	proc.directOutputMu.RLock()
	defer proc.directOutputMu.RUnlock()
	return proc.hasActiveWebSocket
}

// HasActiveWebSocketBySession checks if a session has an active WebSocket connection.
// Uses session-level tracking which survives process restarts.
func (r *InteractiveRunner) HasActiveWebSocketBySession(sessionID string) bool {
	r.sessionWsMu.RLock()
	sessWs, exists := r.sessionWs[sessionID]
	r.sessionWsMu.RUnlock()

	if !exists || sessWs == nil {
		return false
	}

	sessWs.mu.RLock()
	hasWriter := sessWs.writer != nil
	sessWs.mu.RUnlock()

	return hasWriter
}

// WriteToDirectOutput writes data directly to the WebSocket output for a process.
// This is used to send messages like restart notifications to the terminal.
// Returns error if process not found or no direct output is set.
func (r *InteractiveRunner) WriteToDirectOutput(processID string, data []byte) error {
	proc, ok := r.get(processID)
	if !ok {
		return fmt.Errorf("process not found: %s", processID)
	}

	proc.directOutputMu.RLock()
	directWriter := proc.directOutput
	proc.directOutputMu.RUnlock()

	if directWriter == nil {
		return fmt.Errorf("no direct output writer set for process: %s", processID)
	}

	_, err := directWriter.Write(data)
	return err
}

// WriteToDirectOutputBySession writes data directly to the WebSocket output for a session.
// This is used to send messages like restart notifications to the terminal.
// Uses session-level tracking which survives process restarts.
func (r *InteractiveRunner) WriteToDirectOutputBySession(sessionID string, data []byte) error {
	r.sessionWsMu.RLock()
	sessWs, exists := r.sessionWs[sessionID]
	r.sessionWsMu.RUnlock()

	if !exists || sessWs == nil {
		return fmt.Errorf("no WebSocket found for session: %s", sessionID)
	}

	sessWs.mu.RLock()
	writer := sessWs.writer
	sessWs.mu.RUnlock()

	if writer == nil {
		return fmt.Errorf("no direct output writer set for session: %s", sessionID)
	}

	_, err := writer.Write(data)
	return err
}

// containsDSRQuery checks if data contains a Device Status Report (cursor position) query.
// DSR query is: ESC [ 6 n or ESC [ ? 6 n
func containsDSRQuery(data []byte) bool {
	return bytes.Contains(data, []byte("\x1b[6n")) || bytes.Contains(data, []byte("\x1b[?6n"))
}

// containsDA1Query checks if data contains a Primary Device Attributes query.
// DA1 query is: ESC [ c or ESC [ 0 c (but NOT ESC [ <digit> c which is cursor forward)
func containsDA1Query(data []byte) bool {
	// Check for exact ESC [ c sequence
	csiC := []byte("\x1b[c")
	for i := 0; i <= len(data)-len(csiC); i++ {
		if data[i] == '\x1b' && i+2 < len(data) && data[i+1] == '[' && data[i+2] == 'c' {
			// Make sure it's not preceded by a digit (which would make it cursor forward)
			// ESC [ c is valid, ESC [ 0 c is valid, but ESC [ 1 c is cursor forward
			if i+2 == len(data)-1 {
				// ESC [ c at end
				return true
			}
			// Check what's before 'c' - if it's just '[' or '[0' it's DA1
			// We already matched ESC [ c, so this is valid
			return true
		}
	}

	// Check for ESC [ 0 c
	csi0C := []byte("\x1b[0c")
	return bytes.Contains(data, csi0C)
}

