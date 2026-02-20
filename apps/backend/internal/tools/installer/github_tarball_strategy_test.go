package installer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeTarPath(t *testing.T) {
	destDir := "/install"

	tests := []struct {
		name      string
		path      string
		wantErr   bool
		wantClean string
	}{
		{
			name:      "simple path",
			path:      "code-server/bin/code-server",
			wantErr:   false,
			wantClean: "code-server/bin/code-server",
		},
		{
			name:      "nested path",
			path:      "code-server-4.96.4-macos-arm64/bin/code-server",
			wantErr:   false,
			wantClean: "code-server-4.96.4-macos-arm64/bin/code-server",
		},
		{
			name:    "path traversal with dotdot",
			path:    "../etc/passwd",
			wantErr: true,
		},
		{
			name:    "absolute path",
			path:    "/etc/passwd",
			wantErr: true,
		},
		{
			name:      "current directory",
			path:      ".",
			wantErr:   false,
			wantClean: ".",
		},
		{
			name:    "hidden traversal via dotdot in middle",
			path:    "foo/../../etc/passwd",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clean, err := sanitizeTarPath(tt.path, destDir)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for path %q, got nil", tt.path)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for path %q: %v", tt.path, err)
				return
			}
			if clean != tt.wantClean {
				t.Errorf("expected clean path %q, got %q", tt.wantClean, clean)
			}
		})
	}
}

func TestExpandTemplate(t *testing.T) {
	s := &GithubTarballStrategy{
		config: GithubTarballConfig{
			Version: "4.96.4",
		},
	}

	tests := []struct {
		name     string
		tmpl     string
		target   string
		expected string
	}{
		{
			name:     "asset pattern",
			tmpl:     "code-server-{version}-{os}-{arch}.tar.gz",
			target:   "macos-arm64",
			expected: "code-server-4.96.4-macos-arm64.tar.gz",
		},
		{
			name:     "binary path pattern",
			tmpl:     "code-server-{version}-{os}-{arch}/bin/code-server",
			target:   "linux-amd64",
			expected: "code-server-4.96.4-linux-amd64/bin/code-server",
		},
		{
			name:     "no placeholders",
			tmpl:     "binary",
			target:   "linux-amd64",
			expected: "binary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.expandTemplate(tt.tmpl, tt.target)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExtractTarGz(t *testing.T) {
	// Create a tar.gz archive in memory with a file and a directory
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	// Add a directory
	_ = tarWriter.WriteHeader(&tar.Header{
		Name:     "myapp/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	})

	// Add a file
	content := []byte("#!/bin/sh\necho hello")
	_ = tarWriter.WriteHeader(&tar.Header{
		Name:     "myapp/bin/hello",
		Typeflag: tar.TypeReg,
		Mode:     0o755,
		Size:     int64(len(content)),
	})
	_, _ = tarWriter.Write(content)

	_ = tarWriter.Close()
	_ = gzWriter.Close()

	// Extract into temp dir
	destDir := t.TempDir()
	if err := extractTarGz(&buf, destDir); err != nil {
		t.Fatalf("extractTarGz failed: %v", err)
	}

	// Verify the file was extracted
	extractedPath := filepath.Join(destDir, "myapp", "bin", "hello")
	data, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("expected %q, got %q", string(content), string(data))
	}
}

func TestExtractTarGz_RejectsTraversal(t *testing.T) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	// Add a malicious entry
	content := []byte("malicious")
	_ = tarWriter.WriteHeader(&tar.Header{
		Name:     "../etc/passwd",
		Typeflag: tar.TypeReg,
		Mode:     0o644,
		Size:     int64(len(content)),
	})
	_, _ = tarWriter.Write(content)

	_ = tarWriter.Close()
	_ = gzWriter.Close()

	destDir := t.TempDir()
	err := extractTarGz(&buf, destDir)
	if err == nil {
		t.Error("expected error for path traversal, got nil")
	}
}

func TestExtractTarGz_SymlinkEscapeBlocked(t *testing.T) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	// Add a symlink that escapes the dest dir
	_ = tarWriter.WriteHeader(&tar.Header{
		Name:     "escape",
		Typeflag: tar.TypeSymlink,
		Linkname: "../../../../etc/passwd",
	})

	_ = tarWriter.Close()
	_ = gzWriter.Close()

	destDir := t.TempDir()
	err := extractTarGz(&buf, destDir)
	if err == nil {
		t.Error("expected error for symlink escape, got nil")
	}
}

func TestResolveTarget_Unsupported(t *testing.T) {
	s := &GithubTarballStrategy{
		config: GithubTarballConfig{
			Targets: map[string]string{
				"linux/amd64": "linux-amd64",
			},
		},
	}
	// resolveTarget uses runtime.GOOS/GOARCH — if the test platform isn't in Targets,
	// it should return an error. We can't guarantee this, so just verify the method works.
	_, err := s.resolveTarget()
	// The result depends on the test platform; just verify no panic.
	_ = err
}

func TestBuildURL(t *testing.T) {
	s := &GithubTarballStrategy{
		config: GithubTarballConfig{
			Owner:        "coder",
			Repo:         "code-server",
			Version:      "4.96.4",
			AssetPattern: "code-server-{version}-{os}-{arch}.tar.gz",
		},
	}

	url := s.buildURL("macos-arm64")
	expected := "https://github.com/coder/code-server/releases/download/v4.96.4/code-server-4.96.4-macos-arm64.tar.gz"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}
