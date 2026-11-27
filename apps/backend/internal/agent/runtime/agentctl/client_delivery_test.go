package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

func TestDeliveryClientStatusCachesAdoptionDescriptor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/agent/delivery" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(journal.RecoveryDescriptor{
			StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
			SessionID:         "session-1",
			IncarnationID:     "incarnation-1",
			HarnessGeneration: 2,
			StreamID:          "stream-1",
			Stream: &journal.Stream{
				SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
				StreamID: "stream-1", HighWater: 9, Acknowledged: 4, FirstRetained: 5,
			},
			Submissions: []journal.SubmissionSummary{{
				ID: "submission-1", SessionID: "session-1", IncarnationID: "incarnation-1",
				HarnessGeneration: 2, State: journal.SubmissionDispatching,
			}},
		})
	}))
	t.Cleanup(server.Close)
	host, port := splitTestServerHostPort(t, server)
	c := NewClient(host, port, newTestLogger())

	status, err := c.GetDeliveryStatus(context.Background(), "")
	if err != nil {
		t.Fatalf("GetDeliveryStatus: %v", err)
	}
	if status.Stream == nil || status.Stream.Acknowledged != 4 || len(status.Submissions) != 1 {
		t.Fatalf("status = %#v", status)
	}
	capability, advertised := c.DurableDeliveryCapability()
	if !advertised || !capability.Durable || capability.Version != journal.CurrentVersion {
		t.Fatalf("capability = %#v, advertised = %v", capability, advertised)
	}
	cached, cachedOK := c.DeliveryRecoveryDescriptor()
	if !cachedOK || cached == nil || cached.IncarnationID != "incarnation-1" || cached.HarnessGeneration != 2 {
		t.Fatalf("cached descriptor = %#v, ok = %v", cached, cachedOK)
	}
}

func TestDeliveryClientStatusDoesNotOverwriteStableCapability(t *testing.T) {
	c := NewClient("127.0.0.1", 1, newTestLogger())
	c.setDurableDelivery(&DurableDeliveryInfo{
		Version: journal.CurrentVersion,
		Durable: true,
	})
	c.setDeliveryStatus(&DeliveryStatus{
		StorageCapability: journal.StorageCapability{
			Version:    journal.CurrentVersion,
			Durable:    true,
			Unresolved: true,
			Reason:     "unresolved_durable_work",
		},
	})

	capability, advertised := c.DurableDeliveryCapability()
	if !advertised || !capability.Durable || capability.Unresolved || capability.Reason != "" {
		t.Fatalf("capability = %#v, advertised = %t; transient status must not replace stable capability", capability, advertised)
	}
}

func TestDeliveryClientStatusPreservesHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"UNAUTHORIZED"}`))
	}))
	t.Cleanup(server.Close)
	host, port := splitTestServerHostPort(t, server)
	c := NewClient(host, port, newTestLogger())

	_, err := c.GetDeliveryStatus(context.Background(), "")
	var statusErr *DeliveryHTTPError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("error = %v, want DeliveryHTTPError(401)", err)
	}
}

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

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.2
func TestDeliveryClientReplayPageLimitUsesAdvertisedBudget(t *testing.T) {
	legacy := NewClient("127.0.0.1", 1, newTestLogger())
	if got := legacy.DeliveryReplayPageLimit(1000); got != 1 {
		t.Fatalf("legacy peer replay page limit = %d, want 1", got)
	}
	modern := NewClient("127.0.0.1", 1, newTestLogger())
	modern.setDurableDelivery(&DurableDeliveryInfo{
		Version: journal.CurrentVersion, Durable: true,
		EventBytes: journal.DefaultMaxEventBytes, SubmissionBytes: journal.DefaultMaxSubmissionBytes,
		ReplayBytes: journal.DefaultMaxReplayBytes,
	})
	if got := modern.DeliveryReplayPageLimit(1000); got != 1000 {
		t.Fatalf("modern peer replay page limit = %d, want 1000", got)
	}
	capability, advertised := modern.DurableDeliveryCapability()
	if !advertised || capability.ReplayBytes != journal.DefaultMaxReplayBytes ||
		capability.EventBytes != journal.DefaultMaxEventBytes ||
		capability.SubmissionBytes != journal.DefaultMaxSubmissionBytes {
		t.Fatalf("advertised capability = %#v", capability)
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
func TestDeliveryClientReplayRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"events":[{"payload":"`))
		_, _ = w.Write(make([]byte, journal.DefaultMaxReplayBytes+journal.DefaultMaxEventBytes))
	}))
	t.Cleanup(server.Close)
	host, port := splitTestServerHostPort(t, server)
	c := NewClient(host, port, newTestLogger())
	if _, _, err := c.ReplayDelivery(context.Background(), "stream-1", 0, 1); !errors.Is(err, ErrDeliveryReplayResponseTooLarge) {
		t.Fatalf("oversized replay error = %v, want ErrDeliveryReplayResponseTooLarge", err)
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.4
func TestDeliveryClientStatusDecodesFailureSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/status" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"agent_status": "running",
			"delivery_health": map[string]any{
				"state": "delivery_failed",
				"failure": map[string]any{
					"reason": "event_too_large", "revision": 3, "submission_id": "submission-1",
					"session_id": "session-1", "stream_id": "stream-1", "event_bytes": 2048, "event_limit": 1024,
				},
			},
		})
	}))
	t.Cleanup(server.Close)
	host, port := splitTestServerHostPort(t, server)
	status, err := NewClient(host, port, newTestLogger()).GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	failure := status.DeliveryHealth.Failure
	if failure == nil || failure.Reason != "event_too_large" || failure.Revision != 3 ||
		failure.SubmissionID != "submission-1" || failure.EventBytes != 2048 || failure.EventLimit != 1024 {
		t.Fatalf("failure snapshot = %#v", failure)
	}
}
