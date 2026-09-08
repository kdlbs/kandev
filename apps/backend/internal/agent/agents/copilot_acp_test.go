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
