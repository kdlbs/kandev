package mcpconfig

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCursorMCPAuthCredentialsAvailableReadsEligibleSourcesWithoutPublishing(t *testing.T) {
	cursorHome := t.TempDir()
	projects := filepath.Join(cursorHome, "projects")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	writeCursorAuth(t, projects, "personal-project", `{"plugin-figma-search":{"tokens":{"access_token":"secret"}},"other":{"tokens":{"refresh_token":"refresh"}}}`, time.Now())
	writeCursorAuth(t, projects, "kandev-tasks-99", `{"task-only":{"tokens":{"access_token":"secret"}}}`, time.Now())

	availability := CursorMCPAuthCredentialAvailability(cursorHome, []string{
		"plugin-figma-search", "Plugin-figma-search", "task-only", "missing", "",
	})
	if !availability["plugin-figma-search"] {
		t.Fatal("expected eligible source credentials for exact native ID")
	}
	if availability["Plugin-figma-search"] {
		t.Fatal("native ID lookup must remain exact and case-sensitive")
	}
	if availability["task-only"] {
		t.Fatal("task project credentials must not appear in host discovery")
	}
	if availability["missing"] || availability[""] {
		t.Fatal("blank server ID must not match")
	}
	if _, err := os.Lstat(filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename)); !os.IsNotExist(err) {
		t.Fatalf("discovery must not publish the shared auth master: %v", err)
	}
}
