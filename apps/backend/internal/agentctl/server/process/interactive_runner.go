// Package process provides background process execution and output streaming for agentctl.
//
// InteractiveRunner extends the pattern from ProcessRunner to support interactive
// CLI passthrough sessions where users interact directly with agent CLIs through
// a PTY-backed terminal.

package process

import (
	"io"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// InteractiveStartRequest contains parameters for starting an interactive passthrough process.
type InteractiveStartRequest struct {
	SessionID            string            `json:"session_id"`                       // Required: Agent session owning this process
	Command              []string          `json:"command"`                          // Required: Command and args to execute
	WorkingDir           string            `json:"working_dir"`                      // Working directory
	Env                  map[string]string `json:"env,omitempty"`                    // Additional environment variables
	PromptPattern        string            `json:"prompt_pattern,omitempty"`         // Regex pattern to detect agent prompt for turn completion
	IdleTimeout          time.Duration     `json:"idle_timeout,omitempty"`           // Idle timeout for turn detection
	BufferMaxBytes       int64             `json:"buffer_max_bytes,omitempty"`       // Max output buffer size
	StatusDetector       string            `json:"status_detector,omitempty"`        // Status detector type: "claude_code", "codex", ""
	CheckInterval        time.Duration     `json:"check_interval,omitempty"`         // How often to check state (default 100ms)
	StabilityWindow      time.Duration     `json:"stability_window,omitempty"`       // State stability window (default 0)
	ImmediateStart       bool              `json:"immediate_start,omitempty"`        // Start immediately with default dimensions (don't wait for resize)
	DefaultCols          int               `json:"default_cols,omitempty"`           // Default columns if ImmediateStart (default 120)
	DefaultRows          int               `json:"default_rows,omitempty"`           // Default rows if ImmediateStart (default 40)
	InitialCommand       string            `json:"initial_command,omitempty"`        // Command to write to stdin after shell starts (for script terminals)
	DisableTurnDetection bool              `json:"disable_turn_detection,omitempty"` // Disable idle timer and turn detection (for user shell terminals)
	IsUserShell          bool              `json:"is_user_shell,omitempty"`          // Mark as user shell process (excluded from session-level lookups)
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
	writer   DirectOutputWriter
	lastCols uint16
	lastRows uint16
	mu       sync.RWMutex
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
	logger               *logger.Logger
	workspaceTracker     *WorkspaceTracker
	bufferMaxBytes       int64
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
