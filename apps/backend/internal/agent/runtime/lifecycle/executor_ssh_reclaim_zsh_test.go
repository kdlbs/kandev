package lifecycle

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSSHTaskDirReclaimGitDetachedHeadStashUnderZsh(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is unavailable")
	}

	fixture := newSSHReclaimGitFixture(t)
	fixtureGit(t, fixture.taskDir, "checkout", "--detach", "HEAD")
	stashOnCheckout(t, fixture.taskDir, "detached stash")
	if output := fixtureGit(t, fixture.taskDir, "stash", "list", "--format=%gs"); !strings.Contains(output, "(no branch)") {
		t.Fatalf("detached stash description = %q, want Git's detached-HEAD marker", output)
	}
	branchCommand := sshReclaimBranchCmd(fixture.taskDir)
	fixture.runner.intercept = func(cmd string) (string, string, error, bool) {
		if cmd != branchCommand {
			return "", "", nil, false
		}
		command := exec.CommandContext(t.Context(), "bash", "-c", WrapLoginShell("zsh", cmd))
		command.Dir = fixture.runner.workingDir
		command.Env = reclaimTestEnvironment(fixture.runner.home)
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		err := command.Run()
		return stdout.String(), stderr.String(), err, true
	}

	outcome, verdict, err := newTestReclaimer(fixture.runner).Reclaim(t.Context(), fixture.profileRoot, fixture.taskDir)
	if err != nil || outcome != SSHReclaimOutcomeSkipped || verdict.Reason != SSHReclaimSkipStashPresent {
		t.Fatalf("Reclaim = (%q, %+v, %v), want a stash-present safety skip", outcome, verdict, err)
	}
	if commandIndex(fixture.runner.commands, "worktree remove", "") >= 0 || commandIndex(fixture.runner.commands, "rm -rf", "") >= 0 {
		t.Fatalf("detached stash safety skip issued removal: %v", fixture.runner.commands)
	}
	if _, err := os.Stat(fixture.taskDir); err != nil {
		t.Fatalf("task directory was removed: %v", err)
	}
}
