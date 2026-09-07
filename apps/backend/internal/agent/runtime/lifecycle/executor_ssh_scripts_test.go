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
	"github.com/kandev/kandev/internal/task/models"
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

func TestSSHResolvePrepareScriptUsesRemoteWorkspaceForContributionDestination(t *testing.T) {
	destination := models.ContributionDestination{
		Version:  models.ContributionDestinationVersion,
		Provider: models.ContributionDestinationProviderGitHub,
		SourceRepository: models.ContributionDestinationRepository{
			Host: "github.com", Path: "kdlbs/kandev", ProviderID: "100", RemoteURL: "https://github.com/kdlbs/kandev.git",
		},
		TargetRepository: models.ContributionDestinationRepository{
			Host: "github.com", Path: "alice/kandev", ProviderID: "200", RemoteURL: "https://github.com/alice/kandev.git",
		},
	}

	script, err := (&SSHExecutor{}).resolvePrepareScript(&ExecutorCreateRequest{
		Metadata:                 map[string]interface{}{"repository_clone_url": "https://github.com/kdlbs/kandev.git"},
		ContributionDestinations: map[string]models.ContributionDestination{"": destination},
	}, "/remote/task", "/remote/bin/agentctl")
	if err != nil {
		t.Fatalf("resolvePrepareScript() error = %v", err)
	}
	if !strings.Contains(script, "cd '/remote/task'") {
		t.Fatalf("destination setup did not use remote workspace:\n%s", script)
	}
	if strings.Contains(script, "cd '/workspace'") {
		t.Fatalf("destination setup used the local default workspace:\n%s", script)
	}
}

// newSSHPrepareFixture seeds a bare origin with one commit on main and returns
// the temp root, that origin path, and a task workspace that already carries
// the session runtime directory the SSH executor creates before preparation.
func newSSHPrepareFixture(t *testing.T) (root, origin, workspace string) {
	t.Helper()
	root = t.TempDir()
	origin = filepath.Join(root, "origin.git")
	seed := filepath.Join(root, "seed")
	workspace = filepath.Join(root, "workspace")
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
	return root, origin, workspace
}

// TestSSHDefaultPrepareScriptExecutesUnderZsh runs the resolved default
// prepare script through zsh, the login shell macOS SSH targets use unless a
// profile overrides it. An unbraced `$repository_branch:` in the fetch refspec
// made zsh strip the `:r` and fetch `refs/heads/mainefs/remotes/origin/main`.
func TestSSHDefaultPrepareScriptExecutesUnderZsh(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}

	root, origin, workspace := newSSHPrepareFixture(t)
	script, err := (&SSHExecutor{}).resolvePrepareScript(&ExecutorCreateRequest{
		TaskID: "task-1",
		Metadata: map[string]interface{}{
			"repository_clone_url":    origin,
			MetadataKeyBaseBranch:     "main",
			MetadataKeyWorktreeBranch: "feature/task-1",
		},
	}, workspace, "/remote/bin/agentctl")
	if err != nil {
		t.Fatalf("resolvePrepareScript() error = %v", err)
	}
	cmd := exec.Command("zsh", "-e", "-c", script)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("SSH prepare script failed under zsh: %v\n%s\nscript:\n%s", err, output, script)
	}
	if got := strings.TrimSpace(string(runIn(t, workspace, "git", "branch", "--show-current"))); got != "feature/task-1" {
		t.Fatalf("prepared branch = %q, want feature/task-1", got)
	}
	if got := strings.TrimSpace(string(runIn(t, workspace, "git", "rev-parse", "--verify", "origin/main"))); got == "" {
		t.Fatal("origin/main was not fetched")
	}
}

// TestSSHPrepareScriptRepairsPersistedLegacyRefspec covers the upgrade path.
// The profile editor persists the generated default in
// executor_profiles.prepare_script, and resolvePrepareScript prefers that
// stored value over DefaultPrepareScript, so bracing the managed script alone
// leaves every profile created before that change still fetching
// `refs/heads/mainefs/remotes/origin/main` under zsh.
func TestSSHPrepareScriptRepairsPersistedLegacyRefspec(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}

	// Exactly what a pre-fix profile stored: the current default with the
	// refspec expansions unbraced again.
	legacy := strings.ReplaceAll(
		DefaultPrepareScript(executorTypeSSH),
		"+refs/heads/${repository_branch}:refs/remotes/origin/${repository_branch}",
		"+refs/heads/$repository_branch:refs/remotes/origin/$repository_branch",
	)
	if !strings.Contains(legacy, "+refs/heads/$repository_branch:") {
		t.Fatal("legacy fixture did not reproduce the unbraced refspec")
	}

	root, origin, workspace := newSSHPrepareFixture(t)
	script, err := (&SSHExecutor{}).resolvePrepareScript(&ExecutorCreateRequest{
		TaskID: "task-1",
		Metadata: map[string]interface{}{
			MetadataKeySetupScript:    legacy,
			"repository_clone_url":    origin,
			MetadataKeyBaseBranch:     "main",
			MetadataKeyWorktreeBranch: "feature/task-1",
		},
	}, workspace, "/remote/bin/agentctl")
	if err != nil {
		t.Fatalf("resolvePrepareScript() error = %v", err)
	}
	if strings.Contains(script, "+refs/heads/$repository_branch:") {
		t.Fatalf("stored legacy refspec was not repaired:\n%s", script)
	}

	cmd := exec.Command("zsh", "-e", "-c", script)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("repaired legacy prepare script failed under zsh: %v\n%s\nscript:\n%s", err, output, script)
	}
	if got := strings.TrimSpace(string(runIn(t, workspace, "git", "rev-parse", "--verify", "origin/main"))); got == "" {
		t.Fatal("origin/main was not fetched")
	}
}

// TestRepairLegacyFetchRefspec pins the repair to the fragment Kandev itself
// generated, so a hand-written script that deliberately uses a zsh modifier is
// left untouched.
func TestRepairLegacyFetchRefspec(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			name: "kandev fetch refspec",
			in:   `git fetch origin "+refs/heads/$repository_branch:refs/remotes/origin/$repository_branch"`,
			want: `git fetch origin "+refs/heads/${repository_branch}:refs/remotes/origin/${repository_branch}"`,
		},
		{
			name: "contribution base branch refspec",
			in:   `git fetch origin "+refs/heads/$base_branch:refs/remotes/origin/$base_branch"`,
			want: `git fetch origin "+refs/heads/${base_branch}:refs/remotes/origin/${base_branch}"`,
		},
		{
			name: "already braced",
			in:   `git fetch origin "+refs/heads/${repository_branch}:refs/remotes/origin/${repository_branch}"`,
			want: `git fetch origin "+refs/heads/${repository_branch}:refs/remotes/origin/${repository_branch}"`,
		},
		{
			name: "unrelated zsh modifier is preserved",
			in:   `printf '%s\n' "$archive:t"`,
			want: `printf '%s\n' "$archive:t"`,
		},
		{
			name: "unrelated colon expansion is preserved",
			in:   `PATH="$extra_bin:$PATH"`,
			want: `PATH="$extra_bin:$PATH"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := repairLegacyFetchRefspec(tc.in); got != tc.want {
				t.Errorf("repairLegacyFetchRefspec()\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

func TestSSHDefaultPrepareScriptExecutesCheckoutAndSetup(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	root, origin, workspace := newSSHPrepareFixture(t)

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
