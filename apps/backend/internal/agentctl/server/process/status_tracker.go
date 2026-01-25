// Package process provides background process execution and output streaming for agentctl.
//
// StatusTracker uses a virtual terminal emulator (vt10x) to parse agent TUI output
// and detect agent states (working, waiting for approval, waiting for input).

package process

import (
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/tuzig/vt10x"
	"go.uber.org/zap"
)

// AgentState represents the detected state of an agent CLI.
type AgentState string

const (
	StateUnknown         AgentState = "unknown"
	StateWorking         AgentState = "working"
	StateWaitingApproval AgentState = "waiting_approval"
	StateWaitingInput    AgentState = "waiting_input"
)

// StatusDetector is implemented by agent-specific detectors to detect state from terminal content.
type StatusDetector interface {
	// DetectState examines the visible terminal content and returns the detected state.
	// lines contains the visible text lines, glyphs contains the full glyph data with attributes.
	DetectState(lines []string, glyphs [][]vt10x.Glyph) AgentState

	// ShouldAcceptStateChange determines if a state transition should be accepted.
	// This allows detectors to implement stability windows for debouncing.
	ShouldAcceptStateChange(currentState, newState AgentState) bool
}

// StateChangeCallback is called when the detected state changes.
type StateChangeCallback func(sessionID string, state AgentState)

// StatusTrackerConfig contains configuration for the status tracker.
type StatusTrackerConfig struct {
	Rows              int           // Terminal rows (default 24)
	Cols              int           // Terminal columns (default 80)
	CheckInterval     time.Duration // How often to check state (default 100ms)
	StabilityWindow   time.Duration // Time state must be stable before reporting (default 0)
}

// DefaultStatusTrackerConfig returns the default configuration.
func DefaultStatusTrackerConfig() StatusTrackerConfig {
	return StatusTrackerConfig{
		Rows:            24,
		Cols:            80,
		CheckInterval:   100 * time.Millisecond,
		StabilityWindow: 0,
	}
}

// StatusTracker uses a virtual terminal emulator to track agent state.
type StatusTracker struct {
	logger      *logger.Logger
	sessionID   string
	detector    StatusDetector
	callback    StateChangeCallback
	config      StatusTrackerConfig

	// vt10x terminal emulator
	term vt10x.Terminal

	// State tracking
	mu               sync.Mutex
	lastState        AgentState
	lastStateChange  time.Time
	lastCheck        time.Time
	pendingState     AgentState      // State waiting for stability window
	pendingStateTime time.Time       // When pending state was first detected
}

// NewStatusTracker creates a new status tracker with the given detector.
func NewStatusTracker(
	sessionID string,
	detector StatusDetector,
	callback StateChangeCallback,
	config StatusTrackerConfig,
	log *logger.Logger,
) *StatusTracker {
	if config.Rows <= 0 {
		config.Rows = 24
	}
	if config.Cols <= 0 {
		config.Cols = 80
	}
	if config.CheckInterval <= 0 {
		config.CheckInterval = 100 * time.Millisecond
	}

	// Create vt10x terminal emulator
	term := vt10x.New(vt10x.WithSize(config.Cols, config.Rows))

	return &StatusTracker{
		logger:      log.WithFields(zap.String("component", "status-tracker"), zap.String("session_id", sessionID)),
		sessionID:   sessionID,
		detector:    detector,
		callback:    callback,
		config:      config,
		term:        term,
		lastState:   StateUnknown,
	}
}

// Write feeds data to the virtual terminal emulator.
// This should be called with all PTY output data.
func (t *StatusTracker) Write(data []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Feed data to the virtual terminal
	_, _ = t.term.Write(data)
}

// Resize updates the virtual terminal size.
func (t *StatusTracker) Resize(cols, rows int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.term.Resize(cols, rows)
	t.config.Cols = cols
	t.config.Rows = rows
}

// ShouldCheck returns true if it's time to check the terminal state.
func (t *StatusTracker) ShouldCheck() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return time.Since(t.lastCheck) >= t.config.CheckInterval
}

// CheckAndUpdate checks the terminal state and emits a callback if the state changed.
// Returns the current state.
func (t *StatusTracker) CheckAndUpdate() AgentState {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.lastCheck = time.Now()

	// Extract visible lines and glyphs from the terminal
	lines, glyphs := t.extractTerminalContent()

	// Detect state using the agent-specific detector
	detectedState := t.detector.DetectState(lines, glyphs)

	// Handle stability window if configured
	if t.config.StabilityWindow > 0 {
		return t.handleStabilityWindow(detectedState)
	}

	// No stability window - check if detector accepts the transition
	if detectedState != t.lastState {
		if t.detector.ShouldAcceptStateChange(t.lastState, detectedState) {
			t.emitStateChange(detectedState)
		}
	}

	return t.lastState
}

// CurrentState returns the current detected state.
func (t *StatusTracker) CurrentState() AgentState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastState
}

// extractTerminalContent extracts the visible lines and glyphs from the vt10x terminal.
func (t *StatusTracker) extractTerminalContent() ([]string, [][]vt10x.Glyph) {
	rows := t.config.Rows
	cols := t.config.Cols

	lines := make([]string, rows)
	glyphs := make([][]vt10x.Glyph, rows)

	for row := 0; row < rows; row++ {
		rowGlyphs := make([]vt10x.Glyph, cols)
		var rowChars []rune

		for col := 0; col < cols; col++ {
			g := t.term.Cell(col, row)
			rowGlyphs[col] = g
			if g.Char == 0 {
				rowChars = append(rowChars, ' ')
			} else {
				rowChars = append(rowChars, g.Char)
			}
		}

		lines[row] = string(rowChars)
		glyphs[row] = rowGlyphs
	}

	return lines, glyphs
}

// handleStabilityWindow implements debouncing for state changes.
func (t *StatusTracker) handleStabilityWindow(detectedState AgentState) AgentState {
	now := time.Now()

	if detectedState != t.pendingState {
		// New state detected, start stability timer
		t.pendingState = detectedState
		t.pendingStateTime = now
		return t.lastState
	}

	// Same state as pending - check if stable long enough
	if now.Sub(t.pendingStateTime) >= t.config.StabilityWindow {
		if t.pendingState != t.lastState {
			if t.detector.ShouldAcceptStateChange(t.lastState, t.pendingState) {
				t.emitStateChange(t.pendingState)
			}
		}
	}

	return t.lastState
}

// emitStateChange updates the state and calls the callback.
// Must be called with mutex held.
func (t *StatusTracker) emitStateChange(newState AgentState) {
	oldState := t.lastState
	t.lastState = newState
	t.lastStateChange = time.Now()

	t.logger.Debug("agent state changed",
		zap.String("old_state", string(oldState)),
		zap.String("new_state", string(newState)))

	if t.callback != nil {
		// Release lock before callback to prevent deadlocks
		t.mu.Unlock()
		t.callback(t.sessionID, newState)
		t.mu.Lock()
	}
}

// NoOpDetector is a detector that always returns StateUnknown.
// Used when no detector is configured.
type NoOpDetector struct{}

func (d *NoOpDetector) DetectState(lines []string, glyphs [][]vt10x.Glyph) AgentState {
	return StateUnknown
}

func (d *NoOpDetector) ShouldAcceptStateChange(currentState, newState AgentState) bool {
	return true
}
