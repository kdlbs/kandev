package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const selectedTestBranch = "feature/fork-review"

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.1
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.2
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.3
func TestSpritesPrepareScript_SelectedCheckout(t *testing.T) {
	for _, template := range []string{"current", "saved-clone"} {
		for _, source := range []string{"fork", "same-repo", "branch"} {
			t.Run(template+"/"+source, func(t *testing.T) {
				f := newBranchPrepareFixture(t)
				head := seedSelectedCheckout(t, f, source != "fork")
				pr := 3527
				if source == "branch" {
					pr = 0
				}
				req := selectedCheckoutRequest(f, template, selectedTestBranch, pr, "")
				script := resolveSelectedSprites(t, f, req)
				out, err := f.run(t, "bash", "-c", script)
				require.NoError(t, err, out)
				require.Equal(t, selectedTestBranch, f.git(t, "-C", f.workspace, "branch", "--show-current"))
				require.Equal(t, head, f.git(t, "-C", f.workspace, "rev-parse", "HEAD"))
				require.Equal(t, "main", req.Metadata[MetadataKeyBaseBranch])
			})
		}
	}
}

func TestSpritesPrepareScript_SelectedCheckoutBeforeRepositorySetup(t *testing.T) {
	f := newBranchPrepareFixture(t)
	head := seedSelectedCheckout(t, f, false)
	script := DefaultPrepareScript("sprites")
	for _, key := range []string{"git.identity_setup", "github.auth_setup",
		"kandev.agents.install", "kandev.agentctl.install", "kandev.agentctl.start"} {
		script = strings.ReplaceAll(script, "{{"+key+"}}", ":")
	}
	launch := &LaunchRequest{
		BaseBranch: "main", CheckoutBranch: selectedTestBranch, PRNumber: 3527,
		SetupScript: script,
		Metadata: map[string]any{
			"repository_clone_url":    f.origin,
			"repository_setup_script": `test "$(git rev-parse HEAD)" = ` + head,
		},
	}
	taskBranch := nonWorktreeTaskBranch(&EnvPrepareRequest{
		TaskID: "abcdef", TaskTitle: "hello", CheckoutBranch: selectedTestBranch,
	})
	req := &ExecutorCreateRequest{Metadata: buildLaunchMetadata(launch, "", "", taskBranch)}
	out, err := f.run(t, "bash", "-c", resolveSelectedSprites(t, f, req))
	require.NoError(t, err, out)
}

func TestSpritesPrepareScript_SelectedCheckoutRetainedProbeUsesSafeDirectory(t *testing.T) {
	f := newBranchPrepareFixture(t)
	seedSelectedCheckout(t, f, false)
	f.git(t, "clone", f.origin, f.workspace)
	req := selectedCheckoutRequest(f, "current", selectedTestBranch, 3527, selectedTestBranch)
	// Git's test hook simulates a retained checkout owned by another UID.
	f.env = append(f.env, "GIT_TEST_ASSUME_DIFFERENT_OWNER=1")
	out, err := f.run(t, "bash", "-c", resolveSelectedSprites(t, f, req))
	require.NoError(t, err, out)
	require.Equal(t, "main", f.git(t, "-C", f.workspace, "branch", "--show-current"))
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.4
func TestSpritesPrepareScript_SelectedCheckoutMissingRef(t *testing.T) {
	for _, pr := range []int{0, 3527} {
		f := newBranchPrepareFixture(t)
		req := selectedCheckoutRequest(f, "saved-clone", selectedTestBranch, pr, "")
		script := resolveSelectedSprites(t, f, req)
		out, err := f.run(t, "bash", "-c", script+"\nprintf started > agent-started\n")
		require.Error(t, err, out)
		_, err = os.Stat(filepath.Join(f.root, "agent-started"))
		require.True(t, os.IsNotExist(err), "agent must not start after checkout failure")
	}
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.5
func TestSpritesPrepareScript_SelectedCheckoutResume(t *testing.T) {
	f := newBranchPrepareFixture(t)
	head := seedSelectedCheckout(t, f, false)
	// Recorded branch identity must not suppress checkout in newly recreated compute.
	req := selectedCheckoutRequest(f, "current", selectedTestBranch, 3527, selectedTestBranch)
	out, err := f.run(t, "bash", "-c", resolveSelectedSprites(t, f, req))
	require.NoError(t, err, out)
	require.Equal(t, head, f.git(t, "-C", f.workspace, "rev-parse", "HEAD"))
	f.git(t, "-C", f.workspace, "checkout", "-b", "local-work")
	f.git(t, "-C", f.workspace, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "local commit")
	localHead := f.git(t, "-C", f.workspace, "rev-parse", "HEAD")
	require.NoError(t, os.WriteFile(filepath.Join(f.workspace, "review.txt"), []byte("local edits"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(f.workspace, "untracked"), []byte("keep"), 0o600))
	out, err = f.run(t, "bash", "-c", resolveSelectedSprites(t, f, req))
	require.NoError(t, err, out)
	require.Equal(t, "local-work", f.git(t, "-C", f.workspace, "branch", "--show-current"))
	require.Equal(t, localHead, f.git(t, "-C", f.workspace, "rev-parse", "HEAD"))
	data, err := os.ReadFile(filepath.Join(f.workspace, "untracked"))
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
	data, err = os.ReadFile(filepath.Join(f.workspace, "review.txt"))
	require.NoError(t, err)
	require.Equal(t, "local edits", string(data))
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.4
func TestSpritesPrepareScript_SelectedCheckoutConflict(t *testing.T) {
	f := newBranchPrepareFixture(t)
	seedSelectedCheckout(t, f, false)
	f.git(t, "clone", f.origin, f.workspace)
	f.git(t, "-C", f.workspace, "branch", selectedTestBranch)
	base := f.git(t, "-C", f.workspace, "rev-parse", "HEAD")
	req := selectedCheckoutRequest(f, "current", selectedTestBranch, 3527, "")
	out, err := f.run(t, "bash", "-c", resolveSelectedSprites(t, f, req))
	require.Error(t, err, out)
	require.Equal(t, base, f.git(t, "-C", f.workspace, "rev-parse", selectedTestBranch))
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.4
func TestSpritesPrepareScript_SelectedCheckoutLiteralBranch(t *testing.T) {
	f := newBranchPrepareFixture(t)
	head := seedSelectedCheckout(t, f, false)
	branch := "review/'$(touch${IFS}injected)`touch${IFS}injected`;literal"
	req := selectedCheckoutRequest(f, "current", branch, 3527, "")
	out, err := f.run(t, "bash", "-c", resolveSelectedSprites(t, f, req))
	require.NoError(t, err, out)
	require.Equal(t, branch, f.git(t, "-C", f.workspace, "branch", "--show-current"))
	require.Equal(t, head, f.git(t, "-C", f.workspace, "rev-parse", "HEAD"))
	_, err = os.Stat(filepath.Join(f.workspace, "injected"))
	require.True(t, os.IsNotExist(err))
}

func seedSelectedCheckout(t *testing.T, f branchPrepareFixture, publishBranch bool) string {
	t.Helper()
	seed := filepath.Join(f.root, "seed")
	f.git(t, "-C", seed, "checkout", "-b", selectedTestBranch)
	require.NoError(t, os.WriteFile(filepath.Join(seed, "review.txt"), []byte("PR content"), 0o600))
	f.git(t, "-C", seed, "add", "review.txt")
	f.git(t, "-C", seed, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "PR head")
	f.git(t, "-C", seed, "push", f.origin, "HEAD:refs/pull/3527/head")
	if publishBranch {
		f.git(t, "-C", seed, "push", f.origin, selectedTestBranch)
	}
	return f.git(t, "-C", seed, "rev-parse", "HEAD")
}

func selectedCheckoutRequest(f branchPrepareFixture, template, branch string, pr int, previous string) *ExecutorCreateRequest {
	script := DefaultPrepareScript("sprites")
	if template == "saved-clone" {
		script = "git clone --no-local --depth=1 --branch {{repository.branch}} {{repository.clone_url}} {{workspace.path}}\n"
	}
	for _, key := range []string{"git.identity_setup", "github.auth_setup", "repository.setup_script",
		"kandev.agents.install", "kandev.agentctl.install", "kandev.agentctl.start"} {
		script = strings.ReplaceAll(script, "{{"+key+"}}", ":")
	}
	launch := &LaunchRequest{BaseBranch: "main", CheckoutBranch: branch, PRNumber: pr,
		SetupScript: script, Metadata: map[string]any{"repository_clone_url": f.origin}}
	if previous != "" {
		launch.Metadata[MetadataKeyWorktreeBranch] = previous
	}
	taskBranch := nonWorktreeTaskBranch(&EnvPrepareRequest{TaskID: "abcdef", TaskTitle: "hello", CheckoutBranch: branch, WorktreeBranch: previous})
	return &ExecutorCreateRequest{Metadata: buildLaunchMetadata(launch, "", "", taskBranch)}
}

func resolveSelectedSprites(t *testing.T, f branchPrepareFixture, req *ExecutorCreateRequest) string {
	t.Helper()
	script, err := newTestSpritesExecutor(nil).resolvePrepareScript(req)
	require.NoError(t, err)
	return strings.ReplaceAll(script, shellQuote(spritesWorkspacePath), shellQuote(f.workspace))
}
