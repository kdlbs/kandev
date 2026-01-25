// Package process provides background process execution and output streaming for agentctl.
//
// CodexDetector implements agent-specific TUI pattern matching for OpenAI Codex CLI
// to detect states: Working, WaitingApproval, WaitingInput.
//
// Codex requires a stability window because its TUI has intermittent output during
// working state - we need to ensure we don't falsely exit working state.

package process

import (
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tuzig/vt10x"
)

// CodexDetector detects Codex CLI states by analyzing the terminal TUI.
type CodexDetector struct {
	mu                  sync.Mutex
	lastWorkingDetected time.Time
}

// NewCodexDetector creates a new Codex detector.
func NewCodexDetector() *CodexDetector {
	return &CodexDetector{}
}

const (
	// codexMinWorkingExitInterval prevents false exits from working state
	// Codex has intermittent output during processing, so we require 1 second
	// of stability before accepting a transition out of working state.
	codexMinWorkingExitInterval = 1000 * time.Millisecond
)

var (
	// Working State Detection
	// Pattern: Bullet + action + timer
	// Example: "• Working (65s • esc to interrupt)"
	// Example: "◦ Processing request (2m 30s • esc to interrupt)"
	codexWorkingPattern = regexp.MustCompile(
		`^[•◦]\s*.+\(?(\d+h\s+)?(\d+m\s+)?\d+s\s*[•·]\s*(esc|ctrl\+c)\s+to\s+interrup(t)?\)?`,
	)

	// Worked Line Pattern (task complete, waiting for input)
	// Example: "─ Worked for 2m 30s─────────"
	codexWorkedPattern = regexp.MustCompile(`^─\s*Worked\s+for\s+.+─+$`)

	// Starting MCP servers noise (should be ignored during detection)
	mcpServersPattern = regexp.MustCompile(`(?i)starting\s+mcp\s+servers?`)

	// Codex approval patterns
	// Selection arrow with numbered option
	codexSelectionPattern = regexp.MustCompile(`^[›❯]\s*\d+\.\s+`)

	// Confirmation prompt
	codexConfirmPattern = regexp.MustCompile(`(?i)press\s+enter\s+to\s+confirm`)

	// Cancel prompt
	codexCancelPattern = regexp.MustCompile(`(?i)esc\s+to\s+cancel`)

	// Approval required prompt
	codexApprovalPattern = regexp.MustCompile(`(?i)(approve|allow|confirm|proceed)\s*\?`)
)

// DetectState examines the visible terminal content and returns the detected state.
func (d *CodexDetector) DetectState(lines []string, glyphs [][]vt10x.Glyph) AgentState {
	// Check for approval prompts first
	if state := d.detectApproval(lines); state != StateUnknown {
		return state
	}

	// Check for working and waiting states
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")

		// Skip MCP servers noise
		if mcpServersPattern.MatchString(line) {
			continue
		}

		// Check for working pattern
		if codexWorkingPattern.MatchString(line) {
			d.mu.Lock()
			d.lastWorkingDetected = time.Now()
			d.mu.Unlock()
			return StateWorking
		}

		// Check for worked pattern (completed, waiting for input)
		if codexWorkedPattern.MatchString(line) {
			return StateWaitingInput
		}
	}

	return StateUnknown
}

// ShouldAcceptStateChange determines if a state transition should be accepted.
// Codex needs a stability window when exiting working state to prevent false transitions.
func (d *CodexDetector) ShouldAcceptStateChange(currentState, newState AgentState) bool {
	// If exiting working state, require stability window
	if currentState == StateWorking && newState != StateWorking {
		d.mu.Lock()
		lastWorking := d.lastWorkingDetected
		d.mu.Unlock()

		if time.Since(lastWorking) < codexMinWorkingExitInterval {
			// Don't accept - not enough time has passed since last working detection
			return false
		}
	}

	return true
}

// detectApproval detects approval prompts in Codex TUI.
func (d *CodexDetector) detectApproval(lines []string) AgentState {
	hasSelectionArrow := false
	hasConfirmPrompt := false

	for i, line := range lines {
		line = strings.TrimRight(line, " \t")

		// Check for selection arrow
		if codexSelectionPattern.MatchString(line) {
			hasSelectionArrow = true

			// Check nearby lines for confirmation prompt
			for j := i + 1; j < len(lines) && j < i+5; j++ {
				nearbyLine := strings.TrimRight(lines[j], " \t")
				if codexConfirmPattern.MatchString(nearbyLine) ||
					codexCancelPattern.MatchString(nearbyLine) {
					hasConfirmPrompt = true
					break
				}
			}

			if hasConfirmPrompt {
				return StateWaitingApproval
			}
		}

		// Direct approval pattern
		if codexApprovalPattern.MatchString(line) {
			return StateWaitingApproval
		}

		// Standalone confirm/cancel pattern
		if codexConfirmPattern.MatchString(line) && codexCancelPattern.MatchString(line) {
			return StateWaitingApproval
		}
	}

	// If we found a selection arrow without explicit confirmation,
	// still consider it as potentially needing approval
	if hasSelectionArrow {
		return StateWaitingApproval
	}

	return StateUnknown
}
