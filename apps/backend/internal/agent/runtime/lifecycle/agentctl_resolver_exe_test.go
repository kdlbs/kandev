package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// @covers AC-EXECUTORS-SSH-WINDOWS-002.2
func TestAgentctlResolverManifestDownloadsWindowsExeHelperToCache(t *testing.T) {
	const version = "1.2.3"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := []byte("MZ-fake-windows-agentctl-exe")
	bundle := t.TempDir()
	home := t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "standard", "windows/amd64", payload)

	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		writeGzip(t, w, payload)
	}))
	defer server.Close()

	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: home,
		ReleaseBaseURL: server.URL + "/releases/download",
	})
	gotPath, err := resolver.ResolveRemoteBinaryContext(context.Background(), SSHRemotePlatform{GOOS: "windows", GOARCH: "amd64"}, nil)
	if err != nil {
		t.Fatalf("ResolveRemoteBinaryContext(windows/amd64): %v", err)
	}
	if wantReq := "/releases/download/v1.2.3/agentctl-windows-amd64.exe.gz"; requestedPath != wantReq {
		t.Fatalf("download URL path = %q, want %q", requestedPath, wantReq)
	}
	if filepath.Base(gotPath) != "agentctl.exe" {
		t.Fatalf("cached Windows binary filename = %q, want agentctl.exe", filepath.Base(gotPath))
	}
	gotBytes, err := os.ReadFile(gotPath)
	if err != nil || string(gotBytes) != string(payload) {
		t.Fatalf("cached Windows binary bytes = %q, err=%v", gotBytes, err)
	}
}

func TestAgentctlBinaryNameAddsExeSuffixOnlyForWindows(t *testing.T) {
	tests := map[SSHRemotePlatform]string{
		{GOOS: "windows", GOARCH: "amd64"}: "agentctl-windows-amd64.exe",
		{GOOS: "linux", GOARCH: "amd64"}:   "agentctl-linux-amd64",
		{GOOS: "darwin", GOARCH: "arm64"}:  "agentctl-darwin-arm64",
	}
	for platform, want := range tests {
		if got := agentctlBinaryName(platform); got != want {
			t.Errorf("agentctlBinaryName(%s) = %q, want %q", platform, got, want)
		}
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-002.4
func TestAgentctlResolverDefaultReleaseBaseURLIsUpstream(t *testing.T) {
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{})
	if got, want := resolver.options.ReleaseBaseURL, "https://github.com/kdlbs/kandev/releases/download"; got != want {
		t.Fatalf("default release base URL = %q, want %q", got, want)
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-002.1
func TestRemoteHelperManifestRejectsWindowsARM64Record(t *testing.T) {
	manifest := testRemoteHelperManifest("standard")
	replaced := false
	for i := range manifest.Helpers {
		if manifest.Helpers[i].Platform == "windows/amd64" {
			manifest.Helpers[i].Platform = "windows/arm64"
			manifest.Helpers[i].Asset = "agentctl-windows-arm64.exe.gz"
			replaced = true
		}
	}
	if !replaced {
		t.Fatal("test manifest has no windows/amd64 record to replace")
	}
	root := t.TempDir()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, RemoteHelperManifestName), data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = ReadRemoteHelperManifest(root, "1.2.3", strings.Repeat("a", 40))
	if err == nil || !strings.Contains(err.Error(), `"windows/arm64" is unsupported`) {
		t.Fatalf("ReadRemoteHelperManifest error = %v, want windows/arm64 unsupported", err)
	}
}

func TestAgentctlResolverRejectsPlatformsWithoutPublishedHelper(t *testing.T) {
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{BundleDir: t.TempDir()})
	_, err := resolver.ResolveRemoteBinaryContext(context.Background(), SSHRemotePlatform{GOOS: "windows", GOARCH: "arm64"}, nil)
	if err == nil || !strings.Contains(err.Error(), `"windows/arm64"`) {
		t.Fatalf("ResolveRemoteBinaryContext(windows/arm64) error = %v, want unpublished-helper error", err)
	}
}
