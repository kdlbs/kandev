package agents

import "testing"

// TestCopilotACP_PermissionSettings_Curated verifies the four curated flag
// suggestions surfaced to the profile-creation UI. These seed new profiles
// (all disabled by default) and are the origin of the fix for Copilot's
// permission-prompt-every-tool-call behaviour reported by users.
func TestCopilotACP_PermissionSettings_Curated(t *testing.T) {
	ag := NewCopilotACP()
	settings := ag.PermissionSettings()

	wantKeys := []string{"allow_all_tools", "allow_all_paths", "allow_all_urls", "no_ask_user"}
	for _, k := range wantKeys {
		s, ok := settings[k]
		if !ok {
			t.Fatalf("missing curated setting %q", k)
		}
		if !s.Supported {
			t.Errorf("%q should be Supported=true", k)
		}
		if s.Default {
			t.Errorf("%q should default to Enabled=false so new profiles are safe", k)
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
