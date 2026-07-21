package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	envMessages = "KANDEV_DEBUG_AGENT_MESSAGES"
	envDevMode  = "KANDEV_DEBUG_DEV_MODE"
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
