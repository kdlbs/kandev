package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func isolateGlobalGitConfig(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	// The shell client inherits the test environment; a global config override
	// there would redirect `git config --global` away from the remote home.
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	if err := os.Unsetenv("GIT_CONFIG_GLOBAL"); err != nil {
		t.Fatal(err)
	}
}

func TestApplyPluginExecutorGitIdentityWritesGlobalConfigInRemoteHome(t *testing.T) {
	isolateGlobalGitConfig(t)
	remoteHome := t.TempDir()
	name, email := `Ada O'Brien "Lovelace"`, "ada@example.test"
	client := &shellProcessClient{}
	t.Cleanup(func() { stopAll(client) })

	executor := NewPluginRemoteExecutor(nil, newTestLogger())
	executor.applyPluginExecutorGitIdentity(context.Background(), client, &ExecutorCreateRequest{
		SessionID: "session-1",
		Metadata: map[string]interface{}{
			MetadataKeyGitUserName:    name,
			MetadataKeyGitUserEmail:   email,
			MetadataKeyRemoteAuthHome: remoteHome,
		},
	})

	for key, want := range map[string]string{"user.name": name, "user.email": email} {
		out, err := exec.Command("git", "config", "--file", filepath.Join(remoteHome, ".gitconfig"), key).Output()
		if err != nil {
			t.Fatalf("read %s from the remote home: %v", key, err)
		}
		if got := strings.TrimSpace(string(out)); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	for _, command := range client.commands {
		if strings.Contains(command, email) || strings.Contains(command, "Lovelace") {
			t.Fatalf("command line carries the identity: %q", command)
		}
	}
}

func TestApplyPluginExecutorGitIdentityNeedsNameAndEmail(t *testing.T) {
	for name, metadata := range map[string]map[string]interface{}{
		"neither":    {},
		"name only":  {MetadataKeyGitUserName: "Ada"},
		"email only": {MetadataKeyGitUserEmail: "ada@example.test"},
	} {
		t.Run(name, func(t *testing.T) {
			client := &shellProcessClient{}
			executor := NewPluginRemoteExecutor(nil, newTestLogger())
			executor.applyPluginExecutorGitIdentity(context.Background(), client, &ExecutorCreateRequest{
				SessionID: "session-1", Metadata: metadata,
			})
			if len(client.commands) != 0 {
				t.Fatalf("commands = %v, want none", client.commands)
			}
		})
	}
}
