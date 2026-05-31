package main

import (
	"context"
	"os"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func newOverloadedTestAgent() *mockAgent {
	return &mockAgent{
		model:           "mock-fast",
		sessions:        make(map[acp.SessionId]bool),
		commandsEmitted: make(map[acp.SessionId]bool),
	}
}

func TestParseOverloadedCmd(t *testing.T) {
	cases := []struct {
		prompt   string
		wantFail int
		wantOK   bool
	}{
		{"/overloaded", 1, true},
		{"/OVERLOADED", 1, true},
		{"/e2e:overloaded", 1, true},
		{"/overloaded:3", 3, true},
		{"/e2e:overloaded:9", 9, true},
		{"/overloaded:0", 0, true},
		{"<kandev-system>ctx</kandev-system>/overloaded:2", 2, true},
		{"hello", 0, false},
		{"/overload", 0, false},
		{"overloaded", 0, false},
	}
	for _, tc := range cases {
		gotFail, gotOK := parseOverloadedCmd(tc.prompt)
		if gotOK != tc.wantOK || (tc.wantOK && gotFail != tc.wantFail) {
			t.Errorf("parseOverloadedCmd(%q) = (%d, %v), want (%d, %v)", tc.prompt, gotFail, gotOK, tc.wantFail, tc.wantOK)
		}
	}
}

func TestHandleOverloaded_NotTheCommand(t *testing.T) {
	a := newOverloadedTestAgent()
	_, err, handled := a.handleOverloaded(context.Background(), "s1", "hello there")
	if handled {
		t.Fatal("handled = true for a non-overloaded prompt, want false")
	}
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestHandleOverloaded_EmitsProduction529(t *testing.T) {
	a := newOverloadedTestAgent()
	const sid acp.SessionId = "emit529-sess"
	_ = os.Remove(overloadedCounterPath(sid))
	t.Cleanup(func() { _ = os.Remove(overloadedCounterPath(sid)) })
	_, err, handled := a.handleOverloaded(context.Background(), sid, "/overloaded")
	if !handled {
		t.Fatal("handled = false, want true")
	}
	reqErr, ok := err.(*acp.RequestError)
	if !ok {
		t.Fatalf("err type = %T, want *acp.RequestError", err)
	}
	if reqErr.Code != -32603 {
		t.Errorf("code = %d, want -32603", reqErr.Code)
	}
	if !strings.Contains(reqErr.Message, "529 Overloaded") {
		t.Errorf("message = %q, want it to contain '529 Overloaded'", reqErr.Message)
	}
}

func TestHandleOverloaded_FailsExactlyNTimes(t *testing.T) {
	a := newOverloadedTestAgent()
	const sid acp.SessionId = "failN-sess"
	_ = os.Remove(overloadedCounterPath(sid))
	t.Cleanup(func() { _ = os.Remove(overloadedCounterPath(sid)) })

	// failTimes=2: the first two prompts must error (the persisted counter
	// drives this, so it survives across the simulated relaunches the
	// orchestrator performs between retries).
	for i := 1; i <= 2; i++ {
		_, err, handled := a.handleOverloaded(context.Background(), sid, "/overloaded:2")
		if !handled {
			t.Fatalf("attempt %d: handled = false, want true", i)
		}
		if err == nil {
			t.Fatalf("attempt %d: err = nil, want a 529 error", i)
		}
	}
	// The third attempt would recover (emits text via the live conn) — not
	// exercised here to avoid needing a real ACP connection; the recovery path
	// is covered end-to-end by the Playwright spec. Assert the persisted count
	// reached the fail budget so recovery is next.
	if got := nextOverloadedAttempt(sid) - 1; got != 2 {
		t.Errorf("persisted attempt count = %d, want 2", got)
	}
}
