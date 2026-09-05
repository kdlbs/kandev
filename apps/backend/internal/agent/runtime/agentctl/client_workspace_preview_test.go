package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublishWorkspacePreviewForwardsCurrentBufferAndValidatesResponse(t *testing.T) {
	var got WorkspacePreviewRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workspace/html-previews" {
			t.Errorf("request = %s %s, want POST /api/v1/workspace/html-previews", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"port":43127,"path":"/site/index.html","version":4}`))
	}))
	t.Cleanup(srv.Close)
	host, port := splitTestServerHostPort(t, srv)
	client := NewClient(host, port, newTestLogger())

	response, err := client.PublishWorkspacePreview(context.Background(), WorkspacePreviewRequest{
		Repo:    "frontend",
		Path:    "site/index.html",
		Content: "<script>document.body.dataset.ready = 'yes'</script>",
	})
	if err != nil {
		t.Fatalf("PublishWorkspacePreview: %v", err)
	}
	if got.Repo != "frontend" || got.Path != "site/index.html" || !strings.Contains(got.Content, "dataset.ready") {
		t.Fatalf("request = %+v, want repository, path, and current buffer", got)
	}
	if response.Port != 43127 || response.Path != "/site/index.html" || response.Version != 4 {
		t.Fatalf("response = %+v, want validated server response", response)
	}
}

func TestPublishWorkspacePreviewRejectsOversizedContentBeforeNetwork(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	t.Cleanup(srv.Close)
	host, port := splitTestServerHostPort(t, srv)
	client := NewClient(host, port, newTestLogger())

	_, err := client.PublishWorkspacePreview(context.Background(), WorkspacePreviewRequest{
		Path:    "index.html",
		Content: strings.Repeat("x", MaxWorkspacePreviewContentBytes+1),
	})
	if err == nil {
		t.Fatal("PublishWorkspacePreview accepted oversized content")
	}
	if called {
		t.Fatal("PublishWorkspacePreview sent oversized content to agentctl")
	}
}
