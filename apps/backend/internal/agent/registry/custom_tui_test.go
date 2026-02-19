package registry

import (
	"testing"
)

func TestRegisterCustomTUIAgent_Success(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	err := reg.RegisterCustomTUIAgent("my-agent", "My Agent", "my-agent --verbose", "A test agent", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ag, ok := reg.Get("my-agent")
	if !ok {
		t.Fatal("expected agent to be registered")
	}
	if ag.ID() != "my-agent" {
		t.Errorf("expected ID %q, got %q", "my-agent", ag.ID())
	}
	if ag.DisplayName() != "My Agent" {
		t.Errorf("expected display name %q, got %q", "My Agent", ag.DisplayName())
	}
	if ag.Description() != "A test agent" {
		t.Errorf("expected description %q, got %q", "A test agent", ag.Description())
	}

	rt := ag.Runtime()
	if rt == nil {
		t.Fatal("expected non-nil runtime")
	}
	cmd := rt.Cmd.Args()
	if len(cmd) < 2 || cmd[0] != "my-agent" || cmd[1] != "--verbose" {
		t.Errorf("expected command [my-agent --verbose], got %v", cmd)
	}
}

func TestRegisterCustomTUIAgent_ModelTemplate(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	err := reg.RegisterCustomTUIAgent("tmpl-agent", "Template", "my-cli --model {{model}}", "", "best", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ag, ok := reg.Get("tmpl-agent")
	if !ok {
		t.Fatal("expected agent to be registered")
	}
	cmd := ag.Runtime().Cmd.Args()
	found := false
	for _, arg := range cmd {
		if arg == "best" {
			found = true
		}
		if arg == "{{model}}" {
			t.Error("{{model}} should have been replaced")
		}
	}
	if !found {
		t.Errorf("expected 'best' in command args, got %v", cmd)
	}
}

func TestRegisterCustomTUIAgent_ModelTemplateNotReplacedWhenEmpty(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	err := reg.RegisterCustomTUIAgent("no-model", "No Model", "cli --model {{model}}", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ag, _ := reg.Get("no-model")
	cmd := ag.Runtime().Cmd.Args()
	found := false
	for _, arg := range cmd {
		if arg == "{{model}}" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected {{model}} to remain when model is empty, got %v", cmd)
	}
}

func TestRegisterCustomTUIAgent_CommandArgs(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	err := reg.RegisterCustomTUIAgent("extra-args", "Extra", "my-cli", "", "", []string{"--extra", "--flag"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ag, _ := reg.Get("extra-args")
	cmd := ag.Runtime().Cmd.Args()
	if len(cmd) < 3 || cmd[len(cmd)-2] != "--extra" || cmd[len(cmd)-1] != "--flag" {
		t.Errorf("expected command args to include --extra --flag, got %v", cmd)
	}
}

func TestRegisterCustomTUIAgent_EmptyCommand(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	err := reg.RegisterCustomTUIAgent("empty-cmd", "Empty", "", "", "", nil)
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestRegisterCustomTUIAgent_DuplicateID(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	_ = reg.RegisterCustomTUIAgent("dup-agent", "First", "first-cli", "", "", nil)
	err := reg.RegisterCustomTUIAgent("dup-agent", "Second", "second-cli", "", "", nil)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}
