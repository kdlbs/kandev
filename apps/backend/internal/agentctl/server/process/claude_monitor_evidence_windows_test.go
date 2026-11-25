//go:build windows

package process

import (
	"path/filepath"
	"testing"
)

func TestClaudeMonitorEvidenceLaunchUsesWindowsUserProfile(t *testing.T) {
	profileDir, homeDir := t.TempDir(), t.TempDir()
	got := claudeConfigDirFromLaunch(
		claudeACPAgentType,
		[]string{"USERPROFILE=" + profileDir},
		nil,
	)
	if want := filepath.Join(profileDir, ".claude"); got != want {
		t.Fatalf("ClaudeConfigDir from USERPROFILE = %q, want %q", got, want)
	}

	got = claudeConfigDirFromLaunch(
		claudeACPAgentType,
		[]string{"USERPROFILE=" + profileDir, "HOME=" + homeDir},
		nil,
	)
	if want := filepath.Join(profileDir, ".claude"); got != want {
		t.Fatalf("ClaudeConfigDir with both home variables = %q, want USERPROFILE path %q", got, want)
	}
	if got := claudeConfigDirFromLaunch(
		claudeACPAgentType,
		[]string{"USERPROFILE=" + profileDir},
		[]string{"USERPROFILE"},
	); got != "" {
		t.Fatalf("ClaudeConfigDir with stripped USERPROFILE = %q, want empty", got)
	}
}
