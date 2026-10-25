package repoclone

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// @covers AC-TASKS-REMOTE-OPTIONS-003.1
// @covers AC-TASKS-REMOTE-OPTIONS-003.3
func TestAuthenticatedCloneCheckoutOptionsLazyRead(t *testing.T) {
	origin, firstBlob, secondBlob, allowed := checkoutOptionsGitServer(t)
	t.Setenv("GIT_SSL_NO_VERIFY", "true") // Disposable test server has its own certificate.
	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
	cloner.SetGitCredentialProvider(&recordingCredentialProvider{password: "fixture-token"})
	request := GitCredentialRequest{WorkspaceID: "workspace", Provider: "github", ProviderHost: origin, CloneURL: origin + "/repo.git", Owner: "acme", Name: "repo"}
	if err := json.Unmarshal([]byte(`{"checkout_options":{"version":1,"download_mode":"on_demand","sparse_directories":["app"]}}`), &request); err != nil {
		t.Fatal(err)
	}
	path, _, err := cloner.EnsureWorkspaceClonedWithCredentialRequestAndState(context.Background(), request, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cloner.RefreshWorkspaceRepositoryWithCredentialRequestAndState(context.Background(), request, path, "", ""); err != nil {
		t.Fatalf("refresh option-specific cache: %v", err)
	}
	objects := checkoutOptionsGit(t, path, "cat-file", "--batch-all-objects", "--batch-check=%(objectname)")
	if strings.Contains(objects, firstBlob) || strings.Contains(objects, secondBlob) {
		t.Fatal("on-demand clone downloaded historical file contents")
	}
	if _, err := os.Stat(filepath.Join(path, "README.md")); !os.IsNotExist(err) {
		t.Fatal("cache clone populated a working tree before task folder selection")
	}
	read := func(blob string) ([]byte, error) {
		return cloner.runConfiguredGitCombined(context.Background(), gitFetchTimeout, []string{"-C", path, "cat-file", "blob", blob}, func(cmd *exec.Cmd) (func(), error) {
			return configureGitCommand(cmd, &cloneAuth{origin: origin, username: "x-access-token", password: "fixture-token"})
		})
	}
	if out, err := read(firstBlob); err != nil || string(out) != "historical first" {
		t.Fatalf("lazy read after clone cleanup: %q, %v", out, err)
	}
	allowed.Store(false)
	if _, err := read(secondBlob); err == nil {
		t.Fatal("lazy read succeeded after credential revocation")
	}
}

func checkoutOptionsGitServer(t *testing.T) (string, string, string, *atomic.Bool) {
	isolateCheckoutOptionsTestGitEnv(t)
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	checkoutOptionsGit(t, source, "init", "-b", "main")
	checkoutOptionsGit(t, source, "config", "user.name", "Fixture")
	checkoutOptionsGit(t, source, "config", "user.email", "fixture@example.test")
	for name, value := range map[string]string{"README.md": "root", "first.txt": "historical first", "second.txt": "historical second"} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	checkoutOptionsGit(t, source, "add", ".")
	checkoutOptionsGit(t, source, "-c", "commit.gpgsign=false", "commit", "-m", "history")
	first := strings.TrimSpace(checkoutOptionsGit(t, source, "rev-parse", "HEAD:first.txt"))
	second := strings.TrimSpace(checkoutOptionsGit(t, source, "rev-parse", "HEAD:second.txt"))
	checkoutOptionsGit(t, source, "rm", "first.txt", "second.txt")
	checkoutOptionsGit(t, source, "-c", "commit.gpgsign=false", "commit", "-m", "remove historical files")
	bare := filepath.Join(root, "repo.git")
	checkoutOptionsGit(t, root, "clone", "--bare", source, bare)
	checkoutOptionsGit(t, bare, "config", "uploadpack.allowFilter", "true")
	checkoutOptionsGit(t, bare, "config", "uploadpack.allowAnySHA1InWant", "true")
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	backend := &cgi.Handler{Path: git, Args: []string{"http-backend"}, Env: []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1"}}
	allowed := &atomic.Bool{}
	allowed.Store(true)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, password, ok := r.BasicAuth()
		if !ok || password != "fixture-token" || !allowed.Load() {
			w.Header().Set("WWW-Authenticate", `Basic realm="fixture"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		backend.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server.URL, first, second, allowed
}

func checkoutOptionsGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", directory}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func TestAuthenticatedCloneCheckoutOptionsRetainsGitCryptKeys(t *testing.T) {
	origin, _, _, _ := checkoutOptionsGitServer(t)
	t.Setenv("GIT_SSL_NO_VERIFY", "true")
	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
	cloner.SetGitCredentialProvider(&recordingCredentialProvider{password: "fixture-token"})
	request := GitCredentialRequest{WorkspaceID: "workspace", Provider: "github", ProviderHost: origin, CloneURL: origin + "/repo.git", Owner: "acme", Name: "repo"}
	standard, err := cloner.EnsureWorkspaceClonedWithCredentialRequest(context.Background(), request, "", "")
	if err != nil {
		t.Fatal(err)
	}
	keys := filepath.Join(standard, ".git", "git-crypt", "keys")
	if err := os.MkdirAll(keys, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keys, "default"), []byte("key-fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"checkout_options":{"version":1,"download_mode":"on_demand"}}`), &request); err != nil {
		t.Fatal(err)
	}
	cache, err := cloner.EnsureWorkspaceClonedWithCredentialRequest(context.Background(), request, "", "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(cache, ".git", "git-crypt", "keys", "default"))
	if err != nil || string(data) != "key-fixture" {
		t.Fatalf("managed unlock state lost: %q %v", data, err)
	}
}

func isolateCheckoutOptionsTestGitEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES"} {
		if value, exists := os.LookupEnv(key); exists {
			t.Setenv(key, value)
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
	}
}
