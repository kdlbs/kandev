package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// promptFrame is the wire shape the client encodes for agent.prompt. It is
// declared here rather than reused from the production anonymous struct on
// purpose: this test asserts the *contract* agentctl parses, so a rename on
// either side must fail here rather than silently agree.
type promptFrame struct {
	Text             string                 `json:"text"`
	Attachments      []v1.MessageAttachment `json:"attachments"`
	PromptGeneration uint64                 `json:"prompt_generation"`
	Steer            bool                   `json:"steer"`
}

type capturedPrompt struct {
	frame promptFrame
	raw   map[string]json.RawMessage
}

// promptCapture serves the agent stream endpoint, records every agent.prompt
// payload it receives, and answers each with a success response so the client's
// sendStreamRequest round-trip completes.
type promptCapture struct {
	server *httptest.Server

	mu       sync.Mutex
	captured []capturedPrompt
	arrived  chan struct{}
}

func startPromptCapture(t *testing.T) *promptCapture {
	t.Helper()

	pc := &promptCapture{arrived: make(chan struct{}, 8)}
	upgrader := websocket.Upgrader{}
	pc.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if !pc.record(data) {
				continue
			}
			if err := pc.respond(conn, data); err != nil {
				return
			}
		}
	}))
	t.Cleanup(pc.server.Close)
	return pc
}

func (pc *promptCapture) record(data []byte) bool {
	var msg ws.Message
	if err := json.Unmarshal(data, &msg); err != nil || msg.Action != "agent.prompt" {
		return false
	}
	var entry capturedPrompt
	if err := json.Unmarshal(msg.Payload, &entry.frame); err != nil {
		return false
	}
	if err := json.Unmarshal(msg.Payload, &entry.raw); err != nil {
		return false
	}
	pc.mu.Lock()
	pc.captured = append(pc.captured, entry)
	pc.mu.Unlock()
	pc.arrived <- struct{}{}
	return true
}

func (pc *promptCapture) respond(conn *websocket.Conn, data []byte) error {
	var msg ws.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}
	resp, err := ws.NewResponse(msg.ID, msg.Action, struct {
		Success bool `json:"success"`
	}{Success: true})
	if err != nil {
		return err
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, out)
}

// awaitOne blocks until exactly one prompt frame has been captured and returns
// it. Waiting on the arrival signal rather than sleeping keeps the assertion
// deterministic under -race.
func (pc *promptCapture) awaitOne(t *testing.T) capturedPrompt {
	t.Helper()
	select {
	case <-pc.arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for an agent.prompt frame")
	}
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if len(pc.captured) != 1 {
		t.Fatalf("expected exactly 1 captured prompt frame, got %d", len(pc.captured))
	}
	return pc.captured[0]
}

// connectedClient returns a client with a live agent stream against pc.
func (pc *promptCapture) connectedClient(t *testing.T, ctx context.Context) *Client {
	t.Helper()
	c := &Client{
		baseURL:         pc.server.URL,
		httpClient:      &http.Client{Timeout: 5 * time.Second},
		logger:          newTestLogger(),
		pendingRequests: make(map[string]chan *ws.Message),
	}
	t.Cleanup(c.Close)
	if err := c.StreamUpdates(ctx, func(AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("connect agent stream: %v", err)
	}
	return c
}

// TestPromptSteerEncodesSteerFlagAndPreservesPayload pins the client half of the
// steer contract: PromptSteer must set steer:true while carrying exactly the
// same text, attachments and prompt_generation an ordinary Prompt carries. The
// generation is load-bearing — a steer reuses the predecessor's live generation
// so the folded completion is attributed to the still-open waiter.
func TestPromptSteerEncodesSteerFlagAndPreservesPayload(t *testing.T) {
	pc := startPromptCapture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := pc.connectedClient(t, ctx)

	attachments := []v1.MessageAttachment{{
		Type:     "image",
		Data:     "aGVsbG8=",
		MimeType: "image/png",
		Name:     "shot.png",
	}}
	if err := client.PromptSteer(ctx, "stop and read the spec", attachments, 42); err != nil {
		t.Fatalf("PromptSteer: %v", err)
	}

	frame := pc.awaitOne(t).frame
	if !frame.Steer {
		t.Fatalf("PromptSteer must encode steer:true, got %+v", frame)
	}
	if frame.Text != "stop and read the spec" {
		t.Fatalf("steer text not preserved: %q", frame.Text)
	}
	if frame.PromptGeneration != 42 {
		t.Fatalf("steer must carry the caller's prompt_generation, got %d", frame.PromptGeneration)
	}
	if len(frame.Attachments) != 1 || frame.Attachments[0] != attachments[0] {
		t.Fatalf("steer must preserve attachments, got %+v", frame.Attachments)
	}
}

// TestPromptDoesNotSetSteerFlag is the negative half: an ordinary Prompt must
// not carry the steer flag at all, so an agentctl that would honor it never
// sees one from the non-steering path.
func TestPromptDoesNotSetSteerFlag(t *testing.T) {
	pc := startPromptCapture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := pc.connectedClient(t, ctx)

	if err := client.Prompt(ctx, "carry on", nil, 7); err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	got := pc.awaitOne(t)
	if got.frame.Steer {
		t.Fatalf("ordinary Prompt must not set steer, got %+v", got.frame)
	}
	// steer is `omitempty`, so a false value is absent from the wire entirely.
	if _, present := got.raw["steer"]; present {
		t.Fatalf("ordinary Prompt must omit the steer field, got %v", got.raw)
	}
}
