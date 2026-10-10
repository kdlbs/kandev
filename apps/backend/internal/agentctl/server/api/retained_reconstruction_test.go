package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestRetainedReconstructionEvidenceUsesAuthenticatedReadAndSelectedPayload(t *testing.T) {
	root := t.TempDir()
	capability := journal.CheckStorage(root, "session")
	j, err := journal.Open(journal.Config{Path: capability.Path})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	payload := []byte(`{"text":"inspect the original instruction"}`)
	if _, err := j.PutSubmission(ctx, journal.Submission{
		ID: "prompt:one", SessionID: "session", IncarnationID: "incarnation-current",
		HarnessGeneration: 7, StreamID: "stream-current", Hash: journal.SubmissionHash(payload),
		Payload: payload, State: journal.SubmissionDispatching,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.PutSubmission(ctx, journal.Submission{
		ID: "prompt:completed", SessionID: "session", IncarnationID: "incarnation-old",
		HarnessGeneration: 6, StreamID: "stream-old", Hash: journal.SubmissionHash([]byte(`{"text":"done"}`)),
		Payload: []byte(`{"text":"done"}`), State: journal.SubmissionCompleted, TerminalEventRetained: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(ctx, journal.Event{
		SessionID: "session", IncarnationID: "incarnation-current", HarnessGeneration: 7,
		StreamID: "stream-current", SubmissionID: "prompt:one", Type: "message",
		Payload: []byte(`{"text":"already acknowledged"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := j.Acknowledge(ctx, "stream-current", 1); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	manager := &instance.Manager{}
	server := NewControlServer(&config.Config{AuthToken: "secret"}, manager, logger.Default())
	requestBody := map[string]any{
		"root": root, "session_id": "session", "incarnation_id": "incarnation-current", "harness_generation": 7,
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatal(err)
	}
	unauthorized := httptest.NewRequest(http.MethodPost, "/api/v1/delivery/retained/evidence", bytes.NewReader(body))
	unauthorizedResponse := httptest.NewRecorder()
	server.Router().ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated retained evidence status=%d, want 401", unauthorizedResponse.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/delivery/retained/evidence", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("retained evidence status=%d: %s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte(`"payload"`)) || bytes.Contains(response.Body.Bytes(), payload) {
		t.Fatalf("evidence response exposed retained prompt payload: %s", response.Body.String())
	}
	var evidence struct {
		Descriptor        journal.RecoveryDescriptor `json:"descriptor"`
		ProcessTerminated *bool                      `json:"process_terminated,omitempty"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.ProcessTerminated != nil {
		t.Fatalf("missing process identity was converted into proof: %v", *evidence.ProcessTerminated)
	}
	if evidence.Descriptor.SessionID != "session" || evidence.Descriptor.IncarnationID != "incarnation-current" || evidence.Descriptor.HarnessGeneration != 7 || evidence.Descriptor.SubmissionCount != 1 || len(evidence.Descriptor.Submissions) != 1 {
		t.Fatalf("retained descriptor = %+v", evidence.Descriptor)
	}
	candidate := evidence.Descriptor.Submissions[0]
	if candidate.ID != "prompt:one" || candidate.StreamID != "stream-current" || candidate.Hash != journal.SubmissionHash(payload) {
		t.Fatalf("candidate summary = %+v", candidate)
	}
	if len(manager.ListInstances()) != 0 {
		t.Fatal("evidence inspection created a runtime instance")
	}

	fetchBody, err := json.Marshal(map[string]any{
		"root": root, "session_id": "session", "incarnation_id": "incarnation-current",
		"harness_generation": 7, "candidate": candidate,
	})
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/delivery/retained/submission", bytes.NewReader(fetchBody))
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	server.Router().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("selected retained submission status=%d: %s", response.Code, response.Body.String())
	}
	var submission journal.Submission
	if err := json.Unmarshal(response.Body.Bytes(), &submission); err != nil {
		t.Fatal(err)
	}
	if submission.ID != candidate.ID || !bytes.Equal(submission.Payload, payload) || submission.Hash != candidate.Hash {
		t.Fatalf("selected retained submission = %+v", submission)
	}
}
