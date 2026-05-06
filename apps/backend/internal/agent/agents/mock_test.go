package agents

import (
	"regexp"
	"testing"

	"github.com/kandev/kandev/pkg/agent"
)

func TestMockAgent_BuildCommand_NoModelFlag(t *testing.T) {
	// ACP agents apply model via session/set_model after session/new, not
	// via --model CLI flag. BuildCommand must not add --model.
	a := NewMockAgent()
	cmd := a.BuildCommand(CommandOptions{Model: "mock-fast"})
	args := cmd.Args()
	for _, arg := range args {
		if arg == "--model" {
			t.Errorf("--model CLI flag should not be emitted, got %v", args)
		}
	}
}

func TestMockAgent_BuildCommand_NoResumeFlag(t *testing.T) {
	a := NewMockAgent()
	// ACP handles resume via session/load, so --resume should not appear.
	cmd := a.BuildCommand(CommandOptions{Model: "mock-fast", SessionID: "sess-123"})
	args := cmd.Args()

	for _, arg := range args {
		if arg == "--resume" {
			t.Errorf("--resume flag should not be present for ACP agent, got args %v", args)
		}
	}
}

func TestMockAgent_Runtime_CanRecover(t *testing.T) {
	a := NewMockAgent()
	rt := a.Runtime()

	if !rt.SessionConfig.SupportsRecovery() {
		t.Error("expected SupportsRecovery() to return true for mock agent")
	}
}

func TestMockAgent_Runtime_ProtocolACP(t *testing.T) {
	a := NewMockAgent()
	rt := a.Runtime()

	if rt.Protocol != agent.ProtocolACP {
		t.Errorf("expected Protocol = %q, got %q", agent.ProtocolACP, rt.Protocol)
	}
}

func TestMockAgent_Runtime_NoResumeFlag(t *testing.T) {
	a := NewMockAgent()
	rt := a.Runtime()

	// ACP agents handle resume via session/load, not CLI flags.
	if !rt.SessionConfig.ResumeFlag.IsEmpty() {
		t.Error("expected ResumeFlag to be empty for ACP agent")
	}
}

func TestMockAgent_PassthroughConfig_ResumeFlags(t *testing.T) {
	a := NewMockAgent()
	pt := a.PassthroughConfig()

	// Generic resume flag (-c) — still used for TUI passthrough mode
	if pt.ResumeFlag.IsEmpty() {
		t.Error("expected ResumeFlag to be set on PassthroughConfig")
	}
	resumeArgs := pt.ResumeFlag.Args()
	if len(resumeArgs) == 0 || resumeArgs[0] != "-c" {
		t.Errorf("expected ResumeFlag = -c, got %v", resumeArgs)
	}

	// Session-specific resume flag (--resume) — TUI passthrough mode
	if pt.SessionResumeFlag.IsEmpty() {
		t.Error("expected SessionResumeFlag to be set on PassthroughConfig")
	}
	sessionArgs := pt.SessionResumeFlag.Args()
	if len(sessionArgs) == 0 || sessionArgs[0] != "--resume" {
		t.Errorf("expected SessionResumeFlag = --resume, got %v", sessionArgs)
	}
}

func TestNewMockAgentWithID_AppliesIdentityOverrides(t *testing.T) {
	a := NewMockAgentWithID("claude-acp", "Mock Claude", "Claude (Mock)")
	if a.ID() != "claude-acp" {
		t.Errorf("ID() = %q, want claude-acp", a.ID())
	}
	if a.Name() != "Mock Claude" {
		t.Errorf("Name() = %q, want Mock Claude", a.Name())
	}
	if a.DisplayName() != "Claude (Mock)" {
		t.Errorf("DisplayName() = %q, want Claude (Mock)", a.DisplayName())
	}
}

func TestNewMockAgent_DefaultIdentity(t *testing.T) {
	a := NewMockAgent()
	if a.ID() != "mock-agent" {
		t.Errorf("ID() = %q, want mock-agent", a.ID())
	}
	if a.Name() != "Mock Agent" {
		t.Errorf("Name() = %q, want Mock Agent", a.Name())
	}
	if a.DisplayName() != "Mock" {
		t.Errorf("DisplayName() = %q, want Mock", a.DisplayName())
	}
}

func TestMockAgent_PassthroughConfig_PromptPattern(t *testing.T) {
	a := NewMockAgent()
	pattern := a.PassthroughConfig().PromptPattern
	if pattern == "" {
		t.Fatal("expected PromptPattern to be set for mock TUI passthrough")
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("expected PromptPattern to compile: %v", err)
	}

	readyPrompt := "\x1b[1;32m❯\x1b[0m "
	if !re.MatchString(readyPrompt) {
		t.Fatalf("expected PromptPattern to match ready prompt %q", readyPrompt)
	}

	initialPromptLine := "\x1b[1;32m❯\x1b[0m hello from stdin\r\n"
	if re.MatchString(initialPromptLine) {
		t.Fatalf("expected PromptPattern not to match prompt echo %q", initialPromptLine)
	}
}
