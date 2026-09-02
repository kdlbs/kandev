package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientFetchAllUsesCursorAndUpdatedSince(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "next-page" {
			_, _ = w.Write([]byte(`{"servers":[{"name":"com.example/two","description":"Two","version":"2.0.0"}],"metadata":{"nextCursor":""}}`))
			return
		}
		_, _ = w.Write([]byte(`{"servers":[{"server":{"name":"com.example/one","description":"One","version":"1.0.0","packages":[{"registryType":"npm","identifier":"@example/one","version":"1.0.0","transport":{"type":"stdio"}}]},"_meta":{"publisher-provided":{"name":"publisher"}}}],"metadata":{"nextCursor":"next-page"}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	since := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	entries, err := client.FetchAll(context.Background(), ListOptions{UpdatedSince: &since, IncludeDeleted: true})
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(entries) != 2 || entries[0].Name != "com.example/one" || entries[1].Name != "com.example/two" {
		t.Fatalf("entries = %#v", entries)
	}
	if len(requests) != 2 || !strings.Contains(requests[0], "updated_since=") || !strings.Contains(requests[0], "include_deleted=true") || !strings.Contains(requests[1], "cursor=next-page") {
		t.Fatalf("request queries = %#v", requests)
	}
	if len(entries[0].Packages) != 1 || entries[0].Packages[0].RegistryType != "npm" {
		t.Fatalf("package = %#v", entries[0].Packages)
	}
}

func TestClientBoundsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"servers":[{"name":"com.example/huge","description":"` + strings.Repeat("x", 200) + `","version":"1.0.0"}]}`))
	}))
	defer server.Close()
	client, err := NewClientWithOptions(server.URL, ClientOptions{HTTPClient: server.Client(), MaxResponseBytes: 128})
	if err != nil {
		t.Fatalf("NewClientWithOptions: %v", err)
	}
	if _, err := client.List(context.Background(), ListOptions{}); err == nil {
		t.Fatal("oversized response succeeded")
	}
}
