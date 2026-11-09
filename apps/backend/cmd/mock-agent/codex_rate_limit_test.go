package main

import (
	"context"
	"os"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestParseCodexRateLimitCmd(t *testing.T) {
	for _, tc := range []struct {
		prompt   string
		wantFail int
		wantOK   bool
	}{
		{prompt: "/codex-rate-limit", wantFail: 1, wantOK: true},
		{prompt: "/e2e:codex-rate-limit:3", wantFail: 3, wantOK: true},
		{prompt: "<kandev-system>context</kandev-system>/codex-rate-limit:0", wantOK: true},
		{prompt: "/rate-limit", wantOK: false},
		{prompt: "codex-rate-limit", wantOK: false},
	} {
		gotFail, gotOK := parseCodexRateLimitCmd(tc.prompt)
		if gotOK != tc.wantOK || (tc.wantOK && gotFail != tc.wantFail) {
			t.Errorf("parseCodexRateLimitCmd(%q) = (%d, %v), want (%d, %v)", tc.prompt, gotFail, gotOK, tc.wantFail, tc.wantOK)
		}
	}
}

func TestHandleCodexRateLimit_EmitsSyntheticTerminal429(t *testing.T) {
	a := newOverloadedTestAgent()
	const sid acp.SessionId = "codex-rate-limit-sess"
	_ = os.Remove(codexRateLimitCounterPath(sid))
	t.Cleanup(func() { _ = os.Remove(codexRateLimitCounterPath(sid)) })

	for attempt := 1; attempt <= 2; attempt++ {
		_, err, handled := a.handleCodexRateLimit(context.Background(), sid, "/codex-rate-limit:2")
		if !handled {
			t.Fatalf("attempt %d: handled = false, want true", attempt)
		}
		reqErr, ok := err.(*acp.RequestError)
		if !ok {
			t.Fatalf("attempt %d: error type = %T, want *acp.RequestError", attempt, err)
		}
		if reqErr.Message != codexRateLimit429Message {
			t.Fatalf("attempt %d: message = %q, want exact synthetic terminal signature", attempt, reqErr.Message)
		}
	}
	if got := nextCodexRateLimitAttempt(sid) - 1; got != 2 {
		t.Errorf("persisted attempt count = %d, want 2", got)
	}
}
