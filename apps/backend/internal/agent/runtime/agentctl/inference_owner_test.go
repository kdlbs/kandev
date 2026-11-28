package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

func TestInferenceOwnerHTTPFailureRetainsExactAttemptResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"success":false,"error":"attempt already consumed","attempt":{"attempt_id":"issued","owner_id":"owner","instance_id":"instance","state":"unknown","known":false,"settled":false}}`))
	}))
	defer server.Close()
	c := newHTTPOnlyClient(server.URL)
	c.executionID = "instance"
	response, err := c.InferencePrompt(context.Background(), &utility.PromptRequest{AttemptID: "issued", Prompt: "fixture", AgentID: "fake"})
	if err == nil || response == nil || response.Attempt == nil || response.Attempt.AttemptID != "issued" || response.Attempt.OwnerID != "owner" {
		t.Fatalf("transport rejection lost exact owner: response=%+v err=%v", response, err)
	}
	if response.Attempt.Known || response.Attempt.Settled {
		t.Fatalf("HTTP failure invented settlement: %+v", response.Attempt)
	}
}

func TestInferenceOwnerNilPromptNeverSends(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); _, _ = w.Write([]byte(`{"success":true}`)) }))
	defer server.Close()
	_, err := newHTTPOnlyClient(server.URL).InferencePrompt(context.Background(), nil)
	if err == nil || calls.Load() != 0 {
		t.Fatalf("nil request sent=%d err=%v", calls.Load(), err)
	}
}
