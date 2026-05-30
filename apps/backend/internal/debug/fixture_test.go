package debug

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDiscoverNormalizedFiles_FindsPerSessionFiles verifies the debug reader
// still discovers normalized event files after they moved to the per-session
// managed log dir (~/.kandev/logs/acp, overridable via KANDEV_DEBUG_LOG_DIR).
func TestDiscoverNormalizedFiles_FindsPerSessionFiles(t *testing.T) {
	logDir := t.TempDir()
	t.Setenv("KANDEV_DEBUG_LOG_DIR", logDir)

	name := "normalized-acp-claude-acp-sess-123.jsonl"
	line := `{"ts":1,"event":{"type":"message_chunk","session_id":"sess-123","text":"hi"}}` + "\n"
	if err := os.WriteFile(filepath.Join(logDir, name), []byte(line), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	files, err := discoverNormalizedFiles(t.TempDir())
	if err != nil {
		t.Fatalf("discoverNormalizedFiles: %v", err)
	}

	var found *DiscoveredFile
	for i := range files {
		if filepath.Base(files[i].Path) == name {
			found = &files[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("per-session file %q not discovered; got %d files", name, len(files))
	}
	if found.Protocol != "acp" {
		t.Errorf("expected protocol acp, got %q", found.Protocol)
	}
	if found.MessageCount != 1 {
		t.Errorf("expected 1 message, got %d", found.MessageCount)
	}
}

// TestReadNormalizedEvents_ReadsPerSessionFile end-to-ends discovery → read:
// the file the reader discovers must also parse back into messages.
func TestReadNormalizedEvents_ReadsPerSessionFile(t *testing.T) {
	logDir := t.TempDir()
	path := filepath.Join(logDir, "normalized-acp-claude-acp-sess-xyz.jsonl")
	line := `{"ts":1,"event":{"type":"message_chunk","session_id":"sess-xyz","text":"hello"}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	msgs, err := readNormalizedEventsAsMessages(path)
	if err != nil {
		t.Fatalf("readNormalizedEventsAsMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}
