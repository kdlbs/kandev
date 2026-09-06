package client

import (
	"context"
	"encoding/json"
	"errors"
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

func TestPublishWorkspacePreviewPreservesRemoteStatusWithoutResponseBody(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte("source content must not escape the client error"))
			}))
			t.Cleanup(srv.Close)
			host, port := splitTestServerHostPort(t, srv)
			client := NewClient(host, port, newTestLogger())

			_, err := client.PublishWorkspacePreview(context.Background(), WorkspacePreviewRequest{
				Path:    "index.html",
				Content: "<body>current</body>",
			})
			if err == nil {
				t.Fatal("PublishWorkspacePreview returned nil error")
			}
			var statusErr interface {
				error
				StatusCode() int
			}
			if !errors.As(err, &statusErr) {
				t.Fatalf("error = %v, want typed status error", err)
			}
			if statusErr.StatusCode() != status {
				t.Fatalf("status = %d, want %d", statusErr.StatusCode(), status)
			}
			if strings.Contains(err.Error(), "source content must not escape") {
				t.Fatalf("error exposed upstream response body: %v", err)
			}
		})
	}
}

func TestPublishWorkspacePreviewRejectsMalformedSuccessResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	t.Cleanup(srv.Close)
	host, port := splitTestServerHostPort(t, srv)
	client := NewClient(host, port, newTestLogger())

	_, err := client.PublishWorkspacePreview(context.Background(), WorkspacePreviewRequest{
		Path:    "index.html",
		Content: "<body>current</body>",
	})
	if err == nil {
		t.Fatal("PublishWorkspacePreview accepted malformed success response")
	}
	if strings.Contains(err.Error(), "not-json") {
		t.Fatalf("error exposed malformed response body: %v", err)
	}
}
