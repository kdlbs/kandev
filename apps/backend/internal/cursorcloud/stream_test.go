package cursorcloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const streamAgentID = "bc-123e4567-e89b-12d3-a456-426614174000"

func TestStreamEvents(t *testing.T) {
	const stream = "event: status\ndata: {\"runId\":\"run-1\",\"status\":\"RUNNING\"}\n\n" +
		"id: 42-0\nevent: assistant\ndata: {\"text\":\"hello\"}\n\n" +
		"id: 42-0\nevent: interaction_update\ndata: {\"subtype\":\"text-delta\"}\n\n" +
		"id: 43-0\nevent: tool_call\ndata: {\"callId\":\"call-1\",\"name\":\"read_file\",\"status\":\"completed\",\"truncated\":{\"args\":true}}\n\n" +
		"event: heartbeat\ndata: {}\n\n" +
		"id: 43-0\nevent: result\ndata: {\"runId\":\"run-1\",\"status\":\"FINISHED\",\"text\":\"done\"}\n\n" +
		"event: error\ndata: {\"code\":\"temporary\",\"message\":\"provider stream interrupted\"}\n\n" +
		"event: done\ndata: {}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/agents/"+streamAgentID+"/runs/run-1/stream" {
			t.Errorf("request = %s %s, want GET run stream", r.Method, r.URL.Path)
		}
		if r.Header.Get("Accept") != "text/event-stream" || r.Header.Get("Authorization") != "Bearer api-secret" {
			t.Errorf("stream headers = %#v, want SSE accept and bearer auth", r.Header)
		}
		if got := r.Header.Get("Last-Event-ID"); got != "41-0" {
			t.Errorf("Last-Event-ID = %q, want 41-0", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Cursor-Stream-Retention-Seconds", "120")
		_, _ = fmt.Fprint(w, stream)
	}))
	t.Cleanup(server.Close)
	client, err := newClient("api-secret", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	var events []StreamEvent
	result, err := client.StreamRun(context.Background(), streamAgentID, "run-1", "41-0", func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamRun() error = %v", err)
	}
	if result.Retention != 120*time.Second {
		t.Errorf("retention = %v, want 2m", result.Retention)
	}
	if len(events) != 8 {
		t.Fatalf("event count = %d, want all documented stream event frames: %#v", len(events), events)
	}
	if events[0].ID != "" || events[0].Type != "status" {
		t.Errorf("leading status frame = %#v, want no cursor ID", events[0])
	}
	if events[1].ID != events[2].ID || events[1].Type == events[2].Type {
		t.Errorf("shared SSE ID frames = %#v / %#v, want distinct event types preserved", events[1], events[2])
	}
	var tool struct {
		CallID    string `json:"callId"`
		Truncated struct {
			Args bool `json:"args"`
		} `json:"truncated"`
	}
	if err := json.Unmarshal(events[3].Data, &tool); err != nil {
		t.Fatalf("decode tool payload: %v", err)
	}
	if tool.CallID != "call-1" || !tool.Truncated.Args {
		t.Errorf("tool event = %#v, want stable call ID and truncation marker", tool)
	}
	if events[5].Type != "result" || !json.Valid(events[5].Data) || events[6].Type != "error" || events[7].Type != "done" {
		t.Errorf("terminal stream frames = %#v, want result, error, and done events", events[5:])
	}
}

func TestStreamRetentionExpired(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"code":"stream_expired","message":"retained events are no longer available"}`))
	}))
	t.Cleanup(server.Close)
	client, err := newClient("api-secret", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	_, err = client.StreamRun(context.Background(), streamAgentID, "run-1", "old-event", func(StreamEvent) error {
		t.Fatal("expired stream must not emit events")
		return nil
	})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusGone || apiErr.Code != "stream_expired" {
		t.Fatalf("StreamRun() error = %#v, want stream_expired", err)
	}
	if calls != 1 {
		t.Errorf("stream requests = %d, want no retry after retention expiry", calls)
	}
}

func TestStreamMalformedPayloadIsRejectedWithoutEcho(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: assistant\ndata: {sensitive malformed token}\n\n")
	}))
	t.Cleanup(server.Close)
	client, err := newClient("api-secret", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	_, err = client.StreamRun(context.Background(), streamAgentID, "run-1", "", func(StreamEvent) error {
		t.Fatal("malformed frame must not be delivered")
		return nil
	})
	if !errors.Is(err, ErrUnsupportedContract) || strings.Contains(fmt.Sprint(err), "sensitive malformed token") {
		t.Fatalf("StreamRun() error = %v, want sanitized unsupported-contract error", err)
	}
}

func TestStreamResponseRedirectCannotForwardCredentials(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Error("authorization reached redirect target")
		}
	}))
	t.Cleanup(target.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL+"/stream")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(server.Close)
	client, err := newClient("api-secret", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	_, err = client.StreamRun(context.Background(), streamAgentID, "run-1", "", func(StreamEvent) error { return nil })
	if err == nil || redirected.Load() != 0 {
		t.Fatalf("redirect result = %v, target requests = %d; want rejected without following", err, redirected.Load())
	}
}

func TestSSEMultilineDataAndEOFFrame(t *testing.T) {
	var events []StreamEvent
	err := parseSSE(strings.NewReader("event: assistant\ndata: {\"text\":\ndata: \"joined\"}\n"), func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("parseSSE() error = %v", err)
	}
	want := []StreamEvent{{Type: "assistant", Data: []byte("{\"text\":\n\"joined\"}")}}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}
