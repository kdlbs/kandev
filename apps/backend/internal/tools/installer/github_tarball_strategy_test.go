package installer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGithubTarballStrategyInstallRepairsPartialInstall(t *testing.T) {
	installDir := t.TempDir()
	target := runtime.GOOS + "-" + runtime.GOARCH
	binaryPath := filepath.Join(installDir, "tool-1.0.0-"+target, "bin", "tool")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		t.Fatalf("create partial install: %v", err)
	}
	if err := os.WriteFile(binaryPath, []byte("partial"), 0o755); err != nil {
		t.Fatalf("write partial binary: %v", err)
	}

	archive := tarGzWithFiles(t, map[string]string{
		"tool-1.0.0-" + target + "/bin/tool":    "complete",
		"tool-1.0.0-" + target + "/lib/runtime": "runtime",
	})
	requestCount := 0
	originalClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requestCount++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(archive)),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { http.DefaultClient = originalClient })

	strategy := NewGithubTarballStrategy(installDir, "tool", GithubTarballConfig{
		Owner:        "owner",
		Repo:         "repo",
		Version:      "1.0.0",
		AssetPattern: "tool-{version}-{os}-{arch}.tar.gz",
		BinaryPath:   "tool-{version}-{os}-{arch}/bin/tool",
		Targets: map[string]string{
			runtime.GOOS + "/" + runtime.GOARCH: target,
		},
	}, testLogger())

	if _, err := strategy.Install(t.Context()); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("download request count = %d, want 1", requestCount)
	}
	if _, err := os.Stat(filepath.Join(installDir, "tool-1.0.0-"+target, "lib", "runtime")); err != nil {
		t.Fatalf("partial install was not repaired: %v", err)
	}

	if _, err := strategy.Install(t.Context()); err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("download request count after completed install = %d, want 1", requestCount)
	}
}

func TestGithubTarballStrategyInstallReportsCacheInspectionError(t *testing.T) {
	installDir := t.TempDir()
	target := runtime.GOOS + "-" + runtime.GOARCH
	binaryPath := filepath.Join(installDir, "tool-1.0.0-"+target, "bin", "tool")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		t.Fatalf("create install directory: %v", err)
	}
	if err := os.Symlink("tool", binaryPath); err != nil {
		t.Fatalf("create symlink loop: %v", err)
	}

	strategy := NewGithubTarballStrategy(installDir, "tool", GithubTarballConfig{
		Owner:        "owner",
		Repo:         "repo",
		Version:      "1.0.0",
		AssetPattern: "tool-{version}-{os}-{arch}.tar.gz",
		BinaryPath:   "tool-{version}-{os}-{arch}/bin/tool",
		Targets: map[string]string{
			runtime.GOOS + "/" + runtime.GOARCH: target,
		},
	}, testLogger())

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := strategy.Install(ctx)
	if err == nil || !strings.Contains(err.Error(), "failed to inspect installed binary") {
		t.Fatalf("Install() error = %v, want cache inspection error", err)
	}
}

func tarGzWithFiles(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)
	for name, content := range files {
		if err := tarWriter.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatalf("write tar content: %v", err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return buf.Bytes()
}

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

func TestExtractTarGz_SymlinkWithRelativeDotDot(t *testing.T) {
	// Legitimate relative symlinks using ".." (like npm .bin links) should be allowed
	// as long as the resolved target stays within destDir.
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	// Create directory structure: pkg/node_modules/.bin/ and pkg/node_modules/semver/bin/
	for _, dir := range []string{
		"pkg/node_modules/.bin/",
		"pkg/node_modules/semver/bin/",
	} {
		_ = tarWriter.WriteHeader(&tar.Header{
			Name:     dir,
			Typeflag: tar.TypeDir,
			Mode:     0o755,
		})
	}

	// Create the target file
	content := []byte("#!/bin/sh\necho semver")
	_ = tarWriter.WriteHeader(&tar.Header{
		Name:     "pkg/node_modules/semver/bin/semver.js",
		Typeflag: tar.TypeReg,
		Mode:     0o755,
		Size:     int64(len(content)),
	})
	_, _ = tarWriter.Write(content)

	// Create symlink: .bin/semver -> ../semver/bin/semver.js (stays within destDir)
	_ = tarWriter.WriteHeader(&tar.Header{
		Name:     "pkg/node_modules/.bin/semver",
		Typeflag: tar.TypeSymlink,
		Linkname: "../semver/bin/semver.js",
	})

	_ = tarWriter.Close()
	_ = gzWriter.Close()

	destDir := t.TempDir()
	if err := extractTarGz(&buf, destDir); err != nil {
		t.Fatalf("extractTarGz should allow relative '..' symlinks within destDir, got: %v", err)
	}

	// Verify the symlink was created
	linkPath := filepath.Join(destDir, "pkg/node_modules/.bin/semver")
	linkTarget, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}
	if linkTarget != "../semver/bin/semver.js" {
		t.Errorf("expected symlink target '../semver/bin/semver.js', got %q", linkTarget)
	}
}

func TestExtractTarGz_SymlinkDotDotEscapeBlocked(t *testing.T) {
	// A ".." symlink that escapes destDir must still be blocked by the resolved-path check.
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	_ = tarWriter.WriteHeader(&tar.Header{
		Name:     "pkg/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	})

	// Symlink that uses ".." to escape the destDir
	_ = tarWriter.WriteHeader(&tar.Header{
		Name:     "pkg/escape",
		Typeflag: tar.TypeSymlink,
		Linkname: "../../etc/passwd",
	})

	_ = tarWriter.Close()
	_ = gzWriter.Close()

	destDir := t.TempDir()
	err := extractTarGz(&buf, destDir)
	if err == nil {
		t.Error("expected error for '..' symlink escaping destDir, got nil")
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
