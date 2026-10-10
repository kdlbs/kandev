package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

func TestDeliveryRecordEvidenceOwnerFence(t *testing.T) {
	ctx := context.Background()
	payload := []byte(`{"text":"reconstruct the original instruction"}`)
	now := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	candidate := journal.SubmissionSummary{
		ID: "prompt:one", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 7, StreamID: "stream", Hash: journal.SubmissionHash(payload),
		State: journal.SubmissionDispatching, CreatedAt: now, UpdatedAt: now,
	}
	descriptor := journal.RecoveryDescriptor{
		StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
		SessionID:         "session", IncarnationID: "incarnation", HarnessGeneration: 7,
		Submissions: []journal.SubmissionSummary{candidate}, SubmissionCount: 1,
	}
	submission := journal.Submission{
		ID: candidate.ID, SessionID: candidate.SessionID, IncarnationID: candidate.IncarnationID,
		HarnessGeneration: candidate.HarnessGeneration, StreamID: candidate.StreamID,
		Hash: candidate.Hash, Payload: payload, State: candidate.State, CreatedAt: now, UpdatedAt: now,
	}

	t.Run("retained reads are bounded and selected payload stays backend side", func(t *testing.T) {
		var evidenceReads atomic.Int32
		client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer secret" {
				t.Errorf("request authorization = %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/api/v1/instances":
				_, _ = w.Write([]byte(`{"instances":[]}`))
			case "/api/v1/delivery/retained/evidence":
				evidenceReads.Add(1)
				_ = json.NewEncoder(w).Encode(journal.RetainedReconstructionEvidence{Descriptor: descriptor})
			case "/api/v1/delivery/retained/submission":
				_ = json.NewEncoder(w).Encode(submission)
			default:
				t.Errorf("unexpected runtime request %s %s", r.Method, r.URL.Path)
				http.NotFound(w, r)
			}
		})
		manager := newTestManager(t)
		manager.dataDir = t.TempDir()
		owner := runtimeOwnerForTestClient(t, client)
		manager.SetRuntimeOwner(owner)

		request := DeliveryRecordEvidenceRequest{
			TaskID: "task", SessionID: "session", ExecutionID: "old-execution",
			IncarnationID: "incarnation", HarnessGeneration: 7,
		}
		evidence, err := manager.InspectDeliveryRecordEvidence(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if evidence.Descriptor.SubmissionCount != 1 || len(evidence.Descriptor.Submissions) != 1 || evidence.ProcessTerminated != nil {
			t.Fatalf("inspection evidence = %+v", evidence)
		}
		selected, err := manager.ReadDeliveryRecordSubmission(ctx, DeliveryRecordSubmissionRequest{
			TaskID: request.TaskID, SessionID: request.SessionID, IncarnationID: request.IncarnationID,
			HarnessGeneration: request.HarnessGeneration, Candidate: candidate,
		})
		if err != nil {
			t.Fatal(err)
		}
		if selected.ID != candidate.ID || string(selected.Payload) != string(payload) {
			t.Fatalf("selected payload = %+v", selected)
		}
		if evidenceReads.Load() < 2 {
			t.Fatalf("evidence reads = %d, want pre/post-fetch revalidation", evidenceReads.Load())
		}
	})

	t.Run("live successor evidence does not prove the historical execution", func(t *testing.T) {
		var retainedCalls atomic.Int32
		var instancePort int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/api/v1/instances":
				_ = json.NewEncoder(w).Encode(map[string]any{"instances": []map[string]any{{
					"id": "successor-execution", "task_id": "task", "session_id": "session",
					"port": instancePort, "listener_active": true,
				}}})
			case "/api/v1/agent/delivery":
				_ = json.NewEncoder(w).Encode(descriptor)
			case "/api/v1/agent/submissions/prompt:one":
				_ = json.NewEncoder(w).Encode(submission)
			case "/api/v1/delivery/retained/evidence", "/api/v1/delivery/retained/submission":
				retainedCalls.Add(1)
				http.Error(w, "must use the live instance", http.StatusConflict)
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(server.Close)
		endpoint, err := url.Parse(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		instancePort, err = strconv.Atoi(endpoint.Port())
		if err != nil {
			t.Fatal(err)
		}
		client := agentctl.NewClient(endpoint.Hostname(), instancePort, newTestLogger())
		manager := newTestManager(t)
		manager.dataDir = t.TempDir()
		owner := runtimeOwnerForTestClient(t, client)
		manager.SetRuntimeOwner(owner)

		request := DeliveryRecordEvidenceRequest{
			TaskID: "task", SessionID: "session", ExecutionID: "old-execution",
			IncarnationID: "incarnation", HarnessGeneration: 7,
		}
		evidence, err := manager.InspectDeliveryRecordEvidence(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if evidence.LiveExecutionID != "successor-execution" || evidence.ProcessTerminated != nil {
			t.Fatalf("historical evidence was conflated with the live successor: %+v", evidence)
		}
		request.ExecutionID = "successor-execution"
		evidence, err = manager.InspectDeliveryRecordEvidence(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if evidence.ProcessTerminated == nil || *evidence.ProcessTerminated {
			t.Fatalf("matching live historical execution proof = %+v, want false", evidence.ProcessTerminated)
		}
		selected, err := manager.ReadDeliveryRecordSubmission(ctx, DeliveryRecordSubmissionRequest{
			TaskID: "task", SessionID: "session", IncarnationID: "incarnation",
			HarnessGeneration: 7, Candidate: candidate,
		})
		if err != nil || selected.ID != candidate.ID {
			t.Fatalf("live selected submission = %+v, err %v", selected, err)
		}
		if retainedCalls.Load() != 0 {
			t.Fatalf("live peer fell back to retained storage %d times", retainedCalls.Load())
		}
	})

	t.Run("old peer capability stays blocked", func(t *testing.T) {
		client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/api/v1/instances":
				_, _ = w.Write([]byte(`{"instances":[]}`))
			case "/api/v1/delivery/retained/evidence":
				_ = json.NewEncoder(w).Encode(journal.RetainedReconstructionEvidence{
					Descriptor: journal.RecoveryDescriptor{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 7},
				})
			default:
				http.NotFound(w, r)
			}
		})
		manager := newTestManager(t)
		manager.dataDir = t.TempDir()
		owner := runtimeOwnerForTestClient(t, client)
		manager.SetRuntimeOwner(owner)
		_, err := manager.InspectDeliveryRecordEvidence(ctx, DeliveryRecordEvidenceRequest{
			TaskID: "task", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 7,
		})
		if !errors.Is(err, ErrDeliveryRecoveryBlocked) {
			t.Fatalf("old peer error = %v, want a blocked capability result", err)
		}
	})

	t.Run("retired runtime lease rejects a delayed snapshot", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		var releaseOnce sync.Once
		client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/api/v1/instances":
				_, _ = w.Write([]byte(`{"instances":[]}`))
			case "/api/v1/delivery/retained/evidence":
				close(entered)
				<-release
				_ = json.NewEncoder(w).Encode(journal.RetainedReconstructionEvidence{Descriptor: descriptor})
			default:
				http.NotFound(w, r)
			}
		})
		manager := newTestManager(t)
		manager.dataDir = t.TempDir()
		owner := runtimeOwnerForTestClient(t, client)
		manager.SetRuntimeOwner(owner)
		t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

		done := make(chan error, 1)
		go func() {
			_, err := manager.InspectDeliveryRecordEvidence(ctx, DeliveryRecordEvidenceRequest{
				TaskID: "task", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 7,
			})
			done <- err
		}()
		<-entered
		owner.Stop()
		releaseOnce.Do(func() { close(release) })
		if err := <-done; !errors.Is(err, agentctl.ErrRuntimeLeaseRetired) {
			t.Fatalf("inspection error after runtime lease retirement = %v, want ErrRuntimeLeaseRetired", err)
		}
	})
}

func TestDeliveryRecordEvidenceRejectsChangedDescriptor(t *testing.T) {
	ctx := context.Background()
	payload := []byte(`{"text":"instruction"}`)
	now := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	candidate := journal.SubmissionSummary{
		ID: "prompt:one", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 7, StreamID: "stream", Hash: journal.SubmissionHash(payload),
		State: journal.SubmissionDispatching, CreatedAt: now, UpdatedAt: now,
	}
	var reads atomic.Int32
	client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/instances":
			_, _ = w.Write([]byte(`{"instances":[]}`))
		case "/api/v1/delivery/retained/evidence":
			descriptor := journal.RecoveryDescriptor{
				StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
				SessionID:         "session", IncarnationID: "incarnation", HarnessGeneration: 7,
				Submissions: []journal.SubmissionSummary{candidate}, SubmissionCount: 1,
			}
			if reads.Add(1) == 2 {
				descriptor.Submissions[0].UpdatedAt = descriptor.Submissions[0].UpdatedAt.Add(time.Second)
			}
			_ = json.NewEncoder(w).Encode(journal.RetainedReconstructionEvidence{Descriptor: descriptor})
		case "/api/v1/delivery/retained/submission":
			_ = json.NewEncoder(w).Encode(journal.Submission{
				ID: candidate.ID, SessionID: candidate.SessionID, IncarnationID: candidate.IncarnationID,
				HarnessGeneration: candidate.HarnessGeneration, StreamID: candidate.StreamID,
				Hash: candidate.Hash, Payload: payload, State: candidate.State, CreatedAt: now, UpdatedAt: now,
			})
		default:
			http.NotFound(w, r)
		}
	})
	manager := newTestManager(t)
	manager.dataDir = t.TempDir()
	owner := runtimeOwnerForTestClient(t, client)
	manager.SetRuntimeOwner(owner)

	_, err := manager.ReadDeliveryRecordSubmission(ctx, DeliveryRecordSubmissionRequest{
		TaskID: "task", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 7, Candidate: candidate,
	})
	if !errors.Is(err, ErrDeliveryRecordEvidenceStale) {
		t.Fatalf("changed descriptor read error = %v, want stale evidence", err)
	}
}
