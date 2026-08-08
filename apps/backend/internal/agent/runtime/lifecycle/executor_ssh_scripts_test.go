package lifecycle

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestSSHResolvePrepareScriptUsesWorkspaceProviders(t *testing.T) {
	executor := &SSHExecutor{}
	req := &ExecutorCreateRequest{
		TaskID: "task-1",
		Metadata: map[string]interface{}{
			MetadataKeySetupScript:     "printf setup > {{workspace.path}}/custom-marker",
			"repository_clone_url":     "https://github.com/acme/widgets.git",
			MetadataKeyBaseBranch:      "main",
			MetadataKeyWorktreeBranch:  "feature/task-1",
			MetadataKeyRepoSetupScript: "printf repo > repo-marker",
		},
	}

	script, err := executor.resolvePrepareScript(req, "/remote/task", "/remote/bin/agentctl")
	if err != nil {
		t.Fatalf("resolvePrepareScript() error = %v", err)
	}
	for _, want := range []string{
		"/remote/task",
		"custom-marker",
		"kandev-managed: ensure session feature branch is checked out",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("resolved SSH prepare script missing %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "{{") {
		t.Fatalf("resolved SSH prepare script still contains placeholders:\n%s", script)
	}
	if strings.Contains(script, "agentctl --") || strings.Contains(script, "chmod +x /usr/local/bin/agentctl") {
		t.Fatalf("SSH prepare script must not start or install a duplicate agentctl:\n%s", script)
	}
}

func TestSSHDefaultPrepareScriptExecutesCheckoutAndSetup(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	seed := filepath.Join(root, "seed")
	workspace := filepath.Join(root, "workspace")
	runIn(t, root, "git", "init", "--quiet", "--bare", "--initial-branch=main", origin)
	runIn(t, root, "git", "init", "--quiet", "--initial-branch=main", seed)
	runIn(t, seed, "git", "config", "user.email", "test@example.com")
	runIn(t, seed, "git", "config", "user.name", "Test")
	runIn(t, seed, "sh", "-c", "printf repository > README.md")
	runIn(t, seed, "git", "add", "README.md")
	runIn(t, seed, "git", "commit", "--quiet", "-m", "init")
	runIn(t, seed, "git", "remote", "add", "origin", origin)
	runIn(t, seed, "git", "push", "--quiet", "origin", "main")
	runIn(t, root, "mkdir", "-p", filepath.Join(workspace, ".kandev", "sessions", "session-1"))

	executor := &SSHExecutor{}
	req := &ExecutorCreateRequest{
		TaskID: "task-1",
		Metadata: map[string]interface{}{
			"repository_clone_url":     origin,
			MetadataKeyBaseBranch:      "main",
			MetadataKeyWorktreeBranch:  "feature/task-1",
			MetadataKeyRepoSetupScript: "printf setup > setup-marker",
		},
	}
	script, err := executor.resolvePrepareScript(req, workspace, "/remote/bin/agentctl")
	if err != nil {
		t.Fatalf("resolvePrepareScript() error = %v", err)
	}
	cmd := exec.Command("bash", "-e", "-c", script)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("SSH prepare script failed: %v\n%s\nscript:\n%s", err, output, script)
	}

	if got := strings.TrimSpace(string(runIn(t, workspace, "git", "branch", "--show-current"))); got != "feature/task-1" {
		t.Fatalf("prepared branch = %q, want feature/task-1", got)
	}
	if got := strings.TrimSpace(string(runIn(t, workspace, "git", "remote", "get-url", "origin"))); got != origin {
		t.Fatalf("prepared origin = %q, want %q", got, origin)
	}
	if got := strings.TrimSpace(string(runIn(t, workspace, "cat", "README.md"))); got != "repository" {
		t.Fatalf("prepared repository content = %q, want repository", got)
	}
	if got := strings.TrimSpace(string(runIn(t, workspace, "cat", "setup-marker"))); got != "setup" {
		t.Fatalf("setup marker = %q, want setup", got)
	}
	if got := strings.TrimSpace(string(runIn(t, workspace, "cat", filepath.Join(".git", "info", "exclude")))); !strings.Contains(got, "/.kandev/") {
		t.Fatalf("git exclude = %q, want /.kandev/", got)
	}

	runIn(t, workspace, "sh", "-c", "printf local > local.txt")
	reuseCmd := exec.Command("bash", "-e", "-c", script)
	reuseCmd.Dir = root
	if output, err := reuseCmd.CombinedOutput(); err != nil {
		t.Fatalf("reused SSH prepare script failed: %v\n%s", err, output)
	}
	if got := strings.TrimSpace(string(runIn(t, workspace, "cat", "local.txt"))); got != "local" {
		t.Fatalf("reused workspace lost local file: %q", got)
	}
}

func TestSSHDefaultPrepareScriptRejectsConflictingOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	runIn(t, root, "git", "init", "--quiet", workspace)
	runIn(t, workspace, "git", "remote", "add", "origin", "https://github.com/acme/other.git")

	executor := &SSHExecutor{}
	req := &ExecutorCreateRequest{Metadata: map[string]interface{}{
		"repository_clone_url": originURLForTest,
		MetadataKeyBaseBranch:  "main",
	}}
	script, err := executor.resolvePrepareScript(req, workspace, "/remote/bin/agentctl")
	if err != nil {
		t.Fatalf("resolvePrepareScript() error = %v", err)
	}
	if output, err := exec.Command("bash", "-e", "-c", script).CombinedOutput(); err == nil {
		t.Fatalf("conflicting origin unexpectedly succeeded; output: %s", output)
	}
}

func TestSSHCleanupScriptResolvesRemoteWorkspace(t *testing.T) {
	executor := &SSHExecutor{}
	metadata := map[string]interface{}{
		MetadataKeyCleanupScript: "printf cleanup > {{workspace.path}}/cleanup-marker",
		MetadataKeySSHHost:       "remote.example",
	}
	resolved, err := executor.resolveSSHScript(&ExecutorCreateRequest{Metadata: metadata}, "/remote/task", "", metadata[MetadataKeyCleanupScript].(string))
	if err != nil {
		t.Fatalf("resolveSSHScript() error = %v", err)
	}
	if !strings.Contains(resolved, "printf cleanup > '/remote/task'/cleanup-marker") {
		t.Fatalf("cleanup workspace placeholder not resolved: %s", resolved)
	}
	if strings.Contains(resolved, "{{") {
		t.Fatalf("cleanup script still contains placeholders: %s", resolved)
	}
}

func TestSSHStopInstanceRunsTerminalCleanupBeforeControllerTeardown(t *testing.T) {
	tests := []struct {
		name             string
		reason           string
		wantCleanup      bool
		wantStop         bool
		cleanupErr       error
		wantCleanupError bool
	}{
		{
			name:        "task deletion runs cleanup",
			reason:      StopReasonTaskDeleted,
			wantCleanup: true,
			wantStop:    true,
		},
		{
			name:        "task tree archive runs cleanup",
			reason:      StopReasonTaskTreeArchived,
			wantCleanup: true,
			wantStop:    true,
		},
		{
			name:        "cascade delete runs cleanup",
			reason:      StopReasonCascadeDelete,
			wantCleanup: true,
			wantStop:    true,
		},
		{
			name:             "cleanup failure still stops controller",
			reason:           StopReasonSessionArchived,
			wantCleanup:      true,
			wantStop:         true,
			cleanupErr:       errors.New("cleanup failed"),
			wantCleanupError: true,
		},
		{
			name:     "ordinary stop preserves workspace without cleanup",
			reason:   "stopped via API",
			wantStop: true,
		},
		{
			name:   "backend shutdown skips cleanup and remote stop",
			reason: StopReasonBackendShutdown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			executor := NewSSHExecutor(nil, nil, nil, logger.Default())
			var cleanupCalls, stopCalls, closeCalls int
			var cleanupTaskDir, cleanupReason string
			var order []string
			executor.cleanupScript = func(
				_ context.Context,
				_ *ssh.Client,
				taskDir string,
				_ map[string]interface{},
				_ map[string]string,
				_ SSHRemotePlatform,
				_ string,
				reason string,
			) error {
				cleanupCalls++
				order = append(order, "cleanup")
				cleanupTaskDir = taskDir
				cleanupReason = reason
				return tc.cleanupErr
			}
			executor.stopRemote = func(context.Context, *ssh.Client, string, int) error {
				stopCalls++
				order = append(order, "stop")
				return nil
			}
			executor.closeClient = func(*ssh.Client) error {
				closeCalls++
				order = append(order, "close")
				return nil
			}
			executor.sessions["instance-1"] = &sshSessionState{
				client:        &ssh.Client{},
				remoteDir:     "/remote/session",
				remoteTaskDir: "/remote/task",
				metadata:      map[string]interface{}{MetadataKeySSHRemoteTaskDir: "/wrong/metadata/task"},
				prepareEnv:    map[string]string{"GITHUB_TOKEN": "secret"},
			}

			err := executor.StopInstance(context.Background(), &ExecutorInstance{
				InstanceID: "instance-1",
				StopReason: tc.reason,
			}, false)
			if err != nil {
				t.Fatalf("StopInstance() error = %v", err)
			}
			if got := cleanupCalls > 0; got != tc.wantCleanup {
				t.Fatalf("cleanup called = %v, want %v", got, tc.wantCleanup)
			}
			if tc.wantCleanup {
				if cleanupTaskDir != "/remote/task" {
					t.Fatalf("cleanup task directory = %q, want /remote/task", cleanupTaskDir)
				}
				if cleanupReason != tc.reason {
					t.Fatalf("cleanup reason = %q, want %q", cleanupReason, tc.reason)
				}
			}
			if got := stopCalls > 0; got != tc.wantStop {
				t.Fatalf("remote stop called = %v, want %v", got, tc.wantStop)
			}
			if closeCalls != 1 {
				t.Fatalf("SSH client close calls = %d, want 1", closeCalls)
			}
			if tc.wantCleanupError && cleanupCalls != 1 {
				t.Fatalf("cleanup error path calls = %d, want 1", cleanupCalls)
			}
			if tc.wantCleanup {
				if len(order) < 3 || order[0] != "cleanup" {
					t.Fatalf("teardown order = %v, want cleanup before stop/close", order)
				}
			}
		})
	}
}

const originURLForTest = "https://github.com/acme/widgets.git"
