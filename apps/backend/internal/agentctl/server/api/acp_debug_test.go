package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	envMessages = "KANDEV_DEBUG_AGENT_MESSAGES"
	envDevMode  = "KANDEV_DEBUG_DEV_MODE"
	envLogDir   = "KANDEV_DEBUG_LOG_DIR"
	enabled     = "true"
	disabled    = ""
)

func acpTailStatus(t *testing.T, s *Server) int {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/debug/acp/some-session", nil)
	s.router.ServeHTTP(rec, req)
	return rec.Code
}

// TestACPRingTailRoute_Gating verifies the live-tail route is registered only
// when BOTH frame logging and dev mode are on — message logging alone (e.g. a
// non-dev deployment) must not expose it.
func TestACPRingTailRoute_Gating(t *testing.T) {
	// Neither flag → route absent. (Clear explicitly: the test may run inside a
	// dev session where these are already exported.)
	t.Setenv(envMessages, disabled)
	t.Setenv(envDevMode, disabled)
	if status := acpTailStatus(t, newTestServer(t)); status != http.StatusNotFound {
		t.Errorf("expected 404 when disabled, got %d", status)
	}

	// Message logging only → still absent.
	t.Setenv(envMessages, enabled)
	if status := acpTailStatus(t, newTestServer(t)); status != http.StatusNotFound {
		t.Errorf("expected 404 with messages-only, got %d", status)
	}

	// Both flags → route present.
	t.Setenv(envDevMode, enabled)
	if status := acpTailStatus(t, newTestServer(t)); status != http.StatusOK {
		t.Errorf("expected 200 when dev+messages on, got %d", status)
	}
}

// TestACPRingTailHandler_UnknownSession verifies the handler returns a 200 with
// an empty (non-null) events array and echoes the session + parses n.
func TestACPRingTailHandler_UnknownSession(t *testing.T) {
	t.Setenv(envMessages, enabled)
	t.Setenv(envDevMode, enabled)
	s := newTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/debug/acp/no-such-session?n=5", nil)
	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Session string            `json:"session"`
		Count   int               `json:"count"`
		Events  []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Session != "no-such-session" {
		t.Errorf("session = %q, want no-such-session", body.Session)
	}
	if body.Count != 0 {
		t.Errorf("count = %d, want 0", body.Count)
	}
	if body.Events == nil {
		t.Errorf("events should serialize as [] not null")
	}
}

func TestACPDebugExportReturnsOnlyExactSessionFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envMessages, enabled)
	t.Setenv(envDevMode, enabled)
	t.Setenv(envLogDir, dir)
	writeACPTestFile(t, filepath.Join(dir, "raw-acp-claude-session-1.jsonl"), "raw-session-1\n")
	writeACPTestFile(t, filepath.Join(dir, "normalized-acp-claude-session-1.1.jsonl"), "normalized-session-1\n")
	writeACPTestFile(t, filepath.Join(dir, "raw-acp-claude-session-10.jsonl"), "wrong-session\n")
	writeACPTestFile(t, filepath.Join(dir, "notes.txt"), "private\n")

	server := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/debug/acp/session-1/export?max_bytes=4096", nil)
	server.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	reader, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	if len(reader.File) != 2 {
		t.Fatalf("entries = %d, want 2", len(reader.File))
	}
	for _, entry := range reader.File {
		if strings.Contains(entry.Name, "session-10") || entry.Name == "notes.txt" {
			t.Fatalf("unexpected export entry %q", entry.Name)
		}
		body, readErr := io.ReadAll(mustOpenZipEntry(t, entry))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !strings.Contains(string(body), "session-1") {
			t.Fatalf("entry %q body = %q", entry.Name, body)
		}
	}
}

func writeACPTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustOpenZipEntry(t *testing.T, entry *zip.File) io.ReadCloser {
	t.Helper()
	reader, err := entry.Open()
	if err != nil {
		t.Fatal(err)
	}
	return reader
}
