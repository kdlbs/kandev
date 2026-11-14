package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type localReclaimSSHRunner struct {
	workingDir string
	home       string
	commands   []string
	beforeRun  func(string)
	intercept  func(string) (string, string, error, bool)
}

func (r *localReclaimSSHRunner) Run(ctx context.Context, cmd string) (string, string, error) {
	return r.run(ctx, cmd, true)
}

func (r *localReclaimSSHRunner) RunDirect(ctx context.Context, cmd string) (string, string, error) {
	return r.run(ctx, cmd, false)
}

func (r *localReclaimSSHRunner) run(ctx context.Context, cmd string, login bool) (string, string, error) {
	r.commands = append(r.commands, cmd)
	if r.beforeRun != nil {
		r.beforeRun(cmd)
	}
	if r.intercept != nil {
		if stdout, stderr, err, ok := r.intercept(cmd); ok {
			return stdout, stderr, err
		}
	}
	if login {
		cmd = WrapLoginShell("bash", cmd)
	}
	command := exec.CommandContext(ctx, "bash", "-c", cmd)
	command.Dir = r.workingDir
	command.Env = reclaimTestEnvironment(r.home)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

type sshReclaimGitFixture struct {
	profileRoot string
	taskDir     string
	mainRepo    string
	bare        string
	runner      *localReclaimSSHRunner
}

func newSSHReclaimGitFixture(t *testing.T) sshReclaimGitFixture {
	t.Helper()
	base := t.TempDir()
	profileRoot := filepath.Join(base, "profile's workspace")
	taskDir := filepath.Join(profileRoot, "tasks", "task with spaces")
	mainRepo := filepath.Join(base, "retained shared repository")
	bare := filepath.Join(base, "remote.git")
	home := filepath.Join(base, "home")
	workingDir := filepath.Join(base, "unrelated working directory")
	for _, dir := range []string{filepath.Dir(taskDir), home, workingDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create fixture directory %q: %v", dir, err)
		}
	}
	fixtureGit(t, base, "init", "--bare", bare)
	fixtureGit(t, base, "init", "--initial-branch=main", mainRepo)
	configureFixtureGitIdentity(t, mainRepo)
	if err := os.WriteFile(filepath.Join(mainRepo, ".gitignore"), []byte(".env\nnode_modules/\n.kandev/\nnested-child/\nunsafe-child/\n"), 0o600); err != nil {
		t.Fatalf("write ignore rules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mainRepo, "README.md"), []byte("retained repository\n"), 0o600); err != nil {
		t.Fatalf("write repository file: %v", err)
	}
	fixtureGit(t, mainRepo, "add", ".gitignore", "README.md")
	fixtureGit(t, mainRepo, "commit", "-m", "initial commit")
	fixtureGit(t, mainRepo, "remote", "add", "origin", bare)
	fixtureGit(t, mainRepo, "push", "--set-upstream", "origin", "main")
	fixtureGit(t, mainRepo, "fetch", "origin")
	fixtureGit(t, mainRepo, "worktree", "add", "-b", "task/reclaim", taskDir, "main")
	return sshReclaimGitFixture{
		profileRoot: profileRoot,
		taskDir:     taskDir,
		mainRepo:    mainRepo,
		bare:        bare,
		runner:      &localReclaimSSHRunner{workingDir: workingDir, home: home},
	}
}

func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	command.Env = reclaimTestEnvironment(dir)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %q failed: %v\n%s", strings.Join(args, " "), dir, err, output)
	}
	return strings.TrimSpace(string(output))
}

func configureFixtureGitIdentity(t *testing.T, dir string) {
	t.Helper()
	fixtureGit(t, dir, "config", "user.name", "Reclaim Test")
	fixtureGit(t, dir, "config", "user.email", "reclaim@example.invalid")
	fixtureGit(t, dir, "config", "user.useConfigOnly", "true")
}

func reclaimTestEnvironment(home string) []string {
	var env []string
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(key, "GIT_") || key == "HOME" {
			continue
		}
		env = append(env, item)
	}
	return append(env,
		"HOME="+home,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=0")
}

func TestSSHTaskDirReclaimGitIgnoredFiles(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, sshReclaimGitFixture)
		want    SSHReclaimOutcome
		reason  SSHReclaimSkipReason
	}{
		{
			name: "ignored build output and environment file do not block",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				if err := os.MkdirAll(filepath.Join(f.taskDir, "node_modules", "output"), 0o700); err != nil {
					t.Fatalf("create ignored build directory: %v", err)
				}
				writeFixtureFile(t, filepath.Join(f.taskDir, "node_modules", "output", "bundle.js"), "build output")
				writeFixtureFile(t, filepath.Join(f.taskDir, ".env"), "SECRET=value")
				writeFixtureFile(t, filepath.Join(f.taskDir, ".kandev", "runtime", "cache"), "runtime data")
			},
			want: SSHReclaimOutcomeRemoved,
		},
		{
			name: "tracked change blocks",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				writeFixtureFile(t, filepath.Join(f.taskDir, "README.md"), "tracked edit")
			},
			want:   SSHReclaimOutcomeSkipped,
			reason: SSHReclaimSkipDirtyWorktree,
		},
		{
			name: "staged change blocks",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				writeFixtureFile(t, filepath.Join(f.taskDir, "README.md"), "staged edit")
				fixtureGit(t, f.taskDir, "add", "README.md")
			},
			want:   SSHReclaimOutcomeSkipped,
			reason: SSHReclaimSkipDirtyWorktree,
		},
		{
			name: "non-ignored untracked file blocks",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				writeFixtureFile(t, filepath.Join(f.taskDir, "scratch.txt"), "untracked file")
			},
			want:   SSHReclaimOutcomeSkipped,
			reason: SSHReclaimSkipDirtyWorktree,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newSSHReclaimGitFixture(t)
			tc.prepare(t, fixture)
			outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
			assertReclaimResult(t, fixture, outcome, verdict, err, tc.want, tc.reason)
		})
	}
}

func TestSSHTaskDirReclaimGitLinkedRemoval(t *testing.T) {
	fixture := newSSHReclaimGitFixture(t)
	childDir := filepath.Join(fixture.taskDir, "nested-child")
	siblingDir := filepath.Join(filepath.Dir(fixture.profileRoot), "retained sibling")
	createLinkedWorktree(t, fixture, childDir, "task/child")
	createLinkedWorktree(t, fixture, siblingDir, "task/sibling")
	sentinel := filepath.Join(siblingDir, "sibling-sentinel.txt")
	writeFixtureFile(t, sentinel, "keep sibling")
	stashOnCheckout(t, siblingDir, "sibling stash")
	outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
	assertReclaimResult(t, fixture, outcome, verdict, err, SSHReclaimOutcomeRemoved, "")
	worktrees := fixtureGit(t, fixture.mainRepo, "worktree", "list", "--porcelain", "-z")
	for _, removed := range []string{childDir, fixture.taskDir} {
		if strings.Contains(worktrees, "worktree "+removed+"\x00") {
			t.Fatalf("removed checkout %q remains registered:\n%s", removed, strings.ReplaceAll(worktrees, "\x00", "\n"))
		}
	}
	for _, retained := range []string{siblingDir, fixture.mainRepo} {
		if !strings.Contains(worktrees, "worktree "+retained+"\x00") {
			t.Fatalf("retained checkout %q lost its registration:\n%s", retained, strings.ReplaceAll(worktrees, "\x00", "\n"))
		}
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("retained sibling sentinel was removed: %v", err)
	}
	if !strings.Contains(fixtureGit(t, fixture.mainRepo, "stash", "list", "--format=%gs"), "sibling stash") {
		t.Fatal("shared stash was removed with the task worktree")
	}
	if !strings.Contains(fixtureGit(t, fixture.mainRepo, "show-ref", "--verify", "refs/heads/task/reclaim"), "refs/heads/task/reclaim") {
		t.Fatal("task branch ref was removed with the worktree")
	}
	childRemoval := commandIndex(fixture.runner.commands, "worktree remove", "nested-child")
	rootRemoval := commandIndex(fixture.runner.commands, "worktree remove", "/tasks/task with spaces'")
	if childRemoval < 0 || rootRemoval < 0 || childRemoval >= rootRemoval {
		t.Fatalf("linked removals were not child-first: commands=%v", fixture.runner.commands)
	}
	if commandIndex(fixture.runner.commands, "rm -rf", "") >= 0 {
		t.Fatalf("linked cleanup used recursive removal: %v", fixture.runner.commands)
	}
}

func TestSSHTaskDirReclaimGitCommitScope(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, sshReclaimGitFixture)
		outcome SSHReclaimOutcome
		reason  SSHReclaimSkipReason
	}{
		{
			name: "unpublished sibling branch does not block linked checkout",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				fixtureGit(t, f.mainRepo, "checkout", "-b", "task/private")
				writeFixtureFile(t, filepath.Join(f.mainRepo, "private.txt"), "private commit")
				fixtureGit(t, f.mainRepo, "add", "private.txt")
				fixtureGit(t, f.mainRepo, "commit", "-m", "sibling private commit")
				fixtureGit(t, f.mainRepo, "checkout", "main")
			},
			outcome: SSHReclaimOutcomeRemoved,
		},
		{
			name: "unpublished task HEAD blocks linked checkout",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				writeFixtureFile(t, filepath.Join(f.taskDir, "task.txt"), "task commit")
				fixtureGit(t, f.taskDir, "add", "task.txt")
				fixtureGit(t, f.taskDir, "commit", "-m", "task private commit")
			},
			outcome: SSHReclaimOutcomeSkipped,
			reason:  SSHReclaimSkipUnpushedCommits,
		},
		{
			name: "detached published HEAD is safe",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				fixtureGit(t, f.taskDir, "checkout", "--detach", "origin/main")
			},
			outcome: SSHReclaimOutcomeRemoved,
		},
		{
			name: "detached unpublished HEAD is at risk",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				writeFixtureFile(t, filepath.Join(f.taskDir, "task.txt"), "detached commit")
				fixtureGit(t, f.taskDir, "add", "task.txt")
				fixtureGit(t, f.taskDir, "commit", "-m", "detached private commit")
				fixtureGit(t, f.taskDir, "checkout", "--detach", "HEAD")
			},
			outcome: SSHReclaimOutcomeSkipped,
			reason:  SSHReclaimSkipUnpushedCommits,
		},
		{
			name: "standalone clone protects unpublished side branch",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				makeStandaloneClone(t, f, false)
				fixtureGit(t, f.taskDir, "checkout", "-b", "task/private")
				writeFixtureFile(t, filepath.Join(f.taskDir, "private.txt"), "private commit")
				fixtureGit(t, f.taskDir, "add", "private.txt")
				fixtureGit(t, f.taskDir, "commit", "-m", "side branch commit")
				fixtureGit(t, f.taskDir, "checkout", "main")
			},
			outcome: SSHReclaimOutcomeSkipped,
			reason:  SSHReclaimSkipUnpushedCommits,
		},
		{
			name: "pushed shallow standalone clone is safe",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				makeStandaloneClone(t, f, true)
				if got := fixtureGit(t, f.taskDir, "rev-parse", "--is-shallow-repository"); got != "true" {
					t.Fatalf("fixture is shallow = %q, want true", got)
				}
			},
			outcome: SSHReclaimOutcomeRemoved,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newSSHReclaimGitFixture(t)
			tc.prepare(t, fixture)
			outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
			assertReclaimResult(t, fixture, outcome, verdict, err, tc.outcome, tc.reason)
		})
	}
}

func TestSSHTaskDirReclaimGitStashScope(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, sshReclaimGitFixture)
		outcome SSHReclaimOutcome
		reason  SSHReclaimSkipReason
	}{
		{
			name: "other branch stash does not block linked checkout",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				other := filepath.Join(filepath.Dir(f.profileRoot), "other branch")
				createLinkedWorktree(t, f, other, "task/other")
				stashOnCheckout(t, other, "sibling stash")
			},
			outcome: SSHReclaimOutcomeRemoved,
		},
		{
			name:    "default task branch stash blocks reclamation",
			prepare: func(t *testing.T, f sshReclaimGitFixture) { stashOnCheckout(t, f.taskDir, "") },
			outcome: SSHReclaimOutcomeSkipped,
			reason:  SSHReclaimSkipStashPresent,
		},
		{
			name:    "custom task branch stash blocks reclamation",
			prepare: func(t *testing.T, f sshReclaimGitFixture) { stashOnCheckout(t, f.taskDir, "task stash") },
			outcome: SSHReclaimOutcomeSkipped,
			reason:  SSHReclaimSkipStashPresent,
		},
		{
			name: "mixed stashes block when one matches the task branch",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				other := filepath.Join(filepath.Dir(f.profileRoot), "other branch")
				createLinkedWorktree(t, f, other, "task/other")
				stashOnCheckout(t, other, "sibling stash")
				stashOnCheckout(t, f.taskDir, "task stash")
			},
			outcome: SSHReclaimOutcomeSkipped,
			reason:  SSHReclaimSkipStashPresent,
		},
		{
			name: "same named tag does not hide task branch stash",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				fixtureGit(t, f.taskDir, "tag", "task/reclaim")
				stashOnCheckout(t, f.taskDir, "branch collision stash")
			},
			outcome: SSHReclaimOutcomeSkipped,
			reason:  SSHReclaimSkipStashPresent,
		},
		{
			name: "default stash created while detached remains unknown after reattach",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				stashOnDetachedCheckout(t, f.taskDir, "")
			},
			outcome: SSHReclaimOutcomeSkipped,
			reason:  SSHReclaimSkipStashPresent,
		},
		{
			name: "custom stash created while detached remains unknown after reattach",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				stashOnDetachedCheckout(t, f.taskDir, "detached custom stash")
			},
			outcome: SSHReclaimOutcomeSkipped,
			reason:  SSHReclaimSkipStashPresent,
		},
		{
			name: "exact branch delimiter ignores a prefix sibling",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				fixtureGit(t, f.taskDir, "branch", "-m", "task/a")
				other := filepath.Join(filepath.Dir(f.profileRoot), "prefix sibling")
				createLinkedWorktree(t, f, other, "task/a-extra")
				stashOnCheckout(t, other, "prefix stash")
			},
			outcome: SSHReclaimOutcomeRemoved,
		},
		{
			name: "detached HEAD with any stash is preserved",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				fixtureGit(t, f.taskDir, "checkout", "--detach", "origin/main")
				other := filepath.Join(filepath.Dir(f.profileRoot), "detached sibling")
				createLinkedWorktree(t, f, other, "task/other")
				stashOnCheckout(t, other, "sibling stash")
			},
			outcome: SSHReclaimOutcomeSkipped,
			reason:  SSHReclaimSkipStashPresent,
		},
		{
			name: "unknown stash attribution is preserved",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				writeFixtureFile(t, filepath.Join(f.taskDir, "README.md"), "unknown stash content")
				sha := fixtureGit(t, f.taskDir, "stash", "create")
				fixtureGit(t, f.taskDir, "checkout", "--", "README.md")
				fixtureGit(t, f.taskDir, "stash", "store", "-m", "unclassified entry", sha)
			},
			outcome: SSHReclaimOutcomeSkipped,
			reason:  SSHReclaimSkipStashPresent,
		},
		{
			name: "standalone clone protects every stash",
			prepare: func(t *testing.T, f sshReclaimGitFixture) {
				makeStandaloneClone(t, f, false)
				stashOnCheckout(t, f.taskDir, "standalone stash")
			},
			outcome: SSHReclaimOutcomeSkipped,
			reason:  SSHReclaimSkipStashPresent,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newSSHReclaimGitFixture(t)
			tc.prepare(t, fixture)
			outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
			assertReclaimResult(t, fixture, outcome, verdict, err, tc.outcome, tc.reason)
			if tc.reason == SSHReclaimSkipStashPresent && commandIndex(fixture.runner.commands, "worktree remove", "") >= 0 {
				t.Fatalf("stash safety skip issued linked-worktree removal: %v", fixture.runner.commands)
			}
		})
	}
}

func TestSSHTaskDirReclaimGitCheckoutKinds(t *testing.T) {
	t.Run("symlinked profile root", func(t *testing.T) {
		fixture := newSSHReclaimGitFixture(t)
		alias := filepath.Join(filepath.Dir(fixture.profileRoot), "profile alias")
		if err := os.Symlink(fixture.profileRoot, alias); err != nil {
			t.Fatalf("create profile symlink: %v", err)
		}
		path := filepath.Join(alias, "tasks", filepath.Base(fixture.taskDir))
		outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), alias, path)
		assertReclaimResult(t, fixture, outcome, verdict, err, SSHReclaimOutcomeRemoved, "")
	})
	t.Run("standalone clone with another worktree is shared", func(t *testing.T) {
		fixture := newSSHReclaimGitFixture(t)
		makeStandaloneClone(t, fixture, false)
		sibling := filepath.Join(filepath.Dir(fixture.profileRoot), "standalone sibling")
		fixtureGit(t, fixture.taskDir, "worktree", "add", "-b", "task/sibling", sibling, "main")
		outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
		assertReclaimResult(t, fixture, outcome, verdict, err, SSHReclaimOutcomeSkipped, SSHReclaimSkipShared)
	})
	t.Run("separate internal git storage is unsupported", func(t *testing.T) {
		fixture := newSSHReclaimGitFixture(t)
		fixtureGit(t, fixture.mainRepo, "worktree", "remove", "--", fixture.taskDir)
		if err := os.MkdirAll(fixture.taskDir, 0o700); err != nil {
			t.Fatalf("create task root: %v", err)
		}
		storage := filepath.Join(fixture.taskDir, "git-storage")
		fixtureGit(t, filepath.Dir(fixture.profileRoot), "init", "--separate-git-dir", storage, "--initial-branch=main", fixture.taskDir)
		configureFixtureGitIdentity(t, fixture.taskDir)
		writeFixtureFile(t, filepath.Join(fixture.taskDir, "README.md"), "internal storage")
		fixtureGit(t, fixture.taskDir, "add", "README.md")
		fixtureGit(t, fixture.taskDir, "commit", "-m", "internal storage")
		outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
		assertReclaimResult(t, fixture, outcome, verdict, err, SSHReclaimOutcomeSkipped, SSHReclaimSkipProbeFailed)
	})
	t.Run("child repositories without a root checkout remain unsupported", func(t *testing.T) {
		fixture := newSSHReclaimGitFixture(t)
		fixtureGit(t, fixture.mainRepo, "worktree", "remove", "--", fixture.taskDir)
		if err := os.MkdirAll(fixture.taskDir, 0o700); err != nil {
			t.Fatalf("create task root: %v", err)
		}
		child := filepath.Join(fixture.taskDir, "child-repository")
		fixtureGit(t, filepath.Dir(fixture.profileRoot), "clone", "--branch", "main", fixture.bare, child)
		outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
		assertReclaimResult(t, fixture, outcome, verdict, err, SSHReclaimOutcomeSkipped, SSHReclaimSkipProbeFailed)
	})
}

func TestSSHTaskDirReclaimDoesNotFollowTaskDirectorySymlink(t *testing.T) {
	fixture := newSSHReclaimGitFixture(t)
	linkedTask := fixture.taskDir
	canonicalSibling := filepath.Join(filepath.Dir(linkedTask), "task-B")
	fixtureGit(t, fixture.mainRepo, "worktree", "move", linkedTask, canonicalSibling)
	if err := os.Symlink(canonicalSibling, linkedTask); err != nil {
		t.Fatalf("symlink recorded task path: %v", err)
	}

	outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, linkedTask)
	if err != nil {
		t.Fatalf("Reclaim returned error: %v", err)
	}
	if outcome != SSHReclaimOutcomeSkipped || verdict.Reason != SSHReclaimSkipProbeFailed {
		t.Fatalf("Reclaim = (%q, %+v), want a probe-failed safety skip", outcome, verdict)
	}
	if _, err := os.Stat(canonicalSibling); err != nil {
		t.Fatalf("canonical sibling task was changed or removed: %v", err)
	}
	if commandIndex(fixture.runner.commands, "worktree remove", canonicalSibling) >= 0 || commandIndex(fixture.runner.commands, "rm -rf", canonicalSibling) >= 0 {
		t.Fatalf("reclaim issued removal against the symlink target: %v", fixture.runner.commands)
	}
}

func TestSSHTaskDirReclaimPreservesTrailingSpaceTaskName(t *testing.T) {
	fixture := newSSHReclaimGitFixture(t)
	tasksDir := filepath.Dir(fixture.taskDir)
	trailingSpaceTask := filepath.Join(tasksDir, "task-1 ")
	ordinaryTask := filepath.Join(tasksDir, "task-1")
	fixtureGit(t, fixture.mainRepo, "worktree", "move", fixture.taskDir, trailingSpaceTask)
	createLinkedWorktree(t, fixture, ordinaryTask, "task/ordinary")
	fixture.taskDir = trailingSpaceTask

	outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, trailingSpaceTask)
	if err != nil {
		t.Fatalf("Reclaim returned error: %v", err)
	}
	if outcome != SSHReclaimOutcomeRemoved {
		t.Fatalf("Reclaim = (%q, %+v), want the requested trailing-space task removed", outcome, verdict)
	}
	if _, err := os.Stat(trailingSpaceTask); !os.IsNotExist(err) {
		t.Fatalf("requested trailing-space task still exists, stat error = %v", err)
	}
	if _, err := os.Stat(ordinaryTask); err != nil {
		t.Fatalf("ordinary sibling task was changed or removed: %v", err)
	}
	if commandIndex(fixture.runner.commands, "worktree remove", ordinaryTask) >= 0 {
		t.Fatalf("reclaim removed the ordinary sibling instead: %v", fixture.runner.commands)
	}
}

func TestIsTaskDirectoryCheckout(t *testing.T) {
	tests := []struct {
		name        string
		checkoutDir string
		want        bool
	}{
		{name: "task root", checkoutDir: "/workspace/tasks/task-1", want: true},
		{name: "direct child", checkoutDir: "/workspace/tasks/task-1/repo", want: true},
		{name: "nested child", checkoutDir: "/workspace/tasks/task-1/repo/nested", want: false},
		{name: "similar prefix", checkoutDir: "/workspace/tasks/task-10", want: false},
		{name: "outside task root", checkoutDir: "/workspace/repo", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTaskDirectoryCheckout("/workspace/tasks/task-1", tc.checkoutDir); got != tc.want {
				t.Fatalf("isTaskDirectoryCheckout(%q) = %t, want %t", tc.checkoutDir, got, tc.want)
			}
		})
	}
}

func TestSSHTaskDirReclaimGitUnsafeChildVetoesAllRemoval(t *testing.T) {
	fixture := newSSHReclaimGitFixture(t)
	child := filepath.Join(fixture.taskDir, "nested-child")
	createLinkedWorktree(t, fixture, child, "task/child")
	writeFixtureFile(t, filepath.Join(fixture.taskDir, "untracked.txt"), "preserve task")
	outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
	assertReclaimResult(t, fixture, outcome, verdict, err, SSHReclaimOutcomeSkipped, SSHReclaimSkipDirtyWorktree)
	if commandIndex(fixture.runner.commands, "worktree remove", "") >= 0 || commandIndex(fixture.runner.commands, "rm -rf", "") >= 0 {
		t.Fatalf("an unsafe root allowed partial removal: %v", fixture.runner.commands)
	}
}

func TestSSHTaskDirReclaimGitSafeAndUnsafeChildrenAreAllOrNothing(t *testing.T) {
	fixture := newSSHReclaimGitFixture(t)
	safeChild := filepath.Join(fixture.taskDir, "nested-child")
	unsafeChild := filepath.Join(fixture.taskDir, "unsafe-child")
	createLinkedWorktree(t, fixture, safeChild, "task/safe-child")
	createLinkedWorktree(t, fixture, unsafeChild, "task/unsafe-child")
	writeFixtureFile(t, filepath.Join(unsafeChild, "private.txt"), "unpublished child commit")
	fixtureGit(t, unsafeChild, "add", "private.txt")
	fixtureGit(t, unsafeChild, "commit", "-m", "unpublished child commit")
	outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
	assertReclaimResult(t, fixture, outcome, verdict, err, SSHReclaimOutcomeSkipped, SSHReclaimSkipUnpushedCommits)
	if commandIndex(fixture.runner.commands, "worktree remove", "") >= 0 || commandIndex(fixture.runner.commands, "rm -rf", "") >= 0 {
		t.Fatalf("safe checkout was removed before the unsafe sibling was discovered: %v", fixture.runner.commands)
	}
	for _, dir := range []string{fixture.taskDir, safeChild, unsafeChild} {
		if _, statErr := os.Stat(dir); statErr != nil {
			t.Fatalf("checkout %q was partially removed: %v", dir, statErr)
		}
	}
}

func TestSSHTaskDirReclaimGitPreservesLocksAndSubmodules(t *testing.T) {
	t.Run("lock without reason", func(t *testing.T) { assertLockedTaskPreserved(t, false) })
	t.Run("lock with reason", func(t *testing.T) { assertLockedTaskPreserved(t, true) })
	for _, initialized := range []bool{false, true} {
		name := "uninitialized submodule"
		if initialized {
			name = "initialized submodule"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newSSHReclaimGitFixture(t)
			addSubmoduleToFixture(t, fixture, initialized)
			outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
			assertReclaimResult(t, fixture, outcome, verdict, err, SSHReclaimOutcomeSkipped, SSHReclaimSkipProbeFailed)
			if !strings.Contains(verdict.Detail, "submodule") {
				t.Fatalf("skip detail %q does not identify the unsupported submodule layout", verdict.Detail)
			}
			if commandIndex(fixture.runner.commands, "worktree remove", "") >= 0 || commandIndex(fixture.runner.commands, "rm -rf", "") >= 0 {
				t.Fatalf("submodule safety skip issued removal: %v", fixture.runner.commands)
			}
			if initialized {
				assertSubmoduleStatusOptionsAgree(t, fixture.taskDir)
			}
		})
	}
}

func TestSSHTaskDirReclaimGitFailures(t *testing.T) {
	t.Run("unsupported Git path format fails closed", func(t *testing.T) {
		fixture := newSSHReclaimGitFixture(t)
		fixture.runner.intercept = func(cmd string) (string, string, error, bool) {
			if strings.Contains(cmd, "--path-format=absolute") {
				return "unsupported output\n", "", nil, true
			}
			return "", "", nil, false
		}
		outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
		assertReclaimResult(t, fixture, outcome, verdict, err, SSHReclaimOutcomeSkipped, SSHReclaimSkipProbeFailed)
		if commandIndex(fixture.runner.commands, "worktree remove", "") >= 0 || commandIndex(fixture.runner.commands, "rm -rf", "") >= 0 {
			t.Fatalf("unsupported Git output permitted removal: %v", fixture.runner.commands)
		}
	})
	t.Run("symbolic branch probe transport failure preserves checkout", func(t *testing.T) {
		fixture := newSSHReclaimGitFixture(t)
		stashOnCheckout(t, fixture.taskDir, "needs branch identity")
		fixture.runner.intercept = func(cmd string) (string, string, error, bool) {
			if cmd == sshReclaimBranchCmd(fixture.taskDir) {
				return "", "connection lost", errors.New("SSH transport lost"), true
			}
			return "", "", nil, false
		}
		outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
		if err != nil || outcome != SSHReclaimOutcomeSkipped || verdict.Reason != SSHReclaimSkipProbeFailed {
			t.Fatalf("Reclaim = (%q, %+v, %v), want a probe-failed safety skip", outcome, verdict, err)
		}
		if commandIndex(fixture.runner.commands, "worktree remove", "") >= 0 || commandIndex(fixture.runner.commands, "rm -rf", "") >= 0 {
			t.Fatalf("branch probe failure issued removal: %v", fixture.runner.commands)
		}
	})
	t.Run("lock appearing during a probe has no recursive fallback", func(t *testing.T) {
		fixture := newSSHReclaimGitFixture(t)
		locked := false
		fixture.runner.beforeRun = func(cmd string) {
			if !locked && strings.Contains(cmd, "stash list") {
				fixtureGit(t, fixture.mainRepo, "worktree", "lock", "--reason", "appeared during probe", fixture.taskDir)
				locked = true
			}
		}
		outcome, _, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
		if outcome != SSHReclaimOutcomeFailed || err == nil {
			t.Fatalf("Reclaim = (%q, %v), want Git removal failure after lock", outcome, err)
		}
		if _, statErr := os.Stat(fixture.taskDir); statErr != nil {
			t.Fatalf("locked task directory was removed: %v", statErr)
		}
		if commandIndex(fixture.runner.commands, "rm -rf", "") >= 0 {
			t.Fatalf("Git removal failure fell back to recursive removal: %v", fixture.runner.commands)
		}
	})
}

func TestSSHTaskDirReclaimGitStandaloneWithNestedWorktreePreservesStorage(t *testing.T) {
	fixture := newSSHReclaimGitFixture(t)
	makeStandaloneClone(t, fixture, false)
	nested := filepath.Join(fixture.taskDir, "nested-child")
	fixtureGit(t, fixture.taskDir, "worktree", "add", "-b", "task/nested", nested, "main")
	outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
	assertReclaimResult(t, fixture, outcome, verdict, err, SSHReclaimOutcomeSkipped, SSHReclaimSkipShared)
	if _, err := os.Stat(filepath.Join(fixture.taskDir, ".git")); err != nil {
		t.Fatalf("standalone Git storage inside the task directory was removed: %v", err)
	}
}

func assertLockedTaskPreserved(t *testing.T, withReason bool) {
	t.Helper()
	fixture := newSSHReclaimGitFixture(t)
	args := []string{"worktree", "lock"}
	if withReason {
		args = append(args, "--reason", "operator hold")
	}
	args = append(args, fixture.taskDir)
	fixtureGit(t, fixture.mainRepo, args...)
	outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
	assertReclaimResult(t, fixture, outcome, verdict, err, SSHReclaimOutcomeSkipped, SSHReclaimSkipShared)
	if !strings.Contains(verdict.Detail, "lock") {
		t.Fatalf("lock skip detail = %q, want a lock-specific explanation", verdict.Detail)
	}
	if commandIndex(fixture.runner.commands, "worktree remove", "") >= 0 || commandIndex(fixture.runner.commands, "rm -rf", "") >= 0 {
		t.Fatalf("known lock issued a removal command: %v", fixture.runner.commands)
	}
}

func addSubmoduleToFixture(t *testing.T, fixture sshReclaimGitFixture, initialize bool) {
	t.Helper()
	base := filepath.Dir(fixture.profileRoot)
	submoduleBare := filepath.Join(base, "submodule.git")
	submoduleRepo := filepath.Join(base, "submodule source")
	fixtureGit(t, base, "init", "--bare", submoduleBare)
	fixtureGit(t, base, "init", "--initial-branch=main", submoduleRepo)
	fixtureGit(t, submoduleRepo, "config", "user.name", "Reclaim Test")
	fixtureGit(t, submoduleRepo, "config", "user.email", "reclaim@example.invalid")
	writeFixtureFile(t, filepath.Join(submoduleRepo, "README.md"), "submodule source")
	fixtureGit(t, submoduleRepo, "add", "README.md")
	fixtureGit(t, submoduleRepo, "commit", "-m", "submodule initial commit")
	fixtureGit(t, submoduleRepo, "remote", "add", "origin", submoduleBare)
	fixtureGit(t, submoduleRepo, "push", "origin", "main")
	fixtureGit(t, submoduleBare, "symbolic-ref", "HEAD", "refs/heads/main")
	url := "file://" + filepath.ToSlash(submoduleBare)
	fixtureGit(t, fixture.mainRepo, "-c", "protocol.file.allow=always", "submodule", "add", url, "test-submodule")
	fixtureGit(t, fixture.mainRepo, "commit", "-m", "add submodule")
	fixtureGit(t, fixture.mainRepo, "push", "origin", "main")
	fixtureGit(t, fixture.taskDir, "merge", "--ff-only", "main")
	if initialize {
		fixtureGit(t, fixture.taskDir, "-c", "protocol.file.allow=always", "submodule", "update", "--init", "--recursive")
		writeFixtureFile(t, filepath.Join(fixture.taskDir, "test-submodule", "README.md"), "dirty submodule")
	}
}

func assertSubmoduleStatusOptionsAgree(t *testing.T, taskDir string) {
	t.Helper()
	oldStatus := fixtureGit(t, taskDir, "status", "--porcelain", "--untracked-files=all", "--ignored")
	newStatus := fixtureGit(t, taskDir, "status", "--porcelain", "--untracked-files=all", "--ignored=no")
	if oldStatus != newStatus || oldStatus == "" {
		t.Fatalf("status options differ for ordinary submodule config: old=%q new=%q", oldStatus, newStatus)
	}
	oldIgnored := fixtureGit(t, taskDir, "-c", "submodule.test-submodule.ignore=all", "status", "--porcelain", "--untracked-files=all", "--ignored")
	newIgnored := fixtureGit(t, taskDir, "-c", "submodule.test-submodule.ignore=all", "status", "--porcelain", "--untracked-files=all", "--ignored=no")
	if oldIgnored != newIgnored || oldIgnored != "" {
		t.Fatalf("status options differ for ignore-all submodule config: old=%q new=%q", oldIgnored, newIgnored)
	}
}

func assertReclaimResult(
	t *testing.T,
	fixture sshReclaimGitFixture,
	outcome SSHReclaimOutcome,
	verdict SSHReclaimVerdict,
	err error,
	wantOutcome SSHReclaimOutcome,
	wantReason SSHReclaimSkipReason,
) {
	t.Helper()
	if err != nil {
		t.Fatalf("Reclaim returned error: %v", err)
	}
	if outcome != wantOutcome {
		t.Fatalf("Reclaim outcome = %q with verdict %+v, want %q", outcome, verdict, wantOutcome)
	}
	if wantReason != "" && verdict.Reason != wantReason {
		t.Fatalf("Reclaim reason = %q, want %q (detail %q)", verdict.Reason, wantReason, verdict.Detail)
	}
	_, statErr := os.Stat(fixture.taskDir)
	if wantOutcome == SSHReclaimOutcomeRemoved && !os.IsNotExist(statErr) {
		t.Fatalf("task directory stat error = %v, want it removed", statErr)
	}
	if wantOutcome == SSHReclaimOutcomeSkipped && statErr != nil {
		t.Fatalf("task directory stat error = %v, want it preserved", statErr)
	}
}

func createLinkedWorktree(t *testing.T, fixture sshReclaimGitFixture, path, branch string) {
	t.Helper()
	fixtureGit(t, fixture.mainRepo, "worktree", "add", "-b", branch, path, "main")
}

func makeStandaloneClone(t *testing.T, fixture sshReclaimGitFixture, shallow bool) {
	t.Helper()
	fixtureGit(t, fixture.mainRepo, "worktree", "remove", "--", fixture.taskDir)
	args := []string{"clone", "--branch", "main"}
	remote := fixture.bare
	if shallow {
		args = append(args, "--depth=1")
		remote = "file://" + filepath.ToSlash(remote)
	}
	args = append(args, remote, fixture.taskDir)
	fixtureGit(t, filepath.Dir(fixture.profileRoot), args...)
	configureFixtureGitIdentity(t, fixture.taskDir)
}

func stashOnCheckout(t *testing.T, dir, message string) {
	t.Helper()
	writeFixtureFile(t, filepath.Join(dir, "README.md"), "stashed change")
	args := []string{"stash", "push"}
	if message != "" {
		args = append(args, "--message", message)
	}
	args = append(args, "--", "README.md")
	fixtureGit(t, dir, args...)
}

func stashOnDetachedCheckout(t *testing.T, dir, message string) {
	t.Helper()
	fixtureGit(t, dir, "checkout", "--detach", "HEAD")
	writeFixtureFile(t, filepath.Join(dir, "README.md"), "detached stashed change")
	args := []string{"stash", "push"}
	if message != "" {
		args = append(args, "--message", message)
	}
	args = append(args, "--", "README.md")
	fixtureGit(t, dir, args...)
	fixtureGit(t, dir, "checkout", "task/reclaim")
}

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create fixture parent for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content+"\n"), 0o600); err != nil {
		t.Fatalf("write fixture file %q: %v", path, err)
	}
}

func commandIndex(commands []string, first, second string) int {
	for index, command := range commands {
		if strings.Contains(command, first) && strings.Contains(command, second) {
			return index
		}
	}
	return -1
}
