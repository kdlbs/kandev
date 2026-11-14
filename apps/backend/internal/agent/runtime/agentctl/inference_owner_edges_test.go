package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

func TestInferenceOwnerLostPrepareCleansExactFenceBeforeAcknowledging(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "exact", true: "foreign"}[foreign], func(t *testing.T) {
			var preparation string
			var cleanup, release atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/api/v1/inference/attempts":
					var request utility.PrepareAttemptRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					if !request.Retain {
						t.Error("capacity preparation not retained")
					}
					preparation = request.PreparationID
					_, _ = w.Write([]byte(`{`))
				case strings.HasPrefix(r.URL.Path, "/api/v1/inference/preparations/"):
					cleanup.Add(1)
					if !strings.Contains(r.URL.Path, preparation) {
						t.Error("cleanup lost fence")
					}
					id := preparation
					if foreign {
						id = uuid.NewString()
					}
					_ = json.NewEncoder(w).Encode(utility.AttemptStatus{AttemptID: "issued", OwnerID: "owner", InstanceID: "instance", PreparationID: id, Known: true, Settled: true, State: utility.AttemptNotStarted, PromptDispatch: utility.PromptNotSent})
				case strings.HasSuffix(r.URL.Path, "/release"):
					release.Add(1)
					_ = json.NewEncoder(w).Encode(utility.AttemptStatus{AttemptID: "issued", OwnerID: "owner", InstanceID: "instance", Known: true, Settled: true, State: utility.AttemptNotStarted})
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			c := newHTTPOnlyClient(server.URL)
			c.executionID = "instance"
			_, err := c.PrepareInferenceCapacityAttempt(context.Background(), uuid.NewString())
			var failure *InferenceAttemptError
			if !errors.As(err, &failure) || !failure.NoPromptRequestSent || failure.PreparationID != preparation {
				t.Fatalf("lost preparation identity: %v", err)
			}
			if cleanup.Load() != 1 {
				t.Fatalf("cleanup count=%d", cleanup.Load())
			}
			if foreign {
				if failure.Attempt != nil || release.Load() != 0 {
					t.Fatalf("foreign proof acknowledged: %+v release=%d", failure.Attempt, release.Load())
				}
			} else if failure.Attempt == nil || release.Load() != 1 {
				t.Fatalf("exact cancellation not acknowledged: %+v release=%d", failure.Attempt, release.Load())
			}
		})
	}
}

func TestInferenceOwnerForeignObservationNeverBecomesProof(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(utility.AttemptStatus{AttemptID: "different-token", OwnerID: "owner", InstanceID: "foreign", Known: true, Settled: true, State: utility.AttemptTerminated})
	}))
	defer server.Close()
	c := newHTTPOnlyClient(server.URL)
	c.executionID = "instance"
	if _, err := c.CancelPreparedInferenceAttempt(context.Background(), "issued"); err == nil {
		t.Fatal("foreign cancellation response accepted")
	}
	if _, err := c.InferenceAttempt(context.Background(), "issued"); err == nil {
		t.Fatal("foreign GET response accepted")
	}
	if _, err := c.PrepareInferenceCapacityAttempt(context.Background(), uuid.NewString()); err == nil {
		t.Fatal("foreign preparation response accepted")
	}
}

func TestInferenceOwnerOwnedPromptUsesOneIssuedTokenAndLegacyFallback(t *testing.T) {
	for _, supported := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "owned"}[supported], func(t *testing.T) {
			var prepare, prompts atomic.Int32
			attempt := uuid.NewString()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/inference/attempts" {
					prepare.Add(1)
					if !supported {
						w.WriteHeader(404)
						return
					}
					var request utility.PrepareAttemptRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					_ = json.NewEncoder(w).Encode(utility.AttemptStatus{AttemptID: attempt, PreparationID: request.PreparationID, OwnerID: "owner", InstanceID: "instance", Known: true, State: utility.AttemptPrepared})
					return
				}
				prompts.Add(1)
				var request utility.PromptRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if (supported && request.AttemptID != attempt) || (!supported && request.AttemptID != "") {
					t.Errorf("wrong request token: %+v", request)
				}
				_ = json.NewEncoder(w).Encode(utility.PromptResponse{Success: true, Response: "fixture"})
			}))
			defer server.Close()
			c := newHTTPOnlyClient(server.URL)
			c.executionID = "instance"
			request := &utility.PromptRequest{Prompt: "fixture", AgentID: "fake"}
			response, err := c.InferencePromptOwned(context.Background(), request)
			if err != nil || response == nil || !response.Success || prepare.Load() != 1 || prompts.Load() != 1 || request.AttemptID != "" {
				t.Fatalf("one-use request/fallback failed: %+v %v prepare=%d prompt=%d request=%+v", response, err, prepare.Load(), prompts.Load(), request)
			}
		})
	}
}

func TestInferenceOwnerSnapshotMissingInventoryIsNotZero(t *testing.T) {
	for _, tc := range []struct {
		name, body       string
		wantError, known bool
	}{
		{"omitted", `{"owner_id":"owner","instance_id":"instance","known":true}`, true, false},
		{"unknown_attempt", `{"owner_id":"owner","instance_id":"instance","known":true,"attempts":[{"attempt_id":"issued","owner_id":"owner","instance_id":"instance","known":false,"state":"unknown"}]}`, true, false},
		{"known_empty", `{"owner_id":"owner","instance_id":"instance","known":true,"attempts":[]}`, false, true},
		{"unknown_empty", `{"owner_id":"owner","instance_id":"instance","known":false,"attempts":[]}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) }))
			defer server.Close()
			c := newHTTPOnlyClient(server.URL)
			c.executionID = "instance"
			snapshot, err := c.InferenceAttempts(context.Background())
			if tc.wantError {
				if err == nil {
					t.Fatalf("incomplete inventory attested zero: %+v", snapshot)
				}
			} else if err != nil || snapshot.Known != tc.known {
				t.Fatalf("snapshot=%+v err=%v", snapshot, err)
			}
		})
	}
}
