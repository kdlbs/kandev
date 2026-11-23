package agents

import (
	"slices"
	"testing"
)

// TestCopilotACP_PermissionSettings_Curated verifies the four curated flag
// suggestions surfaced to the profile-creation UI. `allow_all_tools` is
// enabled by default so autonomous runs don't stall on per-tool-call
// permission prompts; the other --allow-all-* and --no-ask-user toggles
// stay off as safe defaults.
func TestCopilotACP_PermissionSettings_Curated(t *testing.T) {
	ag := NewCopilotACP()
	settings := ag.PermissionSettings()

	// Pin the expected default for each curated key. Anything not listed
	// here defaults off.
	wantDefault := map[string]bool{
		"allow_all_tools": true,
		"allow_all_paths": false,
		"allow_all_urls":  false,
		"no_ask_user":     false,
	}
	for k, want := range wantDefault {
		s, ok := settings[k]
		if !ok {
			t.Fatalf("missing curated setting %q", k)
		}
		if !s.Supported {
			t.Errorf("%q should be Supported=true", k)
		}
		if s.Default != want {
			t.Errorf("%q Default = %v, want %v", k, s.Default, want)
		}
		if s.ApplyMethod != "cli_flag" {
			t.Errorf("%q should apply as cli_flag, got %q", k, s.ApplyMethod)
		}
		if s.CLIFlag == "" {
			t.Errorf("%q must specify a CLIFlag", k)
		}
	}
}

// TestCopilotACP_BuildCommand_NoCLIFlagSpecialCasing confirms BuildCommand
// itself is flag-agnostic: the cli_flags list travels through
// CommandBuilder.BuildCommand (in the lifecycle package) which appends the
// tokens. This test pins the bare command so any future agent drift is loud.
func TestCopilotACP_BuildCommand_NoCLIFlagSpecialCasing(t *testing.T) {
	ag := NewCopilotACP()
	cmd := ag.BuildCommand(CommandOptions{})
	got := cmd.Args()
	want := ag.ManagedNPMRuntime().CachedACPCommand().Args()
	if len(got) != len(want) {
		t.Fatalf("argv length mismatch\n  got:  %#v\n  want: %#v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("argv[%d] mismatch: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCopilotACP_UsesManagedPackageOnEveryNPMCommandSurface(t *testing.T) {
	ag := NewCopilotACP()
	wantACP := ag.ManagedNPMRuntime().CachedACPCommand().Args()
	wantPassthrough := []string{"npx", "--yes", "--prefer-offline", "@github/copilot"}

	if got := ag.Runtime().Cmd.Args(); !slices.Equal(got, wantACP) {
		t.Fatalf("Runtime Cmd = %#v, want %#v", got, wantACP)
	}
	if got := ag.InferenceConfig().Command.Args(); !slices.Equal(got, wantACP) {
		t.Fatalf("Inference Command = %#v, want %#v", got, wantACP)
	}
	if got := ag.PassthroughConfig().PassthroughCmd.Args(); !slices.Equal(got, wantPassthrough) {
		t.Fatalf("Passthrough Cmd = %#v, want %#v", got, wantPassthrough)
	}
	if got, want := ag.InstallScript(), "npm install -g @github/copilot"; got != want {
		t.Fatalf("InstallScript = %q, want %q", got, want)
	}
}

// TestCopilotACP_BuildCommand_KeepsManagedPackageWhenNativeBinaryIsPresent
// prevents a globally installed Copilot binary from bypassing the managed npm
// runtime selected for capability discovery and future launches.
func TestCopilotACP_BuildCommand_KeepsManagedPackageWhenNativeBinaryIsPresent(t *testing.T) {
	ag := NewCopilotACP()

	if name := ag.NativeBinaryName(); name != "copilot" {
		t.Fatalf("NativeBinaryName() = %q, want %q", name, "copilot")
	}

	native := ag.BuildCommand(CommandOptions{PreferNativeBinary: true}).Args()
	wantNative := ag.ManagedNPMRuntime().CachedACPCommand().Args()
	if len(native) != len(wantNative) {
		t.Fatalf("native argv mismatch\n  got:  %#v\n  want: %#v", native, wantNative)
	}
	for i := range native {
		if native[i] != wantNative[i] {
			t.Errorf("native argv[%d] = %q, want %q", i, native[i], wantNative[i])
		}
	}

	// Default (binary absent) still uses npx.
	npx := ag.BuildCommand(CommandOptions{}).Args()
	if len(npx) == 0 || npx[0] != "npx" {
		t.Errorf("default command should start with npx, got %#v", npx)
	}
}

// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-002.1, AC-AGENTS-COPILOT-CONTEXT-TIER-002.2
func TestCopilotACP_BuildCommand_ContextTier(t *testing.T) {
	ag := NewCopilotACP()
	baseACP := ag.ManagedNPMRuntime().CachedACPCommand().Args()

	t.Run("ACP appends --context for default and long_context", func(t *testing.T) {
		for _, tier := range []string{"default", "long_context"} {
			got := ag.BuildCommand(CommandOptions{
				ConfigOptions: map[string]string{"context_tier": tier},
			}).Args()
			want := append(append([]string(nil), baseACP...), "--context", tier)
			if !slices.Equal(got, want) {
				t.Fatalf("BuildCommand(%q) = %#v, want %#v", tier, got, want)
			}
		}
	})

	t.Run("ACP skips --context when CLIFlagTokens explicitly provides --context", func(t *testing.T) {
		for _, flags := range [][]string{
			{"--context", "default"},
			{"--context=default"},
		} {
			got := ag.BuildCommand(CommandOptions{
				ConfigOptions: map[string]string{"context_tier": "long_context"},
				CLIFlagTokens: flags,
			}).Args()
			if !slices.Equal(got, baseACP) {
				t.Fatalf("BuildCommand with CLIFlagTokens %v = %#v, want %#v", flags, got, baseACP)
			}
		}
	})

	t.Run("ACP ignores unsupported context_tier value", func(t *testing.T) {
		got := ag.BuildCommand(CommandOptions{
			ConfigOptions: map[string]string{"context_tier": "invalid_tier"},
		}).Args()
		if !slices.Equal(got, baseACP) {
			t.Fatalf("BuildCommand(invalid) = %#v, want %#v", got, baseACP)
		}
	})

	t.Run("Passthrough appends --context unless overridden by CLIFlagTokens", func(t *testing.T) {
		got := ag.BuildPassthroughCommand(PassthroughOptions{
			Model:         "gpt-5.4",
			ConfigOptions: map[string]string{"context_tier": "long_context"},
			Prompt:        "hi",
		}).Args()
		want := []string{"npx", "--yes", "--prefer-offline", "@github/copilot", "--model", "gpt-5.4", "--context", "long_context", "hi"}
		if !slices.Equal(got, want) {
			t.Fatalf("BuildPassthroughCommand = %#v, want %#v", got, want)
		}

		gotOverride := ag.BuildPassthroughCommand(PassthroughOptions{
			ConfigOptions: map[string]string{"context_tier": "long_context"},
			CLIFlagTokens: []string{"--context=default"},
		}).Args()
		wantOverride := []string{"npx", "--yes", "--prefer-offline", "@github/copilot", "--context=default"}
		if !slices.Equal(gotOverride, wantOverride) {
			t.Fatalf("BuildPassthroughCommand override = %#v, want %#v", gotOverride, wantOverride)
		}
	})

	t.Run("MockAgent with copilot-acp ID appends --context", func(t *testing.T) {
		mockCopilot := NewMockAgentWithID(CopilotACPAgentID, "GitHub Copilot", "Copilot")
		got := mockCopilot.BuildCommand(CommandOptions{
			ConfigOptions: map[string]string{"context_tier": "long_context"},
		}).Args()
		want := []string{"mock-agent", "--context", "long_context"}
		if !slices.Equal(got, want) {
			t.Fatalf("MockAgent(copilot-acp) BuildCommand = %#v, want %#v", got, want)
		}

		mockClaude := NewMockAgentWithID("claude-acp", "Claude", "Claude")
		gotClaude := mockClaude.BuildCommand(CommandOptions{
			ConfigOptions: map[string]string{"context_tier": "long_context"},
		}).Args()
		if !slices.Equal(gotClaude, []string{"mock-agent"}) {
			t.Fatalf("MockAgent(claude-acp) must ignore copilot context_tier, got %#v", gotClaude)
		}
	})
}
