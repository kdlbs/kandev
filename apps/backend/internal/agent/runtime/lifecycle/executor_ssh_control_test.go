package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/githubauth"
)

// Tests for the HTTP-over-SSH control plane (handshake + create-instance), the
// local port forwarder, and the environment actually forwarded to the remote
// agent instance.

func TestRemoteControlHandshake(t *testing.T) {
	t.Run("returns the issued bearer token", func(t *testing.T) {
		var gotPath, gotContentType, gotBody string
		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			gotPath, gotContentType, gotBody = r.URL.Path, r.Header.Get("Content-Type"), string(body)
			_, _ = io.WriteString(w, `{"token":"handshake-token"}`)
		}))
		defer httpServer.Close()

		server := newFakeSSHServer(t, nil)
		server.forwardTo(strings.TrimPrefix(httpServer.URL, "http://"))

		token, err := remoteControlHandshake(context.Background(), server.dial(t), 39429, "nonce-abc")
		if err != nil {
			t.Fatalf("remoteControlHandshake: %v", err)
		}
		if token != "handshake-token" {
			t.Fatalf("token = %q", token)
		}
		if gotPath != "/auth/handshake" {
			t.Fatalf("path = %q", gotPath)
		}
		if gotContentType != "application/json" {
			t.Fatalf("content type = %q", gotContentType)
		}
		if gotBody != `{"nonce":"nonce-abc"}` {
			t.Fatalf("body = %q", gotBody)
		}
	})

	t.Run("non-200 is an error", func(t *testing.T) {
		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer httpServer.Close()
		server := newFakeSSHServer(t, nil)
		server.forwardTo(strings.TrimPrefix(httpServer.URL, "http://"))

		_, err := remoteControlHandshake(context.Background(), server.dial(t), 39429, "nonce")
		if err == nil || !strings.Contains(err.Error(), "403") {
			t.Fatalf("error = %v, want the status code", err)
		}
	})

	t.Run("empty token is an error", func(t *testing.T) {
		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"token":""}`)
		}))
		defer httpServer.Close()
		server := newFakeSSHServer(t, nil)
		server.forwardTo(strings.TrimPrefix(httpServer.URL, "http://"))

		_, err := remoteControlHandshake(context.Background(), server.dial(t), 39429, "nonce")
		if err == nil || !strings.Contains(err.Error(), "no token") {
			t.Fatalf("error = %v, want missing-token error", err)
		}
	})

	t.Run("unreachable control port is an error", func(t *testing.T) {
		server := newFakeSSHServer(t, nil) // no forward target configured
		_, err := remoteControlHandshake(context.Background(), server.dial(t), 39429, "nonce")
		if err == nil || !strings.Contains(err.Error(), "agentctl handshake") {
			t.Fatalf("error = %v, want dial failure", err)
		}
	})
}

func TestCreateRemoteAgentInstancePostsTheBuiltRequest(t *testing.T) {
	var gotPath, gotAuth string
	var gotReq agentctl.CreateInstanceRequest
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		_, _ = io.WriteString(w, `{"id":"instance-1","port":41234}`)
	}))
	defer httpServer.Close()

	server := newFakeSSHServer(t, nil)
	server.forwardTo(strings.TrimPrefix(httpServer.URL, "http://"))

	req := &ExecutorCreateRequest{
		InstanceID:  "instance-1",
		TaskID:      "task-1",
		SessionID:   "session-1",
		Protocol:    "acp",
		McpMode:     "task",
		AgentConfig: &fakeIdentityAgent{MockAgent: agents.NewMockAgent(), id: "claude-code", name: "Claude Code"},
		Env:         map[string]string{envKeyAnthropicAPIKey: "sk-test"},
		Metadata: map[string]interface{}{
			MetadataKeyBaseBranches: map[string]string{"": "main"},
		},
	}

	port, err := createRemoteAgentInstance(context.Background(), server.dial(t),
		39429, "/remote/task", "/remote/agentctl", req, "bearer-token", newTestLogger())
	if err != nil {
		t.Fatalf("createRemoteAgentInstance: %v", err)
	}
	if port != 41234 {
		t.Fatalf("port = %d, want 41234", port)
	}
	if gotPath != "/api/v1/instances" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer bearer-token" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotReq.ID != "instance-1" || gotReq.SessionID != "session-1" || gotReq.TaskID != "task-1" {
		t.Fatalf("identity fields = %+v", gotReq)
	}
	if gotReq.WorkspacePath != "/remote/task" {
		t.Fatalf("WorkspacePath = %q, want the remote task dir (never the host path)", gotReq.WorkspacePath)
	}
	if gotReq.AgentType != "claude-code" || gotReq.Protocol != "acp" || gotReq.McpMode != "task" {
		t.Fatalf("agent routing fields = %+v", gotReq)
	}
	if gotReq.BaseBranches[""] != "main" {
		t.Fatalf("BaseBranches = %+v", gotReq.BaseBranches)
	}
	if gotReq.Env[envKeyAnthropicAPIKey] != "sk-test" {
		t.Fatalf("Env = %+v, want the allowlisted credential forwarded", gotReq.Env)
	}
}

func TestCreateRemoteAgentInstanceErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "server error carries the body", status: http.StatusInternalServerError, body: "boom", wantErr: "returned 500: boom"},
		{name: "unparseable body", status: http.StatusOK, body: "not json", wantErr: "parse create-instance response"},
		{name: "port zero is rejected", status: http.StatusOK, body: `{"id":"x","port":0}`, wantErr: "returned port 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer httpServer.Close()
			server := newFakeSSHServer(t, nil)
			server.forwardTo(strings.TrimPrefix(httpServer.URL, "http://"))

			_, err := createRemoteAgentInstance(context.Background(), server.dial(t), 39429,
				"/remote/task", "/remote/agentctl", &ExecutorCreateRequest{InstanceID: "i"}, "", newTestLogger())
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestBuildSSHCreateInstanceRequestMapsEveryField(t *testing.T) {
	autoApprove := true
	req := &ExecutorCreateRequest{
		InstanceID:                     "instance-9",
		TaskID:                         "task-9",
		SessionID:                      "session-9",
		Protocol:                       "acp",
		McpMode:                        "office",
		McpProviders:                   []string{"github"},
		AutoApprovePermissions:         false,
		AutoApprovePermissionsOverride: &autoApprove,
		AgentConfig:                    &fakeRuntimeAgent{MockAgent: agents.NewMockAgent(), id: "opencode", requiresKill: true, stripEnv: []string{"NODE_OPTIONS"}},
		Env:                            map[string]string{envKeyOpenAIAPIKey: "sk-1", "UNRELATED": "x"},
		Metadata: map[string]interface{}{
			MetadataKeyBaseBranches: map[string]string{"repo-a": "develop"},
		},
	}

	got := buildSSHCreateInstanceRequest(req, "/remote/task", "/remote/agentctl")

	if got.ID != "instance-9" || got.SessionID != "session-9" || got.TaskID != "task-9" {
		t.Fatalf("identity = %+v", got)
	}
	if got.WorkspacePath != "/remote/task" {
		t.Fatalf("WorkspacePath = %q", got.WorkspacePath)
	}
	if got.AgentType != "opencode" {
		t.Fatalf("AgentType = %q", got.AgentType)
	}
	if got.AutoApprovePermissions == nil || !*got.AutoApprovePermissions {
		t.Fatalf("AutoApprovePermissions = %v, want the profile override to win", got.AutoApprovePermissions)
	}
	if !got.RequiresProcessKill {
		t.Fatal("RequiresProcessKill must come from the agent runtime config")
	}
	if len(got.StripEnv) != 1 || got.StripEnv[0] != "NODE_OPTIONS" {
		t.Fatalf("StripEnv = %v", got.StripEnv)
	}
	if got.BaseBranches["repo-a"] != "develop" {
		t.Fatalf("BaseBranches = %+v", got.BaseBranches)
	}
	if got.McpMode != "office" || len(got.McpProviders) != 1 || got.McpProviders[0] != "github" {
		t.Fatalf("mcp routing = %q / %v", got.McpMode, got.McpProviders)
	}
	if got.Env[envKeyOpenAIAPIKey] != "sk-1" {
		t.Fatalf("Env = %+v, want the allowlisted key", got.Env)
	}
	if _, leaked := got.Env["UNRELATED"]; leaked {
		t.Fatalf("Env leaked a non-allowlisted key: %+v", got.Env)
	}
}

func TestSSHRemoteAgentEnvForwardsOnlyScopedCredentials(t *testing.T) {
	t.Run("nil request and nil env produce nil", func(t *testing.T) {
		if got := sshRemoteAgentEnv(nil); got != nil {
			t.Fatalf("sshRemoteAgentEnv(nil) = %+v", got)
		}
		if got := sshRemoteAgentEnv(&ExecutorCreateRequest{}); got != nil {
			t.Fatalf("sshRemoteAgentEnv(no env) = %+v", got)
		}
	})

	t.Run("only the credential allowlist is forwarded", func(t *testing.T) {
		env := sshRemoteAgentEnv(&ExecutorCreateRequest{Env: map[string]string{
			envKeyClaudeCodeOAuthToken: "oauth",
			envKeyGitHubToken:          "ghp",
			envKeyGitLabHost:           "gitlab.example",
			envKeyAnthropicAPIKey:      "", // empty must never clobber a remote value
			"HOME":                     "/root",
			"PATH":                     "/usr/bin",
		}})
		want := map[string]string{
			envKeyClaudeCodeOAuthToken: "oauth",
			envKeyGitHubToken:          "ghp",
			envKeyGitLabHost:           "gitlab.example",
		}
		if !equalStringMaps(env, want) {
			t.Fatalf("env = %+v, want %+v", env, want)
		}
	})

	t.Run("managed broker env and indexed git config ride along", func(t *testing.T) {
		env := sshRemoteAgentEnv(&ExecutorCreateRequest{Env: map[string]string{
			githubauth.CredentialBrokerURLEnv: "http://127.0.0.1:9/broker",
			githubauth.CredentialLeaseEnv:     "lease-1",
			"GIT_CONFIG_COUNT":                "1",
			"GIT_CONFIG_KEY_0":                "credential.helper",
			"GIT_CONFIG_VALUE_0":              "kandev",
		}})
		if env[githubauth.CredentialBrokerURLEnv] != "http://127.0.0.1:9/broker" ||
			env[githubauth.CredentialLeaseEnv] != "lease-1" {
			t.Fatalf("broker env not forwarded: %+v", env)
		}
		if env["GIT_CONFIG_COUNT"] != "1" || env["GIT_CONFIG_KEY_0"] != "credential.helper" ||
			env["GIT_CONFIG_VALUE_0"] != "kandev" {
			t.Fatalf("indexed git config not forwarded: %+v", env)
		}
	})

	t.Run("approved secret keys are additive and never override", func(t *testing.T) {
		env := sshRemoteAgentEnv(&ExecutorCreateRequest{
			Env: map[string]string{
				envKeyGitHubToken: "managed",
				"DEPLOY_KEY":      "approved-value",
				"EMPTY_KEY":       "",
				"bad key":         "ignored",
			},
			ApprovedSecretEnvKeys: []string{"DEPLOY_KEY", "EMPTY_KEY", "bad key", envKeyGitHubToken},
		})
		if env["DEPLOY_KEY"] != "approved-value" {
			t.Fatalf("approved key not forwarded: %+v", env)
		}
		if _, present := env["EMPTY_KEY"]; present {
			t.Fatalf("empty approved value must be skipped: %+v", env)
		}
		if _, present := env["bad key"]; present {
			t.Fatalf("non-POSIX identifier must be skipped: %+v", env)
		}
		if env[envKeyGitHubToken] != "managed" {
			t.Fatalf("approval must not replace the executor-selected credential: %+v", env)
		}
	})
}

func TestSSHAgentTypeFromReq(t *testing.T) {
	if got := sshAgentTypeFromReq(nil); got != "" {
		t.Fatalf("nil request agent type = %q", got)
	}
	if got := sshAgentTypeFromReq(&ExecutorCreateRequest{}); got != "" {
		t.Fatalf("missing agent config type = %q", got)
	}
	got := sshAgentTypeFromReq(&ExecutorCreateRequest{AgentConfig: newFakeIdentityAgent("codex")})
	if got != "codex" {
		t.Fatalf("agent type = %q, want codex", got)
	}
}

func TestStartPortForwardTunnelsToTheRemotePort(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = io.WriteString(w, "ok")
			return
		}
		w.WriteHeader(http.StatusTeapot)
	}))
	defer httpServer.Close()

	server := newFakeSSHServer(t, nil)
	var forwardedPort uint32
	server.tcpTarget = func(port uint32) string {
		forwardedPort = port
		return strings.TrimPrefix(httpServer.URL, "http://")
	}

	fwd, err := StartPortForward(server.dial(t), 41234, newTestLogger())
	if err != nil {
		t.Fatalf("StartPortForward: %v", err)
	}
	defer func() { _ = fwd.Close() }()

	if fwd.LocalPort() == 0 {
		t.Fatal("LocalPort must be a real bound port")
	}
	if err := waitAgentctlHealthy(context.Background(), fwd.LocalPort(), 5*time.Second); err != nil {
		t.Fatalf("waitAgentctlHealthy through the forward: %v", err)
	}
	if forwardedPort != 41234 {
		t.Fatalf("forwarded remote port = %d, want 41234", forwardedPort)
	}

	if err := fwd.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := fwd.Close(); err != nil {
		t.Fatalf("Close must be idempotent, got %v", err)
	}
}

func TestWaitAgentctlHealthyFailsOnNonSuccessStatus(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer httpServer.Close()

	server := newFakeSSHServer(t, nil)
	server.forwardTo(strings.TrimPrefix(httpServer.URL, "http://"))
	fwd, err := StartPortForward(server.dial(t), 41234, newTestLogger())
	if err != nil {
		t.Fatalf("StartPortForward: %v", err)
	}
	defer func() { _ = fwd.Close() }()

	// waitAgentctlHealthy sleeps 200ms between attempts, so the deadline has to
	// span at least two windows for the retry loop itself to be exercised.
	err = waitAgentctlHealthy(context.Background(), fwd.LocalPort(), 600*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("error = %v, want unreachable after the deadline", err)
	}
}

func TestWaitAgentctlHealthyHonoursContextCancellation(t *testing.T) {
	// Port 1 on loopback refuses connections, so the probe loop always retries
	// and only the context can end it.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitAgentctlHealthy(ctx, 1, 5*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func equalStringMaps(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}
