package agents

import (
	"slices"
	"testing"
)

func TestCodexACP_PassthroughCmd_NoRemovedFullAutoFlag(t *testing.T) {
	pt := NewCodexACP().PassthroughConfig().PassthroughCmd.Args()
	want := []string{"npx", "-y", "@openai/codex"}
	if !slices.Equal(pt, want) {
		t.Fatalf("PassthroughCmd = %#v, want %#v", pt, want)
	}
}

func TestCodexACP_BuildPassthroughCommand_AutoApprove(t *testing.T) {
	cmd := NewCodexACP().BuildPassthroughCommand(PassthroughOptions{
		PermissionValues: map[string]bool{"auto_approve": true},
	})
	want := []string{"npx", "-y", "@openai/codex", "--ask-for-approval", "never"}
	if !slices.Equal(cmd.Args(), want) {
		t.Fatalf("argv = %#v, want %#v", cmd.Args(), want)
	}
}

func TestCodexACP_BuildPassthroughCommand_AutoApproveDisabled(t *testing.T) {
	cmd := NewCodexACP().BuildPassthroughCommand(PassthroughOptions{
		PermissionValues: map[string]bool{"auto_approve": false},
	})
	want := []string{"npx", "-y", "@openai/codex"}
	if !slices.Equal(cmd.Args(), want) {
		t.Fatalf("argv = %#v, want %#v", cmd.Args(), want)
	}
}
