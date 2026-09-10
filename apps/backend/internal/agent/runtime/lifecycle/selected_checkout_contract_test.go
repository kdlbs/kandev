package lifecycle

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSelectedCheckoutSharedResolvers(t *testing.T) {
	for name, resolve := range selectedCheckoutResolvers() {
		t.Run(name, func(t *testing.T) {
			f := newBranchPrepareFixture(t)
			head := seedSelectedCheckout(t, f, false)
			req := selectedCheckoutRequest(f, "saved-clone", selectedTestBranch, 3527, "")
			script, err := resolve(req)
			require.NoError(t, err)
			script = strings.ReplaceAll(script, shellQuote("/workspace"), shellQuote(f.workspace))
			out, err := f.run(t, "bash", "-c", script)
			require.NoError(t, err, out)
			require.Equal(t, head, f.git(t, "-C", f.workspace, "rev-parse", "HEAD"))
			require.Equal(t, selectedTestBranch, f.git(t, "-C", f.workspace, "branch", "--show-current"))
		})
	}
}

func selectedCheckoutResolvers() map[string]func(*ExecutorCreateRequest) (string, error) {
	return map[string]func(*ExecutorCreateRequest) (string, error){
		"sprites":    newTestSpritesExecutor(nil).resolvePrepareScript,
		"docker":     (&DockerExecutor{}).resolvePrepareScript,
		"kubernetes": kubernetesPrepareScript,
		"ssh": func(req *ExecutorCreateRequest) (string, error) {
			return (&SSHExecutor{}).resolvePrepareScript(req, "/workspace", "")
		},
	}
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.3
func TestSelectedCheckoutRemoteContributionOwnsCheckout(t *testing.T) {
	f := newBranchPrepareFixture(t)
	head := seedSelectedCheckout(t, f, true)
	binding := validTestRemoteContribution(3527, "fork/widget")
	binding.HeadBranch, binding.HeadSHA = selectedTestBranch, head
	f.git(t, "config", "--global", "url."+f.origin+".insteadOf", binding.SourceRepository.RemoteURL)
	req := selectedCheckoutRequest(f, "saved-clone", "missing-selection", 9999, "")
	req.RemoteContributions = map[string]models.RemoteContribution{"": binding}
	for name, resolve := range selectedCheckoutResolvers() {
		t.Run(name, func(t *testing.T) {
			script, err := resolve(req)
			require.NoError(t, err)
			require.NotContains(t, script, "materialize explicit checkout")
			require.NotContains(t, script, "ensure session feature branch")
		})
	}
	out, err := f.run(t, "bash", "-e", "-c", resolveSelectedSprites(t, f, req))
	require.NoError(t, err, out)
	require.Equal(t, head, f.git(t, "-C", f.workspace, "rev-parse", "HEAD"))
	require.Equal(t, selectedTestBranch, f.git(t, "-C", f.workspace, "branch", "--show-current"))
	require.Equal(t, binding.ContributionRemoteName(), f.git(t, "-C", f.workspace, "config", "branch."+selectedTestBranch+".remote"))
}

func TestBuildLaunchMetadataSelectedCheckoutAuthority(t *testing.T) {
	for _, branch := range []string{"", selectedTestBranch} {
		req := &LaunchRequest{CheckoutBranch: branch, PRNumber: 3527,
			Metadata:       map[string]any{metadataCheckoutBranch: "forged", metadataCheckoutRef: "refs/pull/1/head", metadataPreserveCheckout: true},
			ExecutorConfig: map[string]string{metadataCheckoutBranch: "profile", metadataCheckoutRef: "refs/pull/2/head"}}
		metadata := buildLaunchMetadata(req, "", "", branch)
		if branch == "" {
			require.NotContains(t, metadata, metadataCheckoutBranch)
			require.NotContains(t, metadata, metadataCheckoutRef)
			require.NotContains(t, metadata, metadataPreserveCheckout)
		} else {
			require.Equal(t, branch, metadata[metadataCheckoutBranch])
			require.Equal(t, "refs/pull/3527/head", metadata[metadataCheckoutRef])
			require.Equal(t, false, metadata[metadataPreserveCheckout])
		}
		require.Equal(t, "forged", req.Metadata[metadataCheckoutBranch])
	}
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.4
func TestSelectedCheckoutRejectsInvalidSelection(t *testing.T) {
	for _, tc := range []struct {
		branch string
		pr     int
	}{
		{"--detach", 3527}, {"@{-1}", 3527}, {"bad..branch", 3527}, {selectedTestBranch, -1},
	} {
		t.Run(tc.branch, func(t *testing.T) {
			f := newBranchPrepareFixture(t)
			seedSelectedCheckout(t, f, false)
			req := selectedCheckoutRequest(f, "saved-clone", tc.branch, tc.pr, "")
			out, err := f.run(t, "bash", "-c", resolveSelectedSprites(t, f, req))
			require.Error(t, err, out)
			base := f.git(t, "--git-dir="+f.origin, "rev-parse", "main")
			require.Equal(t, base, f.git(t, "-C", f.workspace, "rev-parse", "HEAD"))
		})
	}
}

func TestSelectedCheckoutExistingMatchingBranch(t *testing.T) {
	f := newBranchPrepareFixture(t)
	head := seedSelectedCheckout(t, f, true)
	f.git(t, "clone", f.origin, f.workspace)
	f.git(t, "-C", f.workspace, "branch", selectedTestBranch, "origin/"+selectedTestBranch)
	req := selectedCheckoutRequest(f, "current", selectedTestBranch, 3527, "")
	out, err := f.run(t, "bash", "-c", resolveSelectedSprites(t, f, req))
	require.NoError(t, err, out)
	require.Equal(t, head, f.git(t, "-C", f.workspace, "rev-parse", "HEAD"))
	// No fork remote or push route is introduced for review-only checkout.
	require.Equal(t, "origin", f.git(t, "-C", f.workspace, "remote"))
	_, err = f.run(t, "git", "-C", f.workspace, "config", "--get", "branch."+selectedTestBranch+".pushRemote")
	require.Error(t, err)
}

func TestSelectedCheckoutGeneratedBranch(t *testing.T) {
	f := newBranchPrepareFixture(t)
	req := selectedCheckoutRequest(f, "saved-clone", "", 0, "")
	out, err := f.run(t, "bash", "-c", resolveSelectedSprites(t, f, req))
	require.NoError(t, err, out)
	require.Equal(t, "feature/hello-abcdef", f.git(t, "-C", f.workspace, "branch", "--show-current"))
	require.Equal(t, f.git(t, "-C", filepath.Join(f.root, "seed"), "rev-parse", "HEAD"), f.git(t, "-C", f.workspace, "rev-parse", "HEAD"))
}
