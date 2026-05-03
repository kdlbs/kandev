package acp

import (
	"errors"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestMaybeEmitAuthRequired_AuthenticationRequiredErrorEmitsEvent verifies
// that a session/new failure with code -32000 (AuthenticationRequired) emits
// an EventTypeAuthRequired event carrying the cached auth methods so the
// frontend can drive the authenticate → session/new retry without re-running
// initialize.
func TestMaybeEmitAuthRequired_AuthenticationRequiredErrorEmitsEvent(t *testing.T) {
	a := newTestAdapter()
	cached := []streams.AuthMethodInfo{
		{ID: "oauth", Name: "OAuth"},
		{ID: "api-key", Name: "API key"},
	}
	a.mu.Lock()
	a.availableAuthMethods = cached
	a.mu.Unlock()

	authErr := acp.NewAuthRequired(nil)
	emitted := a.maybeEmitAuthRequired(authErr)
	if !emitted {
		t.Fatalf("maybeEmitAuthRequired returned false for AuthenticationRequired error")
	}

	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != streams.EventTypeAuthRequired {
		t.Errorf("event Type = %q, want %q", ev.Type, streams.EventTypeAuthRequired)
	}
	if len(ev.AuthMethods) != len(cached) {
		t.Fatalf("event AuthMethods len = %d, want %d", len(ev.AuthMethods), len(cached))
	}
	for i, m := range cached {
		if ev.AuthMethods[i].ID != m.ID {
			t.Errorf("event AuthMethods[%d].ID = %q, want %q", i, ev.AuthMethods[i].ID, m.ID)
		}
	}
}

// TestMaybeEmitAuthRequired_NonAuthErrorIsIgnored guards against the helper
// firing on unrelated session/new failures (network errors, MCP misconfig,
// etc.). A real auth_required event must be unambiguous.
func TestMaybeEmitAuthRequired_NonAuthErrorIsIgnored(t *testing.T) {
	a := newTestAdapter()

	otherErr := &acp.RequestError{Code: -32603, Message: "internal error"}
	if a.maybeEmitAuthRequired(otherErr) {
		t.Fatalf("maybeEmitAuthRequired returned true for non-auth error code %d", otherErr.Code)
	}
	if events := drainEvents(a); len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

// TestMaybeEmitAuthRequired_PlainErrorIsIgnored covers the case where the
// failure isn't even a JSON-RPC RequestError (e.g. transport-level error
// surfaced as a wrapped Go error).
func TestMaybeEmitAuthRequired_PlainErrorIsIgnored(t *testing.T) {
	a := newTestAdapter()

	if a.maybeEmitAuthRequired(errors.New("connection reset")) {
		t.Fatalf("maybeEmitAuthRequired returned true for plain Go error")
	}
}
