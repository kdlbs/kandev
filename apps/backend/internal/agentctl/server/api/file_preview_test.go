package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestFilePreviewHTTPSelectionAndCompletion(t *testing.T) {
	server, manager, root := newWorkspacePreviewServer(t)
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	if err := os.WriteFile(filepath.Join(root, "model.stl"), []byte("mesh"), 0600); err != nil {
		t.Fatal(err)
	}
	prepare := httptest.NewRecorder()
	server.Router().ServeHTTP(prepare, httptest.NewRequest(http.MethodPost, "/api/v1/workspace/file-preview/prepare", bytes.NewBufferString(`{"path":"model.stl"}`)))
	if prepare.Code != 200 {
		t.Fatalf("prepare status = %d: %s", prepare.Code, prepare.Body.String())
	}
	var meta types.FilePreviewMetadata
	if err := json.Unmarshal(prepare.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if meta.External || meta.Token == "" || meta.SizeBytes != 4 {
		t.Fatalf("metadata = %+v", meta)
	}
	body, _ := json.Marshal(map[string]string{"token": meta.Token})
	read := httptest.NewRecorder()
	server.Router().ServeHTTP(read, httptest.NewRequest(http.MethodPost, "/api/v1/workspace/file-preview/read", bytes.NewReader(body)))
	result := read.Result()
	defer func() { _ = result.Body.Close() }()
	if read.Body.String() != "mesh" || result.Trailer.Get("X-Kandev-Preview-Result") != "complete" {
		t.Fatalf("read: %q trailer %v", read.Body.String(), result.Trailer)
	}
	if result.Header.Get("Cache-Control") != "no-store" || result.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing content protections")
	}
	replay := httptest.NewRecorder()
	server.Router().ServeHTTP(replay, httptest.NewRequest(http.MethodPost, "/api/v1/workspace/file-preview/read", bytes.NewReader(body)))
	if replay.Body.Len() != 0 || replay.Result().Trailer.Get("X-Kandev-Preview-Result") == "complete" {
		t.Fatal("token replay delivered bytes")
	}
}

func TestFilePreviewHTTPTooLarge(t *testing.T) {
	server, manager, root := newWorkspacePreviewServer(t)
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	f, err := os.Create(filepath.Join(root, "large.stl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate((64 << 20) + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/workspace/file-preview/prepare", bytes.NewBufferString(`{"path":"large.stl"}`)))
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

type failedPreviewWriter struct{ *httptest.ResponseRecorder }

func (w failedPreviewWriter) Write(_ []byte) (int, error) { return 0, errors.New("disconnected") }

func TestFilePreviewHTTPFailedMetadataWritesReleasePool(t *testing.T) {
	server, manager, root := newWorkspacePreviewServer(t)
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	if err := os.WriteFile(filepath.Join(root, "model.stl"), []byte("mesh"), 0600); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		writer := failedPreviewWriter{httptest.NewRecorder()}
		server.Router().ServeHTTP(writer, httptest.NewRequest(http.MethodPost, "/api/v1/workspace/file-preview/prepare", bytes.NewBufferString(`{"path":"model.stl"}`)))
	}
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/workspace/file-preview/prepare", bytes.NewBufferString(`{"path":"model.stl"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("orphaned metadata filled the pool: %d %s", response.Code, response.Body.String())
	}
}
