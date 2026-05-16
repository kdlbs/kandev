package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// newHTTPOnlyClient builds a Client wired to a custom HTTP base URL without
// the WebSocket stream — sufficient for testing simple HTTP RPC methods.
func newHTTPOnlyClient(baseURL string) *Client {
	return &Client{
		baseURL:               baseURL,
		httpClient:            &http.Client{Timeout: 5 * time.Second},
		longRunningHTTPClient: &http.Client{Timeout: 5 * time.Minute},
		logger:                newTestLogger(),
		pendingRequests:       make(map[string]chan *ws.Message),
	}
}

func TestSetWorkspacePollMode_PostsExpectedPayload(t *testing.T) {
	type received struct {
		Path        string
		Method      string
		ContentType string
		Body        struct {
			Mode string `json:"mode"`
		}
	}
	var got received

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Path = r.URL.Path
		got.Method = r.Method
		got.ContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&got.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"mode":"slow"}`))
	}))
	defer srv.Close()

	c := newHTTPOnlyClient(srv.URL)

	if err := c.SetWorkspacePollMode(context.Background(), "slow"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Path != "/api/v1/workspace/poll-mode" {
		t.Errorf("path = %q, want /api/v1/workspace/poll-mode", got.Path)
	}
	if got.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.Method)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}
	if got.Body.Mode != "slow" {
		t.Errorf("body.mode = %q, want slow", got.Body.Mode)
	}
}

func TestSetWorkspacePollMode_ReturnsErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad mode"}`))
	}))
	defer srv.Close()

	c := newHTTPOnlyClient(srv.URL)

	err := c.SetWorkspacePollMode(context.Background(), "turbo")
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should mention status code 400, got: %v", err)
	}
}

func TestSetWorkspacePollMode_HonoursContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the context deadline.
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newHTTPOnlyClient(srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := c.SetWorkspacePollMode(ctx, "fast")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
