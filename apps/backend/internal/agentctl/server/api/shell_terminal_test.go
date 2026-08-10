package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func shellTerminalRequest(t *testing.T, srv *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	switch v := body.(type) {
	case nil:
	case string:
		payload = []byte(v)
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		payload = encoded
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	return rec
}

func decodeShellTerminalBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	body := map[string]string{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return body
}

// TestHandleShellTerminalStart_Rejections covers both guards ahead of the PTY
// spawn. A blank terminal_id would otherwise register a session nothing can
// address again.
func TestHandleShellTerminalStart_Rejections(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		name      string
		body      any
		wantError string
	}{
		{"malformed body", "{not json", "invalid request: "},
		{"missing terminal id", shellTerminalStartRequest{Cols: 80, Rows: 24}, "terminal_id is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := shellTerminalRequest(t, srv, http.MethodPost, "/api/v1/shell/terminal/start", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			if got := decodeShellTerminalBody(t, rec)["error"]; !strings.Contains(got, tc.wantError) {
				t.Errorf("error = %q, want it to contain %q", got, tc.wantError)
			}
		})
	}
}

// TestShellTerminalEndpoints_UnknownTerminal pins what each endpoint does for a
// terminal that was never started. The asymmetry is deliberate and load-bearing
// for the UI: reads 404 so a stale tab can detect its terminal is gone, while
// stop succeeds so closing that tab is not an error.
func TestShellTerminalEndpoints_UnknownTerminal(t *testing.T) {
	srv := newTestServer(t)

	buffer := shellTerminalRequest(t, srv, http.MethodGet, "/api/v1/shell/terminal/ghost/buffer", nil)
	if buffer.Code != http.StatusNotFound {
		t.Errorf("buffer status = %d, want 404 (body %s)", buffer.Code, buffer.Body.String())
	}
	if got := decodeShellTerminalBody(t, buffer)["error"]; !strings.Contains(got, "not found") {
		t.Errorf("buffer error = %q, want a not-found report", got)
	}

	stream := shellTerminalRequest(t, srv, http.MethodGet, "/api/v1/shell/terminal/ghost/stream", nil)
	if stream.Code != http.StatusNotFound {
		t.Errorf("stream status = %d, want 404 (body %s)", stream.Code, stream.Body.String())
	}
	if got := decodeShellTerminalBody(t, stream)["error"]; got != "terminal not found" {
		t.Errorf("stream error = %q, want %q", got, "terminal not found")
	}

	stop := shellTerminalRequest(t, srv, http.MethodDelete, "/api/v1/shell/terminal/ghost", nil)
	if stop.Code != http.StatusOK {
		t.Fatalf("stop status = %d, want 200 (body %s)", stop.Code, stop.Body.String())
	}
	if got := decodeShellTerminalBody(t, stop); got["status"] != "stopped" || got["terminal_id"] != "ghost" {
		t.Errorf("stop body = %+v, want a stopped acknowledgement for `ghost`", got)
	}
}

// TestShellTerminalLifecycle drives a real PTY session end to end: start,
// stream input over the binary WebSocket protocol, observe it echoed back,
// resize through the 0x01-prefixed control frame, read the replayed buffer over
// HTTP, then stop. The echo is the only observation that proves the socket
// bytes reach the PTY rather than being swallowed by the read loop.
func TestShellTerminalLifecycle(t *testing.T) {
	srv := newTestServer(t)
	const terminalID = "term-1"

	start := shellTerminalRequest(t, srv, http.MethodPost, "/api/v1/shell/terminal/start", shellTerminalStartRequest{
		TerminalID: terminalID,
		Cols:       100,
		Rows:       30,
	})
	if start.Code != http.StatusOK {
		t.Fatalf("start status = %d (body %s)", start.Code, start.Body.String())
	}
	if got := decodeShellTerminalBody(t, start); got["status"] != "started" || got["terminal_id"] != terminalID {
		t.Fatalf("start body = %+v", got)
	}
	// Registered before any assertion can fail, so a mid-test failure still
	// reaps the PTY and its goroutines rather than tripping goleak.
	t.Cleanup(func() {
		if err := srv.procMgr.ShellManager().StopAll(); err != nil {
			t.Errorf("stop all terminals: %v", err)
		}
	})

	httpServer := httptest.NewServer(srv.Router())
	defer httpServer.Close()
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(httpServer.URL, "http")+"/api/v1/shell/terminal/"+terminalID+"/stream", nil)
	if err != nil {
		t.Fatalf("dial terminal stream: %v", err)
	}

	// A resize control frame must be consumed as a command, never written to
	// the PTY — otherwise its JSON would land in the user's command line.
	resize, err := json.Marshal(map[string]uint16{"cols": 132, "rows": 43})
	if err != nil {
		t.Fatalf("marshal resize: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, append([]byte{shellResizeByte}, resize...)); err != nil {
		t.Fatalf("write resize frame: %v", err)
	}

	const marker = "kandev-terminal-marker"
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("echo "+marker+"\r")); err != nil {
		t.Fatalf("write terminal input: %v", err)
	}

	streamed := readShellTerminalUntil(t, conn, marker)
	if strings.Contains(streamed, `"cols"`) {
		t.Errorf("resize control frame was forwarded to the PTY: %q", streamed)
	}

	buffer := shellTerminalRequest(t, srv, http.MethodGet, "/api/v1/shell/terminal/"+terminalID+"/buffer", nil)
	if buffer.Code != http.StatusOK {
		t.Fatalf("buffer status = %d (body %s)", buffer.Code, buffer.Body.String())
	}
	var replayed ShellBufferResponse
	if err := json.Unmarshal(buffer.Body.Bytes(), &replayed); err != nil {
		t.Fatalf("decode buffer: %v", err)
	}
	if !strings.Contains(replayed.Data, marker) {
		t.Errorf("buffered output = %q, want it to contain the streamed marker", replayed.Data)
	}

	_ = conn.Close()
	stop := shellTerminalRequest(t, srv, http.MethodDelete, "/api/v1/shell/terminal/"+terminalID, nil)
	if stop.Code != http.StatusOK {
		t.Fatalf("stop status = %d (body %s)", stop.Code, stop.Body.String())
	}
	after := shellTerminalRequest(t, srv, http.MethodGet, "/api/v1/shell/terminal/"+terminalID+"/buffer", nil)
	if after.Code != http.StatusNotFound {
		t.Errorf("buffer after stop = %d, want 404 (body %s)", after.Code, after.Body.String())
	}
}

// readShellTerminalUntil accumulates binary frames until the marker appears.
// The PTY emits its prompt and the echoed keystrokes in an unpredictable number
// of frames, so the marker — not a frame count — is the synchronisation point.
func readShellTerminalUntil(t *testing.T, conn *websocket.Conn, marker string) string {
	t.Helper()
	var seen strings.Builder
	deadline := time.Now().Add(wsTestTimeout)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(deadline)
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read terminal frame (seen %q): %v", seen.String(), err)
		}
		if messageType != websocket.BinaryMessage {
			t.Fatalf("frame type = %d, want a binary frame", messageType)
		}
		seen.Write(data)
		if strings.Contains(seen.String(), marker) {
			return seen.String()
		}
	}
	t.Fatalf("marker %q never arrived; saw %q", marker, seen.String())
	return ""
}
