package main

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func newTransportLostTestAgent() *mockAgent {
	return &mockAgent{
		model:           "mock-fast",
		sessions:        make(map[acp.SessionId]bool),
		commandsEmitted: make(map[acp.SessionId]bool),
	}
}

func TestParseTransportLostCmd(t *testing.T) {
	cases := []struct {
		prompt   string
		wantFail int
		wantOK   bool
	}{
		{"/transport-lost", 1, true},
		{"/TRANSPORT-LOST", 1, true},
		{"/e2e:transport-lost", 1, true},
		{"/transport-lost:3", 3, true},
		{"/e2e:transport-lost:9", 9, true},
		{"/transport-lost:0", 0, true},
		{"<kandev-system>ctx</kandev-system>/transport-lost:2", 2, true},
		{"hello", 0, false},
		{"/transport-los", 0, false},
		{"transport-lost", 0, false},
	}
	for _, tc := range cases {
		gotFail, gotOK := parseTransportLostCmd(tc.prompt)
		if gotOK != tc.wantOK || (tc.wantOK && gotFail != tc.wantFail) {
			t.Errorf("parseTransportLostCmd(%q) = (%d, %v), want (%d, %v)", tc.prompt, gotFail, gotOK, tc.wantFail, tc.wantOK)
		}
	}
}

func TestHandleTransportLost_NotTheCommand(t *testing.T) {
	a := newTransportLostTestAgent()
	_, err, handled := a.handleTransportLost(context.Background(), "s1", "hello there")
	if handled {
		t.Fatal("handled = true for a non-transport-lost prompt, want false")
	}
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestHandleTransportLost_EmitsProductionPeerDisconnected(t *testing.T) {
	a := newTransportLostTestAgent()
	const sid acp.SessionId = "emit-peer-disconnected-sess"
	_ = os.Remove(transportLostCounterPath(sid))
	t.Cleanup(func() { _ = os.Remove(transportLostCounterPath(sid)) })
	_, err, handled := a.handleTransportLost(context.Background(), sid, "/transport-lost")
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
	if !strings.Contains(reqErr.Error(), "peer disconnected before response") {
		t.Errorf("error = %q, want it to contain 'peer disconnected before response'", reqErr.Error())
	}
}

func TestHandleTransportLost_FailsExactlyNTimes(t *testing.T) {
	a := newTransportLostTestAgent()
	const sid acp.SessionId = "failN-transport-lost-sess"
	_ = os.Remove(transportLostCounterPath(sid))
	t.Cleanup(func() { _ = os.Remove(transportLostCounterPath(sid)) })

	// failTimes=2: the first two prompts must error (the persisted counter
	// drives this, so it survives across the simulated relaunches the
	// orchestrator performs between retries).
	for i := 1; i <= 2; i++ {
		_, err, handled := a.handleTransportLost(context.Background(), sid, "/transport-lost:2")
		if !handled {
			t.Fatalf("attempt %d: handled = false, want true", i)
		}
		if err == nil {
			t.Fatalf("attempt %d: err = nil, want a peer-disconnected error", i)
		}
	}
	// The third attempt would recover (emits text via the live conn) — not
	// exercised here to avoid needing a real ACP connection; the recovery path
	// is covered end-to-end by the Playwright spec. Assert the persisted count
	// reached the fail budget so recovery is next. Read the counter file
	// directly rather than calling nextTransportLostAttempt, which would
	// increment it as a side effect of the assertion itself.
	raw, err := os.ReadFile(transportLostCounterPath(sid))
	if err != nil {
		t.Fatalf("failed to read persisted attempt count: %v", err)
	}
	got, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("failed to parse persisted attempt count %q: %v", raw, err)
	}
	if got != 2 {
		t.Errorf("persisted attempt count = %d, want 2", got)
	}
}
