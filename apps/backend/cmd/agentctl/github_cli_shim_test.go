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

func TestGitHubCLIShimSelectsRepositoryLease(t *testing.T) {
	var got githubBrokerResolveRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"username":"x-access-token","password":"backend-token"}`)
	}))
	t.Cleanup(server.Close)
	env := githubCredentialTestEnv(server.URL)
	env["PATH"] = "/shim:/usr/bin"
	env["GH_REPO"] = "acme/backend"
	env[envGitHubCredentialScopes] = `[
		{"lease":"frontend-lease","task_id":"task-1","session_id":"session-1","repository_id":"repo-1","owner":"acme","repo":"frontend","host":"github.com"},
		{"lease":"backend-lease","task_id":"task-1","session_id":"session-1","repository_id":"repo-2","owner":"acme","repo":"backend","host":"github.com"}
	]`
	var childToken string
	err := runGitHubCLIShim(
		context.Background(), []string{"pr", "list"}, strings.NewReader(""), io.Discard, io.Discard,
		lookupEnv(env), func() []string { return envMap(env) }, server.Client(), "/shim",
		func(string, string) (string, error) { return "/usr/bin/gh", nil },
		func(_ context.Context, _ string, _ []string, childEnv []string, _ io.Reader, _, _ io.Writer) error {
			childToken = envValue(childEnv, "GH_TOKEN")
			return nil
		},
	)
	if err != nil {
		t.Fatalf("runGitHubCLIShim() error = %v", err)
	}
	if got.Lease != "backend-lease" || got.RepositoryID != "repo-2" {
		t.Fatalf("selected broker scope = %+v", got)
	}
	if childToken != "backend-token" {
		t.Fatalf("child GH_TOKEN = %q, want backend-token", childToken)
	}
}

func TestParseGitHubCLIRepository(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		defaultHost string
		want        githubCLIRepository
	}{
		{name: "owner and repo", raw: "acme/widgets", defaultHost: "github.com", want: githubCLIRepository{host: "github.com", owner: "acme", repo: "widgets"}},
		{name: "enterprise", raw: "github.example.com/acme/widgets", want: githubCLIRepository{host: "github.example.com", owner: "acme", repo: "widgets"}},
		{name: "HTTPS remote", raw: "https://github.com/acme/widgets.git", want: githubCLIRepository{host: "github.com", owner: "acme", repo: "widgets"}},
		{name: "SSH remote", raw: "git@github.com:acme/widgets.git", want: githubCLIRepository{host: "github.com", owner: "acme", repo: "widgets"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseGitHubCLIRepository(test.raw, test.defaultHost)
			if err != nil {
				t.Fatalf("parseGitHubCLIRepository() error = %v", err)
			}
			if *got != test.want {
				t.Fatalf("parseGitHubCLIRepository() = %+v, want %+v", *got, test.want)
			}
		})
	}
}

func TestGitHubCLIRepositoryArgument(t *testing.T) {
	for _, args := range [][]string{
		{"pr", "list", "-R", "acme/widgets"},
		{"pr", "list", "--repo=acme/widgets"},
		{"pr", "list", "-Racme/widgets"},
	} {
		got, found, err := githubCLIRepositoryArgument(args)
		if err != nil || !found || got != "acme/widgets" {
			t.Fatalf("githubCLIRepositoryArgument(%v) = %q, %v, %v", args, got, found, err)
		}
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
