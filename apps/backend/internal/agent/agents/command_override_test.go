package agents

import (
	"errors"
	"slices"
	"testing"
)

func TestValidateCommandOverride(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "empty means no override", value: ""},
		{name: "absolute path", value: "/opt/codex/bin/codex"},
		{name: "absolute path with dots in name", value: "/opt/codex-1.2.3/bin/codex.real"},
		{name: "relative path", value: "codex", wantErr: true},
		{name: "relative with directory", value: "bin/codex", wantErr: true},
		{name: "dot relative", value: "./codex", wantErr: true},
		{name: "not clean parent segment", value: "/opt/../bin/codex", wantErr: true},
		{name: "not clean double slash", value: "/opt//codex", wantErr: true},
		{name: "trailing slash", value: "/opt/codex/", wantErr: true},
		{name: "argv with space", value: "/opt/codex --flag", wantErr: true},
		{name: "leading space", value: " /opt/codex", wantErr: true},
		{name: "tab", value: "/opt/co\tdex", wantErr: true},
		{name: "newline", value: "/opt/codex\n", wantErr: true},
		{name: "nul", value: "/opt/codex\x00", wantErr: true},
		{name: "command substitution", value: "/opt/$(id)/codex", wantErr: true},
		{name: "backtick", value: "/opt/`id`/codex", wantErr: true},
		{name: "semicolon", value: "/opt/codex;id", wantErr: true},
		{name: "pipe", value: "/opt/codex|cat", wantErr: true},
		{name: "ampersand", value: "/opt/codex&", wantErr: true},
		{name: "redirect", value: "/opt/codex>out", wantErr: true},
		{name: "glob", value: "/opt/*/codex", wantErr: true},
		{name: "quote", value: `/opt/"codex"`, wantErr: true},
		{name: "env assignment", value: "/opt/A=b/codex", wantErr: true},
		{name: "home tilde", value: "/opt/~/codex", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCommandOverride(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateCommandOverride(%q) = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidCommandOverride) {
				t.Fatalf("error %v does not wrap ErrInvalidCommandOverride", err)
			}
		})
	}
}

type managedAgent interface {
	Agent
	ManagedNPMRuntimeAgent
}

func TestManagedAgentsHonourCommandOverride(t *testing.T) {
	const path = "/opt/pinned/bin/tool"
	tests := []managedAgent{
		NewCodexAppServer(true), NewCodexACP(), NewClaudeACP(), NewGemini(),
		NewCopilotACP(), NewMuseACP(), NewPiACP(),
	}
	for _, ag := range tests {
		t.Run(ag.ID(), func(t *testing.T) {
			spec := ag.ManagedNPMRuntime()
			got := ag.BuildCommand(CommandOptions{CommandOverride: path}).Args()
			want := append([]string{path}, spec.runtimeArgs()...)
			if !slices.Equal(got, want) {
				t.Fatalf("override argv = %q, want %q", got, want)
			}
			if slices.Contains(got, "npx") || slices.Contains(got, "--prefix") {
				t.Fatalf("override argv %q still uses the npx launch", got)
			}
			def := ag.BuildCommand(CommandOptions{}).Args()
			if len(def) == 0 || def[0] != "npx" {
				t.Fatalf("default argv = %q, want unchanged npx launch", def)
			}
		})
	}
}

func TestCodexAppServerOverrideKeepsAppServerArg(t *testing.T) {
	got := NewCodexAppServer(true).BuildCommand(CommandOptions{CommandOverride: "/opt/codex/bin/codex"}).Args()
	if want := []string{"/opt/codex/bin/codex", "app-server"}; !slices.Equal(got, want) {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

func TestInvalidOverrideNeverBuildsAnOverrideCommand(t *testing.T) {
	got := NewCodexAppServer(true).BuildCommand(CommandOptions{CommandOverride: "relative/codex"}).Args()
	if len(got) == 0 || got[0] == "relative/codex" {
		t.Fatalf("argv = %q, an invalid override must not become argv[0]", got)
	}
}
