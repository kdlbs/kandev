package lifecycle

import (
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRetainedRepositoryProbeChecksIdentityAndPreservesLocalWork(t *testing.T) {
	for _, mode := range []string{"ahead-and-dirty", "wrong-origin", "wrong-branch", "missing-git"} {
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
			if mode == "ahead-and-dirty" {
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
