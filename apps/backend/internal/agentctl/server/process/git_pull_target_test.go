package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type pullSourceFixture struct {
	repo, native, origin, source, baseline, sourceHead string
	remote, sourceBranch                               string
}

// @covers AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.4
func TestGitOperatorPullFallbacks(t *testing.T) {
	for _, target := range []string{"same-name", "main", "master", "current-name"} {
		for _, rebase := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/rebase=%t", target, rebase), func(t *testing.T) {
				branch := target
				if target == "same-name" || target == "current-name" {
					branch = "task-local"
				}
				f := newPullSourceFixture(t, "origin", branch)
				if target != "same-name" {
					runGit(t, f.repo, "branch", "--unset-upstream")
				}
				if target == "master" || target == "current-name" {
					runGit(t, f.origin, "update-ref", "-d", "refs/heads/main")
					runGit(t, f.repo, "update-ref", "-d", "refs/remotes/origin/main")
				}
				assertPullSourceHead(t, f, rebase)
			})
		}
	}
}

func assertPullSourceHead(t *testing.T, f pullSourceFixture, rebase bool) {
	t.Helper()
	config := runGit(t, f.repo, "config", "--local", "--list")
	result, err := NewGitOperator(f.repo, newTestLogger(t), nil).Pull(context.Background(), rebase)
	if err != nil || !result.Success || pullHead(t, f.repo) != f.sourceHead {
		t.Fatalf("Pull: result=%+v err=%v HEAD=%s want=%s", result, err, pullHead(t, f.repo), f.sourceHead)
	}
	if runGit(t, f.repo, "config", "--local", "--list") != config {
		t.Fatal("Pull changed configuration")
	}
}

// @covers AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.5
func TestGitOperatorPullUpstreamOperands(t *testing.T) {
	for _, shape := range []string{"slash remote/custom tracking", "local dot", "primary of multiple merge refs", "unsafe remote", "unsafe ref"} {
		t.Run(shape, func(t *testing.T) {
			remote, branch := "team/upstream", "review"
			if shape == "unsafe remote" {
				remote = "_unsafe"
			}
			if shape == "unsafe ref" {
				branch = "review;unsafe"
			}
			f := newPullSourceFixture(t, remote, branch)
			switch shape {
			case "slash remote/custom tracking":
				for _, repo := range []string{f.repo, f.native} {
					runGit(t, repo, "config", "remote."+remote+".fetch", "+refs/heads/*:refs/remotes/cache/*")
					runGit(t, repo, "fetch", remote)
				}
				if got := strings.TrimSpace(runGit(t, f.repo, "rev-parse", "--abbrev-ref", "@{upstream}")); got != "cache/review" {
					t.Fatalf("upstream storage=%q", got)
				}
			case "local dot":
				for _, repo := range []string{f.repo, f.native} {
					runGit(t, repo, "branch", "local-source", f.sourceHead)
					runGit(t, repo, "branch", "--set-upstream-to=local-source")
				}
			case "primary of multiple merge refs":
				runGit(t, f.repo, "config", "--add", "branch.task-local.merge", "refs/heads/main")
			}
			config := runGit(t, f.repo, "config", "--local", "--list")
			if strings.HasPrefix(shape, "unsafe") {
				assertPullSourceRefused(t, f)
				return
			}
			if shape == "primary of multiple merge refs" {
				if got := strings.TrimSpace(runGit(t, f.repo, "rev-parse", "@{upstream}")); got != f.sourceHead {
					t.Fatalf("Git primary upstream=%q want=%q", got, f.sourceHead)
				}
			} else {
				runGit(t, f.native, "pull", "--ff-only")
				if pullHead(t, f.native) != f.sourceHead {
					t.Fatal("native configured Pull missed source")
				}
			}
			assertPullSourceHead(t, f, false)
			if runGit(t, f.repo, "config", "--local", "--list") != config {
				t.Fatal("Pull changed source configuration")
			}
		})
	}
}

func assertPullSourceRefused(t *testing.T, f pullSourceFixture) {
	t.Helper()
	operator := NewGitOperator(f.repo, newTestLogger(t), nil)
	if operator.getUpstreamRef(context.Background()) == "" {
		t.Fatal("unsafe fixture must report an upstream")
	}
	config := runGit(t, f.repo, "config", "--local", "--list")
	result, err := operator.Pull(context.Background(), false)
	if err != nil || result.Success || result.Error == "" || result.Output != "" || pullHead(t, f.repo) != f.baseline {
		t.Fatalf("unsafe source was not refused before Pull: result=%+v err=%v", result, err)
	}
	if runGit(t, f.repo, "config", "--local", "--list") != config {
		t.Fatal("refusal changed configuration")
	}
}

// @covers AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.3
func TestGitOperatorPullManagedSourcePrecedence(t *testing.T) {
	for _, rebase := range []bool{false, true} {
		t.Run(fmt.Sprintf("rebase=%t", rebase), func(t *testing.T) {
			f := newPullSourceFixture(t, "upstream", "review")
			root := t.TempDir()
			bare, writer := filepath.Join(root, "managed.git"), filepath.Join(root, "writer")
			runGit(t, root, "clone", "--bare", localGitRemotePath(f.origin), bare)
			runGit(t, root, "clone", localGitRemotePath(bare), writer)
			configurePullCheckout(t, writer)
			runGit(t, writer, "checkout", "-b", "managed-review")
			commitPullFile(t, writer, "managed.txt", "managed source\n")
			runGit(t, writer, "push", "origin", "managed-review")
			managedHead := pullHead(t, writer)
			binding := &taskmodels.RemoteContribution{
				Version: taskmodels.RemoteContributionVersion, Provider: taskmodels.RemoteContributionProviderGitHub,
				Kind: taskmodels.RemoteContributionKindPullRequest, CanonicalURL: "https://github.com/acme/widget/pull/7",
				Number: 7, State: taskmodels.RemoteContributionStateOpen, BaseBranch: "main", HeadBranch: "managed-review", HeadSHA: managedHead,
				SourceRepository: taskmodels.RemoteContributionRepository{
					Host: "github.com", Path: "contributor/widget", RemoteURL: "https://github.com/contributor/widget.git",
				},
				CollaborationAllowed: true,
			}
			runGit(t, f.repo, "config", "url.file://"+localGitRemotePath(bare)+".insteadOf", binding.SourceRepository.RemoteURL)
			runGit(t, f.repo, "remote", "add", binding.ContributionRemoteName(), binding.SourceRepository.RemoteURL)
			operator := NewGitOperator(f.repo, newTestLogger(t), nil)
			operator.setRemoteContribution(binding)
			config := runGit(t, f.repo, "config", "--local", "--list")
			result, err := operator.Pull(context.Background(), rebase)
			if err != nil || !result.Success || pullHead(t, f.repo) != managedHead || managedHead == f.sourceHead {
				t.Fatalf("managed source precedence: result=%+v err=%v", result, err)
			}
			if runGit(t, f.repo, "config", "--local", "--list") != config {
				t.Fatal("managed Pull changed configuration")
			}
		})
	}
}

// @covers AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.3
func TestGitOperatorPullContributionDestinationKeepsUpstream(t *testing.T) {
	f := newPullSourceFixture(t, "upstream", "review")
	destination := &taskmodels.ContributionDestination{
		Version: taskmodels.ContributionDestinationVersion, Provider: taskmodels.ContributionDestinationProviderGitHub,
		SourceRepository: taskmodels.ContributionDestinationRepository{
			Host: "github.com", Path: "kdlbs/kandev", ProviderID: "100", RemoteURL: "https://github.com/kdlbs/kandev.git",
		},
		TargetRepository: taskmodels.ContributionDestinationRepository{
			Host: "github.com", Path: "agent/kandev", ProviderID: "200", RemoteURL: "https://github.com/agent/kandev.git",
		},
	}
	runGit(t, f.repo, "config", "url."+f.origin+".insteadOf", destination.TargetRepository.RemoteURL)
	runGit(t, f.repo, "remote", "add", destination.ContributionRemoteName(), destination.TargetRepository.RemoteURL)
	runGit(t, f.repo, "config", "branch.task-local.pushRemote", destination.ContributionRemoteName())
	operator := NewGitOperator(f.repo, newTestLogger(t), nil)
	operator.setContributionDestination(destination)
	config := runGit(t, f.repo, "config", "--local", "--list")
	runGit(t, f.native, "pull", "--ff-only")
	result, err := operator.Pull(context.Background(), false)
	if err != nil || !result.Success || pullHead(t, f.repo) != f.sourceHead || pullHead(t, f.native) != f.sourceHead {
		t.Fatalf("push destination displaced Pull source: result=%+v err=%v", result, err)
	}
	if runGit(t, f.repo, "config", "--local", "--list") != config || strings.TrimSpace(runGit(t, f.origin, "rev-parse", "refs/heads/task-local")) != f.baseline {
		t.Fatal("Pull changed push routing or destination ref")
	}
}

func newPullSourceFixture(t *testing.T, remote, sourceBranch string) pullSourceFixture {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	origin := strings.TrimSpace(runGit(t, repo, "remote", "get-url", "origin"))
	baseline := pullHead(t, repo)
	root := t.TempDir()
	source := filepath.Join(root, "source.git")
	runGit(t, root, "clone", "--bare", localGitRemotePath(origin), source)
	runGit(t, repo, "checkout", "-b", "task-local")
	if remote != "origin" {
		runGit(t, repo, "push", "origin", "HEAD:refs/heads/task-local")
		runGit(t, repo, "remote", "add", remote, localGitRemotePath(source))
	} else {
		source = origin
	}
	runGit(t, repo, "push", remote, "HEAD:refs/heads/"+sourceBranch)
	writer := filepath.Join(root, "writer")
	runGit(t, root, "clone", localGitRemotePath(source), writer)
	configurePullCheckout(t, writer)
	runGit(t, writer, "checkout", sourceBranch)
	commitPullFile(t, writer, "source.txt", "upstream content\n")
	sourceHead := pullHead(t, writer)
	runGit(t, writer, "push", "origin", sourceBranch)
	runGit(t, repo, "fetch", remote)
	runGit(t, repo, "branch", "--set-upstream-to="+remote+"/"+sourceBranch)
	configurePullCheckout(t, repo)
	native := filepath.Join(root, "native")
	runGit(t, root, "clone", localGitRemotePath(origin), native)
	configurePullCheckout(t, native)
	runGit(t, native, "checkout", "-b", "task-local", baseline)
	if remote != "origin" {
		runGit(t, native, "remote", "add", remote, localGitRemotePath(source))
		runGit(t, native, "fetch", remote)
	}
	runGit(t, native, "branch", "--set-upstream-to="+remote+"/"+sourceBranch)
	return pullSourceFixture{repo, native, origin, source, baseline, sourceHead, remote, sourceBranch}
}

func configurePullCheckout(t *testing.T, repo string) {
	t.Helper()
	runGit(t, repo, "config", "user.name", "Pull Test")
	runGit(t, repo, "config", "user.email", "pull@example.test")
	runGit(t, repo, "config", "commit.gpgsign", "false")
	runGit(t, repo, "config", "core.hooksPath", os.DevNull)
	runGit(t, repo, "config", "pull.rebase", "false")
	runGit(t, repo, "config", "pull.ff", "true")
}

func commitPullFile(t *testing.T, repo, name, content string) {
	t.Helper()
	writeFile(t, repo, name, content)
	runGit(t, repo, "add", name)
	runGit(t, repo, "commit", "-m", name)
}

func pullHead(t *testing.T, repo string) string {
	t.Helper()
	return strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
}

// @covers AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.1
// @covers AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.2
// @covers AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.6
func TestGitOperatorPullConfiguredUpstream(t *testing.T) {
	for _, source := range []struct{ remote, branch string }{
		{"upstream", "task-local"},
		{"origin", "review"},
		{"upstream", "review"},
	} {
		for _, rebase := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s/rebase=%t", source.remote, source.branch, rebase), func(t *testing.T) {
				f := newPullSourceFixture(t, source.remote, source.branch)
				tracker := NewWorkspaceTracker(f.repo, newTestLogger(t))
				t.Cleanup(tracker.Stop)
				before, err := tracker.getGitStatus(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if before.RemoteBranch != source.remote+"/"+source.branch || before.RemoteBehind != 1 || before.RemoteAhead != 0 {
					t.Fatalf("unexpected upstream state: %+v", before)
				}
				if pullHead(t, f.repo) != f.baseline {
					t.Fatal("fixture must start at baseline")
				}
				config := runGit(t, f.repo, "config", "--local", "--list")
				nativeArgs := []string{"pull", "--ff-only"}
				if rebase {
					nativeArgs = []string{"pull", "--rebase"}
				}
				runGit(t, f.native, nativeArgs...)
				if pullHead(t, f.native) != f.sourceHead {
					t.Fatal("native configured Pull did not reach source")
				}
				result, err := NewGitOperator(f.repo, newTestLogger(t), nil).Pull(context.Background(), rebase)
				head := pullHead(t, f.repo)
				t.Logf("before=%s upstream=%s behind=%d native=%s actual=%s result=%+v err=%v", f.baseline, before.RemoteBranch, before.RemoteBehind, f.sourceHead, head, result, err)
				if err != nil || !result.Success || head != f.sourceHead {
					t.Fatalf("Pull did not integrate configured source: HEAD=%s want=%s result=%+v err=%v", head, f.sourceHead, result, err)
				}
				if got := runGit(t, f.repo, "config", "--local", "--list"); got != config {
					t.Fatal("Pull changed Git configuration")
				}
			})
		}
	}
	t.Run("rebase replays local work", func(t *testing.T) {
		f := newPullSourceFixture(t, "upstream", "review")
		commitPullFile(t, f.repo, "local.txt", "local work\n")
		commitPullFile(t, f.native, "local.txt", "local work\n")
		runGit(t, f.native, "pull", "--rebase")
		result, err := NewGitOperator(f.repo, newTestLogger(t), nil).Pull(context.Background(), true)
		if err != nil || !result.Success {
			t.Fatalf("rebase Pull: result=%+v err=%v", result, err)
		}
		for _, repo := range []string{f.repo, f.native} {
			runGit(t, repo, "merge-base", "--is-ancestor", f.sourceHead, "HEAD")
			if got := runGit(t, repo, "show", "HEAD:local.txt"); got != "local work\n" {
				t.Fatalf("replayed content=%q", got)
			}
			if pullHead(t, repo) == f.sourceHead {
				t.Fatal("rebase lost local work")
			}
		}
	})
}
