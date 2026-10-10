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

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.11
func TestRetainedDeliveryReadsEvidenceWithoutCreatingInstance(t *testing.T) {
	root := t.TempDir()
	capability := journal.CheckStorage(root, "session")
	j, err := journal.Open(journal.Config{Path: capability.Path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()
	if _, err = j.PutSubmission(ctx, journal.Submission{ID: "prompt", Hash: "hash", SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "stream", State: journal.SubmissionDispatching}); err != nil {
		t.Fatal(err)
	}
	if _, err = j.Append(ctx, journal.Event{SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "stream", SubmissionID: "prompt", Type: "complete", Terminal: true, Payload: []byte(`{"type":"complete"}`)}); err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	manager := &instance.Manager{}
	server := NewControlServer(&config.Config{AuthToken: "secret"}, manager, logger.Default())
	body, _ := json.Marshal(map[string]any{"root": root, "session_id": "session", "execution_id": "execution", "incarnation_id": "session", "harness_generation": 1, "stream_id": "stream", "submission_id": "prompt", "limit": 4})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/delivery/retained", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("retained query status=%d: %s", response.Code, response.Body.String())
	}
	var result struct {
		Events            []journal.Event `json:"events"`
		ProcessTerminated bool            `json:"process_terminated"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 1 || result.Events[0].Sequence != 1 || result.ProcessTerminated {
		t.Fatalf("retained evidence=%+v", result)
	}
	if len(manager.ListInstances()) != 0 {
		t.Fatal("evidence query created a runtime instance")
	}
}
