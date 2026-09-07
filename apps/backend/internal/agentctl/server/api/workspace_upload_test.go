package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func newUploadTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	workDir := t.TempDir()
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: workDir, AuthToken: "test-token"}
	return NewServer(cfg, process.NewManager(cfg, log), nil, nil, log), workDir
}

type uploadForm struct {
	fields  map[string]string
	name    string
	content []byte
}

func (f uploadForm) build(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, v := range f.fields {
		if err := writer.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", f.name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(f.content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}

func postUpload(t *testing.T, server *Server, f uploadForm) *httptest.ResponseRecorder {
	t.Helper()
	body, contentType := f.build(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/file/upload", body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)
	return rec
}

func TestHandleFileUpload_WritesFile(t *testing.T) {
	server, workDir := newUploadTestServer(t)
	content := []byte("uploaded bytes")

	rec := postUpload(t, server, uploadForm{
		fields: map[string]string{
			"dir":           "fixtures",
			"relative_path": "sample.txt",
			"size_bytes":    strconv.Itoa(len(content)),
		},
		name:    "sample.txt",
		content: content,
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Path      string `json:"path"`
		SizeBytes int64  `json:"size_bytes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Path != "fixtures/sample.txt" {
		t.Errorf("path = %q, want fixtures/sample.txt", resp.Path)
	}
	if resp.SizeBytes != int64(len(content)) {
		t.Errorf("size = %d, want %d", resp.SizeBytes, len(content))
	}
	got, err := os.ReadFile(filepath.Join(workDir, "fixtures", "sample.txt"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestHandleFileUpload_NestedRelativePath(t *testing.T) {
	server, workDir := newUploadTestServer(t)

	rec := postUpload(t, server, uploadForm{
		fields: map[string]string{
			"dir":           "",
			"relative_path": "tree/deep/leaf.txt",
			"size_bytes":    "4",
		},
		name:    "leaf.txt",
		content: []byte("leaf"),
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(workDir, "tree", "deep", "leaf.txt")); err != nil {
		t.Fatalf("nested file missing: %v", err)
	}
}

func TestHandleFileUpload_ConflictWithoutResolution(t *testing.T) {
	server, workDir := newUploadTestServer(t)
	if err := os.WriteFile(filepath.Join(workDir, "taken.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := postUpload(t, server, uploadForm{
		fields: map[string]string{
			"relative_path": "taken.txt",
			"size_bytes":    "8",
		},
		name:    "taken.txt",
		content: []byte("incoming"),
	})

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	got, _ := os.ReadFile(filepath.Join(workDir, "taken.txt"))
	if string(got) != "original" {
		t.Errorf("existing file changed: %q", got)
	}
}

func TestHandleFileUpload_KeepBothReportsRenamedPath(t *testing.T) {
	server, workDir := newUploadTestServer(t)
	if err := os.WriteFile(filepath.Join(workDir, "taken.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := postUpload(t, server, uploadForm{
		fields: map[string]string{
			"relative_path": "taken.txt",
			"resolution":    "keep_both",
			"size_bytes":    "8",
		},
		name:    "taken.txt",
		content: []byte("incoming"),
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Path              string `json:"path"`
		ResolutionApplied string `json:"resolution_applied"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Path != "taken-1.txt" {
		t.Errorf("path = %q, want taken-1.txt", resp.Path)
	}
	if resp.ResolutionApplied != "keep_both" {
		t.Errorf("resolution_applied = %q, want keep_both", resp.ResolutionApplied)
	}
}

func TestHandleFileUpload_ContainmentRejected(t *testing.T) {
	server, workDir := newUploadTestServer(t)

	rec := postUpload(t, server, uploadForm{
		fields: map[string]string{
			"relative_path": "../escaped.txt",
			"size_bytes":    "3",
		},
		name:    "escaped.txt",
		content: []byte("bad"),
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(workDir), "escaped.txt")); err == nil {
		t.Fatal("wrote outside the workspace")
	}
}

func TestHandleFileUpload_SizeMismatchRejected(t *testing.T) {
	server, workDir := newUploadTestServer(t)

	rec := postUpload(t, server, uploadForm{
		fields: map[string]string{
			"relative_path": "mismatch.txt",
			"size_bytes":    "999",
		},
		name:    "mismatch.txt",
		content: []byte("short"),
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(workDir, "mismatch.txt")); err == nil {
		t.Error("a file was left behind after a size mismatch")
	}
}

func TestHandleFileUpload_OversizeRejected(t *testing.T) {
	server, _ := newUploadTestServer(t)

	rec := postUpload(t, server, uploadForm{
		fields: map[string]string{
			"relative_path": "big.bin",
			"size_bytes":    strconv.FormatInt(maxWorkspaceUploadBytes+1, 10),
		},
		name:    "big.bin",
		content: []byte("small body, oversized declaration"),
	})

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleFileUpload_UnknownResolutionRejected(t *testing.T) {
	server, _ := newUploadTestServer(t)

	rec := postUpload(t, server, uploadForm{
		fields: map[string]string{
			"relative_path": "x.txt",
			"resolution":    "obliterate",
			"size_bytes":    "1",
		},
		name:    "x.txt",
		content: []byte("x"),
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUploadPreflight(t *testing.T) {
	server, workDir := newUploadTestServer(t)
	if err := os.MkdirAll(filepath.Join(workDir, "fixtures"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "fixtures", "there.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := `{"dir":"fixtures","paths":["there.txt","missing.txt"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/file/upload-preflight", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Conflicts []struct {
			Path  string `json:"path"`
			IsDir bool   `json:"is_dir"`
		} `json:"conflicts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v, want exactly one", resp.Conflicts)
	}
	if resp.Conflicts[0].Path != "fixtures/there.txt" {
		t.Errorf("conflict path = %q, want fixtures/there.txt", resp.Conflicts[0].Path)
	}
}

func TestHandleUploadPreflight_ContainmentIsAnError(t *testing.T) {
	server, _ := newUploadTestServer(t)

	body := `{"dir":"","paths":["../escaped.txt"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/file/upload-preflight", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}
