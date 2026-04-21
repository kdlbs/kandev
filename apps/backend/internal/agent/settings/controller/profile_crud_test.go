package controller

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// TestSeedCLIFlags_FromCopilot verifies that a fresh Copilot profile gets
// the four curated CLI-flag suggestions with Enabled=false, so users see
// them in the UI but nothing changes subprocess behaviour until opt-in.
func TestSeedCLIFlags_FromCopilot(t *testing.T) {
	ag := agents.NewCopilotACP()
	flags := seedCLIFlags(ag)

	wantFlags := map[string]bool{
		"--allow-all-tools": false,
		"--allow-all-paths": false,
		"--allow-all-urls":  false,
		"--no-ask-user":     false,
	}
	if len(flags) != len(wantFlags) {
		t.Fatalf("expected %d seeded flags, got %d: %+v", len(wantFlags), len(flags), flags)
	}
	for _, f := range flags {
		want, ok := wantFlags[f.Flag]
		if !ok {
			t.Errorf("unexpected seeded flag: %q", f.Flag)
			continue
		}
		if f.Enabled != want {
			t.Errorf("%q: Enabled=%v, want %v", f.Flag, f.Enabled, want)
		}
		if f.Description == "" {
			t.Errorf("%q: missing Description", f.Flag)
		}
	}
	// Stable ordering — flags must sort lexicographically so the UI shows
	// them in the same order every time.
	for i := 1; i < len(flags); i++ {
		if flags[i-1].Flag >= flags[i].Flag {
			t.Errorf("flags not sorted: %q >= %q", flags[i-1].Flag, flags[i].Flag)
		}
	}
}

// TestSeedCLIFlags_EmptyForAgentWithNoCurated handles the common case:
// most ACP agents advertise no curated flags so new profiles get an empty
// list and the user can still add custom flags.
func TestSeedCLIFlags_EmptyForAgentWithNoCurated(t *testing.T) {
	ag := agents.NewClaudeACP()
	flags := seedCLIFlags(ag)
	if len(flags) != 0 {
		t.Errorf("expected no curated flags for claude-acp, got %+v", flags)
	}
}

// TestValidateCLIFlagDTOs rejects entries whose flag text is empty or
// whitespace only. Empty descriptions are allowed (custom flags often
// don't have one).
func TestValidateCLIFlagDTOs(t *testing.T) {
	cases := []struct {
		name    string
		flags   []dto.CLIFlagDTO
		wantErr bool
	}{
		{name: "valid", flags: []dto.CLIFlagDTO{{Flag: "--ok", Enabled: true}}},
		{name: "valid with empty description", flags: []dto.CLIFlagDTO{{Flag: "--x"}}},
		{name: "empty flag rejected", flags: []dto.CLIFlagDTO{{Flag: ""}}, wantErr: true},
		{name: "whitespace flag rejected", flags: []dto.CLIFlagDTO{{Flag: "   "}}, wantErr: true},
		{name: "unterminated quote rejected", flags: []dto.CLIFlagDTO{{Flag: `--msg "hi`}}, wantErr: true},
		{name: "trailing backslash rejected", flags: []dto.CLIFlagDTO{{Flag: `--path foo\`}}, wantErr: true},
		{name: "double-quoted empty flag rejected", flags: []dto.CLIFlagDTO{{Flag: `""`}}, wantErr: true},
		{name: "single-quoted empty flag rejected", flags: []dto.CLIFlagDTO{{Flag: `''`}}, wantErr: true},
		{name: "flag with empty quoted value accepted", flags: []dto.CLIFlagDTO{{Flag: `--empty ""`}}},
		{name: "empty list accepted", flags: []dto.CLIFlagDTO{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCLIFlagDTOs(tc.flags)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
