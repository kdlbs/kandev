package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSealAndDrainRuntimeSendsAuthenticatedRequestAndValidatesReceipt(t *testing.T) {
	completedAt := time.Now().UTC().Truncate(time.Second)
	var gotMethod, gotPath, gotAuthorization string
	var gotRequest RuntimeDrainRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Errorf("decode drain request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RuntimeDrainReceipt{
			RequestID:      gotRequest.RequestID,
			ServerIdentity: gotRequest.ExpectedServerIdentity,
			Sealed:         true,
			Drained:        true,
			Scope:          "managed_process_groups",
			Version:        "runtime-seal-drain.v1",
			CompletedAt:    completedAt,
		})
	}))
	t.Cleanup(srv.Close)

	client := newTestControlClient(t, srv, WithControlAuthToken("control-secret"))
	request := RuntimeDrainRequest{
		RequestID:              "drain-1",
		ExpectedServerIdentity: "server-1",
		DeadlineMS:             5000,
	}
	receipt, err := client.SealAndDrainRuntime(context.Background(), request)
	if err != nil {
		t.Fatalf("SealAndDrainRuntime() error: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/runtime/seal-and-drain" {
		t.Fatalf("request = %s %s, want POST /api/v1/runtime/seal-and-drain", gotMethod, gotPath)
	}
	if gotAuthorization != "Bearer control-secret" {
		t.Fatalf("Authorization = %q, want bearer control credential", gotAuthorization)
	}
	if gotRequest != request {
		t.Fatalf("request body = %+v, want %+v", gotRequest, request)
	}
	if receipt.RequestID != request.RequestID || receipt.ServerIdentity != request.ExpectedServerIdentity || !receipt.Sealed || !receipt.Drained || !receipt.CompletedAt.Equal(completedAt) {
		t.Fatalf("receipt = %+v, does not match request and expected completion", receipt)
	}
}

func TestSealAndDrainRuntimeRejectsMismatchedReceipt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RuntimeDrainReceipt{
			RequestID:      "wrong-request",
			ServerIdentity: "server-1",
			Sealed:         true,
			Drained:        true,
			Scope:          "managed_process_groups",
			Version:        "runtime-seal-drain.v1",
			CompletedAt:    time.Now().UTC(),
		})
	}))
	t.Cleanup(srv.Close)

	client := newTestControlClient(t, srv)
	if _, err := client.SealAndDrainRuntime(context.Background(), RuntimeDrainRequest{
		RequestID:              "drain-1",
		ExpectedServerIdentity: "server-1",
		DeadlineMS:             5000,
	}); err == nil {
		t.Fatal("SealAndDrainRuntime() accepted a receipt for another request")
	}
}
