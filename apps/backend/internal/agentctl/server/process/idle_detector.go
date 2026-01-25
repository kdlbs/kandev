// Package process provides background process execution and output streaming for agentctl.
//
// IdleDetector is a simple detector that uses idle timeout to detect when the agent
// has stopped producing output and is likely waiting for input.

package process

import (
	"github.com/tuzig/vt10x"
)

// IdleDetector detects agent idle state based on lack of output.
// Unlike TUI-based detectors, this simply relies on the idle timeout mechanism
// in the InteractiveRunner to trigger turn completion.
type IdleDetector struct{}

// NewIdleDetector creates a new idle-based detector.
func NewIdleDetector() *IdleDetector {
	return &IdleDetector{}
}

// DetectState always returns StateUnknown since idle detection is handled
// by the InteractiveRunner's idle timer, not by analyzing terminal content.
// When the idle timer fires, it triggers turn complete directly.
func (d *IdleDetector) DetectState(lines []string, glyphs [][]vt10x.Glyph) AgentState {
	// Idle detection doesn't analyze terminal content.
	// Turn completion is triggered by the idle timer in InteractiveRunner.
	return StateUnknown
}

// ShouldAcceptStateChange always returns true since state changes are handled
// by the idle timer mechanism.
func (d *IdleDetector) ShouldAcceptStateChange(currentState, newState AgentState) bool {
	return true
}
