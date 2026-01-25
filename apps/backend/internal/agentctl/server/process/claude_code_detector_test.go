package process

import (
	"testing"

	"github.com/tuzig/vt10x"
)

func TestClaudeCodeDetector_DetectState_Working(t *testing.T) {
	detector := NewClaudeCodeDetector()

	tests := []struct {
		name     string
		lines    []string
		expected AgentState
	}{
		{
			name: "working with ellipsis and esc interrupt",
			lines: []string{
				"",
				"✻ Billowing... (esc to interrupt)",
				"",
			},
			expected: StateWorking,
		},
		{
			name: "working with dots and ctrl+c interrupt",
			lines: []string{
				"",
				"✻ Reading files... (ctrl+c to interrupt)",
				"",
			},
			expected: StateWorking,
		},
		{
			name: "working with different bullet symbol",
			lines: []string{
				"* Processing request... (esc to interrupt)",
			},
			expected: StateWorking,
		},
		{
			name: "working with star symbol",
			lines: []string{
				"  ★ Analyzing code... (esc to interrupt)",
			},
			expected: StateWorking,
		},
		{
			name: "not working - no interrupt hint",
			lines: []string{
				"✻ Billowing…",
			},
			expected: StateUnknown,
		},
		{
			name: "not working - incomplete pattern",
			lines: []string{
				"Some random text",
			},
			expected: StateUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create empty glyphs slice matching lines
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

func TestClaudeCodeDetector_DetectState_WaitingInput(t *testing.T) {
	detector := NewClaudeCodeDetector()

	tests := []struct {
		name     string
		lines    []string
		expected AgentState
	}{
		{
			name: "tip line indicates waiting for input",
			lines: []string{
				"Some output",
				"⎿ Tip: Press Enter to continue",
				"",
			},
			expected: StateWaitingInput,
		},
		{
			name: "tip with non-breaking spaces",
			lines: []string{
				"\u00a0\u00a0⎿\u00a0Tip: Use arrow keys",
			},
			expected: StateWaitingInput,
		},
		{
			name: "hint line",
			lines: []string{
				"⎿ Hint: Type your message",
			},
			expected: StateWaitingInput,
		},
		{
			name: "next line",
			lines: []string{
				"⎿ Next: Choose an option",
			},
			expected: StateWaitingInput,
		},
		{
			name: "input box with tip inside",
			lines: []string{
				"────────────────────",
				"",
				"⎿ Tip: Enter to send",
				"────────────────────",
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

func TestClaudeCodeDetector_DetectState_WaitingApproval(t *testing.T) {
	detector := NewClaudeCodeDetector()

	tests := []struct {
		name     string
		lines    []string
		expected AgentState
	}{
		{
			name: "enter to select prompt",
			lines: []string{
				"Options:",
				"1. Option A",
				"2. Option B",
				"Press enter to select",
			},
			expected: StateWaitingApproval,
		},
		{
			name: "do you want to proceed",
			lines: []string{
				"Changes detected",
				"Do you want to proceed?",
			},
			expected: StateWaitingApproval,
		},
		{
			name: "do you want to create file",
			lines: []string{
				"Do you want to create this file?",
			},
			expected: StateWaitingApproval,
		},
		{
			name: "submit answers prompt",
			lines: []string{
				"Ready to submit your answers?",
			},
			expected: StateWaitingApproval,
		},
		{
			name: "yes/no with allow",
			lines: []string{
				"Allow access to file? [y/n]",
			},
			expected: StateWaitingApproval,
		},
		{
			name: "yes/no with approve",
			lines: []string{
				"Approve changes? [Y/N]",
			},
			expected: StateWaitingApproval,
		},
		{
			name: "selection arrow with confirm nearby",
			lines: []string{
				"❯ 1. First option",
				"  2. Second option",
				"Press Enter to confirm",
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

func TestClaudeCodeDetector_ShouldAcceptStateChange(t *testing.T) {
	detector := NewClaudeCodeDetector()

	// Claude Code detector always accepts state changes (no stability window)
	tests := []struct {
		current  AgentState
		new      AgentState
		expected bool
	}{
		{StateUnknown, StateWorking, true},
		{StateWorking, StateWaitingInput, true},
		{StateWorking, StateUnknown, true},
		{StateWaitingApproval, StateWorking, true},
	}

	for _, tt := range tests {
		result := detector.ShouldAcceptStateChange(tt.current, tt.new)
		if result != tt.expected {
			t.Errorf("ShouldAcceptStateChange(%v, %v) = %v, want %v",
				tt.current, tt.new, result, tt.expected)
		}
	}
}

func TestClaudeCodeDetector_FindSeparatorLines(t *testing.T) {
	detector := NewClaudeCodeDetector()

	lines := []string{
		"Some text",
		"──────────────────────────",
		"Input area",
		"──────────────────────────",
		"More text",
	}

	indices := detector.findSeparatorLines(lines)

	if len(indices) != 2 {
		t.Errorf("findSeparatorLines() found %d separators, want 2", len(indices))
	}
	if len(indices) >= 2 {
		if indices[0] != 1 || indices[1] != 3 {
			t.Errorf("findSeparatorLines() = %v, want [1, 3]", indices)
		}
	}
}
