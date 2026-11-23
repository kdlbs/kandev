package lifecycle

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type sshContributionFixture struct {
	root      string
	workspace string
	targetURL string
	binding   *models.RemoteContribution
	sourceSHA string
}

// newSSHContributionFixture builds a target repository and a contributor fork
// whose feature/remote branch holds the contribution head. The provider URLs
// resolve to local bare repositories through a private global Git config,
// which child processes (including PowerShell on Windows) inherit.
func newSSHContributionFixture(t *testing.T) sshContributionFixture {
	t.Helper()
	root := t.TempDir()
	targetBare := filepath.Join(root, "target.git")
	sourceBare := filepath.Join(root, "source.git")
	targetSeed := filepath.Join(root, "target-seed")
	sourceSeed := filepath.Join(root, "source-seed")
	workspace := filepath.Join(root, "workspace")
	for _, args := range [][]string{
		{"init", "--bare", "--initial-branch=main", targetBare},
		{"init", "--bare", "--initial-branch=main", sourceBare},
		{"init", "--initial-branch=main", targetSeed},
		{"init", "--initial-branch=main", sourceSeed},
	} {
		runSSHContributionGit(t, root, args...)
	}
	for _, repo := range []string{targetSeed, sourceSeed} {
		runSSHContributionGit(t, repo, "config", "user.email", "test@example.com")
		runSSHContributionGit(t, repo, "config", "user.name", "Test User")
	}
	writeFixtureFile(t, filepath.Join(targetSeed, "README.md"), "target\n")
	runSSHContributionGit(t, targetSeed, "add", "README.md")
	runSSHContributionGit(t, targetSeed, "commit", "-m", "target base")
	runSSHContributionGit(t, targetSeed, "remote", "add", "origin", targetBare)
	runSSHContributionGit(t, targetSeed, "push", "origin", "main")

	writeFixtureFile(t, filepath.Join(sourceSeed, "README.md"), "source\n")
	runSSHContributionGit(t, sourceSeed, "add", "README.md")
	runSSHContributionGit(t, sourceSeed, "commit", "-m", "source base")
	runSSHContributionGit(t, sourceSeed, "checkout", "-b", "feature/remote")
	writeFixtureFile(t, filepath.Join(sourceSeed, "change.txt"), "contribution\n")
	runSSHContributionGit(t, sourceSeed, "add", "change.txt")
	runSSHContributionGit(t, sourceSeed, "commit", "-m", "source contribution")
	sourceSHA := strings.TrimSpace(runSSHContributionGit(t, sourceSeed, "rev-parse", "HEAD"))
	runSSHContributionGit(t, sourceSeed, "remote", "add", "origin", sourceBare)
	runSSHContributionGit(t, sourceSeed, "push", "origin", "main", "feature/remote")

	targetURL := "https://github.com/acme/widget.git"
	sourceURL := "https://github.com/contributor/widget.git"
	configPath := filepath.Join(root, "gitconfig")
	config := "[url \"" + fixtureFileURL(targetBare) + "\"]\n\tinsteadOf = " + targetURL + "\n" +
		"[url \"" + fixtureFileURL(sourceBare) + "\"]\n\tinsteadOf = " + sourceURL + "\n"
	writeFixtureFile(t, configPath, config)
	t.Setenv("GIT_CONFIG_GLOBAL", configPath)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	binding := &models.RemoteContribution{
		Version:      models.RemoteContributionVersion,
		Provider:     models.RemoteContributionProviderGitHub,
		Kind:         models.RemoteContributionKindPullRequest,
		CanonicalURL: "https://github.com/acme/widget/pull/7",
		Number:       7,
		State:        models.RemoteContributionStateOpen,
		BaseBranch:   "main",
		HeadBranch:   "feature/remote",
		HeadSHA:      sourceSHA,
		SourceRepository: models.RemoteContributionRepository{
			Host: "github.com", Path: "contributor/widget", RemoteURL: sourceURL,
		},
		CollaborationAllowed: true,
	}
	return sshContributionFixture{root: root, workspace: workspace, targetURL: targetURL, binding: binding, sourceSHA: sourceSHA}
}

func fixtureFileURL(path string) string {
	if runtime.GOOS == sshRemoteGOOSWindows {
		return "file:///" + filepath.ToSlash(path)
	}
	return "file://" + path
}
