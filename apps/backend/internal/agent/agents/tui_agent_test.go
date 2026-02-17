package agents

import (
	"context"
	"testing"

	"github.com/kandev/kandev/pkg/agent"
)

func TestNewTUIAgent_Defaults(t *testing.T) {
	a := NewTUIAgent(TUIAgentConfig{
		AgentID:   "test-tui",
		AgentName: "TestTUI",
		Command:   "testtui",
		Desc:      "A test TUI agent.",
	})

	if a.ID() != "test-tui" {
		t.Errorf("ID() = %q, want %q", a.ID(), "test-tui")
	}
	if a.Name() != "TestTUI" {
		t.Errorf("Name() = %q, want %q", a.Name(), "TestTUI")
	}
	if a.DisplayName() != "TestTUI" {
		t.Errorf("DisplayName() should default to AgentName, got %q", a.DisplayName())
	}
	if a.DisplayOrder() != 99 {
		t.Errorf("DisplayOrder() = %d, want 99 (default)", a.DisplayOrder())
	}
	if !a.Enabled() {
		t.Error("Enabled() should return true")
	}

	rt := a.Runtime()
	if rt.Protocol != agent.ProtocolACP {
		t.Errorf("Protocol = %q, want %q (default)", rt.Protocol, agent.ProtocolACP)
	}

	pt := a.PassthroughConfig()
	if pt.IdleTimeout.Seconds() != 3 {
		t.Errorf("IdleTimeout = %v, want 3s", pt.IdleTimeout)
	}
	if pt.BufferMaxBytes != DefaultBufferMaxBytes {
		t.Errorf("BufferMaxBytes = %d, want %d", pt.BufferMaxBytes, DefaultBufferMaxBytes)
	}
}

func intPtr(v int) *int { return &v }

func TestNewTUIAgent_CustomValues(t *testing.T) {
	a := NewTUIAgent(TUIAgentConfig{
		AgentID:     "lazydocker",
		AgentName:   "Lazydocker",
		Command:     "lazydocker",
		Desc:        "Docker TUI.",
		Display:     "Docker UI",
		Order:       intPtr(50),
		WaitForTerm: true,
		CommandArgs: []string{"--debug"},
	})

	if a.DisplayName() != "Docker UI" {
		t.Errorf("DisplayName() = %q, want %q", a.DisplayName(), "Docker UI")
	}
	if a.DisplayOrder() != 50 {
		t.Errorf("DisplayOrder() = %d, want 50", a.DisplayOrder())
	}

	cmd := a.BuildCommand(CommandOptions{})
	args := cmd.Args()
	if len(args) != 2 || args[0] != "lazydocker" || args[1] != "--debug" {
		t.Errorf("BuildCommand() = %v, want [lazydocker --debug]", args)
	}

	pt := a.PassthroughConfig()
	if !pt.WaitForTerminal {
		t.Error("WaitForTerminal should be true")
	}

	ptCmd := pt.PassthroughCmd.Args()
	if len(ptCmd) != 2 || ptCmd[0] != "lazydocker" || ptCmd[1] != "--debug" {
		t.Errorf("PassthroughCmd = %v, want [lazydocker --debug]", ptCmd)
	}
}

func TestNewTUIAgent_OrderZeroIsValid(t *testing.T) {
	a := NewTUIAgent(TUIAgentConfig{
		AgentID: "zero-order", AgentName: "ZeroOrder", Command: "zo", Desc: "desc",
		Order: intPtr(0),
	})
	if a.DisplayOrder() != 0 {
		t.Errorf("DisplayOrder() = %d, want 0 (explicit zero should not be overridden)", a.DisplayOrder())
	}
}

func TestNewTUIAgent_Logo(t *testing.T) {
	// No logos
	a := NewTUIAgent(TUIAgentConfig{
		AgentID: "no-logo", AgentName: "NoLogo", Command: "nol", Desc: "No logo.",
	})
	if a.Logo(LogoLight) != nil {
		t.Error("Logo(Light) should be nil when no logo provided")
	}
	if a.Logo(LogoDark) != nil {
		t.Error("Logo(Dark) should be nil when no logo provided")
	}

	// With logos
	light := []byte("<svg>light</svg>")
	dark := []byte("<svg>dark</svg>")
	b := NewTUIAgent(TUIAgentConfig{
		AgentID: "with-logo", AgentName: "WithLogo", Command: "wl", Desc: "With logo.",
		LogoLight: light, LogoDark: dark,
	})
	if string(b.Logo(LogoLight)) != string(light) {
		t.Errorf("Logo(Light) = %q, want %q", b.Logo(LogoLight), light)
	}
	if string(b.Logo(LogoDark)) != string(dark) {
		t.Errorf("Logo(Dark) = %q, want %q", b.Logo(LogoDark), dark)
	}
}

func TestNewTUIAgent_ModelMethods(t *testing.T) {
	a := NewTUIAgent(TUIAgentConfig{
		AgentID: "tui", AgentName: "TUI", Command: "tui", Desc: "desc",
	})

	if a.DefaultModel() != "" {
		t.Errorf("DefaultModel() = %q, want empty", a.DefaultModel())
	}

	models, err := a.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if models.SupportsDynamic {
		t.Error("SupportsDynamic should be false")
	}
	if models.Models != nil {
		t.Errorf("Models should be nil, got %v", models.Models)
	}
}

func TestNewTUIAgent_PermissionSettings(t *testing.T) {
	a := NewTUIAgent(TUIAgentConfig{
		AgentID: "tui", AgentName: "TUI", Command: "tui", Desc: "desc",
	})

	if a.PermissionSettings() != nil {
		t.Error("PermissionSettings() should return nil for TUI agents")
	}
}

func TestNewTUIAgent_CustomDetectOpts(t *testing.T) {
	customDetect := WithFileExists("/nonexistent/path")
	a := NewTUIAgent(TUIAgentConfig{
		AgentID:    "custom-detect",
		AgentName:  "CustomDetect",
		Command:    "cd",
		Desc:       "Custom detection.",
		DetectOpts: []DetectOption{customDetect},
	})

	result, err := a.IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled() error = %v", err)
	}
	// Custom detect with nonexistent path should return not available
	if result.Available {
		t.Error("expected not available for nonexistent path")
	}
}
