package agents

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
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

// TestTUIAgentDerivesSupportsMCPFromStrategy pins the mechanism that makes the
// flag survive. Discovery writes IsInstalled's SupportsMCP back over the
// agents.supports_mcp column on every sweep (controller.upsertAgent), so a flag
// set independently at creation is reverted within a boot. Deriving it here
// makes that write keep the flag correct instead of clobbering it.
func TestTUIAgentDerivesSupportsMCPFromStrategy(t *testing.T) {
	withStrategy := NewTUIAgent(TUIAgentConfig{
		AgentID: "with", AgentName: "with", Command: "echo",
		MCPStrategy: mcpconfig.ClaudeStrategy{},
	})
	result, err := withStrategy.IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled: %v", err)
	}
	if !result.SupportsMCP {
		t.Error("SupportsMCP = false for an agent with a strategy, want true")
	}

	without := NewTUIAgent(TUIAgentConfig{AgentID: "without", AgentName: "without", Command: "echo"})
	result, err = without.IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled: %v", err)
	}
	if result.SupportsMCP {
		t.Error("SupportsMCP = true for an agent with no strategy, want false")
	}
}

// TestTUIAgentCarriesStrategyIntoPassthroughConfig pins the field the launch
// path actually reads: applyPassthroughMCP returns immediately when
// PassthroughConfig.MCPStrategy is nil, so a strategy that does not land here
// injects nothing no matter what else is configured.
func TestTUIAgentCarriesStrategyIntoPassthroughConfig(t *testing.T) {
	a := NewTUIAgent(TUIAgentConfig{
		AgentID: "x", AgentName: "x", Command: "echo",
		MCPStrategy: mcpconfig.CodexStrategy{},
	})
	if a.PassthroughConfig().MCPStrategy == nil {
		t.Fatal("PassthroughConfig.MCPStrategy = nil, want the configured strategy")
	}
}

// TestInteractiveMCPToolsSplitFromPassthroughOnly pins the distinction between
// two questions that used to share one predicate. IsPassthroughOnly governs
// execution mode (it seeds the default profile as CLI passthrough), so it must
// stay true for every TUI agent. Interactive-tool registration is separate: a
// TUI agent wrapping a real MCP-capable CLI should get ask_user_question, which
// it never could while both answers came from the same type assertion.
func TestInteractiveMCPToolsSplitFromPassthroughOnly(t *testing.T) {
	withStrategy := NewTUIAgent(TUIAgentConfig{
		AgentID: "with", AgentName: "with", Command: "echo",
		MCPStrategy: mcpconfig.ClaudeStrategy{},
	})
	without := NewTUIAgent(TUIAgentConfig{AgentID: "without", AgentName: "without", Command: "echo"})

	// Both are passthrough-only: neither has an ACP protocol mode. Weakening
	// this would seed them as ACP agents and break them outright.
	if !IsPassthroughOnly(withStrategy) {
		t.Error("IsPassthroughOnly = false for a TUI agent with a strategy, want true")
	}
	if !IsPassthroughOnly(without) {
		t.Error("IsPassthroughOnly = false for a plain TUI agent, want true")
	}

	if !SupportsInteractiveMCPTools(withStrategy) {
		t.Error("SupportsInteractiveMCPTools = false for a TUI agent with a strategy, want true")
	}
	if SupportsInteractiveMCPTools(without) {
		t.Error("SupportsInteractiveMCPTools = true for a plain TUI agent, want false")
	}
}
