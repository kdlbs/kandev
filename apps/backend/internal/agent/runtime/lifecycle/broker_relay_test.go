package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/task/models"
)

const testBrokerURL = "http://backend.example:8080/api/v1/git/credentials/resolve"

func TestSameHostBrokerUpstream(t *testing.T) {
	env := map[string]string{githubauth.CredentialBrokerURLEnv: testBrokerURL}
	cases := map[string]string{
		string(models.ExecutorTypeLocal):        testBrokerURL,
		string(models.ExecutorTypeWorktree):     testBrokerURL,
		string(models.ExecutorTypeLocalDocker):  "",
		string(models.ExecutorTypeRemoteDocker): "",
		string(models.ExecutorTypeSprites):      "",
		string(models.ExecutorTypeKubernetes):   "",
		string(models.ExecutorTypeSSH):          "",
	}
	for executorType, want := range cases {
		if got := sameHostBrokerUpstream(executorType, env); got != want {
			t.Errorf("%s upstream = %q, want %q", executorType, got, want)
		}
	}
	if got := sameHostBrokerUpstream(string(models.ExecutorTypeLocal), map[string]string{}); got != "" {
		t.Errorf("no broker env upstream = %q, want empty", got)
	}
}

func TestWithBrokerRelayURL(t *testing.T) {
	env := map[string]string{githubauth.CredentialBrokerURLEnv: testBrokerURL, "A": "b"}
	got := withBrokerRelayURL(string(models.ExecutorTypeLocal), env, 4321)
	if got[githubauth.CredentialBrokerURLEnv] != "http://127.0.0.1:4321/git-credentials/resolve" || got["A"] != "b" {
		t.Fatalf("relay env = %#v", got)
	}
	if env[githubauth.CredentialBrokerURLEnv] != testBrokerURL {
		t.Fatal("input env was modified")
	}
	if out := withBrokerRelayURL(string(models.ExecutorTypeSSH), env, 4321); out[githubauth.CredentialBrokerURLEnv] != testBrokerURL {
		t.Fatalf("ssh env rewritten: %#v", out)
	}
	if out := withBrokerRelayURL(string(models.ExecutorTypeLocal), env, 0); out[githubauth.CredentialBrokerURLEnv] != testBrokerURL {
		t.Fatalf("no port env rewritten: %#v", out)
	}
}

// An instance created without an upstream has no relay route, so a passthrough
// started on it keeps the operator broker URL even with the policy enabled.
func TestPassthroughEnvKeepsBrokerURLWhenInstanceHasNoRelay(t *testing.T) {
	build := func(noRelay bool) string {
		mgr := &Manager{logger: newTestLogger(), sessionMCPPolicy: NewSessionMCPPolicy(true)}
		execution := &AgentExecution{
			ID: "exec-1", TaskID: "task-1", SessionID: "session-1", AgentProfileID: "profile-1",
			ExecutorType: string(models.ExecutorTypeLocal), standalonePort: 4321, noBrokerRelay: noRelay,
		}
		execution.setRuntimeEnvironment(map[string]string{githubauth.CredentialBrokerURLEnv: testBrokerURL})
		env, err := mgr.buildPassthroughEnv(context.Background(), execution, nil)
		if err != nil {
			t.Fatalf("buildPassthroughEnv: %v", err)
		}
		return env[githubauth.CredentialBrokerURLEnv]
	}
	if got := build(true); got != testBrokerURL {
		t.Fatalf("no-relay instance broker URL = %q, want %q", got, testBrokerURL)
	}
	if got := build(false); got == testBrokerURL || got == "" {
		t.Fatalf("relay instance broker URL = %q, want the instance relay", got)
	}
}
