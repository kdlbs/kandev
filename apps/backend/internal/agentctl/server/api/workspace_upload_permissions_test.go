package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.7
func TestHandleFileUpload_ReplacementPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX script/mode assertions; native Windows tracker coverage is separate")
	}
	t.Run("registered route retains executable script", func(t *testing.T) {
		server, dir := newPermissionUploadServer(t)
		path := filepath.Join(dir, "run-task.sh")
		initial := "#!/bin/sh\nprintf 'route original\\n'\n"
		incoming := "#!/bin/sh\nprintf 'route replacement\\n'\n"
		seedRoutePermissionFile(t, path, initial, 0o755)
		assertRoutePermissionScript(t, path, "route original\n")
		rec := postUpload(t, server, permissionUploadForm("run-task.sh", "replace", incoming))
		assertPermissionUploadResponse(t, rec, "run-task.sh", incoming, "replace")
		assertRoutePermissionFile(t, path, incoming, 0o755)
		assertRoutePermissionScript(t, path, "route replacement\n")
		assertRoutePermissionNoTemps(t, dir)
	})
	t.Run("registered route retains restricted content", func(t *testing.T) {
		server, dir := newPermissionUploadServer(t)
		path := filepath.Join(dir, "secret.dat")
		seedRoutePermissionFile(t, path, "restricted original", 0o600)
		incoming := "restricted replacement\x00\xff"
		rec := postUpload(t, server, permissionUploadForm("secret.dat", "replace", incoming))
		assertPermissionUploadResponse(t, rec, "secret.dat", incoming, "replace")
		assertRoutePermissionFile(t, path, incoming, 0o600)
		assertRoutePermissionNoTemps(t, dir)
	})
}

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.8
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.9
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.6
func TestHandleFileUpload_ReplacementCompatibility(t *testing.T) {
	t.Run("new and keep both use creation defaults", func(t *testing.T) {
		server, dir := newPermissionUploadServer(t)
		rec := postUpload(t, server, permissionUploadForm("mode-control.dat", "", "creation control"))
		assertPermissionUploadResponse(t, rec, "mode-control.dat", "creation control", "")
		mode := routePermissionMode(t, filepath.Join(dir, "mode-control.dat"))
		path := filepath.Join(dir, "original.dat")
		seedRoutePermissionFile(t, path, "retained private bytes", 0o600)
		rec = postUpload(t, server, permissionUploadForm("original.dat", "keep_both", "separate bytes"))
		assertPermissionUploadResponse(t, rec, "original-1.dat", "separate bytes", "keep_both")
		assertRoutePermissionFile(t, path, "retained private bytes", 0o600)
		assertRoutePermissionFile(t, filepath.Join(dir, "original-1.dat"), "separate bytes", mode)
		rec = postUpload(t, server, permissionUploadForm("missing-replace.dat", "replace", "new destination"))
		assertPermissionUploadResponse(t, rec, "missing-replace.dat", "new destination", "replace")
		assertRoutePermissionFile(t, filepath.Join(dir, "missing-replace.dat"), "new destination", mode)
		assertRoutePermissionNoTemps(t, dir)
	})
	for _, tc := range []struct {
		name   string
		status int
	}{
		{name: "unresolved conflict", status: http.StatusConflict},
		{name: "short replacement", status: http.StatusBadRequest},
		{name: "long replacement", status: http.StatusBadRequest},
		{name: "unauthenticated replacement", status: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, dir := newPermissionUploadServer(t)
			path := filepath.Join(dir, "unchanged.dat")
			seedRoutePermissionFile(t, path, "untouched original", 0o600)
			form := permissionUploadForm("unchanged.dat", "replace", "incoming bytes")
			switch tc.name {
			case "unresolved conflict":
				delete(form.fields, "resolution")
			case "short replacement":
				form.fields["size_bytes"] = strconv.Itoa(len(form.content) + 1)
			case "long replacement":
				form.fields["size_bytes"] = strconv.Itoa(len(form.content) - 1)
			}
			var rec *httptest.ResponseRecorder
			if tc.name == "unauthenticated replacement" {
				body, contentType := form.build(t)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/file/upload", body)
				req.Header.Set("Content-Type", contentType)
				rec = httptest.NewRecorder()
				server.Router().ServeHTTP(rec, req)
			} else {
				rec = postUpload(t, server, form)
			}
			if rec.Code != tc.status {
				t.Fatalf("status=%d, want=%d, body=%s", rec.Code, tc.status, rec.Body.String())
			}
			assertRoutePermissionFile(t, path, "untouched original", 0o600)
			assertRoutePermissionNoTemps(t, dir)
		})
	}
}

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-003.1
func TestHandleFileUpload_ReplacementContainment(t *testing.T) {
	for _, kind := range []string{"file symlink", "directory symlink", "parent traversal"} {
		t.Run(kind, func(t *testing.T) {
			server, dir := newPermissionUploadServer(t)
			outside := t.TempDir()
			path := filepath.Join(outside, "outside.dat")
			seedRoutePermissionFile(t, path, "outside original", 0o600)
			relativePath := "alias"
			switch kind {
			case "file symlink":
				if err := os.Symlink(path, filepath.Join(dir, "alias")); err != nil {
					t.Skipf("native file symlink unavailable: %v", err)
				}
			case "directory symlink":
				if err := os.Symlink(outside, filepath.Join(dir, "alias")); err != nil {
					t.Skipf("native directory symlink unavailable: %v", err)
				}
				relativePath = "alias/outside.dat"
			case "parent traversal":
				var err error
				relativePath, err = filepath.Rel(dir, path)
				if err != nil {
					t.Fatal(err)
				}
				relativePath = filepath.ToSlash(relativePath)
			}
			rec := postUpload(t, server, permissionUploadForm(relativePath, "replace", "escape bytes"))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d, want=400, body=%s", rec.Code, rec.Body.String())
			}
			assertRoutePermissionFile(t, path, "outside original", 0o600)
			assertRoutePermissionNoTemps(t, dir)
			assertRoutePermissionNoTemps(t, outside)
		})
	}
}

func newPermissionUploadServer(t *testing.T) (*Server, string) {
	t.Helper()
	server, dir := newUploadTestServer(t)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.procMgr.StopForTeardown(ctx); err != nil {
			t.Errorf("stop owned upload manager: %v", err)
		}
	})
	return server, dir
}

func permissionUploadForm(path, resolution, content string) uploadForm {
	fields := map[string]string{
		"relative_path": path,
		"size_bytes":    strconv.Itoa(len(content)),
	}
	if resolution != "" {
		fields["resolution"] = resolution
	}
	return uploadForm{fields: fields, name: filepath.Base(path), content: []byte(content)}
}

func assertPermissionUploadResponse(t *testing.T, rec *httptest.ResponseRecorder, path, content, resolution string) {
	t.Helper()
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d, want=201, body=%s", rec.Code, rec.Body.String())
	}
	var response workspaceUploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Path != path || response.SizeBytes != int64(len(content)) || response.ResolutionApplied != resolution {
		t.Fatalf("response=%+v, want path=%q size=%d resolution=%q", response, path, len(content), resolution)
	}
}

func seedRoutePermissionFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0o600); err != nil && !os.IsNotExist(err) {
			t.Errorf("restore owned route fixture: %v", err)
		}
	})
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	assertRoutePermissionFile(t, path, content, mode)
}

func routePermissionMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func assertRoutePermissionFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	actual := routePermissionMode(t, path)
	if runtime.GOOS == "windows" {
		actual &= 0o200
		mode &= 0o200
	}
	if actual != mode {
		t.Errorf("route destination mode=%04o, want=%04o", actual, mode)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != content {
		t.Fatalf("route destination bytes=%q, want=%q, err=%v", data, content, err)
	}
}

func assertRoutePermissionScript(t *testing.T, path, expected string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path)
	cmd.WaitDelay = time.Second
	output, err := cmd.CombinedOutput()
	if err != nil || string(output) != expected {
		t.Fatalf("registered-upload script output=%q, want=%q, err=%v", output, expected, err)
	}
}

func assertRoutePermissionNoTemps(t *testing.T, dir string) {
	t.Helper()
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(entry.Name(), ".kandev-upload-") {
			t.Errorf("route left upload staging artifact: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
