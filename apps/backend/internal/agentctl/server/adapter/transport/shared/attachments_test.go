package shared

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

func testLogger() *zap.Logger {
	return zap.NewNop()
}

func TestNewAttachmentManager(t *testing.T) {
	mgr := NewAttachmentManager("/tmp/work", testLogger())
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if mgr.workDir != "/tmp/work" {
		t.Errorf("workDir = %q, want %q", mgr.workDir, "/tmp/work")
	}
	if mgr.sessionID != "" {
		t.Errorf("sessionID = %q, want empty", mgr.sessionID)
	}
}

func TestSetSessionID(t *testing.T) {
	mgr := NewAttachmentManager("/tmp/work", testLogger())
	mgr.SetSessionID("sess-123")
	if mgr.sessionID != "sess-123" {
		t.Errorf("sessionID = %q, want %q", mgr.sessionID, "sess-123")
	}
}

func TestSaveAttachments_EmptyList(t *testing.T) {
	mgr := NewAttachmentManager("/tmp/work", testLogger())
	mgr.SetSessionID("sess-1")

	saved, err := mgr.SaveAttachments(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved != nil {
		t.Errorf("expected nil, got %v", saved)
	}
}

func TestSaveAttachments_MissingSessionID(t *testing.T) {
	mgr := NewAttachmentManager("/tmp/work", testLogger())

	_, err := mgr.SaveAttachments([]v1.MessageAttachment{{Type: "image", Data: "abc"}})
	if err == nil {
		t.Fatal("expected error when sessionID is empty")
	}
}

func TestSaveAttachments_MissingWorkDir(t *testing.T) {
	mgr := NewAttachmentManager("", testLogger())
	mgr.SetSessionID("sess-1")

	_, err := mgr.SaveAttachments([]v1.MessageAttachment{{Type: "image", Data: "abc"}})
	if err == nil {
		t.Fatal("expected error when workDir is empty")
	}
}

func TestSaveAttachments_ImageAttachment(t *testing.T) {
	workDir := t.TempDir()
	mgr := NewAttachmentManager(workDir, testLogger())
	mgr.SetSessionID("sess-img")

	content := []byte{0x89, 0x50, 0x4E, 0x47} // PNG header bytes
	encoded := base64.StdEncoding.EncodeToString(content)

	saved, err := mgr.SaveAttachments([]v1.MessageAttachment{
		{Type: "image", Data: encoded, MimeType: "image/png", Name: "screenshot.png"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("expected 1 saved, got %d", len(saved))
	}

	s := saved[0]
	if s.Name != "screenshot.png" {
		t.Errorf("Name = %q, want %q", s.Name, "screenshot.png")
	}
	if s.Type != "image" {
		t.Errorf("Type = %q, want %q", s.Type, "image")
	}
	if s.MimeType != "image/png" {
		t.Errorf("MimeType = %q, want %q", s.MimeType, "image/png")
	}
	expectedRel := filepath.Join(".kandev", "attachments", "sess-img", "screenshot.png")
	if s.RelPath != expectedRel {
		t.Errorf("RelPath = %q, want %q", s.RelPath, expectedRel)
	}

	// Verify file was written
	data, err := os.ReadFile(s.AbsPath)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if len(data) != len(content) {
		t.Errorf("file size = %d, want %d", len(data), len(content))
	}
}

func TestSaveAttachments_ResourceAttachment(t *testing.T) {
	workDir := t.TempDir()
	mgr := NewAttachmentManager(workDir, testLogger())
	mgr.SetSessionID("sess-res")

	content := []byte("Hello, PDF content")
	encoded := base64.StdEncoding.EncodeToString(content)

	saved, err := mgr.SaveAttachments([]v1.MessageAttachment{
		{Type: "resource", Data: encoded, MimeType: "application/pdf", Name: "report.pdf"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("expected 1 saved, got %d", len(saved))
	}

	s := saved[0]
	if s.Name != "report.pdf" {
		t.Errorf("Name = %q, want %q", s.Name, "report.pdf")
	}
	if s.Type != "resource" {
		t.Errorf("Type = %q, want %q", s.Type, "resource")
	}

	data, err := os.ReadFile(s.AbsPath)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("file content = %q, want %q", string(data), string(content))
	}
}

func TestSaveAttachments_MultipleAttachments(t *testing.T) {
	workDir := t.TempDir()
	mgr := NewAttachmentManager(workDir, testLogger())
	mgr.SetSessionID("sess-multi")

	img := base64.StdEncoding.EncodeToString([]byte("image-data"))
	pdf := base64.StdEncoding.EncodeToString([]byte("pdf-data"))

	saved, err := mgr.SaveAttachments([]v1.MessageAttachment{
		{Type: "image", Data: img, MimeType: "image/jpeg", Name: "photo.jpg"},
		{Type: "resource", Data: pdf, MimeType: "application/pdf", Name: "doc.pdf"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(saved) != 2 {
		t.Fatalf("expected 2 saved, got %d", len(saved))
	}

	if saved[0].Name != "photo.jpg" {
		t.Errorf("first Name = %q, want %q", saved[0].Name, "photo.jpg")
	}
	if saved[1].Name != "doc.pdf" {
		t.Errorf("second Name = %q, want %q", saved[1].Name, "doc.pdf")
	}
}

func TestSaveAttachments_DuplicateNamesDoNotClobber(t *testing.T) {
	workDir := t.TempDir()
	mgr := NewAttachmentManager(workDir, testLogger())
	mgr.SetSessionID("sess-dupe")

	first := base64.StdEncoding.EncodeToString([]byte("first"))
	second := base64.StdEncoding.EncodeToString([]byte("second"))
	third := base64.StdEncoding.EncodeToString([]byte("third"))

	saved, err := mgr.SaveAttachments([]v1.MessageAttachment{
		{Type: "resource", Data: first, MimeType: "text/plain", Name: "report.txt"},
		{Type: "resource", Data: second, MimeType: "text/plain", Name: "report.txt"},
		{Type: "resource", Data: third, MimeType: "text/plain", Name: "report.txt"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(saved) != 3 {
		t.Fatalf("expected 3 saved, got %d", len(saved))
	}

	wantNames := []string{"report.txt", "report-2.txt", "report-3.txt"}
	wantContents := []string{"first", "second", "third"}
	for i := range saved {
		if saved[i].Name != wantNames[i] {
			t.Errorf("saved[%d].Name = %q, want %q", i, saved[i].Name, wantNames[i])
		}
		data, err := os.ReadFile(saved[i].AbsPath)
		if err != nil {
			t.Fatalf("failed to read saved[%d]: %v", i, err)
		}
		if string(data) != wantContents[i] {
			t.Errorf("saved[%d] content = %q, want %q", i, string(data), wantContents[i])
		}
	}
}

func TestSaveAttachments_DuplicateNamesAcrossCallsDoNotClobber(t *testing.T) {
	workDir := t.TempDir()
	mgr := NewAttachmentManager(workDir, testLogger())
	mgr.SetSessionID("sess-dupe-calls")

	first := base64.StdEncoding.EncodeToString([]byte("first"))
	second := base64.StdEncoding.EncodeToString([]byte("second"))

	firstSaved, err := mgr.SaveAttachments([]v1.MessageAttachment{
		{Type: "resource", Data: first, MimeType: "text/plain", Name: "report.txt"},
	})
	if err != nil {
		t.Fatalf("unexpected first save error: %v", err)
	}
	secondSaved, err := mgr.SaveAttachments([]v1.MessageAttachment{
		{Type: "resource", Data: second, MimeType: "text/plain", Name: "report.txt"},
	})
	if err != nil {
		t.Fatalf("unexpected second save error: %v", err)
	}

	if firstSaved[0].Name != "report.txt" {
		t.Fatalf("first name = %q, want report.txt", firstSaved[0].Name)
	}
	if secondSaved[0].Name != "report-2.txt" {
		t.Fatalf("second name = %q, want report-2.txt", secondSaved[0].Name)
	}

	firstData, err := os.ReadFile(firstSaved[0].AbsPath)
	if err != nil {
		t.Fatalf("failed to read first file: %v", err)
	}
	secondData, err := os.ReadFile(secondSaved[0].AbsPath)
	if err != nil {
		t.Fatalf("failed to read second file: %v", err)
	}
	if string(firstData) != "first" {
		t.Errorf("first content = %q, want first", string(firstData))
	}
	if string(secondData) != "second" {
		t.Errorf("second content = %q, want second", string(secondData))
	}
}

func TestSaveAttachments_ConcurrentDuplicateNamesDoNotClobber(t *testing.T) {
	workDir := t.TempDir()
	const workers = 20

	var wg sync.WaitGroup
	savedCh := make(chan SavedAttachment, workers)
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()

			mgr := NewAttachmentManager(workDir, testLogger())
			mgr.SetSessionID("sess-concurrent")

			content := fmt.Sprintf("payload-%02d", i)
			saved, err := mgr.SaveAttachments([]v1.MessageAttachment{
				{
					Type:     "resource",
					Data:     base64.StdEncoding.EncodeToString([]byte(content)),
					MimeType: "text/plain",
					Name:     "report.txt",
				},
			})
			if err != nil {
				errCh <- err
				return
			}
			if len(saved) != 1 {
				errCh <- fmt.Errorf("saved count = %d, want 1", len(saved))
				return
			}
			savedCh <- saved[0]
		}()
	}

	wg.Wait()
	close(savedCh)
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("unexpected save error: %v", err)
		}
	}

	names := make(map[string]bool, workers)
	contents := make(map[string]bool, workers)
	for saved := range savedCh {
		if names[saved.Name] {
			t.Fatalf("duplicate saved name %q", saved.Name)
		}
		names[saved.Name] = true

		data, err := os.ReadFile(saved.AbsPath)
		if err != nil {
			t.Fatalf("failed to read %s: %v", saved.AbsPath, err)
		}
		contents[string(data)] = true
	}
	if len(names) != workers {
		t.Fatalf("saved unique names = %d, want %d", len(names), workers)
	}
	if len(contents) != workers {
		t.Fatalf("saved unique contents = %d, want %d", len(contents), workers)
	}
}

func TestSaveAttachments_NoName_GeneratesFromMime(t *testing.T) {
	workDir := t.TempDir()
	mgr := NewAttachmentManager(workDir, testLogger())
	mgr.SetSessionID("sess-noname")

	encoded := base64.StdEncoding.EncodeToString([]byte("data"))

	saved, err := mgr.SaveAttachments([]v1.MessageAttachment{
		{Type: "image", Data: encoded, MimeType: "image/jpeg"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("expected 1 saved, got %d", len(saved))
	}
	if saved[0].Name != "attachment.jpg" {
		t.Errorf("Name = %q, want %q", saved[0].Name, "attachment.jpg")
	}
}

func TestSaveAttachments_InvalidBase64_Skipped(t *testing.T) {
	workDir := t.TempDir()
	mgr := NewAttachmentManager(workDir, testLogger())
	mgr.SetSessionID("sess-invalid")

	saved, err := mgr.SaveAttachments([]v1.MessageAttachment{
		{Type: "resource", Data: "not-valid-base64!!!", MimeType: "text/plain", Name: "bad.txt"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(saved) != 0 {
		t.Errorf("expected 0 saved (invalid base64 skipped), got %d", len(saved))
	}
}

func TestCleanup(t *testing.T) {
	workDir := t.TempDir()
	mgr := NewAttachmentManager(workDir, testLogger())
	mgr.SetSessionID("sess-cleanup")

	// Save an attachment
	encoded := base64.StdEncoding.EncodeToString([]byte("cleanup-test"))
	saved, err := mgr.SaveAttachments([]v1.MessageAttachment{
		{Type: "resource", Data: encoded, MimeType: "text/plain", Name: "temp.txt"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(saved[0].AbsPath); err != nil {
		t.Fatalf("file should exist before cleanup: %v", err)
	}

	// Cleanup
	mgr.Cleanup()

	// Verify directory is gone
	dir := filepath.Join(workDir, ".kandev", "attachments", "sess-cleanup")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("directory should not exist after cleanup, got err: %v", err)
	}
}

func TestCleanup_EmptySessionID_NoOp(t *testing.T) {
	mgr := NewAttachmentManager("/tmp/work", testLogger())
	// Should not panic
	mgr.Cleanup()
}

func TestCleanup_EmptyWorkDir_NoOp(t *testing.T) {
	mgr := NewAttachmentManager("", testLogger())
	mgr.SetSessionID("sess-1")
	// Should not panic
	mgr.Cleanup()
}

func TestCleanup_Idempotent(t *testing.T) {
	workDir := t.TempDir()
	mgr := NewAttachmentManager(workDir, testLogger())
	mgr.SetSessionID("sess-idem")

	encoded := base64.StdEncoding.EncodeToString([]byte("data"))
	_, _ = mgr.SaveAttachments([]v1.MessageAttachment{
		{Type: "resource", Data: encoded, MimeType: "text/plain", Name: "f.txt"},
	})

	// Cleanup twice — second call should not panic
	mgr.Cleanup()
	mgr.Cleanup()
}

func TestCleanup_SessionIsolation(t *testing.T) {
	workDir := t.TempDir()
	mgr1 := NewAttachmentManager(workDir, testLogger())
	mgr1.SetSessionID("sess-A")
	mgr2 := NewAttachmentManager(workDir, testLogger())
	mgr2.SetSessionID("sess-B")

	encoded := base64.StdEncoding.EncodeToString([]byte("data"))

	saved1, _ := mgr1.SaveAttachments([]v1.MessageAttachment{
		{Type: "resource", Data: encoded, MimeType: "text/plain", Name: "a.txt"},
	})
	saved2, _ := mgr2.SaveAttachments([]v1.MessageAttachment{
		{Type: "resource", Data: encoded, MimeType: "text/plain", Name: "b.txt"},
	})

	// Cleanup session A
	mgr1.Cleanup()

	// Session A file gone
	if _, err := os.Stat(saved1[0].AbsPath); !os.IsNotExist(err) {
		t.Error("session A file should be deleted after cleanup")
	}

	// Session B file still exists
	if _, err := os.Stat(saved2[0].AbsPath); err != nil {
		t.Errorf("session B file should still exist: %v", err)
	}

	// Cleanup session B
	mgr2.Cleanup()
	if _, err := os.Stat(saved2[0].AbsPath); !os.IsNotExist(err) {
		t.Error("session B file should be deleted after cleanup")
	}
}

func TestBuildAttachmentPrompt_Empty(t *testing.T) {
	result := BuildAttachmentPrompt(nil, true)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestBuildAttachmentPrompt_SingleWritableFile(t *testing.T) {
	saved := []SavedAttachment{
		{RelPath: ".kandev/attachments/s1/report.pdf", Name: "report.pdf"},
	}
	result := BuildAttachmentPrompt(saved, true)
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if !contains(result, "report.pdf") {
		t.Errorf("result should contain filename, got: %q", result)
	}
	if !contains(result, ".kandev/attachments/s1/report.pdf") {
		t.Errorf("result should contain path, got: %q", result)
	}
	if !contains(result, "writable") {
		t.Errorf("result should tell the agent the file is writable, got: %q", result)
	}
}

func TestBuildAttachmentPrompt_SingleReadOnlyFile(t *testing.T) {
	saved := []SavedAttachment{
		{RelPath: ".kandev/attachments/s1/report.pdf", Name: "report.pdf"},
	}
	result := BuildAttachmentPrompt(saved, false)
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if contains(result, "writable") {
		t.Errorf("read-only result should not call the file writable, got: %q", result)
	}
	if contains(result, "modify") {
		t.Errorf("read-only result should not tell the agent to modify the file, got: %q", result)
	}
	if !contains(result, "access it") {
		t.Errorf("read-only result should tell the agent to access the file, got: %q", result)
	}
}

func TestBuildAttachmentPrompt_MultipleFiles(t *testing.T) {
	saved := []SavedAttachment{
		{RelPath: ".kandev/attachments/s1/a.pdf", Name: "a.pdf"},
		{RelPath: ".kandev/attachments/s1/b.png", Name: "b.png"},
	}
	result := BuildAttachmentPrompt(saved, true)
	if !contains(result, "a.pdf") {
		t.Errorf("result should contain a.pdf, got: %q", result)
	}
	if !contains(result, "b.png") {
		t.Errorf("result should contain b.png, got: %q", result)
	}
}

func TestBuildAttachmentPrompt_SanitizesPromptValues(t *testing.T) {
	saved := []SavedAttachment{
		{
			RelPath: ".kandev/attachments/s1/<bad>\npath.pdf",
			Name:    "report<one>\nplease.pdf",
		},
	}
	result := BuildAttachmentPrompt(saved, true)
	if contains(result, "<") || contains(result, ">") || contains(result, "\nplease") {
		t.Errorf("result contains unsanitized prompt values: %q", result)
	}
	if !contains(result, "report(one) please.pdf") {
		t.Errorf("result should contain sanitized filename, got: %q", result)
	}
	if !contains(result, ".kandev/attachments/s1/(bad) path.pdf") {
		t.Errorf("result should contain sanitized path, got: %q", result)
	}
}

func TestExtensionFromMimeType(t *testing.T) {
	tests := []struct {
		mimeType string
		want     string
	}{
		{"image/png", ".png"},
		{"image/jpeg", ".jpg"},
		{"image/gif", ".gif"},
		{"image/webp", ".webp"},
		{"application/pdf", ".pdf"},
		{"text/plain", ".txt"},
		{"application/json", ".json"},
		{"text/csv", ".csv"},
		{"text/html", ".html"},
		{"text/markdown", ".md"},
		{"application/octet-stream", ""},
		{"video/mp4", ""},
	}

	for _, tt := range tests {
		got := extensionFromMimeType(tt.mimeType)
		if got != tt.want {
			t.Errorf("extensionFromMimeType(%q) = %q, want %q", tt.mimeType, got, tt.want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
