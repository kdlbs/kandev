package handlers

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRepositoryCreateRequestJSONIncludesCopyFiles verifies that the
// copy_files field is wired through the JSON encoding/decoding for both the
// HTTP and WS create-repository request shapes. Failure here means the
// handler will silently drop a `copy_files` value sent by the client.
func TestRepositoryCreateRequestJSONIncludesCopyFiles(t *testing.T) {
	t.Run("http_marshal_contains_copy_files", func(t *testing.T) {
		req := httpCreateRepositoryRequest{Name: "r", CopyFiles: ".env"}
		b, err := json.Marshal(&req)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(b), `"copy_files":".env"`) {
			t.Errorf("missing copy_files in JSON: %s", b)
		}
	})

	t.Run("http_unmarshal_populates_copy_files", func(t *testing.T) {
		var req httpCreateRepositoryRequest
		if err := json.Unmarshal([]byte(`{"name":"r","copy_files":".env, *.local"}`), &req); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if req.CopyFiles != ".env, *.local" {
			t.Errorf("CopyFiles = %q, want %q", req.CopyFiles, ".env, *.local")
		}
	})

	t.Run("ws_unmarshal_populates_copy_files", func(t *testing.T) {
		var req wsCreateRepositoryRequest
		if err := json.Unmarshal([]byte(`{"workspace_id":"w","name":"r","copy_files":".env"}`), &req); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if req.CopyFiles != ".env" {
			t.Errorf("CopyFiles = %q, want %q", req.CopyFiles, ".env")
		}
	})
}

// TestRepositoryUpdateRequestJSONCopyFilesPointer verifies the pointer-style
// copy_files field on update requests round-trips correctly — nil when
// omitted, non-nil and dereferenceable to the supplied value when present.
func TestRepositoryUpdateRequestJSONCopyFilesPointer(t *testing.T) {
	t.Run("http_unmarshal_sets_pointer", func(t *testing.T) {
		var req httpUpdateRepositoryRequest
		if err := json.Unmarshal([]byte(`{"copy_files":".env, *.local"}`), &req); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if req.CopyFiles == nil {
			t.Fatal("CopyFiles is nil, want non-nil")
		}
		if *req.CopyFiles != ".env, *.local" {
			t.Errorf("*CopyFiles = %q, want %q", *req.CopyFiles, ".env, *.local")
		}
	})

	t.Run("http_omitted_leaves_nil", func(t *testing.T) {
		var req httpUpdateRepositoryRequest
		if err := json.Unmarshal([]byte(`{"name":"r"}`), &req); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if req.CopyFiles != nil {
			t.Errorf("CopyFiles = %v, want nil", req.CopyFiles)
		}
	})

	t.Run("ws_unmarshal_sets_pointer", func(t *testing.T) {
		var req wsUpdateRepositoryRequest
		if err := json.Unmarshal([]byte(`{"id":"x","copy_files":""}`), &req); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if req.CopyFiles == nil {
			t.Fatal("CopyFiles is nil, want non-nil pointer to empty string")
		}
		if *req.CopyFiles != "" {
			t.Errorf("*CopyFiles = %q, want empty string", *req.CopyFiles)
		}
	})
}
