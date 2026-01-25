package process

import (
	"testing"
	"time"

	"github.com/tuzig/vt10x"
)

func TestCodexDetector_DetectState_Working(t *testing.T) {
	detector := NewCodexDetector()

	tests := []struct {
		name     string
		lines    []string
		expected AgentState
	}{
		{
			name: "working with seconds timer",
			lines: []string{
				"• Working (65s • esc to interrupt)",
			},
			expected: StateWorking,
		},
		{
			name: "working with minutes and seconds",
			lines: []string{
				"• Processing request (2m 30s • esc to interrupt)",
			},
			expected: StateWorking,
		},
		{
			name: "working with hollow bullet",
			lines: []string{
				"◦ Analyzing (5s • esc to interrupt)",
			},
			expected: StateWorking,
		},
		{
			name: "working with hours minutes seconds",
			lines: []string{
				"• Long task (1h 30m 45s • esc to interrupt)",
			},
			expected: StateWorking,
		},
		{
			name: "working with ctrl+c",
			lines: []string{
				"• Working (10s • ctrl+c to interrupt)",
			},
			expected: StateWorking,
		},
		{
			name: "not working - MCP servers noise",
			lines: []string{
				"Starting MCP servers...",
				"• Working (5s • esc to interrupt)",
			},
			expected: StateWorking, // Should still detect working despite MCP noise
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			glyphs := make([][]vt10x.Glyph, len(tt.lines))
			for i, line := range tt.lines {
				glyphs[i] = make([]vt10x.Glyph, len(line))
			}

			result := detector.DetectState(tt.lines, glyphs)
			if result != tt.expected {
				t.Errorf("DetectState() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCodexDetector_DetectState_WaitingInput(t *testing.T) {
	detector := NewCodexDetector()

	tests := []struct {
		name     string
		lines    []string
		expected AgentState
	}{
		{
			name: "worked line indicates completion",
			lines: []string{
				"─ Worked for 2m 30s────────────────",
			},
			expected: StateWaitingInput,
		},
		{
			name: "worked line with different duration",
			lines: []string{
				"─ Worked for 45s─────────────────────",
			},
			expected: StateWaitingInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			glyphs := make([][]vt10x.Glyph, len(tt.lines))
			for i, line := range tt.lines {
				glyphs[i] = make([]vt10x.Glyph, len(line))
			}

			result := detector.DetectState(tt.lines, glyphs)
			if result != tt.expected {
				t.Errorf("DetectState() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCodexDetector_DetectState_WaitingApproval(t *testing.T) {
	detector := NewCodexDetector()

	tests := []struct {
		name     string
		lines    []string
		expected AgentState
	}{
		{
			name: "selection with confirm prompt",
			lines: []string{
				"› 1. Option one",
				"  2. Option two",
				"Press enter to confirm",
			},
			expected: StateWaitingApproval,
		},
		{
			name: "selection with cancel prompt",
			lines: []string{
				"❯ 1. First choice",
				"esc to cancel",
			},
			expected: StateWaitingApproval,
		},
		{
			name: "approval question with question mark",
			lines: []string{
				"Do you approve?",
			},
			expected: StateWaitingApproval,
		},
		{
			name: "confirm question with question mark",
			lines: []string{
				"Please confirm?",
			},
			expected: StateWaitingApproval,
		},
		{
			name: "confirm and cancel on same line",
			lines: []string{
				"press enter to confirm, esc to cancel",
			},
			expected: StateWaitingApproval,
		},
		{
			name: "standalone selection arrow",
			lines: []string{
				"› 1. Some option",
			},
			expected: StateWaitingApproval,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			glyphs := make([][]vt10x.Glyph, len(tt.lines))
			for i, line := range tt.lines {
				glyphs[i] = make([]vt10x.Glyph, len(line))
			}

			result := detector.DetectState(tt.lines, glyphs)
			if result != tt.expected {
				t.Errorf("DetectState() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCodexDetector_ShouldAcceptStateChange_StabilityWindow(t *testing.T) {
	detector := NewCodexDetector()

	// Set last working time to now
	detector.mu.Lock()
	detector.lastWorkingDetected = time.Now()
	detector.mu.Unlock()

	// Exiting working state immediately should be rejected
	if detector.ShouldAcceptStateChange(StateWorking, StateWaitingInput) {
		t.Error("ShouldAcceptStateChange should reject immediate exit from working state")
	}

	// Wait for stability window
	time.Sleep(codexMinWorkingExitInterval + 50*time.Millisecond)

	// Now it should be accepted
	if !detector.ShouldAcceptStateChange(StateWorking, StateWaitingInput) {
		t.Error("ShouldAcceptStateChange should accept exit from working state after stability window")
	}
}

func TestCodexDetector_ShouldAcceptStateChange_NonWorkingTransitions(t *testing.T) {
	detector := NewCodexDetector()

	// Non-working state transitions should always be accepted
	tests := []struct {
		current  AgentState
		new      AgentState
		expected bool
	}{
		{StateUnknown, StateWorking, true},
		{StateUnknown, StateWaitingInput, true},
		{StateWaitingInput, StateWorking, true},
		{StateWaitingApproval, StateWaitingInput, true},
	}

	for _, tt := range tests {
		result := detector.ShouldAcceptStateChange(tt.current, tt.new)
		if result != tt.expected {
			t.Errorf("ShouldAcceptStateChange(%v, %v) = %v, want %v",
				tt.current, tt.new, result, tt.expected)
		}
	}
}
