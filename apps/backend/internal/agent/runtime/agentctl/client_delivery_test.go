package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

func TestDeliveryClientUsesDurableStatusAndReplayCursors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/agent/delivery/stream" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("stream_id") != "stream-1" || r.URL.Query().Get("after") != "4" || r.URL.Query().Get("limit") != "2" {
			t.Fatalf("query = %v", r.URL.Query())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"events": []journal.Event{{StreamID: "stream-1", Sequence: 5, Type: "complete"}},
			"stream": journal.Stream{StreamID: "stream-1", HighWater: 5},
		})
	}))
	t.Cleanup(server.Close)
	host, port := splitTestServerHostPort(t, server)
	c := NewClient(host, port, newTestLogger())

	events, stream, err := c.ReplayDelivery(context.Background(), "stream-1", 4, 2)
	if err != nil {
		t.Fatalf("ReplayDelivery: %v", err)
	}
	if len(events) != 1 || events[0].Sequence != 5 || stream.HighWater != 5 {
		t.Fatalf("replay = %#v, %#v", events, stream)
	}
}

func TestDeliveryClientAcceptsSubmissionAndAcknowledges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/submissions":
			if r.Method != http.MethodPost {
				t.Fatalf("submission method = %s", r.Method)
			}
			var submission journal.Submission
			if err := json.NewDecoder(r.Body).Decode(&submission); err != nil {
				t.Fatalf("decode submission: %v", err)
			}
			submission.State = journal.SubmissionAccepted
			_ = json.NewEncoder(w).Encode(submission)
		case "/api/v1/agent/delivery/stream/ack":
			if r.Method != http.MethodPost {
				t.Fatalf("ack method = %s", r.Method)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	host, port := splitTestServerHostPort(t, server)
	c := NewClient(host, port, newTestLogger())

	submission, err := c.SubmitDelivery(context.Background(), journal.Submission{
		ID: "submission-1", SessionID: "session-1", Hash: journal.SubmissionHash([]byte("prompt")), Payload: []byte("prompt"),
	})
	if err != nil {
		t.Fatalf("SubmitDelivery: %v", err)
	}
	if submission.State != journal.SubmissionAccepted {
		t.Fatalf("state = %q, want accepted", submission.State)
	}
	if err := c.AcknowledgeDelivery(context.Background(), "stream-1", 5); err != nil {
		t.Fatalf("AcknowledgeDelivery: %v", err)
	}
}
