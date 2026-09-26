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
	"sync"
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

func TestInstallMaterializedAttachment_NeverOverwritesConcurrentNames(t *testing.T) {
	dir := t.TempDir()
	type result struct {
		name string
		err  error
		data []byte
	}
	results := make(chan result, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tmpPath := filepath.Join(dir, "upload-"+strconv.Itoa(i))
			content := []byte("content-" + strconv.Itoa(i))
			if err := os.WriteFile(tmpPath, content, 0o600); err != nil {
				results <- result{err: err}
				return
			}
			name, err := installMaterializedAttachment(dir, tmpPath, "same.txt")
			var got []byte
			if err == nil {
				got, err = os.ReadFile(filepath.Join(dir, name))
			}
			results <- result{name: name, err: err, data: got}
		}(i)
	}
	wg.Wait()
	close(results)
	seen := make(map[string]struct{})
	for got := range results {
		if got.err != nil {
			t.Fatal(got.err)
		}
		if _, exists := seen[got.name]; exists {
			t.Fatalf("duplicate destination name %q", got.name)
		}
		seen[got.name] = struct{}{}
		if len(got.data) == 0 {
			t.Fatalf("empty installed file for %q", got.name)
		}
	}
}
