package acp

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const claudeSubagentReportLine = `{"type":"assistant","message":{"content":[{"type":"text","text":"symlinked report"}]}}`

func TestClaudeSubagentReportReaderRejectsDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent-123.output")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := readClaudeSubagentReportWithin(t, "agent-123", dir); got != "" {
		t.Fatalf("directory report = %q, want empty", got)
	}
}

func TestClaudeSubagentReportReaderFollowsSymlinkToRegularFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "provider-task.jsonl")
	if err := os.WriteFile(target, []byte(claudeSubagentReportLine), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "agent-123.output")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got := readClaudeSubagentReportWithin(t, "agent-123", link); got != "symlinked report" {
		t.Fatalf("symlink report = %q, want the target's assistant text", got)
	}
}

// readClaudeSubagentReportWithin fails the test instead of hanging the run if
// the reader blocks on a non-regular file.
func readClaudeSubagentReportWithin(t *testing.T, agentID, path string) string {
	t.Helper()
	done := make(chan string, 1)
	go func() { done <- readClaudeSubagentReport(agentID, path, true) }()
	select {
	case got := <-done:
		return got
	case <-time.After(5 * time.Second):
		t.Fatalf("readClaudeSubagentReport blocked on %s", path)
		return ""
	}
}
