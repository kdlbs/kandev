package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestUploadWorkspaceFile_StreamsMultipartVerbatim(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusCreated,
		`{"path":"fixtures/sample-1.json","size_bytes":7,"resolution_applied":"keep_both"}`))

	content := "payload"
	result, err := newHTTPOnlyClient(srv.URL).UploadWorkspaceFile(context.Background(), WorkspaceUploadParams{
		Dir:          "fixtures",
		Repo:         "",
		RelativePath: "sample.json",
		Resolution:   "keep_both",
		SizeBytes:    int64(len(content))}, strings.NewReader(content))
	if err != nil {
		t.Fatalf("UploadWorkspaceFile: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/workspace/file/upload" {
		t.Errorf("request = %s %s, want POST /api/v1/workspace/file/upload", got.Method, got.Path)
	}

	fields, fileName, fileBytes := parseMaterializeUpload(t, got)
	if fields["dir"] != "fixtures" {
		t.Errorf("dir = %q, want fixtures", fields["dir"])
	}
	if fields["relative_path"] != "sample.json" {
		t.Errorf("relative_path = %q, want sample.json", fields["relative_path"])
	}
	if fields["resolution"] != "keep_both" {
		t.Errorf("resolution = %q, want keep_both", fields["resolution"])
	}
	if fields["size_bytes"] != "7" {
		t.Errorf("size_bytes = %q, want 7", fields["size_bytes"])
	}
	if fileName != "sample.json" {
		t.Errorf("file part name = %q, want sample.json", fileName)
	}
	if string(fileBytes) != content {
		t.Errorf("file bytes = %q, want %q streamed verbatim", fileBytes, content)
	}

	// The server-chosen path is authoritative after a keep-both rename.
	if result.Path != "fixtures/sample-1.json" {
		t.Errorf("Path = %q, want the server-chosen fixtures/sample-1.json", result.Path)
	}
	if result.ResolutionApplied != "keep_both" {
		t.Errorf("ResolutionApplied = %q, want keep_both", result.ResolutionApplied)
	}
}

func TestUploadWorkspaceFile_ConflictIsASentinel(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusConflict,
		`{"error":"upload destination already exists: taken.txt"}`))

	_, err := newHTTPOnlyClient(srv.URL).UploadWorkspaceFile(context.Background(), WorkspaceUploadParams{
		RelativePath: "taken.txt",
		SizeBytes:    1}, strings.NewReader("x"))

	if !errors.Is(err, ErrWorkspaceUploadConflict) {
		t.Fatalf("err = %v, want ErrWorkspaceUploadConflict", err)
	}
	if !strings.Contains(err.Error(), "taken.txt") {
		t.Errorf("err = %v, want the server message preserved", err)
	}
}

func TestUploadWorkspaceFile_SurfacesServerError(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusBadRequest,
		`{"error":"path traversal detected in relative_path"}`))

	_, err := newHTTPOnlyClient(srv.URL).UploadWorkspaceFile(context.Background(), WorkspaceUploadParams{
		RelativePath: "../escaped.txt",
		SizeBytes:    1}, strings.NewReader("x"))

	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, ErrWorkspaceUploadConflict) {
		t.Error("a 400 must not be reported as a conflict")
	}
	if !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("err = %v, want the server message surfaced", err)
	}
}

func TestUploadWorkspaceFile_RejectsInvalidArgumentsBeforeAnyRequest(t *testing.T) {
	tests := []struct {
		name    string
		upload  WorkspaceUploadParams
		content io.Reader
		wantErr string
	}{
		{
			name:   "blank path",
			upload: WorkspaceUploadParams{RelativePath: "   ", SizeBytes: 1}, content: strings.NewReader("x"),
			wantErr: "path and content are required",
		},
		{
			name:    "nil content",
			upload:  WorkspaceUploadParams{RelativePath: "a.txt", SizeBytes: 1},
			wantErr: "path and content are required",
		},
		{
			name:   "negative size",
			upload: WorkspaceUploadParams{RelativePath: "a.txt", SizeBytes: -1}, content: strings.NewReader("x"),
			wantErr: "size exceeds maximum",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := captureServer(t, jsonResponder(http.StatusCreated, `{}`))
			_, err := newHTTPOnlyClient(srv.URL).UploadWorkspaceFile(context.Background(), tc.upload, tc.content)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
			}
			if got.Method != "" {
				t.Error("an invalid argument must not reach the network")
			}
		})
	}
}

func TestPreflightWorkspaceUpload(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK,
		`{"conflicts":[{"path":"fixtures/there.txt","is_dir":false}]}`))

	conflicts, err := newHTTPOnlyClient(srv.URL).PreflightWorkspaceUpload(
		context.Background(), "fixtures", "", []string{"there.txt", "missing.txt"},
	)
	if err != nil {
		t.Fatalf("PreflightWorkspaceUpload: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/workspace/file/upload-preflight" {
		t.Errorf("request = %s %s, want POST /api/v1/workspace/file/upload-preflight", got.Method, got.Path)
	}
	body := string(got.Body)
	for _, want := range []string{`"dir":"fixtures"`, `"there.txt"`, `"missing.txt"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body %s missing %s", body, want)
		}
	}
	if len(conflicts) != 1 || conflicts[0].Path != "fixtures/there.txt" {
		t.Fatalf("conflicts = %+v, want the single reported conflict", conflicts)
	}
}

func TestPreflightWorkspaceUpload_SurfacesServerError(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusBadRequest, `{"error":"path outside workspace"}`))

	_, err := newHTTPOnlyClient(srv.URL).PreflightWorkspaceUpload(
		context.Background(), "", "", []string{"../escaped.txt"},
	)
	if err == nil || !strings.Contains(err.Error(), "path outside workspace") {
		t.Fatalf("err = %v, want the server message surfaced", err)
	}
}
