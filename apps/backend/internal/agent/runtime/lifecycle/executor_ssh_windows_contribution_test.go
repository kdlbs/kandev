package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// The Windows contribution checkout follows the POSIX branch selection: it
// never resets a branch, so unpushed agent commits survive a re-prepare.
// @covers AC-EXECUTORS-SSH-WINDOWS-001.8
func TestSSHWindowsRemoteContributionScriptNeverResetsABranch(t *testing.T) {
	fixture := sshContributionFixture{
		workspace: `C:\k\tasks\task-1`,
		targetURL: "https://github.com/acme/widget.git",
		binding: &models.RemoteContribution{
			BaseBranch: "main", HeadBranch: "feature/remote", HeadSHA: strings.Repeat("a", 40),
			Provider: models.RemoteContributionProviderGitHub,
			SourceRepository: models.RemoteContributionRepository{
				Host: "github.com", Path: "contributor/widget", RemoteURL: "https://github.com/contributor/widget.git",
			},
		},
	}
	script := sshWindowsRemoteContributionScript(fixture.workspace, fixture.targetURL, fixture.binding)
	if strings.Contains(script, "checkout -B") {
		t.Fatal("contribution checkout must not reset an existing branch with checkout -B")
	}
	for _, want := range []string{
		"merge-base --is-ancestor $__kExpectedHead HEAD",
		"checkout -b $__kCheckout $__kContribRef",
		"throw 'kandev: contribution checkout failed'",
		"throw 'kandev: contribution push identity conflict'",
		"'/.kandev/'",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("contribution script missing %q", want)
		}
	}
	destination := sshWindowsContributionDestinationScript(fixture.workspace, &models.ContributionDestination{
		TargetRepository: models.ContributionDestinationRepository{RemoteURL: "https://github.com/me/widget.git"},
	})
	if strings.Contains(destination, "--replace-all") || !strings.Contains(destination, "contribution destination push identity conflict") {
		t.Fatal("destination script must refuse a conflicting push URL instead of replacing it")
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.8
func TestSSHWindowsExecRemoteContributionReusesLocalDescendant(t *testing.T) {
	requireWindowsExecution(t)
	fixture := newSSHContributionFixture(t)
	runContribution := windowsContributionRunner(t, fixture)

	runContribution(t)
	if got := windowsFixtureGit(t, fixture.workspace, "rev-parse", "HEAD"); got != fixture.sourceSHA {
		t.Fatalf("initial HEAD = %q, want %q", got, fixture.sourceSHA)
	}
	wantUpstream := fixture.binding.ContributionRemoteName() + "/" + fixture.binding.HeadBranch
	if got := windowsFixtureGit(t, fixture.workspace, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); got != wantUpstream {
		t.Fatalf("initial upstream = %q, want %q", got, wantUpstream)
	}
	exclude, err := os.ReadFile(filepath.Join(fixture.workspace, ".git", "info", "exclude"))
	if err != nil || !strings.Contains(string(exclude), "/.kandev/") {
		t.Fatalf("info/exclude = %q, %v; want /.kandev/", exclude, err)
	}

	writeFixtureFile(t, filepath.Join(fixture.workspace, "agent-change.txt"), "agent change")
	windowsFixtureGit(t, fixture.workspace, "add", "agent-change.txt")
	windowsFixtureGit(t, fixture.workspace, "-c", "user.name=k", "-c", "user.email=k@example.com", "commit", "-q", "-m", "agent contribution")
	localHEAD := windowsFixtureGit(t, fixture.workspace, "rev-parse", "HEAD")

	runContribution(t)
	if got := windowsFixtureGit(t, fixture.workspace, "rev-parse", "HEAD"); got != localHEAD {
		t.Fatalf("re-prepared HEAD = %q, want the unpushed local commit %q", got, localHEAD)
	}
	if got := windowsFixtureGit(t, fixture.workspace, "branch", "--show-current"); got != fixture.binding.HeadBranch {
		t.Fatalf("re-prepared branch = %q, want %q", got, fixture.binding.HeadBranch)
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.8
func TestSSHWindowsExecRemoteContributionKeepsDivergedBranches(t *testing.T) {
	requireWindowsExecution(t)
	fixture := newSSHContributionFixture(t)
	runContribution := windowsContributionRunner(t, fixture)
	runContribution(t)
	base := windowsFixtureGit(t, fixture.workspace, "rev-parse", "origin/main")
	suffix := strings.TrimPrefix(fixture.binding.ContributionRemoteName(), "contrib-")
	suffixed := fixture.binding.HeadBranch + "-kandev-" + suffix

	for _, diverged := range []string{fixture.binding.HeadBranch, suffixed} {
		windowsFixtureGit(t, fixture.workspace, "checkout", "-q", "--detach")
		windowsFixtureGit(t, fixture.workspace, "branch", "-f", diverged, base)
	}
	runContribution(t)
	if got := windowsFixtureGit(t, fixture.workspace, "branch", "--show-current"); got != suffixed+"-1" {
		t.Fatalf("checkout branch = %q, want %q", got, suffixed+"-1")
	}
	if got := windowsFixtureGit(t, fixture.workspace, "rev-parse", "HEAD"); got != fixture.sourceSHA {
		t.Fatalf("HEAD = %q, want %q", got, fixture.sourceSHA)
	}
	for _, diverged := range []string{fixture.binding.HeadBranch, suffixed} {
		if got := windowsFixtureGit(t, fixture.workspace, "rev-parse", diverged); got != base {
			t.Fatalf("diverged branch %s moved to %q; it must keep %q", diverged, got, base)
		}
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.8
func TestSSHWindowsExecRemoteContributionRefusesPushIdentityConflict(t *testing.T) {
	requireWindowsExecution(t)
	fixture := newSSHContributionFixture(t)
	runContribution := windowsContributionRunner(t, fixture)
	runContribution(t)
	remote := fixture.binding.ContributionRemoteName()
	windowsFixtureGit(t, fixture.workspace, "config", "remote."+remote+".pushurl", "https://github.com/someone-else/widget.git")

	client := newLocalWindowsSSHServer(t).dial(t)
	script := sshWindowsRemoteContributionScript(fixture.workspace, fixture.targetURL, fixture.binding)
	_, stderr, err := runPowerShell(context.Background(), client, "powershell", script, false)
	if err == nil || !strings.Contains(stderr, "contribution push identity conflict") {
		t.Fatalf("contribution with a foreign push URL: err=%v stderr=%q, want push identity conflict", err, stderr)
	}
}

func windowsContributionRunner(t *testing.T, fixture sshContributionFixture) func(*testing.T) {
	t.Helper()
	client := newLocalWindowsSSHServer(t).dial(t)
	script := sshWindowsRemoteContributionScript(fixture.workspace, fixture.targetURL, fixture.binding)
	return func(t *testing.T) {
		t.Helper()
		for _, shell := range windowsTestShells() {
			if _, stderr, err := runPowerShell(context.Background(), client, shell, script, false); err != nil {
				t.Fatalf("[%s] contribution script: %v\nstderr=%s", shell, err, stderr)
			}
		}
	}
}

func windowsFixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(runSSHContributionGit(t, dir, args...))
}
