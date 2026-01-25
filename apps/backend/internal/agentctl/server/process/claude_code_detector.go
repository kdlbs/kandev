// Package process provides background process execution and output streaming for agentctl.
//
// ClaudeCodeDetector implements agent-specific TUI pattern matching for Claude Code CLI
// to detect states: Working, WaitingApproval, WaitingInput.

package process

import (
	"regexp"
	"strings"

	"github.com/tuzig/vt10x"
)

// ClaudeCodeDetector detects Claude Code CLI states by analyzing the terminal TUI.
type ClaudeCodeDetector struct{}

// NewClaudeCodeDetector creates a new Claude Code detector.
func NewClaudeCodeDetector() *ClaudeCodeDetector {
	return &ClaudeCodeDetector{}
}

var (
	// Working State Detection
	// Pattern: Task line with symbol + ellipsis + interrupt hint
	// Example: "✻ Billowing… (ctrl+c to interrupt)"
	// Example: "✻ Reading files... (esc to interrupt)"
	workingTaskPattern = regexp.MustCompile(
		`^\s*[✻✽✶∴·○◆▪▫□■☐☑☒★☆✓✔✗✘⚬⚫⚪⬤◯▸▹►▻◂◃◄◅✢*]\s+.+[…\.]{2,}\s*\((esc|ctrl\+c)\s+to\s+interrupt`,
	)

	// Tip/Hint line pattern (indicates waiting for input)
	// Example: "⎿ Tip: Press Enter to continue"
	tipPattern = regexp.MustCompile(`^[\s\x{00a0}]*⎿[\s\x{00a0}]+(?:Tip|Next|Hint):`)

	// Separator line pattern (input box boundaries)
	// A line that's mostly horizontal box-drawing characters
	separatorPattern = regexp.MustCompile(`^[─━═┄┅┈┉\-]{10,}$`)

	// Approval patterns
	// Pattern: Navigation prompt (menu selection)
	enterToSelectPattern = regexp.MustCompile(`(?i)enter\s+to\s+select`)

	// Pattern: Answer submission
	submitAnswersPattern = regexp.MustCompile(`(?i)ready\s+to\s+submit\s+your\s+answers`)

	// Pattern: File operations
	doYouWantToPattern = regexp.MustCompile(`(?i)do\s+you\s+want\s+to\s+`)

	// Pattern: Procedural confirmation
	doYouWantToProceedPattern = regexp.MustCompile(`(?i)do\s+you\s+want\s+to\s+proceed`)

	// Pattern: Selection arrow for menu
	selectionArrowPattern = regexp.MustCompile(`^[\s]*[❯>]\s*\d+\.`)

	// Pattern: Yes/No confirmation prompt
	yesNoPattern = regexp.MustCompile(`(?i)\[?y/?n\]?`)
)

// DetectState examines the visible terminal content and returns the detected state.
func (d *ClaudeCodeDetector) DetectState(lines []string, glyphs [][]vt10x.Glyph) AgentState {
	// Try approval detection first (highest priority)
	if state := d.detectApproval(lines); state != StateUnknown {
		return state
	}

	// Then working/waiting detection
	return d.detectWorkingAndWaiting(lines, glyphs)
}

// ShouldAcceptStateChange determines if a state transition should be accepted.
// Claude Code doesn't need a stability window.
func (d *ClaudeCodeDetector) ShouldAcceptStateChange(currentState, newState AgentState) bool {
	return true
}

// detectWorkingAndWaiting detects working and waiting for input states.
func (d *ClaudeCodeDetector) detectWorkingAndWaiting(lines []string, glyphs [][]vt10x.Glyph) AgentState {
	// Find input box boundaries (lines of horizontal separator characters)
	separatorIndices := d.findSeparatorLines(lines)

	// Check for working task pattern in the visible lines
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if workingTaskPattern.MatchString(line) {
			return StateWorking
		}
	}

	// If we have an input box, look for tip/hint line (indicates waiting for input)
	if len(separatorIndices) >= 2 {
		inputBoxStart := separatorIndices[len(separatorIndices)-2]
		inputBoxEnd := separatorIndices[len(separatorIndices)-1]

		for i := inputBoxEnd - 1; i >= inputBoxStart; i-- {
			if i >= 0 && i < len(lines) {
				if tipPattern.MatchString(lines[i]) {
					return StateWaitingInput
				}
			}
		}
	}

	// Also check for tip pattern anywhere in the visible area
	for _, line := range lines {
		if tipPattern.MatchString(line) {
			return StateWaitingInput
		}
	}

	return StateUnknown
}

// detectApproval detects approval prompts.
func (d *ClaudeCodeDetector) detectApproval(lines []string) AgentState {
	// Search from bottom up since prompts are typically at the bottom
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimRight(lines[i], " \t")

		// Check for various approval patterns
		if enterToSelectPattern.MatchString(line) {
			return StateWaitingApproval
		}

		if doYouWantToProceedPattern.MatchString(line) {
			return StateWaitingApproval
		}

		if submitAnswersPattern.MatchString(line) {
			return StateWaitingApproval
		}

		// "Do you want to [create/delete/etc]"
		if doYouWantToPattern.MatchString(line) {
			return StateWaitingApproval
		}

		// Selection arrow with confirmation context
		if selectionArrowPattern.MatchString(line) {
			// Check if there's a confirmation prompt nearby
			for j := i + 1; j < len(lines) && j < i+5; j++ {
				nearbyLine := strings.TrimRight(lines[j], " \t")
				if strings.Contains(strings.ToLower(nearbyLine), "confirm") ||
					strings.Contains(strings.ToLower(nearbyLine), "enter to") {
					return StateWaitingApproval
				}
			}
		}

		// Yes/No prompt
		if yesNoPattern.MatchString(line) && (strings.Contains(strings.ToLower(line), "?") ||
			strings.Contains(strings.ToLower(line), "allow") ||
			strings.Contains(strings.ToLower(line), "approve")) {
			return StateWaitingApproval
		}
	}

	return StateUnknown
}

// findSeparatorLines finds lines that are horizontal separators (input box boundaries).
func (d *ClaudeCodeDetector) findSeparatorLines(lines []string) []int {
	var indices []int

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) >= 10 && separatorPattern.MatchString(trimmed) {
			indices = append(indices, i)
		}
	}

	return indices
}
