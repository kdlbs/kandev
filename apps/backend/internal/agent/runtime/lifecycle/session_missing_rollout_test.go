package lifecycle

import (
	"errors"
	"fmt"
	"testing"

	"github.com/coder/acp-go-sdk"
)

func TestIsMissingProviderSessionErrRequiresMatchingStructuredEvidence(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "matching wrapped request error",
			err: fmt.Errorf("load failed: %w", &acp.RequestError{
				Code:    -32603,
				Message: "Internal error",
				Data: map[string]any{
					"details": "no rollout found for thread id saved-session",
				},
			}),
			want: true,
		},
		{
			name: "different request code",
			err: &acp.RequestError{
				Code:    -32002,
				Message: "Internal error",
				Data:    map[string]any{"details": "no rollout found for thread id saved-session"},
			},
		},
		{
			name: "different request message",
			err: &acp.RequestError{
				Code:    -32603,
				Message: "Provider failed",
				Data:    map[string]any{"details": "no rollout found for thread id saved-session"},
			},
		},
		{
			name: "different session",
			err: &acp.RequestError{
				Code:    -32603,
				Message: "Internal error",
				Data:    map[string]any{"details": "no rollout found for thread id other-session"},
			},
		},
		{
			name: "matching JSON with trailing text",
			err: errors.New(
				`{"code":-32603,"message":"Internal error","data":{"details":"no rollout found for thread id saved-session"}} trailing`,
			),
		},
		{
			name: "matching auggie session not found error",
			err: fmt.Errorf("load failed: %w", &acp.RequestError{
				Code:    -32602,
				Message: "Invalid params",
				Data: map[string]any{
					"details": "Session not found: saved-session",
				},
			}),
			want: true,
		},
		{
			name: "matching auggie session not found in json string",
			err: errors.New(
				`load session failed: failed to load session: {"code":-32602,"message":"Invalid params","data":{"details":"Session not found: saved-session"}}`,
			),
			want: true,
		},
		{
			name: "auggie session not found for different session",
			err: &acp.RequestError{
				Code:    -32602,
				Message: "Invalid params",
				Data:    map[string]any{"details": "Session not found: other-session"},
			},
		},
		{
			name: "auggie session not found with case mismatch",
			err: &acp.RequestError{
				Code:    -32602,
				Message: "Invalid params",
				Data:    map[string]any{"details": "Session not found: SAVED-SESSION"},
			},
		},
		{
			name: "matching codex archived session",
			err: fmt.Errorf("load failed: %w", &acp.RequestError{
				Code:    -32603,
				Message: "Internal error",
				Data: map[string]any{
					"details": "session saved-session is archived. Run `codex unarchive saved-session` to unarchive it first.",
				},
			}),
			want: true,
		},
		{
			name: "matching codex archived session in json string",
			err: errors.New(
				`load session failed: failed to load session: {"code":-32603,"message":"Internal error","data":{"details":"session saved-session is archived. It can no longer be loaded."}}`,
			),
			want: true,
		},
		{
			name: "codex archived session for different session",
			err: &acp.RequestError{
				Code:    -32603,
				Message: "Internal error",
				Data:    map[string]any{"details": "session other-session is archived. Run `codex unarchive other-session` to unarchive it first."},
			},
		},
		{
			name: "internal error naming the session but not archived",
			err: &acp.RequestError{
				Code:    -32603,
				Message: "Internal error",
				Data:    map[string]any{"details": "session saved-session could not be read"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isMissingProviderSessionErr(test.err, "saved-session", "codex-acp"); got != test.want {
				t.Fatalf("isMissingProviderSessionErr() = %t, want %t", got, test.want)
			}
		})
	}
}
