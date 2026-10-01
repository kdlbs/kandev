package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/githubauth"
)

func TestCheckoutCredentialEnvironmentRealGitPath(t *testing.T) {
	helperPath := buildAgentctlCredentialFixture(t)
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

	input := checkoutCredentialProducerEnvironment(broker.URL, helperPath)
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

type checkoutCredentialBrokerRequest struct {
	RepositoryID string `json:"repository_id"`
	Path         string `json:"path"`
}

func checkoutCredentialProducerEnvironment(brokerURL, helperPath string) map[string]string {
	return map[string]string{
		githubauth.CredentialBrokerURLEnv:         brokerURL,
		githubauth.CredentialHelperPathEnv:        helperPath,
		githubauth.CredentialLeaseEnv:             "synthetic-lease",
		githubauth.CredentialReissueCapabilityEnv: "synthetic-capability",
		githubauth.CredentialTaskIDEnv:            "task-1",
		githubauth.CredentialSessionIDEnv:         "session-1",
		githubauth.CredentialRepositoryEnv:        "repository-1",
		githubauth.CredentialOwnerEnv:             "acme",
		githubauth.CredentialRepoEnv:              "widgets",
		githubauth.CredentialHostEnv:              "github.com",
		githubauth.CredentialScopesEnv:            `[{"lease":"synthetic-lease","reissue_capability":"synthetic-capability","task_id":"task-1","session_id":"session-1","repository_id":"repository-1","owner":"acme","repo":"widgets","host":"github.com","path":"/acme/widgets.git"}]`,
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

func buildAgentctlCredentialFixture(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve credential test source path")
	}
	backendDir := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../../"))
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
