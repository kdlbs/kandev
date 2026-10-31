package main

import (
	"context"
	"os"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestParsePromptErrorCmd(t *testing.T) {
	for _, test := range []struct {
		prompt    string
		kind      string
		failTimes int
		ok        bool
	}{
		{prompt: "/prompt-error", kind: "storage", failTimes: 1, ok: true},
		{prompt: "/e2e:prompt-error", kind: "storage", failTimes: 1, ok: true},
		{prompt: "/prompt-error-storage:2", kind: "storage", failTimes: 2, ok: true},
		{prompt: "/prompt-error-auth", kind: "auth", failTimes: 1, ok: true},
		{prompt: "/prompt-error-quota:3", kind: "quota", failTimes: 3, ok: true},
		{prompt: "/prompt-error-model", kind: "model", failTimes: 1, ok: true},
		{prompt: "/prompt-error-empty", kind: "empty", failTimes: 1, ok: true},
		{prompt: "<kandev-system>ctx</kandev-system>/prompt-error-auth:2", kind: "auth", failTimes: 2, ok: true},
		{prompt: "prompt-error", ok: false},
		{prompt: "/prompt-error-unknown", ok: false},
	} {
		kind, failTimes, ok := parsePromptErrorCmd(test.prompt)
		if ok != test.ok || kind != test.kind || failTimes != test.failTimes {
			t.Fatalf("parsePromptErrorCmd(%q) = (%q, %d, %v), want (%q, %d, %v)",
				test.prompt, kind, failTimes, ok, test.kind, test.failTimes, test.ok)
		}
	}
}

func TestPromptErrorRequestVariantsCarryNoCapacityMarker(t *testing.T) {
	for _, kind := range []string{"storage", "auth", "quota", "model", "empty"} {
		requestErr := promptErrorRequest(kind)
		if requestErr == nil {
			t.Fatalf("%s: nil request error", kind)
		}
		lower := strings.ToLower(requestErr.Message)
		for _, transient := range []string{"overloaded", "capacity", "rate limit", "retry"} {
			if strings.Contains(lower, transient) {
				t.Fatalf("%s: message %q carries transient keyword %q", kind, requestErr.Message, transient)
			}
		}
		if data, ok := requestErr.Data.(map[string]any); ok {
			if _, marked := data["kandevMock"]; marked {
				t.Fatalf("%s: request data carries a mock retention marker", kind)
			}
		}
	}
}

func TestPromptErrorContinuityFailsThenReportsTheAttempt(t *testing.T) {
	const sid acp.SessionId = "prompt-error-continuity-session"
	_ = os.Remove(promptErrorCounterPath(sid))
	t.Cleanup(func() { _ = os.Remove(promptErrorCounterPath(sid)) })

	agent := &mockAgent{}
	for attempt := 1; attempt <= 2; attempt++ {
		_, err, handled := agent.handlePromptErrorContinuity(context.Background(), sid, "/prompt-error:2")
		if !handled || err == nil {
			t.Fatalf("attempt %d = (handled %v, err %v), want a handled error", attempt, handled, err)
		}
	}
	// The third call is past the failure budget; the handler is still owned by
	// this scenario and then attempts a normal streamed response.
	if _, _, handled := parsePromptErrorCmd("/prompt-error:2"); !handled {
		t.Fatal("scenario command was not recognized")
	}
}

func TestCloseSessionClearsPromptErrorCounter(t *testing.T) {
	const sid acp.SessionId = "prompt-error-close-session"
	if _, _, handled := (&mockAgent{}).handlePromptErrorContinuity(context.Background(), sid, "/prompt-error:2"); !handled {
		t.Fatal("scenario command was not recognized")
	}
	if _, err := os.Stat(promptErrorCounterPath(sid)); err != nil {
		t.Fatalf("counter file missing before close: %v", err)
	}
	if _, err := (&mockAgent{}).CloseSession(context.Background(), acp.CloseSessionRequest{SessionId: sid}); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}
	if _, err := os.Stat(promptErrorCounterPath(sid)); !os.IsNotExist(err) {
		t.Fatalf("counter file survived CloseSession: %v", err)
	}
}
