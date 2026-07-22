package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type recordingSpriteReconnectControl struct {
	calls   []string
	created *ExecutorCreateRequest
}

func (c *recordingSpriteReconnectControl) Delete(_ context.Context, _ string) error {
	c.calls = append(c.calls, "DELETE")
	return nil
}

func (c *recordingSpriteReconnectControl) Create(
	_ context.Context,
	req *ExecutorCreateRequest,
) (int, error) {
	c.calls = append(c.calls, "POST")
	c.created = req
	return 41002, nil
}

func TestRewriteGitHubSSHToHTTPS(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "scp style",
			in:   "git@github.com:kdlbs/agents-protocol-debug.git",
			want: "https://github.com/kdlbs/agents-protocol-debug.git",
		},
		{
			name: "ssh scheme",
			in:   "ssh://git@github.com/kdlbs/agents-protocol-debug.git",
			want: "https://github.com/kdlbs/agents-protocol-debug.git",
		},
		{
			name: "non github",
			in:   "git@gitlab.com:org/repo.git",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, rewriteGitHubSSHToHTTPS(tc.in))
		})
	}
}

func TestInjectTokenIntoURL_RewritesGitHubSSHWhenTokenExists(t *testing.T) {
	env := map[string]string{"GITHUB_TOKEN": "test-token"}
	got := injectGitHubTokenIntoCloneURL("git@github.com:kdlbs/agents-protocol-debug.git", env)
	require.Equal(t, "https://x-access-token:test-token@github.com/kdlbs/agents-protocol-debug.git", got)
}

func TestInjectTokenIntoURL_HonoursGHTokenFallback(t *testing.T) {
	// Sprites used to ignore GH_TOKEN; since unifying with Docker, both env
	// var names are accepted (GITHUB_TOKEN wins when both are set).
	env := map[string]string{"GH_TOKEN": "gh-token"}
	got := injectGitHubTokenIntoCloneURL("https://github.com/org/repo.git", env)
	require.Equal(t, "https://x-access-token:gh-token@github.com/org/repo.git", got)
}

func TestInjectTokenIntoURL_NeverEmbedsWorkspaceGitLabToken(t *testing.T) {
	env := map[string]string{
		"GITLAB_TOKEN":       "glpat-workspace",
		"KANDEV_GITLAB_HOST": "http://gitlab.internal:8080",
	}
	got := injectGitHubTokenIntoCloneURL("http://gitlab.internal:8080/group/repo.git", env)
	require.Equal(t, "http://gitlab.internal:8080/group/repo.git", got)
	require.NotContains(t, got, "glpat-workspace")
}

func TestInjectTokenIntoURL_NeverUsesGitLabTokenAcrossHosts(t *testing.T) {
	env := map[string]string{
		"GITLAB_TOKEN":       "glpat-workspace",
		"KANDEV_GITLAB_HOST": "https://gitlab.internal",
	}
	got := injectGitHubTokenIntoCloneURL("https://gitlab.com/group/repo.git", env)
	require.Equal(t, "https://gitlab.com/group/repo.git", got)
}

func TestIsTransientUploadError(t *testing.T) {
	require.True(t, isTransientUploadError(errors.New("request canceled (Client.Timeout exceeded while awaiting headers)")))
	require.True(t, isTransientUploadError(errors.New("connection reset by peer")))
	require.True(t, isTransientUploadError(errors.New("write /usr/local/bin/agentctl: HTTP 502")))
	require.True(t, isTransientUploadError(errors.New("upload failed: status 429")))
	require.False(t, isTransientUploadError(errors.New("permission denied")))
	require.False(t, isTransientUploadError(errors.New("upload failed: HTTP 400")))
}

func TestSpriteCreateInstanceRequestCarriesRefreshedEnvironment(t *testing.T) {
	req := &ExecutorCreateRequest{
		InstanceID: "instance-1",
		TaskID:     "task-1",
		SessionID:  "session-1",
		Env: map[string]string{
			envKeyGitHubCredentialBrokerURL: "https://kandev.example/api/v1/github/credentials/resolve",
			envKeyGitHubCredentialLease:     "new-lease-after-restart",
			"GIT_CONFIG_COUNT":              "1",
			"GIT_CONFIG_KEY_0":              "credential.https://github.com.helper",
			"GIT_CONFIG_VALUE_0":            "!agentctl git-credential",
			"GITHUB_TOKEN":                  "explicit-profile-token",
		},
	}

	got := spriteCreateInstanceRequest(req)
	if got.Env[envKeyGitHubCredentialLease] != "new-lease-after-restart" {
		t.Fatalf("lease = %q, want refreshed lease", got.Env[envKeyGitHubCredentialLease])
	}
	if got.Env["GIT_CONFIG_VALUE_0"] != "!agentctl git-credential" {
		t.Fatalf("git config = %q", got.Env["GIT_CONFIG_VALUE_0"])
	}
	if got.Env["GITHUB_TOKEN"] != "explicit-profile-token" {
		t.Fatalf("explicit profile token = %q", got.Env["GITHUB_TOKEN"])
	}
	req.Env[envKeyGitHubCredentialLease] = "mutated"
	if got.Env[envKeyGitHubCredentialLease] != "new-lease-after-restart" {
		t.Fatal("CreateInstanceRequest env aliases the mutable launch request")
	}
}

func TestSpritesBrokerReconnectRequiresCredentialRefresh(t *testing.T) {
	managed := map[string]string{
		envKeyGitHubCredentialBrokerURL: "https://kandev.example/api/v1/github/credentials/resolve",
		envKeyGitHubCredentialLease:     "new-lease",
	}
	if !hasManagedGitHubBrokerEnv(managed) {
		t.Fatal("managed broker reconnect must refresh the running subprocess")
	}
	control := &recordingSpriteReconnectControl{}
	req := &ExecutorCreateRequest{Env: managed}
	port, err := replaceSpriteReconnectInstance(context.Background(), control, req, "instance-1")
	if err != nil {
		t.Fatalf("replaceSpriteReconnectInstance() error = %v", err)
	}
	if port != 41002 || strings.Join(control.calls, ",") != "DELETE,POST" {
		t.Fatalf("replacement result: port=%d calls=%v", port, control.calls)
	}
	if control.created.Env[envKeyGitHubCredentialLease] != "new-lease" {
		t.Fatalf("replacement lease = %q", control.created.Env[envKeyGitHubCredentialLease])
	}
	if hasManagedGitHubBrokerEnv(map[string]string{"GITHUB_TOKEN": "explicit-profile-token"}) {
		t.Fatal("explicit profile token reconnect must preserve existing behavior")
	}
}
