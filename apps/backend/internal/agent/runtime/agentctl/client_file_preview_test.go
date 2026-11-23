package client

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFilePreviewClientRequiresCompletion(t *testing.T) {
	for _, complete := range []bool{false, true} {
		t.Run(map[bool]string{false: "incomplete", true: "complete"}[complete], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Trailer", "X-Kandev-Preview-Result")
				w.Header().Set("Content-Type", "application/octet-stream")
				_, _ = w.Write([]byte("mesh"))
				if complete {
					w.Header().Set("X-Kandev-Preview-Result", "complete")
				}
			}))
			defer server.Close()
			c := &Client{baseURL: server.URL, httpClient: server.Client(), longRunningHTTPClient: server.Client()}
			var out bytes.Buffer
			err := c.CopyFilePreview(context.Background(), "token", 4, &out)
			if (err == nil) != complete {
				t.Fatalf("completion=%v error=%v", complete, err)
			}
			if complete && out.String() != "mesh" {
				t.Fatal("content changed")
			}
		})
	}
}

func TestFilePreviewClientPrepareAndCancel(t *testing.T) {
	var cancelled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Error("wrong method")
		}
		switch r.URL.Path {
		case "/api/v1/workspace/file-preview/prepare":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"one-use","path":"/executor/model.stl","external":true,"size_bytes":4}`))
		case "/api/v1/workspace/file-preview/cancel":
			cancelled = true
			w.WriteHeader(204)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c := &Client{baseURL: server.URL, longRunningHTTPClient: server.Client()}
	meta, err := c.PrepareFilePreview(t.Context(), "/executor/model.stl", "")
	if err != nil || meta == nil || !meta.External || meta.Token != "one-use" {
		t.Fatalf("prepare: %+v %v", meta, err)
	}
	if err := c.CancelFilePreview(t.Context(), meta.Token); err != nil {
		t.Fatal(err)
	}
	if !cancelled {
		t.Fatal("remote selection not released")
	}
}
