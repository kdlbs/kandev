package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/worktree"
)

func TestCheckoutCredentialEnvironmentRealGitPath(t *testing.T) {
	helperPath := testutil.BuildAgentctl(t)
	requests := make(chan checkoutCredentialBrokerRequest, 1)
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request checkoutCredentialBrokerRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode broker request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"username":"synthetic-user","password":"synthetic-token"}`))
	}))
	defer broker.Close()

	input := checkoutCredentialProducerEnvironment(broker.URL, helperPath, "task-1", "session-1")
	checkoutEnv := buildWorktreeCreateRequest(&EnvPrepareRequest{Env: input}).CheckoutEnv
	output, err := runIsolatedGitCredentialFill(t, checkoutEnv, "acme/widgets.git")
	if err != nil || !strings.Contains(output, "username=synthetic-user") || !strings.Contains(output, "password=synthetic-token") {
		t.Fatalf("Git credential fill failed for the scoped repository: %v: %s", err, output)
	}
	if got := <-requests; got.Path != "/acme/widgets.git" || got.RepositoryID != "repository-1" {
		t.Fatalf("broker request = %+v, want the scoped repository path", got)
	}

	output, err = runIsolatedGitCredentialFill(t, checkoutEnv, "acme/other.git")
	if err == nil || !strings.Contains(output, "git repository does not match any credential lease scope") {
		t.Fatalf("Git credential fill for another repository output = %q, error = %v, want scope rejection", output, err)
	}
	select {
	case request := <-requests:
		t.Fatalf("out-of-scope repository reached broker: %+v", request)
	default:
	}
}

// @covers AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.2 AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.3
func TestCheckoutCredentialEnvironmentThroughWorktreeCreate(t *testing.T) {
	testutil.ClearGitRepositoryEnvironment(t)
	testutil.ClearGitConfigEnvironment(t)
	helperPath := testutil.BuildAgentctl(t)
	home := t.TempDir()
	configureLifecycleCredentialEnvironment(t, home)

	source, sourceSHA := initLifecycleCredentialSource(t)
	runLifecycleCredentialGit(t, source, "update-server-info")
	tlsServer, proxy := newLifecycleCredentialProxy(t, source)
	t.Cleanup(tlsServer.Close)
	t.Cleanup(proxy.Close)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("https_proxy", proxy.URL)

	requests := make(chan checkoutCredentialBrokerRequest, 16)
	var brokerCalls atomic.Int32
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request checkoutCredentialBrokerRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		brokerCalls.Add(1)
		requests <- request
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"username":"synthetic-user","password":"synthetic-token"}`))
	}))
	defer broker.Close()

	repositoryPath := filepath.Join(t.TempDir(), "clone")
	runLifecycleCredentialGit(t, home, "clone", "--branch", "main", source, repositoryPath)
	contributionURL := "https://github.com/acme/widgets.git"
	runLifecycleCredentialGit(t, repositoryPath, "remote", "set-url", "origin", contributionURL)
	binding := models.RemoteContribution{
		Version:      models.RemoteContributionVersion,
		Provider:     models.RemoteContributionProviderGitHub,
		Kind:         models.RemoteContributionKindPullRequest,
		CanonicalURL: "https://github.com/acme/widgets/pull/7",
		Number:       7,
		State:        models.RemoteContributionStateOpen,
		BaseBranch:   "main",
		HeadBranch:   "contributor/feature",
		HeadSHA:      sourceSHA,
		SourceRepository: models.RemoteContributionRepository{
			Host: "github.com", Path: "acme/widgets", RemoteURL: contributionURL,
		},
		CollaborationAllowed: true,
	}

	manager, err := worktree.NewManager(worktree.Config{
		Enabled: true, TasksBasePath: t.TempDir(), BranchPrefix: "kandev/",
	}, &lifecycleCredentialWorktreeStore{worktrees: make(map[string]*worktree.Worktree)}, nil)
	if err != nil {
		t.Fatalf("create worktree manager: %v", err)
	}
	wantTasks := make(map[string]string, 3)
	for round := 1; round <= 3; round++ {
		taskID := fmt.Sprintf("credential-child-%d", round)
		sessionID := fmt.Sprintf("credential-session-%d", round)
		prepareReq := &EnvPrepareRequest{
			TaskID: taskID, SessionID: sessionID, RepositoryID: "repository-1",
			RepositoryPath: repositoryPath, BaseBranch: binding.BaseBranch, CheckoutBranch: binding.HeadBranch,
			RemoteContribution: &binding, TaskDirName: taskID, RepoName: "widgets",
			Env: checkoutCredentialProducerEnvironment(broker.URL, helperPath, taskID, sessionID),
		}
		createReq := buildWorktreeCreateRequest(prepareReq)
		created, err := manager.Create(context.Background(), createReq)
		if err != nil {
			t.Fatalf("authenticated worktree creation %d failed: %v", round, err)
		}
		if got := strings.TrimSpace(runLifecycleCredentialGit(t, created.Path, "rev-parse", "HEAD")); got != sourceSHA {
			t.Fatalf("worktree %d HEAD = %q, want contribution head %q", round, got, sourceSHA)
		}
		wantTasks[taskID] = sessionID
	}
	if brokerCalls.Load() == 0 {
		t.Fatal("authenticated contribution fetch never resolved credentials")
	}
	seenTasks := make(map[string]bool, len(wantTasks))
	for len(requests) > 0 {
		request := <-requests
		if request.Path != "/acme/widgets.git" || request.RepositoryID != "repository-1" {
			t.Errorf("broker request = %+v, want the filtered repository scope", request)
		}
		sessionID, ok := wantTasks[request.TaskID]
		if !ok || request.SessionID != sessionID {
			t.Errorf("broker task/session = %s/%s, want a test checkout identity", request.TaskID, request.SessionID)
			continue
		}
		seenTasks[request.TaskID] = true
	}
	if len(seenTasks) != len(wantTasks) {
		t.Fatalf("broker calls covered %d of %d checkout tasks", len(seenTasks), len(wantTasks))
	}
}

type checkoutCredentialBrokerRequest struct {
	TaskID       string `json:"task_id"`
	SessionID    string `json:"session_id"`
	RepositoryID string `json:"repository_id"`
	Path         string `json:"path"`
}

func configureLifecycleCredentialEnvironment(t *testing.T, home string) {
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

func initLifecycleCredentialSource(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "source.git")
	work := filepath.Join(root, "source-work")
	runLifecycleCredentialGit(t, root, "init", "--bare", bare)
	runLifecycleCredentialGit(t, root, "init", "-b", "main", work)
	runLifecycleCredentialGit(t, work, "config", "user.email", "test@example.com")
	runLifecycleCredentialGit(t, work, "config", "user.name", "Test User")
	writeLifecycleCredentialFile(t, filepath.Join(work, "README.md"), "source\n")
	runLifecycleCredentialGit(t, work, "add", "README.md")
	runLifecycleCredentialGit(t, work, "commit", "-m", "source base")
	runLifecycleCredentialGit(t, work, "checkout", "-b", "contributor/feature")
	writeLifecycleCredentialFile(t, filepath.Join(work, "change.txt"), "contribution\n")
	runLifecycleCredentialGit(t, work, "add", "change.txt")
	runLifecycleCredentialGit(t, work, "commit", "-m", "contribution")
	sha := strings.TrimSpace(runLifecycleCredentialGit(t, work, "rev-parse", "HEAD"))
	runLifecycleCredentialGit(t, work, "remote", "add", "origin", bare)
	runLifecycleCredentialGit(t, work, "push", "origin", "main", "contributor/feature")
	return bare, sha
}

func runLifecycleCredentialGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func writeLifecycleCredentialFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newLifecycleCredentialProxy(t *testing.T, source string) (*httptest.Server, *httptest.Server) {
	t.Helper()
	files := http.FileServer(http.Dir(filepath.Dir(source)))
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		r.URL.Path = strings.Replace(r.URL.Path, "/acme/widgets.git", "/source.git", 1)
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
		bridgeLifecycleCredentialTunnel(upstream, downstream, buffer)
	}))
	return tlsServer, proxy
}

func bridgeLifecycleCredentialTunnel(upstream net.Conn, downstream net.Conn, buffer io.Reader) {
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

type lifecycleCredentialWorktreeStore struct {
	worktrees map[string]*worktree.Worktree
}

func (s *lifecycleCredentialWorktreeStore) CreateWorktree(_ context.Context, wt *worktree.Worktree) error {
	s.worktrees[wt.ID] = wt
	return nil
}

func (s *lifecycleCredentialWorktreeStore) GetWorktreeByID(_ context.Context, id string) (*worktree.Worktree, error) {
	return s.worktrees[id], nil
}

func (s *lifecycleCredentialWorktreeStore) GetWorktreeBySessionID(_ context.Context, sessionID string) (*worktree.Worktree, error) {
	for _, wt := range s.worktrees {
		if wt.SessionID == sessionID {
			return wt, nil
		}
	}
	return nil, nil
}

func (s *lifecycleCredentialWorktreeStore) GetWorktreesByTaskID(_ context.Context, taskID string) ([]*worktree.Worktree, error) {
	var result []*worktree.Worktree
	for _, wt := range s.worktrees {
		if wt.TaskID == taskID {
			result = append(result, wt)
		}
	}
	return result, nil
}

func (s *lifecycleCredentialWorktreeStore) GetWorktreesByRepositoryID(_ context.Context, repositoryID string) ([]*worktree.Worktree, error) {
	var result []*worktree.Worktree
	for _, wt := range s.worktrees {
		if wt.RepositoryID == repositoryID {
			result = append(result, wt)
		}
	}
	return result, nil
}

func (s *lifecycleCredentialWorktreeStore) UpdateWorktree(_ context.Context, wt *worktree.Worktree) error {
	s.worktrees[wt.ID] = wt
	return nil
}

func (s *lifecycleCredentialWorktreeStore) DeleteWorktree(_ context.Context, id string) error {
	delete(s.worktrees, id)
	return nil
}

func (s *lifecycleCredentialWorktreeStore) ListActiveWorktrees(_ context.Context) ([]*worktree.Worktree, error) {
	var result []*worktree.Worktree
	for _, wt := range s.worktrees {
		if wt.Status == worktree.StatusActive {
			result = append(result, wt)
		}
	}
	return result, nil
}

func (s *lifecycleCredentialWorktreeStore) ListActiveWorktreePaths(_ context.Context) ([]string, error) {
	var result []string
	for _, wt := range s.worktrees {
		if wt.Status == worktree.StatusActive && wt.Path != "" {
			result = append(result, wt.Path)
		}
	}
	return result, nil
}

func (s *lifecycleCredentialWorktreeStore) CountActiveWorktreeReferences(context.Context, string, []string) (int, error) {
	return 0, nil
}

func checkoutCredentialProducerEnvironment(brokerURL, helperPath, taskID, sessionID string) map[string]string {
	return map[string]string{
		githubauth.CredentialBrokerURLEnv:         brokerURL,
		githubauth.CredentialHelperPathEnv:        helperPath,
		githubauth.CredentialLeaseEnv:             "synthetic-lease",
		githubauth.CredentialReissueCapabilityEnv: "synthetic-capability",
		githubauth.CredentialTaskIDEnv:            taskID,
		githubauth.CredentialSessionIDEnv:         sessionID,
		githubauth.CredentialRepositoryEnv:        "repository-1",
		githubauth.CredentialOwnerEnv:             "acme",
		githubauth.CredentialRepoEnv:              "widgets",
		githubauth.CredentialHostEnv:              "github.com",
		githubauth.CredentialScopesEnv:            fmt.Sprintf(`[{"lease":"synthetic-lease","reissue_capability":"synthetic-capability","task_id":%q,"session_id":%q,"repository_id":"repository-1","owner":"acme","repo":"widgets","host":"github.com","path":"/acme/widgets.git"}]`, taskID, sessionID),
		// This indexed block is the executor's configureGitCredentialEnvironment output.
		"GIT_CONFIG_COUNT":   "4",
		"GIT_CONFIG_KEY_0":   "http.version",
		"GIT_CONFIG_VALUE_0": "HTTP/1.1",
		"GIT_CONFIG_KEY_1":   "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_1": "",
		"GIT_CONFIG_KEY_2":   "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_2": githubauth.ManagedGitCredentialHelper,
		"GIT_CONFIG_KEY_3":   globalCredentialUseHTTPPathKey,
		"GIT_CONFIG_VALUE_3": "true",
	}
}

func runIsolatedGitCredentialFill(t *testing.T, checkoutEnv map[string]string, path string) (string, error) {
	t.Helper()
	home := t.TempDir()
	cmd := exec.Command("git", "credential", "fill")
	cmd.Dir = home
	cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\npath=" + path + "\n\n")
	cmd.Env = isolatedGitCredentialEnvironment(home, checkoutEnv)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func isolatedGitCredentialEnvironment(home string, checkoutEnv map[string]string) []string {
	env := make([]string, 0, len(os.Environ())+len(checkoutEnv)+5)
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if strings.HasPrefix(key, "GIT_") || strings.HasPrefix(key, "KANDEV_GITHUB_") ||
			key == "HOME" || key == "XDG_CONFIG_HOME" || key == "GITHUB_TOKEN" || key == "GH_TOKEN" ||
			strings.Contains(strings.ToUpper(key), "_PROXY") {
			continue
		}
		env = append(env, value)
	}
	env = append(env,
		"HOME="+home,
		"XDG_CONFIG_HOME="+home,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+filepath.Join(home, "missing.gitconfig"),
		"GIT_TERMINAL_PROMPT=0",
	)
	for key, value := range checkoutEnv {
		env = append(env, key+"="+value)
	}
	return env
}
