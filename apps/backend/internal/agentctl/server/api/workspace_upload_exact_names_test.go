package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-001.3
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-001.4
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-001.8
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-003.1
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.1
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.2
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.4
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.6
func TestWorkspaceUploadExactNames(t *testing.T) {
	for _, tc := range []struct{ name, dir, relative, expected string }{
		{"ordinary root", "", "ordinary.txt", "ordinary.txt"},
		{"spaced file", "", " report.txt ", " report.txt "},
		{"spaced destination", " folder ", "report.txt", " folder /report.txt"},
		{"whitespace destination", " ", "report.txt", " /report.txt"},
		{"nested names", " top ", " inner / leaf.txt ", " top / inner / leaf.txt "},
		{"whitespace component", "", "folder/ ", "folder/ "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probeExactUploadName(t, tc.expected)
			server, root := newPermissionUploadServer(t)
			assertExactPreflight(t, server, tc.dir, tc.relative, http.StatusOK, "")
			rec := postExactUpload(t, server, tc.dir, tc.relative, "", "new bytes")
			assertPermissionUploadResponse(t, rec, tc.expected, "new bytes", "")
			assertExactContent(t, root, tc.expected, "new bytes")
		})
	}
	t.Run("false and true conflict", testExactUploadConflicts)
	t.Run("late conflict", testExactUploadLateConflict)
	t.Run("invalid paths", testExactUploadInvalidPaths)
}

func testExactUploadConflicts(t *testing.T) {
	probeExactUploadName(t, " fixture / file.txt ")
	server, root := newPermissionUploadServer(t)
	seedExactUpload(t, root, "fixture/file.txt", "unspaced sentinel")
	assertExactPreflight(t, server, " fixture ", " file.txt ", http.StatusOK, "")
	rec := postExactUpload(t, server, " fixture ", " file.txt ", "", "first bytes")
	assertPermissionUploadResponse(t, rec, " fixture / file.txt ", "first bytes", "")
	assertExactContent(t, root, "fixture/file.txt", "unspaced sentinel")
	assertExactPreflight(t, server, " fixture ", " file.txt ", http.StatusOK, " fixture / file.txt ")
	rec = postExactUpload(t, server, " fixture ", " file.txt ", "", "refused")
	if rec.Code != http.StatusConflict {
		t.Fatalf("unresolved status = %d, body = %s", rec.Code, rec.Body.String())
	}
	assertExactContent(t, root, " fixture / file.txt ", "first bytes")
	rec = postExactUpload(t, server, " fixture ", " file.txt ", "replace", "replacement")
	assertPermissionUploadResponse(t, rec, " fixture / file.txt ", "replacement", "replace")
	assertExactContent(t, root, " fixture / file.txt ", "replacement")
	rec = postExactUpload(t, server, " fixture ", " file.txt ", "keep_both", "second bytes")
	assertPermissionUploadResponse(t, rec, " fixture / file-1.txt ", "second bytes", "keep_both")
	assertExactContent(t, root, " fixture / file.txt ", "replacement")
	assertExactContent(t, root, " fixture / file-1.txt ", "second bytes")
	assertExactContent(t, root, "fixture/file.txt", "unspaced sentinel")
}

func testExactUploadLateConflict(t *testing.T) {
	probeExactUploadName(t, " late.txt ")
	server, root := newPermissionUploadServer(t)
	assertExactPreflight(t, server, "", " late.txt ", http.StatusOK, "")
	seedExactUpload(t, root, " late.txt ", "late sentinel")
	rec := postExactUpload(t, server, "", " late.txt ", "", "refused")
	if rec.Code != http.StatusConflict {
		t.Fatalf("late conflict status = %d, body = %s", rec.Code, rec.Body.String())
	}
	assertExactContent(t, root, " late.txt ", "late sentinel")
}

func testExactUploadInvalidPaths(t *testing.T) {
	for _, tc := range []struct{ dir, relative string }{
		{"", ""}, {"", " \t "}, {"", "../escape"}, {"", "/escape"},
		{"../escape", "file"}, {"/escape", "file"},
	} {
		t.Run(tc.dir+"|"+tc.relative, func(t *testing.T) {
			server, _ := newPermissionUploadServer(t)
			assertExactPreflight(t, server, tc.dir, tc.relative, http.StatusBadRequest, "")
			rec := postExactUpload(t, server, tc.dir, tc.relative, "", "refused")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func probeExactUploadName(t *testing.T, name string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Skipf("native filesystem cannot create path components %q: %v", name, err)
	}
	if err := os.WriteFile(path, []byte("probe"), 0o600); err != nil {
		t.Skipf("native filesystem cannot create name %q: %v", name, err)
	}
	// Checking directory entries detects native name normalization, not merely lookup aliases.
	current := root
	for _, segment := range bytes.Split([]byte(name), []byte("/")) {
		entries, err := os.ReadDir(current)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, entry := range entries {
			found = found || entry.Name() == string(segment)
		}
		if !found {
			t.Skipf("native filesystem does not preserve component %q", segment)
		}
		current = filepath.Join(current, string(segment))
	}
}

func assertExactPreflight(t *testing.T, server *Server, dir, relative string, status int, conflict string) {
	t.Helper()
	payload, err := json.Marshal(uploadPreflightRequest{Dir: dir, Paths: []string{relative}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/file/upload-preflight", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)
	if rec.Code != status {
		t.Fatalf("preflight status = %d, want %d, body = %s", rec.Code, status, rec.Body.String())
	}
	if status != http.StatusOK {
		return
	}
	var response uploadPreflightResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if conflict == "" && len(response.Conflicts) == 0 {
		return
	}
	if len(response.Conflicts) != 1 || response.Conflicts[0].Path != conflict || response.Conflicts[0].IsDir {
		t.Fatalf("conflicts = %+v, want %q", response.Conflicts, conflict)
	}
}

func postExactUpload(t *testing.T, server *Server, dir, relative, resolution, content string) *httptest.ResponseRecorder {
	t.Helper()
	return postUpload(t, server, uploadForm{
		fields: map[string]string{"dir": dir, "relative_path": relative, "resolution": resolution, "size_bytes": strconv.Itoa(len(content))},
		name:   "upload.bin", content: []byte(content),
	})
}

func seedExactUpload(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertExactContent(t *testing.T, root, relative, content string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil || string(got) != content {
		t.Fatalf("content at %q = %q, error %v; want %q", relative, got, err, content)
	}
}

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-001.8
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.1
func TestWorkspaceUploadExactNamesLineBreaks(t *testing.T) {
	for _, field := range []string{"dir", "relative_path", "repo"} {
		for _, lineBreak := range []string{"\n", "\r", "\r\n"} {
			t.Run(field+strconv.Quote(lineBreak), func(t *testing.T) {
				server, root := newPermissionUploadServer(t)
				fields := map[string]string{"dir": "", "relative_path": "file.txt", "repo": "", "resolution": "replace", "size_bytes": "8"}
				fields[field] = "part" + lineBreak + "name"
				if field == "repo" {
					probeExactUploadName(t, fields[field]+"/probe.txt")
					if err := os.Mkdir(filepath.Join(root, fields[field]), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				request := uploadPreflightRequest{Dir: fields["dir"], Repo: fields["repo"], Paths: []string{fields["relative_path"]}}
				assertUploadLineBreakPreflight(t, server, request)
				rec := postUpload(t, server, uploadForm{fields: fields, name: "file.txt", content: []byte("incoming")})
				if rec.Code != http.StatusBadRequest {
					t.Errorf("upload status = %d, want 400: %s", rec.Code, rec.Body.String())
				}
				checkDir := root
				if field == "repo" {
					checkDir = filepath.Join(root, fields[field])
				}
				entries, err := os.ReadDir(checkDir)
				if err != nil || len(entries) != 0 {
					t.Errorf("destination entries = %v, error = %v; want empty", entries, err)
				}
			})
		}
	}
	for _, field := range []string{"dir", "repo"} {
		t.Run("empty-list-"+field, func(t *testing.T) {
			server, _ := newPermissionUploadServer(t)
			request := uploadPreflightRequest{Paths: []string{}}
			if field == "dir" {
				request.Dir = "dir\nname"
			} else {
				request.Repo = "repo\rname"
			}
			assertUploadLineBreakPreflight(t, server, request)
		})
	}
	t.Run("near-identical sentinels", testUploadLineBreakSentinels)
	t.Run("tab names", func(t *testing.T) {
		probeExactUploadName(t, "\tdir\t/\tfile.txt\t")
		server, root := newPermissionUploadServer(t)
		assertExactPreflight(t, server, "\tdir\t", "\tfile.txt\t", http.StatusOK, "")
		rec := postExactUpload(t, server, "\tdir\t", "\tfile.txt\t", "", "tab bytes")
		assertPermissionUploadResponse(t, rec, "\tdir\t/\tfile.txt\t", "tab bytes", "")
		assertExactContent(t, root, "\tdir\t/\tfile.txt\t", "tab bytes")
	})
}

func testUploadLineBreakSentinels(t *testing.T) {
	probeExactUploadName(t, "report\n.txt")
	probeExactUploadName(t, "report\r\n.txt")
	server, root := newPermissionUploadServer(t)
	seedExactUpload(t, root, "report\n.txt", "JSON target")
	seedExactUpload(t, root, "report\r\n.txt", "multipart target")
	assertUploadLineBreakPreflight(t, server, uploadPreflightRequest{Paths: []string{"report\n.txt"}})
	rec := postExactUpload(t, server, "", "report\r\n.txt", "replace", "incoming")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("serialized upload status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	assertExactContent(t, root, "report\n.txt", "JSON target")
	assertExactContent(t, root, "report\r\n.txt", "multipart target")
}

func assertUploadLineBreakPreflight(t *testing.T, server *Server, request uploadPreflightRequest) {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/file/upload-preflight", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("preflight status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}
