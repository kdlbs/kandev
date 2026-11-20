package types

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestNormalizeSessionOperationFailure(t *testing.T) {
	input := &SessionOperationFailure{
		Code: SessionOperationFailureCodeAuthenticationRequired,
		AuthMethods: []streams.AuthMethodInfo{{
			ID:          "cursor_login",
			Name:        "Cursor CLI login",
			Description: "Sign in with token=private-token-value from /Users/alice/.cursor",
			Meta:        map[string]any{"token": "private-token-value"},
			TerminalAuth: &streams.TerminalAuth{
				Command: "cursor-agent",
				Args:    []string{"login"},
				Label:   "Sign in",
			},
		}},
	}
	got := NormalizeSessionOperationFailure(input)
	if got == nil || got.Code != SessionOperationFailureCodeAuthenticationRequired {
		t.Fatalf("normalized failure = %#v, want authentication_required", got)
	}
	if len(got.AuthMethods) != 1 {
		t.Fatalf("auth methods = %#v, want one safe method", got.AuthMethods)
	}
	method := got.AuthMethods[0]
	if method.ID != "cursor_login" || method.Name != "Cursor CLI login" {
		t.Fatalf("method identity = (%q, %q)", method.ID, method.Name)
	}
	if strings.Contains(method.Description, "private-token-value") || strings.Contains(method.Description, "/Users/alice") {
		t.Fatalf("description contains unsafe source data: %q", method.Description)
	}
	if method.Meta != nil {
		t.Fatalf("raw method metadata was retained: %#v", method.Meta)
	}
	if method.TerminalAuth == nil || method.TerminalAuth.Command != "cursor-agent" || len(method.TerminalAuth.Args) != 1 || method.TerminalAuth.Args[0] != "login" {
		t.Fatalf("terminal auth = %#v", method.TerminalAuth)
	}
	input.AuthMethods[0].TerminalAuth.Args[0] = "mutated"
	if got.AuthMethods[0].TerminalAuth.Args[0] != "login" {
		t.Fatal("normalized failure shares mutable terminal arguments with input")
	}
}

func TestNormalizeSessionOperationFailureCodeOnlyAndUnsupported(t *testing.T) {
	if got := NormalizeSessionOperationFailure(&SessionOperationFailure{Code: SessionOperationFailureCodeAuthenticationRequired}); got == nil || len(got.AuthMethods) != 0 {
		t.Fatalf("code-only failure = %#v, want valid authentication evidence", got)
	}
	for _, code := range []string{"", "internal_error", "authentication_required\n"} {
		if got := NormalizeSessionOperationFailure(&SessionOperationFailure{Code: code}); got != nil {
			t.Errorf("code %q normalized to %#v, want rejection", code, got)
		}
	}
}

func TestNormalizeSessionOperationFailureBoundsAndUnsafeTerminalMetadata(t *testing.T) {
	methods := make([]streams.AuthMethodInfo, 0, MaxSessionOperationAuthMethods+2)
	for i := 0; i < MaxSessionOperationAuthMethods+2; i++ {
		methods = append(methods, streams.AuthMethodInfo{ID: "method-" + string(rune('a'+i)), Name: "Sign in"})
	}
	methods = append(methods, streams.AuthMethodInfo{ID: strings.Repeat("x", MaxSessionOperationAuthMethodIDBytes+1), Name: "too long"})
	got := NormalizeSessionOperationFailure(&SessionOperationFailure{
		Code:        SessionOperationFailureCodeAuthenticationRequired,
		AuthMethods: methods,
	})
	if len(got.AuthMethods) != MaxSessionOperationAuthMethods {
		t.Fatalf("auth method count = %d, want at most %d", len(got.AuthMethods), MaxSessionOperationAuthMethods)
	}
	longID := NormalizeSessionOperationFailure(&SessionOperationFailure{
		Code:        SessionOperationFailureCodeAuthenticationRequired,
		AuthMethods: []streams.AuthMethodInfo{{ID: strings.Repeat("x-", MaxSessionOperationAuthMethodIDBytes), Name: "Too long"}},
	})
	if len(longID.AuthMethods) != 0 {
		t.Fatalf("overlong method ID was retained: %#v", longID.AuthMethods)
	}
	unsafeTerminal := NormalizeSessionOperationFailure(&SessionOperationFailure{
		Code: SessionOperationFailureCodeAuthenticationRequired,
		AuthMethods: []streams.AuthMethodInfo{{ID: "unsafe-terminal", Name: "Sign in", TerminalAuth: &streams.TerminalAuth{
			Command: "cursor-agent",
			Args:    []string{"login; echo secret"},
		}}},
	})
	if len(unsafeTerminal.AuthMethods) != 1 || unsafeTerminal.AuthMethods[0].TerminalAuth != nil {
		t.Fatalf("unsafe terminal metadata projection = %#v, want method without terminal control", unsafeTerminal.AuthMethods)
	}

	oversized := &SessionOperationFailure{Code: SessionOperationFailureCodeAuthenticationRequired, AuthMethods: []streams.AuthMethodInfo{
		{ID: "method", Name: "Sign in", Description: strings.Repeat("sign in ", 70)},
	}}
	bounded := NormalizeSessionOperationFailure(oversized)
	if len(bounded.AuthMethods) != 1 || bounded.AuthMethods[0].Description != "" {
		t.Fatalf("oversized description projection = %#v, want omitted description", bounded.AuthMethods)
	}

	largeMethods := make([]streams.AuthMethodInfo, MaxSessionOperationAuthMethods)
	for i := range largeMethods {
		largeMethods[i] = streams.AuthMethodInfo{ID: "method-" + string(rune('a'+i)), Name: "Sign in", Description: strings.Repeat("safe text ", 50)}
	}
	combined := NormalizeSessionOperationFailure(&SessionOperationFailure{
		Code:        SessionOperationFailureCodeAuthenticationRequired,
		AuthMethods: largeMethods,
	})
	encoded, err := json.Marshal(combined.AuthMethods)
	if err != nil {
		t.Fatalf("marshal normalized methods: %v", err)
	}
	if len(encoded) > MaxSessionOperationAuthMethodBytes {
		t.Fatalf("normalized methods use %d bytes, want at most %d", len(encoded), MaxSessionOperationAuthMethodBytes)
	}
}
