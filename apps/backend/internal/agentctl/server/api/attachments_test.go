package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func TestMaterializeAttachment_StreamsIntoSessionDirectory(t *testing.T) {
	workDir := t.TempDir()
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: workDir, AuthToken: "test-token"}
	server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)

	content := []byte("diagnostic bytes")
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{
		"session_id":    "acp-session",
		"attachment_id": "attachment-1",
		"name":          "diagnostic.zip",
		"mime_type":     "application/zip",
		"size_bytes":    "16",
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", "diagnostic.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/attachments/materialize", &body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, req)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result materializedAttachmentResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Name != "diagnostic.zip" || result.SizeBytes != int64(len(content)) {
		t.Fatalf("response = %+v", result)
	}
	path := filepath.Join(workDir, ".kandev", "attachments", "acp-session", result.Name)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("materialized content = %q, want %q", got, content)
	}
}

func TestMaterializeAttachment_RejectsDescriptorSizeMismatch(t *testing.T) {
	workDir := t.TempDir()
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: workDir, AuthToken: "test-token"}
	server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{
		"session_id":    "acp-session",
		"attachment_id": "attachment-1",
		"name":          "diagnostic.zip",
		"mime_type":     "application/zip",
		"size_bytes":    "17",
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", "diagnostic.zip")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("diagnostic bytes"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/attachments/materialize", &body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, req)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestMaterializeAttachment_RejectsPathTraversalIdentity(t *testing.T) {
	workDir := t.TempDir()
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: workDir, AuthToken: "test-token"}
	server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{
		"session_id":    "../escape",
		"attachment_id": "attachment-1",
		"name":          "diagnostic.zip",
		"mime_type":     "application/zip",
		"size_bytes":    "0",
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", "diagnostic.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(nil); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/attachments/materialize", &body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
