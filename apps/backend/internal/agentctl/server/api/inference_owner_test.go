package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func inferenceOwnerServer(t *testing.T) (*Server, *process.Manager) {
	t.Helper()
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), InstanceID: "owned-instance", SessionID: "parent-session", TaskID: "parent-task", DurableJournalPath: filepath.Join(t.TempDir(), "delivery.bbolt")}
	log := newTestLogger()
	manager := process.NewManager(cfg, log)
	server := NewServer(cfg, manager, nil, nil, log)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := manager.StopForTeardown(ctx); err != nil {
			t.Error(err)
		}
	})
	return server, manager
}

func inferenceOwnerRequest(t *testing.T, s *Server, method, path string, body any, wantCode int) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, "/api/v1/inference"+path, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	result := httptest.NewRecorder()
	s.Router().ServeHTTP(result, request)
	if result.Code != wantCode {
		t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, result.Code, wantCode, result.Body.String())
	}
	var value map[string]any
	if err := json.Unmarshal(result.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestInferenceOwnerAPITracksPreparedCancelledAndDuplicateAttempts(t *testing.T) {
	s, _ := inferenceOwnerServer(t)
	snapshot := inferenceOwnerRequest(t, s, http.MethodGet, "/attempts", nil, 200)
	if snapshot["known"] != true || snapshot["owner_id"] == "" || snapshot["instance_id"] != "owned-instance" {
		t.Fatalf("fresh inventory missing: %+v", snapshot)
	}
	preparation := map[string]any{"preparation_id": uuid.NewString(), "call_id": uuid.NewString(), "retain": true}
	prepared := inferenceOwnerRequest(t, s, http.MethodPost, "/attempts", preparation, 200)
	id, ok := prepared["attempt_id"].(string)
	if !ok || id == "" || prepared["state"] != "prepared" || prepared["settled"] != false || prepared["known"] != true {
		t.Fatalf("invalid preparation: %+v", prepared)
	}
	again := inferenceOwnerRequest(t, s, http.MethodPost, "/attempts", preparation, 200)
	if again["attempt_id"] != id {
		t.Fatalf("retry duplicated attempt: %+v", again)
	}
	cancelled := inferenceOwnerRequest(t, s, http.MethodPost, "/attempts/"+id+"/cancel", nil, 200)
	assertInferenceNotStarted(t, cancelled, id)
	assertInferenceNotStarted(t, inferenceOwnerRequest(t, s, http.MethodPost, "/attempts/"+id+"/cancel", nil, 200), id)
	inferenceOwnerRequest(t, s, http.MethodPost, "/attempts", preparation, 409)
	observed := inferenceOwnerRequest(t, s, http.MethodGet, "/attempts/"+id, nil, 200)
	if observed["retained"] != true {
		t.Fatalf("unacknowledged terminal evidence pruned: %+v", observed)
	}
	inferenceOwnerRequest(t, s, http.MethodPost, "/attempts/"+id+"/release", nil, 200)
}

func assertInferenceNotStarted(t *testing.T, status map[string]any, id string) {
	t.Helper()
	if status["attempt_id"] != id || status["state"] != "not_started" || status["known"] != true || status["settled"] != true || status["prompt_dispatch"] != "not_sent" {
		t.Fatalf("missing exact unsent proof: %+v", status)
	}
}

func TestInferenceOwnerAPICancelBeforeLatePrepareFencesStart(t *testing.T) {
	s, _ := inferenceOwnerServer(t)
	preparation := uuid.NewString()
	cancelled := inferenceOwnerRequest(t, s, http.MethodPost, "/preparations/"+preparation+"/cancel", nil, 200)
	late := inferenceOwnerRequest(t, s, http.MethodPost, "/attempts", map[string]any{"preparation_id": preparation}, 409)
	id, _ := cancelled["attempt_id"].(string)
	if id == "" || late["attempt_id"] != id {
		t.Fatalf("late prepare escaped cancellation: %+v / %+v", cancelled, late)
	}
	assertInferenceNotStarted(t, late, id)
	prompt := map[string]any{"prompt": "never launch", "agent_id": "claude-code", "attempt_id": id}
	response := inferenceOwnerRequest(t, s, http.MethodPost, "/prompt", prompt, 409)
	status, ok := response["attempt"].(map[string]any)
	if !ok {
		t.Fatalf("no rejected attempt identity: %+v", response)
	}
	assertInferenceNotStarted(t, status, id)
}

func TestInferenceOwnerAPIExecutionFailureProvesUnstartedAndRejectsReplay(t *testing.T) {
	s, _ := inferenceOwnerServer(t)
	prepared := inferenceOwnerRequest(t, s, http.MethodPost, "/attempts", map[string]any{"preparation_id": uuid.NewString()}, 200)
	id := prepared["attempt_id"].(string)
	prompt := map[string]any{"prompt": "fixture", "agent_id": "claude-code", "attempt_id": id}
	response := inferenceOwnerRequest(t, s, http.MethodPost, "/prompt", prompt, 200)
	if response["success"] != false {
		t.Fatalf("missing-config execution unexpectedly succeeded: %+v", response)
	}
	status, ok := response["attempt"].(map[string]any)
	if !ok {
		t.Fatalf("execution failure dropped identity: %+v", response)
	}
	assertInferenceNotStarted(t, status, id)
	inferenceOwnerRequest(t, s, http.MethodPost, "/prompt", prompt, 409)
}

func TestInferenceOwnerAPIRespectsExistingTeardownAdmission(t *testing.T) {
	s, manager := inferenceOwnerServer(t)
	manager.CloseAdmission()
	inferenceOwnerRequest(t, s, http.MethodPost, "/attempts", map[string]any{"preparation_id": uuid.NewString()}, 503)
}

func TestInferenceOwnerAPIOldJournalIsUnknownNotEmpty(t *testing.T) {
	s, manager := inferenceOwnerServer(t)
	original := inferenceOwnerRequest(t, s, http.MethodGet, "/attempts", nil, 200)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := manager.StopForTeardown(ctx); err != nil {
		t.Fatal(err)
	}
	log := newTestLogger()
	nextManager := process.NewManager(s.cfg, log)
	next := NewServer(s.cfg, nextManager, nil, nil, log)
	defer func() {
		if err := nextManager.StopForTeardown(ctx); err != nil {
			t.Error(err)
		}
	}()
	snapshot := inferenceOwnerRequest(t, next, http.MethodGet, "/attempts", nil, 200)
	if snapshot["known"] != false || snapshot["owner_id"] == original["owner_id"] {
		t.Fatalf("restart invented continuity: %+v", snapshot)
	}
}

func TestInferenceOwnerAPIWaitsForDrainBeforeFinalProof(t *testing.T) {
	s, manager := inferenceOwnerServer(t)
	prepared := inferenceOwnerRequest(t, s, http.MethodPost, "/attempts", map[string]any{"preparation_id": uuid.NewString(), "retain": true}, 200)
	_, release, err := manager.BeginOwnedOperation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	manager.CloseAdmission()
	if got := manager.FinalInferenceSnapshot(); got != nil {
		t.Fatalf("proof before drain: %+v", got)
	}
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		done <- manager.StopForTeardown(ctx)
	}()
	if got := manager.FinalInferenceSnapshot(); got != nil {
		t.Fatalf("owned operation not drained: %+v", got)
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("teardown did not drain")
	}
	got := manager.FinalInferenceSnapshot()
	if got == nil || len(got.Attempts) != 1 {
		t.Fatalf("missing final retained proof: %+v", got)
	}
	attempt := got.Attempts[0]
	if attempt.AttemptID != prepared["attempt_id"] || !attempt.Known || !attempt.Settled || attempt.State != "not_started" {
		t.Fatalf("wrong final preparation proof: %+v", attempt)
	}
	got.Attempts[0].OwnerID = "mutated-copy"
	if manager.FinalInferenceSnapshot().Attempts[0].OwnerID == "mutated-copy" {
		t.Fatal("caller mutated retained proof")
	}
}

func TestInferenceOwnerAPIFailedDrainDoesNotPublishProof(t *testing.T) {
	_, manager := inferenceOwnerServer(t)
	_, release, err := manager.BeginOwnedOperation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.StopForTeardown(ctx); err == nil {
		t.Fatal("undrained teardown reported success")
	}
	if got := manager.FinalInferenceSnapshot(); got != nil {
		t.Fatalf("failed drain published proof: %+v", got)
	}
}
