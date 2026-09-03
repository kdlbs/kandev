package agents

import (
	"slices"
	"testing"
)

// @covers AC-CLI-PASSTHROUGH-LAUNCH-001.1
func TestClaudeACP_PassthroughCmd_DefaultOmitsVerbose(t *testing.T) {
	got := NewClaudeACP().PassthroughConfig().PassthroughCmd.Args()
	want := []string{"npx", "-y", "@anthropic-ai/claude-code"}
	if !slices.Equal(got, want) {
		t.Fatalf("PassthroughCmd = %#v, want %#v", got, want)
	}
}

// @covers AC-CLI-PASSTHROUGH-LAUNCH-001.2
func TestClaudeACP_PassthroughCmd_VerboseOptIn(t *testing.T) {
	got := NewClaudeACP().BuildPassthroughCommand(PassthroughOptions{
		CLIFlagTokens: []string{"--verbose"},
	}).Args()
	want := []string{"npx", "-y", "@anthropic-ai/claude-code", "--verbose"}
	if !slices.Equal(got, want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
}

func TestStandardPassthrough_DeduplicatesPermissionCLIFlags(t *testing.T) {
	agent := StandardPassthrough{
		Cfg: PassthroughConfig{Supported: true, PassthroughCmd: NewCommand("auggie")},
		PermSettings: map[string]PermissionSetting{
			"allow_indexing": {Supported: true, ApplyMethod: PermissionApplyMethodCLIFlag, CLIFlag: "--allow-indexing"},
		},
	}

	got := agent.BuildPassthroughCommand(PassthroughOptions{
		PermissionValues: map[string]bool{"allow_indexing": true},
		CLIFlagTokens:    []string{"--allow-indexing"},
	}).Args()
	want := []string{"auggie", "--allow-indexing"}
	if !slices.Equal(got, want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
}
