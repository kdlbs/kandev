package client

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestRepairManagedRuntimeCachePostsExactPackageSpec(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"success":true}`))
	client := newHTTPOnlyClient(srv.URL)

	if err := client.RepairManagedRuntimeCache(context.Background(), "@scope/managed-acp@1.2.3"); err != nil {
		t.Fatalf("RepairManagedRuntimeCache: %v", err)
	}
	if got.Method != http.MethodPost || got.Path != "/api/v1/agent/managed-runtime/cache-repair" {
		t.Fatalf("request = %s %s, want POST /api/v1/agent/managed-runtime/cache-repair", got.Method, got.Path)
	}
	var request RepairManagedRuntimeCacheRequest
	if err := json.Unmarshal(got.Body, &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if request.PackageSpec != "@scope/managed-acp@1.2.3" {
		t.Fatalf("package spec = %q, want exact selected spec", request.PackageSpec)
	}
}

func TestRepairManagedRuntimeCacheRejectsUnversionedSpecBeforeHTTP(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"success":true}`))
	client := newHTTPOnlyClient(srv.URL)

	if err := client.RepairManagedRuntimeCache(context.Background(), "managed-acp"); err == nil {
		t.Fatal("RepairManagedRuntimeCache(unversioned) = nil, want rejection")
	}
	if got.Method != "" {
		t.Fatalf("request method = %q, want no HTTP request", got.Method)
	}
}
