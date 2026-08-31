package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestManagerTrackerGitEnvironmentUsesInstanceEnvironment(t *testing.T) {
	t.Setenv("KANDEV_TEST_TRACKER_CREDENTIAL", "ambient-credential")
	t.Setenv("KANDEV_TEST_AMBIENT_ONLY", "ambient-only")
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	t.Setenv("GCM_INTERACTIVE", "Always")
	t.Setenv("GIT_ASKPASS", "ambient-askpass")
	t.Setenv("SSH_ASKPASS", "ambient-ssh-askpass")
	t.Setenv("GIT_SSH_COMMAND", "ambient-ssh")

	instanceEnv := []string{
		"KANDEV_TEST_TRACKER_CREDENTIAL=instance-credential",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.extraheader",
		"GIT_CONFIG_VALUE_0=Authorization: Bearer instance-token",
		"GIT_TERMINAL_PROMPT=1",
		"GCM_INTERACTIVE=Always",
		"GIT_ASKPASS=instance-askpass",
		"SSH_ASKPASS=instance-ssh-askpass",
	}
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:  t.TempDir(),
		AgentEnv: instanceEnv,
	}, newTestLogger(t))
	t.Cleanup(mgr.stopWorkspaceTrackers)

	cmd := mgr.GetWorkspaceTracker().gitCommand(context.Background(), false, "status")
	env := environmentMap(cmd.Env)

	want := map[string]string{
		"KANDEV_TEST_TRACKER_CREDENTIAL": "instance-credential",
		"GIT_CONFIG_COUNT":               "1",
		"GIT_CONFIG_KEY_0":               "http.extraheader",
		"GIT_CONFIG_VALUE_0":             "Authorization: Bearer instance-token",
		"GIT_TERMINAL_PROMPT":            "0",
		"GCM_INTERACTIVE":                "Never",
		"GIT_ASKPASS":                    "echo",
		"SSH_ASKPASS":                    "/bin/false",
		"GIT_SSH_COMMAND":                "ssh -oBatchMode=yes",
	}
	for key, wantValue := range want {
		if got := env[key]; got != wantValue {
			t.Errorf("%s = %q, want %q", key, got, wantValue)
		}
	}
	if _, ok := env["KANDEV_TEST_AMBIENT_ONLY"]; ok {
		t.Fatal("ambient tracker credential marker unexpectedly present")
	}
	if env["GIT_SSH_COMMAND"] == "ambient-ssh" {
		t.Fatal("ambient SSH command unexpectedly reached tracker")
	}
}

func TestWorkspaceGitEnvironmentForcesExplicitSSHBatchMode(t *testing.T) {
	tracker := NewWorkspaceTracker(t.TempDir(), newTestLogger(t))
	tracker.SetGitEnvironment([]string{
		"GIT_SSH_COMMAND=ssh -i /instance/key -oBatchMode=no",
	})

	regular := environmentMap(tracker.gitCommand(context.Background(), false, "fetch").Env)
	polling := environmentMap(tracker.pollingGitCommand(context.Background(), "status").Env)
	for name, env := range map[string]map[string]string{
		"regular": regular,
		"polling": polling,
	} {
		want := "ssh -oBatchMode=yes -i /instance/key -oBatchMode=no"
		if got := env["GIT_SSH_COMMAND"]; got != want {
			t.Errorf("%s GIT_SSH_COMMAND = %q, want %q", name, got, want)
		}
	}
}

func TestForceGitSSHBatchModeSupportsDirectOpenSSHOnly(t *testing.T) {
	tests := map[string]string{
		"ssh -i /instance/key -oBatchMode=no": "ssh -oBatchMode=yes -i /instance/key -oBatchMode=no",
		`'path with spaces/ssh' -i key`:       `'path with spaces/ssh' -oBatchMode=yes -i key`,
		`env FOO=bar ssh -i key`:              defaultGitSSHCommand,
		`FOO=bar ssh -i key`:                  defaultGitSSHCommand,
		`exec ssh -i key`:                     defaultGitSSHCommand,
		`plink -i key`:                        defaultGitSSHCommand,
	}
	for command, want := range tests {
		if got := forceGitSSHBatchMode(command); got != want {
			t.Errorf("forceGitSSHBatchMode(%q) = %q, want %q", command, got, want)
		}
	}
}

func TestWorkspaceGitEnvironmentSnapshotsAreDetached(t *testing.T) {
	instanceEnv := []string{"KANDEV_TEST_TRACKER_VALUE=initial"}
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "repo"), 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:  workspace,
		AgentEnv: instanceEnv,
	}, newTestLogger(t))
	t.Cleanup(mgr.stopWorkspaceTrackers)

	root := mgr.GetWorkspaceTracker()
	lazy, err := mgr.GetWorkspaceTrackerFor("repo")
	if err != nil {
		t.Fatalf("lazy tracker: %v", err)
	}

	instanceEnv[0] = "KANDEV_TEST_TRACKER_VALUE=source-mutated"
	root.SetGitEnvironment([]string{"KANDEV_TEST_TRACKER_VALUE=root-only"})

	if got := environmentMap(root.gitCommand(context.Background(), false, "status").Env)["KANDEV_TEST_TRACKER_VALUE"]; got != "root-only" {
		t.Fatalf("root tracker value = %q, want root-only", got)
	}
	if got := environmentMap(lazy.gitCommand(context.Background(), false, "status").Env)["KANDEV_TEST_TRACKER_VALUE"]; got != "initial" {
		t.Fatalf("lazy tracker value = %q, want initial", got)
	}
}

func environmentMap(env []string) map[string]string {
	result := make(map[string]string, len(env))
	for _, entry := range env {
		for i := 0; i < len(entry); i++ {
			if entry[i] != '=' {
				continue
			}
			result[entry[:i]] = entry[i+1:]
			break
		}
	}
	return result
}
