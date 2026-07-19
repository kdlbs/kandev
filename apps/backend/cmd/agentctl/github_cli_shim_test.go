package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallGitHubCLIShimCreatesGHEntrypoint(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "agentctl")
	if err := os.WriteFile(binary, []byte("agentctl"), 0o700); err != nil {
		t.Fatal(err)
	}

	shimDir, cleanup, err := installGitHubCLIShim(binary, root)
	if err != nil {
		t.Fatalf("installGitHubCLIShim() error = %v", err)
	}
	t.Cleanup(cleanup)
	entrypoint := filepath.Join(shimDir, githubCLIShimName())
	if _, err := os.Stat(entrypoint); err != nil {
		t.Fatalf("stat gh shim: %v", err)
	}
	if filepath.Dir(entrypoint) != shimDir {
		t.Fatalf("entrypoint = %q, want inside %q", entrypoint, shimDir)
	}
}

func TestPathWithoutDirectoryPreservesPathList(t *testing.T) {
	path := strings.Join([]string{"/bin", "/shim", "/usr/bin"}, string(os.PathListSeparator))
	want := strings.Join([]string{"/bin", "/usr/bin"}, string(os.PathListSeparator))
	if got := pathWithoutDirectory(path, "/shim"); got != want {
		t.Fatalf("pathWithoutDirectory() = %q, want %q", got, want)
	}
}

func TestIsGitHubCLIShimInvocation(t *testing.T) {
	for _, name := range []string{"/tmp/shims/gh", `C:\\shims\\gh.exe`} {
		if !isGitHubCLIShimInvocation(name) {
			t.Fatalf("isGitHubCLIShimInvocation(%q) = false", name)
		}
	}
	if isGitHubCLIShimInvocation("/usr/local/bin/agentctl") {
		t.Fatal("agentctl executable was mistaken for gh shim")
	}
}

func TestGitHubCLIShimRefreshesAndIsolatesEachInvocation(t *testing.T) {
	issued := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		issued++
		_ = json.NewEncoder(w).Encode(map[string]string{
			"username": "x-access-token",
			"password": "fresh-token-" + string(rune('0'+issued)),
		})
	}))
	t.Cleanup(server.Close)
	env := githubCredentialTestEnv(server.URL)
	env["PATH"] = "/shim:/usr/bin"
	env["GITHUB_TOKEN"] = "parent-token-must-not-win"
	var childTokens []string
	var configDirs []string
	runner := func(_ context.Context, executable string, args []string, childEnv []string, _ io.Reader, _, _ io.Writer) error {
		if executable != "/usr/bin/gh" {
			t.Errorf("executable = %q, want /usr/bin/gh", executable)
		}
		if len(args) != 2 || args[0] != "pr" || args[1] != "list" {
			t.Errorf("args = %v", args)
		}
		childTokens = append(childTokens, envValue(childEnv, "GH_TOKEN"))
		configDirs = append(configDirs, envValue(childEnv, "GH_CONFIG_DIR"))
		if got := envValue(childEnv, "GITHUB_TOKEN"); got != "" {
			t.Errorf("child GITHUB_TOKEN = %q, want removed", got)
		}
		return nil
	}
	lookPath := func(file, path string) (string, error) {
		if file != "gh" || path != "/usr/bin" {
			t.Fatalf("lookPath(%q, %q)", file, path)
		}
		return "/usr/bin/gh", nil
	}

	for range 2 {
		if err := runGitHubCLIShim(
			context.Background(), []string{"pr", "list"}, strings.NewReader(""), io.Discard, io.Discard,
			lookupEnv(env), func() []string { return envMap(env) }, server.Client(), "/shim", lookPath, runner,
		); err != nil {
			t.Fatalf("runGitHubCLIShim() error = %v", err)
		}
	}
	if got, want := strings.Join(childTokens, ","), "fresh-token-1,fresh-token-2"; got != want {
		t.Fatalf("child GH_TOKEN values = %q, want %q", got, want)
	}
	if configDirs[0] == "" || configDirs[0] != configDirs[1] {
		t.Fatalf("GH_CONFIG_DIR values = %v, want stable isolated directory", configDirs)
	}
}

func envMap(env map[string]string) []string {
	result := make([]string, 0, len(env))
	for key, value := range env {
		result = append(result, key+"="+value)
	}
	return result
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}
