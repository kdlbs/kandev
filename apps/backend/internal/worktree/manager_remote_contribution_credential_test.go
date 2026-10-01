package worktree

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/githubauth"
)

func TestCreateWorktree_RemoteContributionAuthenticatedFetchKeepsExactHead(t *testing.T) {
	helperPath := buildWorktreeCredentialHelper(t)
	home := t.TempDir()
	configureWorktreeCredentialTestEnvironment(t, home)

	source, sourceSHA := initContributionSource(t)
	runGit(t, source, "update-server-info")
	tlsServer, proxy := newContributionCredentialProxy(t, source)
	defer tlsServer.Close()
	defer proxy.Close()
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("https_proxy", proxy.URL)

	var brokerCalls atomic.Int32
	brokerRequests := make(chan worktreeCredentialBrokerRequest, 16)
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request worktreeCredentialBrokerRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		brokerCalls.Add(1)
		brokerRequests <- request
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"username":"synthetic-user","password":"synthetic-token"}`))
	}))
	defer broker.Close()

	repoPath := filepath.Join(t.TempDir(), "clone")
	runGit(t, home, "clone", "--branch", "main", source, repoPath)
	contributionURL := "https://github.com/target/widget.git"
	runGit(t, repoPath, "remote", "set-url", "origin", contributionURL)
	binding := testRemoteContribution(sourceSHA, contributionURL)
	binding.SourceRepository.Path = "target/widget"
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	for round := 1; round <= 3; round++ {
		taskID := fmt.Sprintf("credential-child-%d", round)
		sessionID := fmt.Sprintf("credential-session-%d", round)
		checkoutEnv := worktreeContributionCredentialEnvironment(broker.URL, helperPath, taskID, sessionID)
		wt, err := mgr.Create(context.Background(), CreateRequest{
			TaskID: taskID, SessionID: sessionID,
			RepositoryID: "target-repository", RepositoryPath: repoPath, BaseBranch: binding.BaseBranch,
			CheckoutBranch: binding.HeadBranch, RemoteContribution: &binding,
			TaskDirName: fmt.Sprintf("credential-child-%d", round), RepoName: "widget", CheckoutEnv: checkoutEnv,
		})
		if err != nil {
			t.Fatalf("authenticated contribution checkout %d failed: %v", round, err)
		}
		if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != sourceSHA {
			t.Fatalf("checkout %d HEAD = %q, want provider head %q", round, got, sourceSHA)
		}
	}
	if brokerCalls.Load() == 0 {
		t.Fatal("authenticated contribution fetch never resolved credentials")
	}
	wantTasks := map[string]string{
		"credential-child-1": "credential-session-1",
		"credential-child-2": "credential-session-2",
		"credential-child-3": "credential-session-3",
	}
	for len(brokerRequests) > 0 {
		request := <-brokerRequests
		if request.Path != "/target/widget.git" || request.RepositoryID != "target-repository" {
			t.Errorf("broker request = %+v, want the scoped target repository", request)
		}
		sessionID, ok := wantTasks[request.TaskID]
		if !ok {
			t.Errorf("broker request task ID = %q, want a test checkout task", request.TaskID)
		}
		if request.SessionID != sessionID {
			t.Errorf("broker request session ID = %q, want %q", request.SessionID, sessionID)
		}
	}
}

type worktreeCredentialBrokerRequest struct {
	TaskID       string `json:"task_id"`
	SessionID    string `json:"session_id"`
	RepositoryID string `json:"repository_id"`
	Path         string `json:"path"`
}

func buildWorktreeCredentialHelper(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve credential test source path")
	}
	backendDir := filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	name := "agentctl"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", path, "./cmd/agentctl")
	cmd.Dir = backendDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build credential helper fixture: %v: %s", err, output)
	}
	return path
}

func configureWorktreeCredentialTestEnvironment(t *testing.T, home string) {
	t.Helper()
	for name, value := range map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": home, "GIT_CONFIG_NOSYSTEM": "1",
		"GIT_CONFIG_COUNT": "0", "GIT_CONFIG_GLOBAL": filepath.Join(home, "missing.gitconfig"),
		"GIT_CONFIG_PARAMETERS": "", "HTTP_PROXY": "", "http_proxy": "",
		"NO_PROXY": "127.0.0.1,localhost", "no_proxy": "127.0.0.1,localhost",
		"GIT_SSL_NO_VERIFY": "true",
	} {
		t.Setenv(name, value)
	}
}

func worktreeContributionCredentialEnvironment(brokerURL, helperPath, taskID, sessionID string) map[string]string {
	return map[string]string{
		githubauth.CredentialBrokerURLEnv:         brokerURL,
		githubauth.CredentialHelperPathEnv:        helperPath,
		githubauth.CredentialLeaseEnv:             "synthetic-lease",
		githubauth.CredentialReissueCapabilityEnv: "synthetic-capability",
		githubauth.CredentialTaskIDEnv:            taskID,
		githubauth.CredentialSessionIDEnv:         sessionID,
		githubauth.CredentialRepositoryEnv:        "target-repository",
		githubauth.CredentialOwnerEnv:             "target",
		githubauth.CredentialRepoEnv:              "widget",
		githubauth.CredentialHostEnv:              "github.com",
		githubauth.CredentialScopesEnv:            fmt.Sprintf(`[{"lease":"synthetic-lease","reissue_capability":"synthetic-capability","task_id":%q,"session_id":%q,"repository_id":"target-repository","owner":"target","repo":"widget","host":"github.com","path":"/target/widget.git"}]`, taskID, sessionID),
		"GIT_CONFIG_COUNT":                        "3",
		"GIT_CONFIG_KEY_0":                        "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_0":                      "",
		"GIT_CONFIG_KEY_1":                        "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_1":                      githubauth.ManagedGitCredentialHelper,
		"GIT_CONFIG_KEY_2":                        "credential.useHttpPath",
		"GIT_CONFIG_VALUE_2":                      "true",
	}
}

func newContributionCredentialProxy(t *testing.T, source string) (*httptest.Server, *httptest.Server) {
	t.Helper()
	files := http.FileServer(http.Dir(filepath.Dir(source)))
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		r.URL.Path = strings.Replace(r.URL.Path, "/target/widget.git", "/source.git", 1)
		files.ServeHTTP(w, r)
	}))
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "github.com:443" {
			http.Error(w, "unexpected destination", http.StatusBadRequest)
			return
		}
		upstream, err := net.Dial("tcp", tlsServer.Listener.Addr().String())
		if err != nil {
			http.Error(w, "connect failed", http.StatusBadGateway)
			return
		}
		downstream, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			_ = upstream.Close()
			return
		}
		_, _ = fmt.Fprint(downstream, "HTTP/1.1 200 Connection established\r\n\r\n")
		_ = buffer.Flush()
		bridgeContributionCredentialTunnel(upstream, downstream, buffer)
	}))
	return tlsServer, proxy
}

func bridgeContributionCredentialTunnel(upstream net.Conn, downstream net.Conn, buffer io.Reader) {
	var closeOnce sync.Once
	closeTunnel := func() {
		closeOnce.Do(func() {
			_ = upstream.Close()
			_ = downstream.Close()
		})
	}
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, buffer)
		closeTunnel()
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(downstream, upstream)
		closeTunnel()
		done <- struct{}{}
	}()
	<-done
	<-done
}
