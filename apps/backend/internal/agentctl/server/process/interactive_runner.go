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
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// InteractiveStartRequest contains parameters for starting an interactive passthrough process.
type InteractiveStartRequest struct {
	SessionID      string            `json:"session_id"`                 // Required: Agent session owning this process
	Command        []string          `json:"command"`                    // Required: Command and args to execute
	WorkingDir     string            `json:"working_dir"`                // Working directory
	Env            map[string]string `json:"env,omitempty"`              // Additional environment variables
	PromptPattern  string            `json:"prompt_pattern,omitempty"`   // Regex pattern to detect agent prompt for turn completion
	IdleTimeout    time.Duration     `json:"idle_timeout,omitempty"`     // Idle timeout for turn detection
	BufferMaxBytes int64             `json:"buffer_max_bytes,omitempty"` // Max output buffer size
	StatusDetector string            `json:"status_detector,omitempty"`  // Status detector type: "claude_code", "codex", ""
	CheckInterval   time.Duration    `json:"check_interval,omitempty"`   // How often to check state (default 100ms)
	StabilityWindow time.Duration    `json:"stability_window,omitempty"` // State stability window (default 0)
	ImmediateStart  bool             `json:"immediate_start,omitempty"`  // Start immediately with default dimensions (don't wait for resize)
	DefaultCols     int              `json:"default_cols,omitempty"`     // Default columns if ImmediateStart (default 120)
	DefaultRows     int              `json:"default_rows,omitempty"`     // Default rows if ImmediateStart (default 40)
	InitialCommand       string `json:"initial_command,omitempty"`        // Command to write to stdin after shell starts (for script terminals)
	DisableTurnDetection bool   `json:"disable_turn_detection,omitempty"` // Disable idle timer and turn detection (for user shell terminals)
	IsUserShell          bool   `json:"is_user_shell,omitempty"`          // Mark as user shell process (excluded from session-level lookups)
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
	waitDone   chan struct{} // closed when wait() returns (cmd.Wait completed)
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

// userShellEntry tracks a user shell with its metadata.
type userShellEntry struct {
	ProcessID      string
	Label          string    // Display name (e.g., "Terminal" or "Terminal 2")
	InitialCommand string    // Command that was run when shell started (empty for plain shells)
	Closable       bool      // Whether the terminal can be closed (first terminal is not closable)
	CreatedAt      time.Time // When the shell was created (for stable ordering)
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

	// User shell processes - key: "sessionId:terminalId"
	userShellsMu sync.RWMutex
	userShells   map[string]*userShellEntry
}

// NewInteractiveRunner creates a new interactive process runner.
func NewInteractiveRunner(workspaceTracker *WorkspaceTracker, log *logger.Logger, bufferMaxBytes int64) *InteractiveRunner {
	return &InteractiveRunner{
		logger:           log.WithFields(zap.String("component", "interactive-runner")),
		workspaceTracker: workspaceTracker,
		bufferMaxBytes:   bufferMaxBytes,
		processes:        make(map[string]*interactiveProcess),
		sessionWs:        make(map[string]*sessionWebSocket),
		userShells:       make(map[string]*userShellEntry),
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

// createStatusDetector creates a status detector for TUI state tracking.
// Currently always returns an idle detector that relies on the idle timer mechanism.
func createStatusDetector() StatusDetector {
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

// lazyStartAndResize handles the common pattern of lazy-starting a process on the first
// resize and then resizing its PTY and status tracker. All three public Resize* methods
// delegate here to avoid duplicating the start-once + resize logic.
func (r *InteractiveRunner) lazyStartAndResize(proc *interactiveProcess, cols, rows uint16, logFields ...zap.Field) error {
	// Lazy start: spawn process on first resize when we have exact dimensions
	var startErr error
	proc.startOnce.Do(func() {
		fields := append([]zap.Field{zap.Uint16("cols", cols), zap.Uint16("rows", rows)}, logFields...)
		r.logger.Info("first resize received - starting process", fields...)
		startErr = r.startProcess(proc, int(cols), int(rows))
	})
	if startErr != nil {
		return fmt.Errorf("failed to start process on first resize: %w", startErr)
	}

	proc.mu.Lock()
	ptyInstance := proc.ptmx
	statusTracker := proc.statusTracker
	proc.mu.Unlock()

	// Resize the PTY (Unix: SIGWINCH, Windows: ConPTY dimensions)
	if ptyInstance != nil {
		if err := ptyInstance.Resize(cols, rows); err != nil {
			return fmt.Errorf("failed to resize PTY: %w", err)
		}
	}

	// Also resize the status tracker's virtual terminal (if present)
	if statusTracker != nil {
		statusTracker.Resize(int(cols), int(rows))
	}

	return nil
}

// ResizeByProcessID resizes the PTY for a specific process by its ID.
// On first resize, this triggers lazy process start at the exact frontend dimensions.
// This is preferred over ResizeBySession when the process ID is known, as it avoids
// ambiguity when multiple processes exist for the same session.
func (r *InteractiveRunner) ResizeByProcessID(processID string, cols, rows uint16) error {
	proc, ok := r.get(processID)
	if !ok {
		return fmt.Errorf("process not found: %s", processID)
	}

	if err := r.lazyStartAndResize(proc, cols, rows,
		zap.String("process_id", processID),
		zap.String("session_id", proc.info.SessionID),
	); err != nil {
		return err
	}

	r.logger.Debug("resized PTY",
		zap.String("process_id", processID),
		zap.String("session_id", proc.info.SessionID),
		zap.Uint16("cols", cols),
		zap.Uint16("rows", rows))

	return nil
}

// ResizeBySession resizes the PTY for a process by session ID.
// On first resize, this triggers lazy process start at the exact frontend dimensions.
// Skips user shell processes to avoid conflicts with passthrough processes.
func (r *InteractiveRunner) ResizeBySession(sessionID string, cols, rows uint16) error {
	r.mu.RLock()
	var proc *interactiveProcess
	for _, p := range r.processes {
		if p.info.SessionID == sessionID && !p.isUserShell {
			proc = p
			break
		}
	}
	r.mu.RUnlock()

	if proc == nil {
		return fmt.Errorf("no process found for session %s", sessionID)
	}

	if err := r.lazyStartAndResize(proc, cols, rows,
		zap.String("session_id", sessionID),
	); err != nil {
		return err
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

// SetDirectOutput sets a direct output writer for a process.
// When set, PTY output bypasses the event bus and goes directly to this writer.
// This is used for the dedicated binary WebSocket in passthrough mode.
// For non-user-shell processes, also tracks the WebSocket at the session level
// to survive process restarts. User shells are excluded from session-level tracking
// to prevent overwriting the agent terminal's WebSocket.
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

	// Track at session level for restart support (agent passthrough only).
	// User shells have their own tracking via userShells map and must not
	// overwrite the agent terminal's session-level WebSocket.
	if !proc.isUserShell {
		r.sessionWsMu.Lock()
		r.sessionWs[sessionID] = &sessionWebSocket{writer: writer}
		r.sessionWsMu.Unlock()
	}

	r.logger.Info("direct output set for process",
		zap.String("process_id", processID),
		zap.String("session_id", sessionID),
		zap.Bool("is_user_shell", proc.isUserShell))

	return nil
}

// ClearDirectOutput clears the direct output writer for a process.
// Output will return to the normal event bus path.
// For non-user-shell processes, also clears the session-level WebSocket tracking.
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

	// Clear session-level tracking (agent passthrough only)
	if !proc.isUserShell {
		r.sessionWsMu.Lock()
		delete(r.sessionWs, sessionID)
		r.sessionWsMu.Unlock()
	}

	r.logger.Info("direct output cleared for process",
		zap.String("process_id", processID),
		zap.String("session_id", sessionID))

	return nil
}

// ClearDirectOutputBySession clears the direct output for a session.
// This is used when the agent terminal WebSocket disconnects.
// Only clears non-user-shell processes to avoid disrupting user shell terminals.
func (r *InteractiveRunner) ClearDirectOutputBySession(sessionID string) {
	// Clear session-level tracking
	r.sessionWsMu.Lock()
	delete(r.sessionWs, sessionID)
	r.sessionWsMu.Unlock()

	// Clear from non-user-shell processes with this session
	r.mu.RLock()
	for _, proc := range r.processes {
		if proc.info.SessionID == sessionID && !proc.isUserShell {
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
// Skips user shell processes to avoid conflicts with passthrough processes.
func (r *InteractiveRunner) GetPtyWriterBySession(sessionID string) (io.Writer, string, error) {
	r.mu.RLock()
	var proc *interactiveProcess
	for _, p := range r.processes {
		if p.info.SessionID == sessionID && !p.isUserShell {
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

// UserShellOptions contains optional parameters for starting a user shell.
type UserShellOptions struct {
	Label          string // Display name (e.g., "Terminal" or script name)
	InitialCommand string // Command to run after shell starts
	Closable       *bool  // Whether the terminal can be closed (nil = auto-determine)
}

// CreateUserShellResult contains the result of creating a new user shell.
type CreateUserShellResult struct {
	TerminalID string `json:"terminal_id"`
	Label      string `json:"label"`
	Closable   bool   `json:"closable"`
}

// CreateUserShell creates a new user shell terminal with auto-assigned ID and label.
// The first shell for a session is labeled "Terminal" and is not closable.
// Subsequent shells are labeled "Terminal 2", "Terminal 3", etc. and are closable.
// The entry is registered atomically to prevent races with ListUserShells.
func (r *InteractiveRunner) CreateUserShell(sessionID string) CreateUserShellResult {
	r.userShellsMu.Lock()
	defer r.userShellsMu.Unlock()

	// Count existing plain shell terminals for this session
	prefix := sessionID + ":"
	shellCount := 0
	for key, entry := range r.userShells {
		if strings.HasPrefix(key, prefix) {
			terminalID := key[len(prefix):]
			// Only count plain shells (not script terminals)
			if strings.HasPrefix(terminalID, "shell-") && entry.InitialCommand == "" {
				shellCount++
			}
		}
	}

	// Generate terminal ID and label
	terminalID := "shell-" + uuid.New().String()

	var label string
	var closable bool
	if shellCount == 0 {
		label = "Terminal"
		closable = false // First terminal is not closable
	} else {
		label = fmt.Sprintf("Terminal %d", shellCount+1)
		closable = true
	}

	// Register the entry so ListUserShells includes it immediately
	r.userShells[prefix+terminalID] = &userShellEntry{
		ProcessID:      "", // No process yet - will be started when WebSocket connects
		Label:          label,
		InitialCommand: "",
		Closable:       closable,
		CreatedAt:      time.Now().UTC(),
	}

	return CreateUserShellResult{
		TerminalID: terminalID,
		Label:      label,
		Closable:   closable,
	}
}

// RegisterScriptShell registers a script terminal entry so ListUserShells returns it.
// The actual process is not started until the WebSocket connects (StartUserShell handles that).
func (r *InteractiveRunner) RegisterScriptShell(sessionID, terminalID, label, initialCommand string) {
	key := sessionID + ":" + terminalID

	r.userShellsMu.Lock()
	defer r.userShellsMu.Unlock()

	r.userShells[key] = &userShellEntry{
		ProcessID:      "", // No process yet - will be started when WebSocket connects
		Label:          label,
		InitialCommand: initialCommand,
		Closable:       true, // Script terminals are always closable
		CreatedAt:      time.Now().UTC(),
	}
}

// StartUserShell starts or returns an existing user shell for a terminal tab.
// Each terminal tab gets its own independent shell process.
// If opts.InitialCommand is provided, it will be written to stdin after the shell starts.
func (r *InteractiveRunner) StartUserShell(ctx context.Context, sessionID, terminalID, workingDir, preferredShell string, opts *UserShellOptions) (*InteractiveProcessInfo, error) {
	key := sessionID + ":" + terminalID

	// Normalize options
	if opts == nil {
		opts = &UserShellOptions{}
	}
	if opts.Label == "" {
		opts.Label = "Terminal"
	}

	// Check if shell entry already exists (auto-created by ListUserShells or RegisterScriptShell)
	var existingEntry *userShellEntry
	r.userShellsMu.RLock()
	entry, exists := r.userShells[key]
	if exists {
		// If entry has a process, check if it's still alive
		if entry.ProcessID != "" {
			if info, ok := r.Get(entry.ProcessID, false); ok {
				r.userShellsMu.RUnlock()
				return info, nil
			}
			// Process died - we'll start a new one below
		}
		// Entry exists but no process (pre-registered) or process died
		// Keep the existing metadata (label, closable, createdAt, initialCommand)
		existingEntry = entry
	}
	r.userShellsMu.RUnlock()

	// Use initial command from pre-registered entry if not provided in opts
	initialCommand := opts.InitialCommand
	if initialCommand == "" && existingEntry != nil {
		initialCommand = existingEntry.InitialCommand
	}

	req := InteractiveStartRequest{
		SessionID:            sessionID,
		Command:              defaultShellCommand(preferredShell),
		WorkingDir:           workingDir,
		InitialCommand:       initialCommand,
		DisableTurnDetection: true, // User shells must not trigger turn complete / MarkReady
		IsUserShell:          true, // Exclude from session-level lookups (ResizeBySession, GetPtyWriterBySession)
	}

	info, err := r.Start(ctx, req)
	if err != nil {
		return nil, err
	}

	// Track the user shell with metadata
	// If entry already exists (auto-created by ListUserShells), preserve its metadata
	r.userShellsMu.Lock()
	if existingEntry != nil {
		// Update existing entry with the new process ID
		existingEntry.ProcessID = info.ID
		r.userShells[key] = existingEntry
	} else {
		// Create new entry
		closable := true // Default: closable
		if opts.Closable != nil {
			closable = *opts.Closable
		}
		r.userShells[key] = &userShellEntry{
			ProcessID:      info.ID,
			Label:          opts.Label,
			InitialCommand: opts.InitialCommand,
			Closable:       closable,
			CreatedAt:      time.Now().UTC(),
		}
	}
	r.userShellsMu.Unlock()

	r.logger.Info("started user shell",
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
		zap.String("process_id", info.ID),
		zap.String("shell", req.Command[0]),
		zap.String("working_dir", workingDir),
		zap.String("label", opts.Label),
		zap.String("initial_command", opts.InitialCommand))

	return info, nil
}

// UserShellInfo contains information about a running user shell.
type UserShellInfo struct {
	TerminalID     string    `json:"terminal_id"`
	ProcessID      string    `json:"process_id"`
	Running        bool      `json:"running"`
	Label          string    `json:"label"`           // Display name (e.g., "Terminal" or "Terminal 2")
	Closable       bool      `json:"closable"`        // Whether the terminal can be closed
	InitialCommand string    `json:"initial_command"` // Command that was run (empty for plain shells)
	CreatedAt      time.Time `json:"created_at"`      // When the shell was created (for stable ordering)
}

// ListUserShells returns all user shells for a session, sorted by creation time.
// If no plain shell terminals exist, automatically creates the first "Terminal" entry.
func (r *InteractiveRunner) ListUserShells(sessionID string) []UserShellInfo {
	r.userShellsMu.Lock()
	defer r.userShellsMu.Unlock()

	prefix := sessionID + ":"
	var shells []UserShellInfo
	hasPlainShell := false

	for key, entry := range r.userShells {
		if strings.HasPrefix(key, prefix) {
			terminalID := key[len(prefix):]
			// Check if process is still alive
			_, running := r.Get(entry.ProcessID, false)
			shells = append(shells, UserShellInfo{
				TerminalID:     terminalID,
				ProcessID:      entry.ProcessID,
				Running:        running,
				Label:          entry.Label,
				Closable:       entry.Closable,
				InitialCommand: entry.InitialCommand,
				CreatedAt:      entry.CreatedAt,
			})
			// Check if this is a plain shell (not a script terminal)
			if entry.InitialCommand == "" {
				hasPlainShell = true
			}
		}
	}

	// Auto-create the first "Terminal" if no plain shells exist
	if !hasPlainShell {
		terminalID := "shell-" + uuid.New().String()
		now := time.Now().UTC()
		entry := &userShellEntry{
			ProcessID:      "", // No process yet - will be started when WebSocket connects
			Label:          "Terminal",
			InitialCommand: "",
			Closable:       false, // First terminal is not closable
			CreatedAt:      now,
		}
		r.userShells[prefix+terminalID] = entry

		shells = append(shells, UserShellInfo{
			TerminalID:     terminalID,
			ProcessID:      "",
			Running:        false,
			Label:          "Terminal",
			Closable:       false,
			InitialCommand: "",
			CreatedAt:      now,
		})
	}

	// Sort by creation time for stable ordering
	sort.Slice(shells, func(i, j int) bool {
		return shells[i].CreatedAt.Before(shells[j].CreatedAt)
	})

	return shells
}

// StopUserShell stops a user shell for a terminal tab.
func (r *InteractiveRunner) StopUserShell(ctx context.Context, sessionID, terminalID string) error {
	key := sessionID + ":" + terminalID

	r.userShellsMu.Lock()
	entry, exists := r.userShells[key]
	if exists {
		delete(r.userShells, key)
	}
	r.userShellsMu.Unlock()

	if !exists {
		return nil
	}

	r.logger.Info("stopping user shell",
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
		zap.String("process_id", entry.ProcessID))

	return r.Stop(ctx, entry.ProcessID)
}

// ResizeUserShell resizes the PTY for a user shell.
func (r *InteractiveRunner) ResizeUserShell(sessionID, terminalID string, cols, rows uint16) error {
	key := sessionID + ":" + terminalID

	r.userShellsMu.RLock()
	entry, exists := r.userShells[key]
	r.userShellsMu.RUnlock()

	if !exists {
		return fmt.Errorf("no user shell found for session %s terminal %s", sessionID, terminalID)
	}

	proc, ok := r.get(entry.ProcessID)
	if !ok {
		return fmt.Errorf("process not found: %s", entry.ProcessID)
	}

	return r.lazyStartAndResize(proc, cols, rows,
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
	)
}

// GetUserShellPtyWriter returns the PTY writer for a user shell.
func (r *InteractiveRunner) GetUserShellPtyWriter(sessionID, terminalID string) (io.Writer, string, error) {
	key := sessionID + ":" + terminalID

	r.userShellsMu.RLock()
	entry, exists := r.userShells[key]
	r.userShellsMu.RUnlock()

	if !exists {
		return nil, "", fmt.Errorf("no user shell found for session %s terminal %s", sessionID, terminalID)
	}

	writer, err := r.GetPtyWriter(entry.ProcessID)
	if err != nil {
		return nil, entry.ProcessID, err
	}

	return writer, entry.ProcessID, nil
}

// ClearUserShellDirectOutput clears the direct output for a user shell.
func (r *InteractiveRunner) ClearUserShellDirectOutput(sessionID, terminalID string) {
	key := sessionID + ":" + terminalID

	r.userShellsMu.RLock()
	entry, exists := r.userShells[key]
	r.userShellsMu.RUnlock()

	if !exists {
		return
	}

	proc, ok := r.get(entry.ProcessID)
	if !ok {
		return
	}

	proc.directOutputMu.Lock()
	proc.directOutput = nil
	proc.hasActiveWebSocket = false
	proc.directOutputMu.Unlock()

	r.logger.Info("direct output cleared for user shell",
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID))
}

// containsDSRQuery checks if data contains a Device Status Report (cursor position) query.
// DSR query is: ESC [ 6 n or ESC [ ? 6 n
func containsDSRQuery(data []byte) bool {
	return bytes.Contains(data, []byte("\x1b[6n")) || bytes.Contains(data, []byte("\x1b[?6n"))
}

// containsDA1Query checks if data contains a Primary Device Attributes query.
// DA1 query is: ESC [ c or ESC [ 0 c (but NOT ESC [ <digit> c where digit is 1-9,
// since those are cursor forward sequences).
func containsDA1Query(data []byte) bool {
	for i := 0; i+2 < len(data); i++ {
		if data[i] != '\x1b' || data[i+1] != '[' {
			continue
		}
		// ESC [ c — DA1 with no parameter
		if data[i+2] == 'c' {
			return true
		}
		// ESC [ 0 c — DA1 with explicit 0 parameter
		if data[i+2] == '0' && i+3 < len(data) && data[i+3] == 'c' {
			return true
		}
		// ESC [ <1-9> c would be cursor forward — skip it
	}
	return false
}

