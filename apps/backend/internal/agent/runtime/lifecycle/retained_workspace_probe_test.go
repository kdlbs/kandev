package lifecycle

import (
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetainedRepositoryProbeChecksIdentityAndPreservesLocalWork(t *testing.T) {
	for _, mode := range []string{"ahead-and-dirty", "instead-of-rewrite", "wrong-origin", "wrong-branch", "missing-git"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			destination := "secondary-'quoted-main"
			directory := filepath.Join(root, destination)
			require.NoError(t, os.MkdirAll(directory, 0o700))
			initGitRepoAt(t, directory)
			origin := "https://example.com/secondary.git"
			runGitForPreparerTest(t, directory, "remote", "add", "origin", origin)
			runGitForPreparerTest(t, directory, "update-ref", "refs/remotes/origin/main", "HEAD")
			runGitForPreparerTest(t, directory, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "unpublished local work")
			require.NoError(t, os.WriteFile(filepath.Join(directory, "untracked.txt"), []byte("keep this local work\n"), 0o600))
			switch mode {
			case "instead-of-rewrite":
				runGitForPreparerTest(t, directory, "config", "--local", "url.ssh://rewritten.example/.insteadOf", "https://example.com/")
			case "wrong-origin":
				runGitForPreparerTest(t, directory, "remote", "set-url", "origin", "https://example.com/another.git")
			case "wrong-branch":
				runGitForPreparerTest(t, directory, "checkout", "-b", "another")
			case "missing-git":
				require.NoError(t, os.Rename(filepath.Join(directory, ".git"), filepath.Join(directory, "saved-git")))
			}
			command := retainedWorkspaceProbeCommand(root, []WorkspaceRepositoryMaterialization{{Destination: destination, RepositoryURL: origin, BaseBranch: "main"}})
			var before string
			if mode != "missing-git" {
				before = runGitForPreparerTest(t, directory, "show-ref") + runGitForPreparerTest(t, directory, "status", "--porcelain")
			}
			probe := exec.Command("sh", "-c", command)
			probe.Env = filterLocalTestGitEnv(os.Environ())
			err := probe.Run()
			if mode == "ahead-and-dirty" || mode == "instead-of-rewrite" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			content, readErr := os.ReadFile(filepath.Join(directory, "untracked.txt"))
			require.NoError(t, readErr)
			require.Equal(t, "keep this local work\n", string(content))
			if mode != "missing-git" {
				require.Equal(t, before, runGitForPreparerTest(t, directory, "show-ref")+runGitForPreparerTest(t, directory, "status", "--porcelain"))
			}
		})
	}
}

func TestRetainedRepositoryProbeAcceptsContributionBranchWithSuffix(t *testing.T) {
	for _, mode := range []string{"valid-suffix", "wrong-contribution-remote", "wrong-upstream"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			destination := "widget-feature-remote"
			directory := filepath.Join(root, destination)
			require.NoError(t, os.MkdirAll(directory, 0o700))
			initGitRepoAt(t, directory)
			targetOrigin := "https://example.com/acme/widget.git"
			runGitForPreparerTest(t, directory, "remote", "add", "origin", targetOrigin)
			baseSHA := strings.TrimSpace(runGitForPreparerTest(t, directory, "rev-parse", "HEAD"))
			runGitForPreparerTest(t, directory, "update-ref", "refs/remotes/origin/main", baseSHA)

			binding := validTestRemoteContribution(7, "contributor/widget")
			binding.HeadSHA = baseSHA
			contributionRemote := binding.ContributionRemoteName()
			runGitForPreparerTest(t, directory, "remote", "add", contributionRemote, binding.SourceRepository.RemoteURL)
			runGitForPreparerTest(t, directory, "update-ref", "refs/remotes/"+contributionRemote+"/"+binding.HeadBranch, baseSHA)
			localBranch := binding.HeadBranch + "-kandev-retained"
			runGitForPreparerTest(t, directory, "checkout", "-b", localBranch)
			runGitForPreparerTest(t, directory, "branch", "--set-upstream-to", contributionRemote+"/"+binding.HeadBranch)
			runGitForPreparerTest(t, directory, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "unpublished contribution work")
			require.NoError(t, os.WriteFile(filepath.Join(directory, "untracked.txt"), []byte("keep contribution work\n"), 0o600))
			switch mode {
			case "wrong-contribution-remote":
				runGitForPreparerTest(t, directory, "remote", "set-url", contributionRemote, "https://github.com/another/widget.git")
			case "wrong-upstream":
				runGitForPreparerTest(t, directory, "branch", "--set-upstream-to", "origin/main")
			}

			probeCommand := retainedWorkspaceProbeCommand(root, []WorkspaceRepositoryMaterialization{{
				Destination: destination, RepositoryURL: targetOrigin, BaseBranch: binding.BaseBranch,
				CheckoutBranch: binding.HeadBranch, RemoteContribution: &binding,
			}})
			before := runGitForPreparerTest(t, directory, "show-ref") + runGitForPreparerTest(t, directory, "status", "--porcelain")
			probe := exec.Command("sh", "-c", probeCommand)
			probe.Env = filterLocalTestGitEnv(os.Environ())
			err := probe.Run()
			if mode == "valid-suffix" {
				require.NoError(t, err, "a valid contribution upstream must allow its collision-suffixed branch")
			} else {
				require.Error(t, err, "contribution identity mismatches must fail closed")
			}
			content, readErr := os.ReadFile(filepath.Join(directory, "untracked.txt"))
			require.NoError(t, readErr)
			require.Equal(t, "keep contribution work\n", string(content))
			require.Equal(t, before, runGitForPreparerTest(t, directory, "show-ref")+runGitForPreparerTest(t, directory, "status", "--porcelain"))
		})
	}
}

func TestRetainedRepositoryProbeRequiresCommitHead(t *testing.T) {
	for _, mode := range []string{"missing-object", "non-commit-object"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			destination := "secondary-main"
			directory := filepath.Join(root, destination)
			require.NoError(t, os.MkdirAll(directory, 0o700))
			initGitRepoAt(t, directory)
			origin := "https://example.com/secondary.git"
			runGitForPreparerTest(t, directory, "remote", "add", "origin", origin)
			require.NoError(t, os.WriteFile(filepath.Join(directory, "untracked.txt"), []byte("preserve local work\n"), 0o600))
			if mode == "missing-object" {
				require.NoError(t, os.WriteFile(filepath.Join(directory, ".git", "refs", "heads", "main"), []byte(strings.Repeat("1", 40)+"\n"), 0o600))
			} else {
				blobPath := filepath.Join(directory, "blob.txt")
				require.NoError(t, os.WriteFile(blobPath, []byte("not a commit\n"), 0o600))
				blobSHA := strings.TrimSpace(runGitForPreparerTest(t, directory, "hash-object", "-w", "-t", "blob", blobPath))
				require.NoError(t, os.WriteFile(filepath.Join(directory, ".git", "refs", "heads", "main"), []byte(blobSHA+"\n"), 0o600))
			}
			refPath := filepath.Join(directory, ".git", "refs", "heads", "main")
			beforeRef, err := os.ReadFile(refPath)
			require.NoError(t, err)

			command := retainedWorkspaceProbeCommand(root, []WorkspaceRepositoryMaterialization{{
				Destination: destination, RepositoryURL: origin, BaseBranch: "main",
			}})
			probe := exec.Command("sh", "-c", command)
			probe.Env = filterLocalTestGitEnv(os.Environ())
			require.Error(t, probe.Run(), "a missing or non-commit HEAD object must fail verification")
			afterRef, err := os.ReadFile(refPath)
			require.NoError(t, err)
			require.Equal(t, beforeRef, afterRef, "verification must preserve the invalid ref for repair")
			content, err := os.ReadFile(filepath.Join(directory, "untracked.txt"))
			require.NoError(t, err)
			require.Equal(t, "preserve local work\n", string(content))
		})
	}
}
