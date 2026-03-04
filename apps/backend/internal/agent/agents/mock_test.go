package agents

import (
	"testing"
)

func TestMockAgent_BuildCommand_WithSessionID(t *testing.T) {
	a := NewMockAgent()
	cmd := a.BuildCommand(CommandOptions{Model: "mock-fast", SessionID: "sess-123"})
	args := cmd.Args()

	foundResume := false
	for i, arg := range args {
		if arg == "--resume" {
			foundResume = true
			if i+1 >= len(args) {
				t.Fatal("--resume flag has no value")
			}
			if args[i+1] != "sess-123" {
				t.Errorf("--resume value = %q, want %q", args[i+1], "sess-123")
			}
			break
		}
	}
	if !foundResume {
		t.Errorf("expected --resume flag in args %v", args)
	}
}

func TestMockAgent_BuildCommand_WithoutSessionID(t *testing.T) {
	a := NewMockAgent()
	cmd := a.BuildCommand(CommandOptions{Model: "mock-fast"})
	args := cmd.Args()

	for _, arg := range args {
		if arg == "--resume" {
			t.Errorf("--resume flag should not be present without SessionID, got args %v", args)
		}
	}
}

func TestMockAgent_BuildCommand_WithModel(t *testing.T) {
	a := NewMockAgent()
	cmd := a.BuildCommand(CommandOptions{Model: "mock-fast", SessionID: "sess-456"})
	args := cmd.Args()

	foundModel := false
	foundResume := false
	for i, arg := range args {
		if arg == "--model" && i+1 < len(args) && args[i+1] == "mock-fast" {
			foundModel = true
		}
		if arg == "--resume" && i+1 < len(args) && args[i+1] == "sess-456" {
			foundResume = true
		}
	}
	if !foundModel {
		t.Errorf("expected --model mock-fast in args %v", args)
	}
	if !foundResume {
		t.Errorf("expected --resume sess-456 in args %v", args)
	}
}

func TestMockAgent_Runtime_CanRecover(t *testing.T) {
	a := NewMockAgent()
	rt := a.Runtime()

	if !rt.SessionConfig.SupportsRecovery() {
		t.Error("expected SupportsRecovery() to return true for mock agent")
	}
}

func TestMockAgent_Runtime_ResumeFlag(t *testing.T) {
	a := NewMockAgent()
	rt := a.Runtime()

	if rt.SessionConfig.ResumeFlag.IsEmpty() {
		t.Error("expected ResumeFlag to be set on mock agent RuntimeConfig")
	}
	flagArgs := rt.SessionConfig.ResumeFlag.Args()
	if len(flagArgs) == 0 || flagArgs[0] != "--resume" {
		t.Errorf("expected ResumeFlag args to start with --resume, got %v", flagArgs)
	}
}

func TestMockAgent_PassthroughConfig_ResumeFlags(t *testing.T) {
	a := NewMockAgent()
	pt := a.PassthroughConfig()

	// Generic resume flag (-c)
	if pt.ResumeFlag.IsEmpty() {
		t.Error("expected ResumeFlag to be set on PassthroughConfig")
	}
	resumeArgs := pt.ResumeFlag.Args()
	if len(resumeArgs) == 0 || resumeArgs[0] != "-c" {
		t.Errorf("expected ResumeFlag = -c, got %v", resumeArgs)
	}

	// Session-specific resume flag (--resume)
	if pt.SessionResumeFlag.IsEmpty() {
		t.Error("expected SessionResumeFlag to be set on PassthroughConfig")
	}
	sessionArgs := pt.SessionResumeFlag.Args()
	if len(sessionArgs) == 0 || sessionArgs[0] != "--resume" {
		t.Errorf("expected SessionResumeFlag = --resume, got %v", sessionArgs)
	}
}
