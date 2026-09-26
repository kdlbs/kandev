package api

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/process/probe"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// turnStartRecordingAdapter adds the optional TurnStartRecorder half onto a
// bare AgentAdapter, mirroring recordingAdapter/steerableAdapter's
// embed-and-panic-on-touch pattern in prompt_steer_routing_test.go.
type turnStartRecordingAdapter struct {
	adapter.AgentAdapter
	turnStart time.Time
	recorded  bool
}

func (a turnStartRecordingAdapter) RecordedTurnStart(_ string) (time.Time, bool) {
	return a.turnStart, a.recorded
}

func TestHandleWSBackgroundProbe_NoAdapter_Errors(t *testing.T) {
	s := newTestServer(t)
	msg, _ := ws.NewRequest("req-1", "agent.background.probe", map[string]string{"session_id": "sess-1"})

	resp := s.handleWSBackgroundProbe(context.Background(), msg)

	if resp.Type != ws.MessageTypeError {
		t.Fatalf("expected an error response, got %q", resp.Type)
	}
}

// AC-46: an adapter that doesn't implement TurnStartRecorder at all (not an
// ACP adapter) is one of the failure conditions that must resolve to
// unknown, not an error.
func TestHandleWSBackgroundProbe_AdapterWithoutTurnStartRecorder_Unknown(t *testing.T) {
	s := newTestServer(t)
	s.procMgr.SetAdapterForTest(&recordingAdapter{})
	msg, _ := ws.NewRequest("req-1", "agent.background.probe", map[string]string{"session_id": "sess-1"})

	resp := s.handleWSBackgroundProbe(context.Background(), msg)
	assertBackgroundProbeResult(t, resp, "unknown")
}

// AC-46: no recorded turn start for this session (e.g. probed before any
// turn ever started) resolves to unknown.
func TestHandleWSBackgroundProbe_NoRecordedTurnStart_Unknown(t *testing.T) {
	s := newTestServer(t)
	s.procMgr.SetAdapterForTest(turnStartRecordingAdapter{recorded: false})
	msg, _ := ws.NewRequest("req-1", "agent.background.probe", map[string]string{"session_id": "sess-1"})

	resp := s.handleWSBackgroundProbe(context.Background(), msg)
	assertBackgroundProbeResult(t, resp, "unknown")
}

// AC-45: the request carries only session_id (no timestamp — the turn start
// was already recorded adapter-side, per D3); the response is always
// exactly one of the three valid result literals.
//
// newTestServer's procMgr has no real agent process running, so AgentPID()
// returns 0 — D9's "agent process exited" case. This exercises the exact
// production path a live server takes between adapter recovery and process
// launch, and must resolve to unknown rather than any tri-state-tolerant
// answer: a pid-0 root must never be walked into a false "live".
func TestHandleWSBackgroundProbe_RecordedTurnStart_NoRunningProcess_Unknown(t *testing.T) {
	s := newTestServer(t)
	s.procMgr.SetAdapterForTest(turnStartRecordingAdapter{turnStart: time.Now(), recorded: true})
	msg, _ := ws.NewRequest("req-1", "agent.background.probe", map[string]string{"session_id": "sess-1"})

	resp := s.handleWSBackgroundProbe(context.Background(), msg)
	assertBackgroundProbeResult(t, resp, "unknown")
}

// AC-002.4: the handler must forward req.SessionID into
// probe.ProbeBackgroundWorkloads unchanged. Every other case in this file
// short-circuits before the identity scan (no adapter, no recorder, or
// AgentPID()==0 reading unknown before sessionID is ever used), so none of
// them can catch a call site that drops or blanks the session id — this
// stubs the probe seam to capture what the handler actually forwards.
func TestHandleWSBackgroundProbe_ForwardsSessionID(t *testing.T) {
	s := newTestServer(t)
	s.procMgr.SetAdapterForTest(turnStartRecordingAdapter{turnStart: time.Now(), recorded: true})

	var gotSessionID string
	orig := probeBackgroundWorkloads
	probeBackgroundWorkloads = func(_ int, _ time.Time, sessionID string) (probe.Result, error) {
		gotSessionID = sessionID
		return probe.ResultLive, nil
	}
	t.Cleanup(func() { probeBackgroundWorkloads = orig })

	msg, _ := ws.NewRequest("req-1", "agent.background.probe", map[string]string{"session_id": "sess-42"})
	resp := s.handleWSBackgroundProbe(context.Background(), msg)

	assertBackgroundProbeResult(t, resp, "live")
	if gotSessionID != "sess-42" {
		t.Fatalf("handler forwarded session id %q, want %q", gotSessionID, "sess-42")
	}
}

func assertBackgroundProbeResult(t *testing.T, resp *ws.Message, want string) {
	t.Helper()
	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("expected a response (not error), got %q", resp.Type)
	}
	var payload BackgroundProbeResponse
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if payload.Result != want {
		t.Fatalf("got result %q, want %q", payload.Result, want)
	}
}
