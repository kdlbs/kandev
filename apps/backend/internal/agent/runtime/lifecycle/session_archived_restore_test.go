package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.1
// @covers AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.2
func TestRestoreCoordinatorArchivedEvidencePreservesIdentity(t *testing.T) {
	tests := []struct {
		name, provider, details string
		code                    int
		message                 string
		missing                 bool
	}{
		{"archived", "codex-acp", "session saved-session is archived", -32603, "Internal error", true},
		{"archived with guidance", "codex-acp", "session saved-session is archived. Run `codex unarchive saved-session` to unarchive it first.", -32603, "Internal error", true},
		{"custom provider", "custom-acp", "session saved-session is archived.", -32603, "Internal error", false},
		{"auggie provider", "auggie", "session saved-session is archived.", -32603, "Internal error", false},
		{"unknown provider", "", "session saved-session is archived.", -32603, "Internal error", false},
		{"word lookalike", "codex-acp", "session saved-session is archivedness", -32603, "Internal error", false},
		{"negated suffix", "codex-acp", "session saved-session is archived-compatible but still available", -32603, "Internal error", false},
		{"foreign session", "codex-acp", "session another-session is archived.", -32603, "Internal error", false},
		{"longer session", "codex-acp", "session saved-session-extra is archived.", -32603, "Internal error", false},
		{"case mismatch", "codex-acp", "session SAVED-SESSION is archived.", -32603, "Internal error", false},
		{"wrong code", "codex-acp", "session saved-session is archived.", -32602, "Internal error", false},
		{"wrong message", "codex-acp", "session saved-session is archived.", -32603, "Provider failed", false},
		{"unsupported internal error", "codex-acp", "session saved-session could not be read", -32603, "Internal error", false},
		{"existing rollout shape", "custom-acp", "no rollout found for thread id saved-session", -32603, "Internal error", true},
		{"existing auggie shape", "auggie", "Session not found: saved-session", -32602, "Invalid params", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestErr := &acp.RequestError{Code: tt.code, Message: tt.message, Data: map[string]any{"details": tt.details}}
			encoded, err := json.Marshal(requestErr)
			if err != nil {
				t.Fatal(err)
			}
			for _, wire := range []bool{false, true} {
				loadErr := fmt.Errorf("load failed: %w", requestErr)
				if wire {
					loadErr = errors.New("load session failed: failed to load session: " + string(encoded))
				}
				assertArchivedRestoreBlocked(t, tt.provider, loadErr, tt.missing)
			}
		})
	}
	for _, message := range []string{
		"Internal error: session saved-session is archived.",
		`{"code":-32603,"message":"Internal error","data":{"details":"session saved-session is archived."}} trailing`,
		`{"code":-32603,"message":"Internal error","data":{"details":"session saved-session is archived."}`,
	} {
		assertArchivedRestoreBlocked(t, "codex-acp", errors.New(message), false)
	}
}

func assertArchivedRestoreBlocked(t *testing.T, provider string, loadErr error, missing bool) {
	t.Helper()
	loads, creates := 0, 0
	result, err := (&RestoreCoordinator{}).Restore(context.Background(), RestoreCoordinatorRequest{
		Identity:  RestoreIdentity{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, NativeSessionID: "saved-session"},
		AgentType: provider,
		Action:    RestoreActionNativeResume,
		Snapshot:  &models.ContinuationSnapshot{Content: "saved history"},
	}, RestoreCoordinatorHooks{
		LoadNative: func(_ context.Context, id string) error {
			loads++
			if id != "saved-session" {
				t.Fatalf("load ID = %q", id)
			}
			return loadErr
		},
		CreateNative: func(context.Context, string) (string, error) {
			creates++
			return "replacement", nil
		},
	})
	want := RestoreReasonUnknown
	if missing {
		want = RestoreReasonNativeStateMissing
	}
	var restoreErr *RestoreRequiredError
	if !errors.As(err, &restoreErr) || !errors.Is(err, loadErr) {
		t.Fatalf("error = %v, want blocked restore retaining cause", err)
	}
	if result.Decision.Reason != want || result.Decision.AllowsContextContinuation != missing {
		t.Fatalf("decision = %+v, want reason %s, continuation %t", result.Decision, want, missing)
	}
	if result.NativeSessionID != "saved-session" || result.DispatchAllowed || result.Decision.ActionAuthorized ||
		!result.Decision.PreserveNativeIdentity || result.Decision.Outcome != RestoreOutcomeBlocked || loads != 1 || creates != 0 {
		t.Fatalf("result = %+v, load/new = %d/%d", result, loads, creates)
	}
}
