package agents

import (
	"testing"
	"time"
)

// Custom TUI agents wrap arbitrary CLIs, commonly Ink-based TUIs such as Claude
// Code. Ink coalesces multi-byte stdin reads into a paste burst and absorbs a
// trailing "\r" into the pasted content instead of dispatching Enter, and it
// enables bracketed-paste mode so ESC[200~…ESC[201~ delimiters break input.
// The built-in Claude passthrough agent works around both; custom TUI agents
// must inherit the same defaults or programmatic PTY prompts (peer messaging,
// queued-message drain, workflow auto-start) land in the input box unsubmitted.
func TestNewTUIAgentDefaultsToInkSafePassthrough(t *testing.T) {
	a := NewTUIAgent(TUIAgentConfig{
		AgentID:   "custom-tui",
		AgentName: "custom-tui",
		Command:   "claude",
		Desc:      "custom",
	})

	pt := a.PassthroughConfig()
	if !pt.DisableBracketedPaste {
		t.Error("DisableBracketedPaste = false, want true (send prompt bytes verbatim; Ink breaks on bracketed-paste delimiters)")
	}
	if pt.SubmitDelay != 150*time.Millisecond {
		t.Errorf("SubmitDelay = %v, want 150ms (split submit byte into a discrete keystroke so Ink does not absorb it into a paste burst)", pt.SubmitDelay)
	}
	if got := EffectiveSubmitSequence(pt.SubmitSequence); got != "\r" {
		t.Errorf("effective submit sequence = %q, want %q", got, "\r")
	}
}
