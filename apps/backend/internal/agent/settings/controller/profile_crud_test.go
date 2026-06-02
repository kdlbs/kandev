package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

// TestSeedCLIFlags_FromCopilot verifies that a fresh Copilot profile gets
// the four curated CLI-flag suggestions seeded with the expected default
// state. `--allow-all-tools` is on by default so autonomous runs don't
// stall on per-tool-call permission prompts; the other --allow-all-* and
// --no-ask-user flags stay off as safe defaults until users opt in.
func TestSeedCLIFlags_FromCopilot(t *testing.T) {
	ag := agents.NewCopilotACP()
	flags := seedCLIFlags(ag)

	wantFlags := map[string]bool{
		"--allow-all-tools": true,
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

// TestSeedCLIFlags_FromCodexACP seeds codex-acp -c config overrides (off by
// default) and never @openai/codex passthrough flags like --ask-for-approval.
func TestSeedCLIFlags_FromCodexACP(t *testing.T) {
	flags := seedCLIFlags(agents.NewCodexACP())

	want := map[string]bool{
		"-c approval_policy=never":                           false,
		agents.CodexACPSandboxDiskFullReadCLIFlag: false,
	}
	if len(flags) != len(want) {
		t.Fatalf("expected %d seeded flags, got %d: %+v", len(want), len(flags), flags)
	}
	for _, f := range flags {
		enabled, ok := want[f.Flag]
		if !ok {
			t.Errorf("unexpected seeded flag: %q", f.Flag)
			continue
		}
		if f.Enabled != enabled {
			t.Errorf("%q: Enabled=%v, want %v", f.Flag, f.Enabled, enabled)
		}
		if f.Description == "" {
			t.Errorf("%q: missing Description", f.Flag)
		}
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

func TestValidateProfileEnvVarDTOs(t *testing.T) {
	cases := []struct {
		name    string
		envVars []dto.ProfileEnvVarDTO
		wantErr bool
	}{
		{name: "valid value", envVars: []dto.ProfileEnvVarDTO{{Key: "FOO", Value: "bar"}}},
		{name: "valid secret", envVars: []dto.ProfileEnvVarDTO{{Key: "TOKEN", SecretID: "sec-1"}}},
		{name: "empty key rejected", envVars: []dto.ProfileEnvVarDTO{{Key: ""}}, wantErr: true},
		{name: "whitespace key rejected", envVars: []dto.ProfileEnvVarDTO{{Key: "   "}}, wantErr: true},
		{name: "equals key rejected", envVars: []dto.ProfileEnvVarDTO{{Key: "BAD=KEY", Value: "x"}}, wantErr: true},
		{name: "null key rejected", envVars: []dto.ProfileEnvVarDTO{{Key: "BAD\x00KEY", Value: "x"}}, wantErr: true},
		{name: "duplicate key rejected", envVars: []dto.ProfileEnvVarDTO{{Key: "FOO", Value: "one"}, {Key: " FOO ", Value: "two"}}, wantErr: true},
		{name: "value and secret rejected", envVars: []dto.ProfileEnvVarDTO{{Key: "FOO", Value: "bar", SecretID: "sec-1"}}, wantErr: true},
		{name: "null value rejected", envVars: []dto.ProfileEnvVarDTO{{Key: "FOO", Value: "bad\x00val"}}, wantErr: true},
		{name: "KANDEV prefix rejected", envVars: []dto.ProfileEnvVarDTO{{Key: "KANDEV_TASK_ID", Value: "x"}}, wantErr: true},
		{name: "TASK_DESCRIPTION rejected", envVars: []dto.ProfileEnvVarDTO{{Key: "TASK_DESCRIPTION", Value: "x"}}, wantErr: true},
		{name: "too many entries rejected", envVars: func() []dto.ProfileEnvVarDTO {
			out := make([]dto.ProfileEnvVarDTO, maxProfileEnvVars+1)
			for i := range out {
				out[i] = dto.ProfileEnvVarDTO{Key: fmt.Sprintf("K%d", i), Value: "v"}
			}
			return out
		}(), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateProfileEnvVarDTOs(tc.envVars)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestCreateProfile_PersistsEnvVars(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{"test-agent": &testAgent{
		id:          "test-agent",
		name:        "test-agent",
		displayName: "Test Agent",
		enabled:     true,
	}})
	st := newFakeStore()
	agent := &models.Agent{ID: "agent-1", Name: "test-agent"}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	ctrl.repo = st

	profile, err := ctrl.CreateProfile(context.Background(), CreateProfileRequest{
		AgentID: "agent-1",
		Name:    "With env",
		EnvVars: []dto.ProfileEnvVarDTO{{Key: "FOO", Value: "bar"}},
	})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if len(profile.EnvVars) != 1 || profile.EnvVars[0].Key != "FOO" || profile.EnvVars[0].Value != "bar" {
		t.Fatalf("response env vars: %+v", profile.EnvVars)
	}
	if len(st.created) != 1 || len(st.created[0].EnvVars) != 1 || st.created[0].EnvVars[0].Key != "FOO" {
		t.Fatalf("stored env vars: %+v", st.created)
	}
}

func TestCreateAgentProfiles_PersistsEnvVars(t *testing.T) {
	ctrl := newTestController(nil)
	st := newFakeStore()
	ctrl.repo = st

	profiles, err := ctrl.createAgentProfiles(context.Background(), "agent-1", "Test Agent", []CreateAgentProfileRequest{{
		Name:    "With env",
		Model:   "model-1",
		EnvVars: []dto.ProfileEnvVarDTO{{Key: "FOO", Value: "bar"}},
	}}, &testAgent{id: "test-agent", name: "test-agent"})
	if err != nil {
		t.Fatalf("createAgentProfiles: %v", err)
	}
	if len(profiles) != 1 || len(profiles[0].EnvVars) != 1 || profiles[0].EnvVars[0].Key != "FOO" {
		t.Fatalf("created env vars: %+v", profiles)
	}
}
