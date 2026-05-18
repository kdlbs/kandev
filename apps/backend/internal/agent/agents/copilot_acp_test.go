package agents

import "testing"

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
	want := []string{"npx", "-y", "@github/copilot", "--acp"}
	if len(got) != len(want) {
		t.Fatalf("argv length mismatch\n  got:  %#v\n  want: %#v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("argv[%d] mismatch: got %q, want %q", i, got[i], want[i])
		}
	}
}
