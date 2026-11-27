package api

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func TestInstallMaterializedAttachment_LinkUnsupported(t *testing.T) {
	for _, linkErr := range []error{syscall.EPERM, syscall.ENOTSUP, syscall.EXDEV} {
		t.Run(linkErr.Error(), func(t *testing.T) {
			original := linkMaterializedAttachment
			t.Cleanup(func() { linkMaterializedAttachment = original })
			linkMaterializedAttachment = func(_, _ string) error { return linkErr }
			dir := t.TempDir()
			source := filepath.Join(dir, ".upload-test")
			if err := os.WriteFile(source, []byte("complete attachment"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "image.png"), []byte("existing"), 0o600); err != nil {
				t.Fatal(err)
			}
			name, err := installMaterializedAttachment(dir, source, "image.png")
			if err != nil {
				t.Fatalf("install without hard links: %v", err)
			}
			if name != "image-2.png" {
				t.Fatalf("name = %q", name)
			}
			for name, want := range map[string]string{"image.png": "existing", "image-2.png": "complete attachment"} {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || string(data) != want {
					t.Fatalf("%s = %q, %v", name, data, err)
				}
			}
			info, err := os.Stat(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("permissions = %o", info.Mode().Perm())
			}
			if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("temporary source remains: %v", err)
			}
		})
	}
}

func forceUnsupportedAttachmentLink(t *testing.T) {
	t.Helper()
	original := linkMaterializedAttachment
	t.Cleanup(func() { linkMaterializedAttachment = original })
	linkMaterializedAttachment = func(_, _ string) error { return syscall.EPERM }
}

func TestCopyMaterializedAttachment_CleansFailedCopy(t *testing.T) {
	// Opening a directory succeeds, but reading its bytes fails.
	dir := t.TempDir()
	destination := filepath.Join(t.TempDir(), "attachment.txt")
	if err := copyMaterializedAttachment(dir, destination); err == nil {
		t.Fatal("copying an unreadable source succeeded")
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial destination remains: %v", err)
	}
}

func TestInstallMaterializedAttachment_FallbackPreservesSymlink(t *testing.T) {
	forceUnsupportedAttachmentLink(t)
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "target.txt")
	if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "same.txt")
	if err := os.Symlink(target, destination); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	source := filepath.Join(dir, ".upload-test")
	if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	name, err := installMaterializedAttachment(dir, source, "same.txt")
	if err != nil || name != "same-2.txt" {
		t.Fatalf("install = %q, %v", name, err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "untouched" {
		t.Fatalf("symlink target = %q, %v", data, err)
	}
	if got, err := os.Readlink(destination); err != nil || got != target {
		t.Fatalf("symlink = %q, %v", got, err)
	}
}

func TestInstallMaterializedAttachment_MissingSource(t *testing.T) {
	forceUnsupportedAttachmentLink(t)
	dir := t.TempDir()
	name, err := installMaterializedAttachment(dir, filepath.Join(dir, "absent"), "file.txt")
	if name != "" || !errors.Is(err, os.ErrNotExist) || !errors.Is(err, syscall.EPERM) {
		t.Fatalf("install = %q, %v", name, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("destination entries = %v, %v", entries, err)
	}
}

type attachmentFaultWriter struct {
	writeErr error
	closeErr error
	short    bool
	closed   bool
}

func (w *attachmentFaultWriter) Write(p []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if w.short {
		return len(p) - 1, nil
	}
	return len(p), nil
}

func (w *attachmentFaultWriter) Close() error { w.closed = true; return w.closeErr }

func TestWriteMaterializedAttachment_RejectsIncompleteCopies(t *testing.T) {
	writeErr := errors.New("disk full")
	closeErr := errors.New("flush failed")
	for _, tc := range []struct {
		name   string
		writer attachmentFaultWriter
		size   int64
		want   error
	}{
		{name: "write", writer: attachmentFaultWriter{writeErr: writeErr}, size: 4, want: writeErr},
		{name: "close", writer: attachmentFaultWriter{closeErr: closeErr}, size: 4, want: closeErr},
		{name: "short write", writer: attachmentFaultWriter{short: true}, size: 4, want: io.ErrShortWrite},
		{name: "short source", size: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := writeMaterializedAttachment(&tc.writer, strings.NewReader("data"), tc.size)
			if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("copy error = %v, want %v", err, tc.want)
			}
			if !tc.writer.closed {
				t.Fatal("failed copy left destination open")
			}
		})
	}
}

func TestMaterializeAttachment_FailedFallbackCleansAndLogs(t *testing.T) {
	forceUnsupportedAttachmentLink(t)
	linkMaterializedAttachment = func(source, _ string) error {
		// Turn the staged file into an unreadable source after upload validation.
		if err := os.Remove(source); err != nil {
			return err
		}
		if err := os.Mkdir(source, 0o700); err != nil {
			return err
		}
		return syscall.EPERM
	}
	workDir := t.TempDir()
	log, observed := newObservedGitAPILogger(t)
	cfg := &config.InstanceConfig{Port: 0, WorkDir: workDir, AuthToken: "test-token"}
	server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{
		"session_id": "session", "attachment_id": "attachment", "name": "image.png",
		"mime_type": "image/png", "size_bytes": "7",
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", "image.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("private")); err != nil {
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
	if response.Code != http.StatusInternalServerError || strings.TrimSpace(response.Body.String()) != `{"error":"attachment storage unavailable"}` {
		t.Fatalf("response = %d, %s", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(filepath.Join(workDir, ".kandev", "attachments", "session"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed upload left files: %v, %v", entries, err)
	}
	logs := observed.FilterMessage("install agent attachment").All()
	if len(logs) != 1 {
		t.Fatalf("storage error logs = %d", len(logs))
	}
	cause, ok := logs[0].ContextMap()["error"].(string)
	if !ok || !strings.Contains(cause, syscall.EPERM.Error()) || strings.Contains(cause, "private") {
		t.Fatalf("logged cause = %q", cause)
	}
}
