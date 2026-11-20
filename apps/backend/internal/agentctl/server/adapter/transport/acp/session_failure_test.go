package acp

import (
	"errors"
	"fmt"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestProjectSessionOperationFailure(t *testing.T) {
	methods := []streams.AuthMethodInfo{{
		ID:          "cursor_login",
		Name:        "Cursor login",
		Description: "Sign in",
		Meta:        map[string]any{"secret": "must not cross the response"},
	}}
	projected := ProjectSessionOperationFailure(fmt.Errorf("wrapped ACP response: %w", &acpsdk.RequestError{
		Code: -32000, Message: "provider authentication rejected",
	}), methods)
	if projected == nil || projected.Code != types.SessionOperationFailureCodeAuthenticationRequired {
		t.Fatalf("projected failure = %#v, want authentication_required", projected)
	}
	if len(projected.AuthMethods) != 1 || projected.AuthMethods[0].Meta != nil {
		t.Fatalf("projected auth methods = %#v", projected.AuthMethods)
	}
	if codeOnly := ProjectSessionOperationFailure(&acpsdk.RequestError{Code: -32000}, nil); codeOnly == nil || len(codeOnly.AuthMethods) != 0 {
		t.Fatalf("code-only projection = %#v, want authentication_required", codeOnly)
	}

	for _, err := range []error{
		&acpsdk.RequestError{Code: -32603, Message: "authentication required"},
		errors.New("provider text quotes ACP code -32000"),
	} {
		if got := ProjectSessionOperationFailure(err, methods); got != nil {
			t.Errorf("non-auth ACP failure %v projected as %#v", err, got)
		}
	}
}
