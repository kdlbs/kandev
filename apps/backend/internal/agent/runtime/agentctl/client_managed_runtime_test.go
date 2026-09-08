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

	if err := client.RepairManagedRuntimeCacheWithEnvironment(
		context.Background(),
		"@scope/managed-acp@1.2.3",
		map[string]string{"NPM_CONFIG_CACHE": "/probe/cache"},
		[]string{"npm_config_cache"},
	); err != nil {
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
	if request.Env["NPM_CONFIG_CACHE"] != "/probe/cache" {
		t.Fatalf("environment = %#v, want probe cache override", request.Env)
	}
	if len(request.StripEnv) != 1 || request.StripEnv[0] != "npm_config_cache" {
		t.Fatalf("strip environment = %#v, want probe strip list", request.StripEnv)
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
