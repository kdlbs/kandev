package acp

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestSessionModeEventFields pins what a session-mode event may claim after
// session/set_mode was answered. An agent that answers but never reports a
// mode used to publish the mode it held before the request, which persisted
// over the caller's choice and showed a mismatch for a mode that had very
// likely applied.
func TestSessionModeEventFields(t *testing.T) {
	tests := []struct {
		name          string
		requested     string
		result        streams.ModeResult
		wantCurrent   string
		wantRequested string
	}{
		{
			name:        "confirmed apply reports the requested mode without a mismatch",
			requested:   "plan",
			result:      streams.ModeResult{Requested: "plan", Effective: "plan", Confirmed: true},
			wantCurrent: "plan",
		},
		{
			name:          "observed clamp reports the clamped mode and the request",
			requested:     "bypassPermissions",
			result:        streams.ModeResult{Requested: "bypassPermissions", Effective: "default", Confirmed: true},
			wantCurrent:   "default",
			wantRequested: "bypassPermissions",
		},
		{
			name:          "silence does not fabricate an effective mode",
			requested:     "plan",
			result:        streams.ModeResult{Requested: "plan", Confirmed: false},
			wantRequested: "plan",
		},
		{
			name:          "agent that never reported a mode keeps the request separate",
			requested:     "plan",
			result:        streams.ModeResult{Requested: "plan", Effective: "", Confirmed: false},
			wantRequested: "plan",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			current, requested := sessionModeEventFields(tc.requested, tc.result)
			if current != tc.wantCurrent {
				t.Errorf("current mode = %q, want %q", current, tc.wantCurrent)
			}
			if requested != tc.wantRequested {
				t.Errorf("requested mode = %q, want %q", requested, tc.wantRequested)
			}
		})
	}
}
