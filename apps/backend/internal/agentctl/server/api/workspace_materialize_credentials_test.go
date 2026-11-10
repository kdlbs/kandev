package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/githubauth"
)

// TestMaterializeCredentialHelper runs in a child process launched by Git.
func TestMaterializeCredentialHelper(t *testing.T) {
	if os.Getenv("KANDEV_TEST_MATERIALIZE_HELPER") != "1" {
		return
	}
	os.Exit(runMaterializeCredentialHelper())
}

func runMaterializeCredentialHelper() int {
	if os.Args[len(os.Args)-1] != "get" {
		return 0
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	req, err := http.NewRequest(http.MethodPost, os.Getenv(githubauth.CredentialBrokerURLEnv), nil)
	if err != nil {
		return 2
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv(githubauth.CredentialLeaseEnv))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 2
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	_, _ = io.Copy(os.Stdout, resp.Body)
	return 0
}

func materializeCredentialEnvironment(t *testing.T, broker, lease string) []string {
	t.Helper()
	values := map[string]string{
		"KANDEV_TEST_MATERIALIZE_HELPER": "1",
		"GIT_CONFIG_GLOBAL":              os.DevNull, "GIT_CONFIG_NOSYSTEM": "1",
		"GIT_CONFIG_COUNT": "3", "GIT_CONFIG_KEY_0": "credential.helper", "GIT_CONFIG_VALUE_0": "",
		"GIT_CONFIG_KEY_1":   "credential.helper",
		"GIT_CONFIG_VALUE_1": `!"${KANDEV_GITHUB_CREDENTIAL_HELPER_PATH}" -test.run=TestMaterializeCredentialHelper --`,
		"GIT_CONFIG_KEY_2":   "materialize.instance", "GIT_CONFIG_VALUE_2": lease,
		githubauth.CredentialBrokerURLEnv: broker, githubauth.CredentialLeaseEnv: lease,
	}
	env, err := config.CollectAgentEnvWithError(values)
	if err != nil {
		t.Fatal(err)
	}
	// Tool installation belongs to the runtime, and can be absent from the instance overlay.
	result := make([]string, 0, len(env))
	for _, entry := range env {
		if !strings.HasPrefix(entry, githubauth.CredentialHelperPathEnv+"=") {
			result = append(result, entry)
		}
	}
	return result
}

func materializeCredentialRequest(t *testing.T, s *Server, locator, destination string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(MaterializeRepositoryRequest{RepositoryURL: locator, Destination: destination, BaseBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/materialize-repository", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

func materializeGitServer(t *testing.T, private bool) string {
	t.Helper()
	origin := createMaterializeOrigin(t)
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	backend := &cgi.Handler{Path: git, Args: []string{"http-backend"}, Env: []string{"GIT_PROJECT_ROOT=" + filepath.Dir(origin), "GIT_HTTP_EXPORT_ALL=1"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if private && (!ok || user != "fixture" || password != "fixture-password") {
			w.Header().Set("WWW-Authenticate", `Basic realm="materialize"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		backend.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/" + filepath.Base(origin)
}

func isolateMaterializeCredentialEnvironment(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_CONFIG_") || strings.HasPrefix(key, "KANDEV_GITHUB_") {
			t.Setenv(key, "")
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(githubauth.CredentialHelperPathEnv, executable)
}

// @covers AC-EXECUTORS-PLUGIN-002.5, AC-EXECUTORS-PLUGIN-002.6, AC-EXECUTORS-PLUGIN-002.7
func TestWorkspaceMaterializeRepository_ManagedCredentials(t *testing.T) {
	isolateMaterializeCredentialEnvironment(t)
	locator := materializeGitServer(t, true)
	var redemptions atomic.Int32
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer instance-lease" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		redemptions.Add(1)
		_, _ = fmt.Fprint(w, "username=fixture\npassword=fixture-password\n")
	}))
	t.Cleanup(broker.Close)
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), AgentEnv: materializeCredentialEnvironment(t, broker.URL, "instance-lease")}
	log := newTestLogger()
	mgr := process.NewManager(cfg, log)
	s := NewServer(cfg, mgr, nil, nil, log)
	w := materializeCredentialRequest(t, s, locator, "private")
	if w.Code != http.StatusCreated {
		t.Fatalf("materialize status = %d, body = %s; want 201", w.Code, w.Body.String())
	}
	if redemptions.Load() == 0 {
		t.Fatal("instance broker was not reached")
	}
	destination := filepath.Join(cfg.WorkDir, "private")
	if got := strings.TrimSpace(materializeGitOutputForTest(t, destination, "config", "--get", "remote.origin.url")); got != locator {
		t.Fatalf("origin = %q, want credential-free locator", got)
	}
	contents, err := os.ReadFile(filepath.Join(destination, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"instance-lease", "fixture-password", broker.URL} {
		if bytes.Contains(contents, []byte(secret)) {
			t.Fatal("authorization persisted in repository config")
		}
	}
	env, err := mgr.MaterializationGitEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	runner := materializeGitRunner{env: env}
	beforeFetch := redemptions.Load()
	if _, err := runner.materializeGitOutput(context.Background(), "-C", destination, "fetch", "origin"); err != nil {
		t.Fatalf("authenticated fetch: %v", err)
	}
	if redemptions.Load() <= beforeFetch {
		t.Fatal("fetch did not redeem instance credentials")
	}
	output, err := runner.materializeCheckoutGit(context.Background(), []string{"-C", destination, "config", "--get", "materialize.instance"}, "")
	if err != nil || strings.TrimSpace(string(output)) != "instance-lease" {
		t.Fatalf("checkout runner missed instance config: %q, %v", output, err)
	}
	reused := materializeCredentialRequest(t, s, locator, "private")
	if reused.Code != http.StatusOK {
		t.Fatalf("reuse status = %d, want 200", reused.Code)
	}
	assertMaterializeDeniedInstance(t, broker.URL, locator)
}

func assertMaterializeDeniedInstance(t *testing.T, brokerURL, locator string) {
	t.Helper()
	log := newTestLogger()
	// A second instance must not inherit the first one's lease.
	cfg2 := &config.InstanceConfig{WorkDir: t.TempDir(), AgentEnv: materializeCredentialEnvironment(t, brokerURL, "denied-lease")}
	s2 := NewServer(cfg2, process.NewManager(cfg2, log), nil, nil, log)
	denied := materializeCredentialRequest(t, s2, locator, "private")
	if denied.Code != http.StatusUnprocessableEntity || strings.Contains(denied.Body.String(), "denied-lease") {
		t.Fatalf("denied response = %d %s", denied.Code, denied.Body.String())
	}
	entries, err := os.ReadDir(cfg2.WorkDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("failed clone left a checkout or temporary directory")
	}
}

func TestWorkspaceMaterializeRepository_PublicWithoutManagedCredentials(t *testing.T) {
	isolateMaterializeCredentialEnvironment(t)
	locator := materializeGitServer(t, false)
	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), AgentEnv: materializeCredentialEnvironment(t, "", "")}
	s := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
	response := materializeCredentialRequest(t, s, locator, "public")
	if response.Code != http.StatusCreated {
		t.Fatalf("public materialization = %d %s", response.Code, response.Body.String())
	}
}

func TestWorkspaceMaterializeRepository_MalformedCredentialsFailBeforeGit(t *testing.T) {
	var requests atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(http.StatusForbidden) }))
	t.Cleanup(remote.Close)
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), AgentEnv: []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=secret-sentinel"}}
	log := newTestLogger()
	s := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
	response := materializeCredentialRequest(t, s, remote.URL+"/private.git", "private")
	if response.Code != http.StatusUnprocessableEntity || strings.Contains(response.Body.String(), "secret-sentinel") {
		t.Fatalf("malformed environment response = %d %s", response.Code, response.Body.String())
	}
	if requests.Load() != 0 {
		t.Fatal("malformed environment reached remote Git server")
	}
}
