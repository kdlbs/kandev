package lifecycle

import (
	"errors"
	"fmt"
	"testing"

	"github.com/coder/acp-go-sdk"
)

func TestIsMissingProviderRolloutErrRequiresMatchingStructuredEvidence(t *testing.T) {
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isMissingProviderRolloutErr(test.err, "saved-session"); got != test.want {
				t.Fatalf("isMissingProviderRolloutErr() = %t, want %t", got, test.want)
			}
		})
	}
}
