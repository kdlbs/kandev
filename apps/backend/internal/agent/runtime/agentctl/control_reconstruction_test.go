package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

func TestControlClientReadsRetainedReconstructionEvidenceAndPayload(t *testing.T) {
	payload := []byte(`{"text":"original instruction"}`)
	now := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	candidate := journal.SubmissionSummary{
		ID: "prompt:one", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 7, StreamID: "stream", Hash: journal.SubmissionHash(payload),
		State: journal.SubmissionDispatching, CreatedAt: now, UpdatedAt: now,
	}
	submission := journal.Submission{
		ID: candidate.ID, SessionID: candidate.SessionID, IncarnationID: candidate.IncarnationID,
		HarnessGeneration: candidate.HarnessGeneration, StreamID: candidate.StreamID,
		Hash: candidate.Hash, Payload: payload, State: candidate.State, CreatedAt: now, UpdatedAt: now,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer retained-secret" {
			t.Errorf("authorization = %q, want bearer token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/delivery/retained/evidence":
			var request journal.RetainedReconstructionEvidenceRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if request.Root != "/backend/session-data" || request.SessionID != "session" || request.IncarnationID != "incarnation" || request.HarnessGeneration != 7 {
				t.Errorf("evidence request = %+v", request)
			}
			_ = json.NewEncoder(w).Encode(journal.RetainedReconstructionEvidence{
				Descriptor: journal.RecoveryDescriptor{
					StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
					SessionID:         "session", IncarnationID: "incarnation", HarnessGeneration: 7,
					Submissions: []journal.SubmissionSummary{candidate}, SubmissionCount: 1,
				},
			})
		case "/api/v1/delivery/retained/submission":
			var request journal.RetainedReconstructionSubmissionRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if request.Candidate.ID != candidate.ID || request.HarnessGeneration != candidate.HarnessGeneration {
				t.Errorf("submission request = %+v", request)
			}
			_ = json.NewEncoder(w).Encode(submission)
		default:
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := newTestControlClient(t, server, WithControlAuthToken("retained-secret"))

	evidence, err := client.ReadRetainedReconstructionEvidence(context.Background(), journal.RetainedReconstructionEvidenceRequest{
		Root: "/backend/session-data", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Descriptor.SubmissionCount != 1 || evidence.Descriptor.Submissions[0].ID != candidate.ID || evidence.ProcessTerminated != nil {
		t.Fatalf("evidence = %+v", evidence)
	}
	selected, err := client.ReadRetainedReconstructionSubmission(context.Background(), journal.RetainedReconstructionSubmissionRequest{
		Root: "/backend/session-data", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 7, Candidate: candidate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != candidate.ID || string(selected.Payload) != string(payload) {
		t.Fatalf("selected submission = %+v", selected)
	}
}
