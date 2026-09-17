package lifecycle

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestSSHOfficeProxyRequiresRunTokenAndPreservesAPIPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/office/runtime/comments" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL + "/api/v1")
	handler := sshManagedProxy(target, "run-token")
	for _, tc := range []struct {
		path, token string
		status      int
	}{
		{"/api/v1/office/runtime/comments", "", http.StatusUnauthorized},
		{"/api/v1/office/runtime/comments", "other-token", http.StatusUnauthorized},
		{"/api/v1/tasks", "run-token", http.StatusForbidden},
		{"/api/v1/office/runtime/comments", "run-token", http.StatusCreated},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, nil)
		req.Header.Set("Authorization", "Bearer "+tc.token)
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		if out.Code != tc.status {
			t.Fatalf("%s: status %d, want %d", tc.path, out.Code, tc.status)
		}
	}
}

func TestSSHOfficeEnvUsesRunIdentityAndRemoteCLI(t *testing.T) {
	req := &ExecutorCreateRequest{Env: map[string]string{
		"KANDEV_RUN_TOKEN": "run-token", "KANDEV_API_KEY": "run-token",
		"KANDEV_API_URL": "http://127.0.0.1:4567/api/v1", "KANDEV_TASK_ID": "work-task",
		"KANDEV_CLI": "/local/agentctl", "UNRELATED_SECRET": "do-not-forward",
		"KANDEV_RUNTIME_API_PREFIX": "/api/v1/orchestration",
	}}
	got := sshRemoteContributionEnv(req, "/remote/agentctl")
	if got["KANDEV_RUN_TOKEN"] != "run-token" || got["KANDEV_TASK_ID"] != "work-task" || got["KANDEV_CLI"] != "/remote/agentctl" {
		t.Fatal("Runtime identity or remote CLI missing")
	}
	if got["KANDEV_RUNTIME_API_PREFIX"] != "/api/v1/orchestration" {
		t.Fatal("runtime API prefix missing on SSH executor")
	}
	if _, ok := got["UNRELATED_SECRET"]; ok {
		t.Fatal("unrelated secret forwarded")
	}
	delete(req.Env, "KANDEV_RUN_TOKEN")
	if got := sshRemoteAgentEnv(req); got["KANDEV_API_KEY"] != "" {
		t.Fatal("Office identity forwarded without run token")
	}
}

func TestSSHOfficeRuntimeInitialAndRefreshedAgentEnv(t *testing.T) {
	runtime := &sshManagedRuntime{apiURL: "http://127.0.0.1:4567/api/v1", cli: "/remote/agentctl"}
	runtime.token.Store("initial")
	initial := map[string]string{}
	if err := runtime.prepareEnv(initial); err != nil {
		t.Fatalf("initial subprocess inherits bootstrap identity: %v", err)
	}
	if initial["KANDEV_RUN_TOKEN"] != "initial" {
		t.Fatal("initial identity missing")
	}
	refreshed := map[string]string{"KANDEV_RUN_TOKEN": "next", "KANDEV_API_KEY": "next", "KANDEV_RUN_ID": "next-run"}
	if err := runtime.prepareEnv(refreshed); err != nil {
		t.Fatal(err)
	}
	if refreshed["KANDEV_CLI"] != "/remote/agentctl" || runtime.token.Load() != "next" {
		t.Fatal("run identity or remote CLI not refreshed")
	}
}

func TestSSHResumedInstanceRetainsOfficeEnvironmentBinding(t *testing.T) {
	executor := &SSHExecutor{logger: newTestLogger()}
	state := &sshSessionState{
		target: &SSHTarget{Host: "remote"}, forwarder: &SSHPortForwarder{localPort: 4567},
		prepareAgentEnv: func(env map[string]string) error { env["KANDEV_CLI"] = "/remote/agentctl"; return nil },
	}
	instance := executor.buildResumedInstance(&ExecutorCreateRequest{}, state)
	if instance.PrepareAgentEnv == nil {
		t.Fatal("reused SSH instance lost its run environment binding")
	}
}
