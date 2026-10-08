package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

const plainHTTPPath = "new file mode.txt"

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.6
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.7
func TestGitComparisonPlainOutputHTTP(t *testing.T) {
	fixture := newGitAPIFixture(t)
	writeFileAPI(t, fixture.repo, plainHTTPPath, "before\n")
	runGitAPI(t, fixture.repo, "add", ".")
	runGitAPI(t, fixture.repo, "commit", "-m", "tracked baseline")
	base := fixture.head(t)
	writeFileAPI(t, fixture.repo, plainHTTPPath, "after \x1b[31mliteral\x1b[0m\n")
	runGitAPI(t, fixture.repo, "add", ".")
	runGitAPI(t, fixture.repo, "commit", "-m", "literal content")
	head := fixture.head(t)
	for _, mode := range []struct{ name, ui, diff string }{
		{"disabled", "false", "false"},
		{"forced", "always", "always"},
		{"diff_override", "false", "always"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			runGitAPI(t, fixture.repo, "config", "color.ui", mode.ui)
			runGitAPI(t, fixture.repo, "config", "color.diff", mode.diff)
			before := plainHTTPRepositoryState(t, fixture.repo)
			t.Cleanup(func() {
				if plainHTTPRepositoryState(t, fixture.repo) != before {
					t.Error("HTTP comparison changed repository state")
				}
			})
			patch := runGitAPI(t, fixture.repo, "diff", "--no-color", "--src-prefix=a/", "--dst-prefix=b/", base)
			t.Run("commit", func(t *testing.T) {
				var result process.CommitDiffResult
				readStatusMetadataHTTP(t, fixture.server, "/api/v1/git/commit/"+head, &result)
				if !result.Success || result.CommitSHA != head || result.FilesChanged != 1 || len(result.Files) != 1 || result.Insertions != 1 || result.Deletions != 1 {
					t.Fatalf("commit = %+v", result)
				}
				assertStatusMetadataHTTPFile(t, result.Files, plainHTTPPath, plainHTTPPath, "modified", 1, 1, patch)
			})
			t.Run("cumulative", func(t *testing.T) {
				var result process.CumulativeDiffResult
				readStatusMetadataHTTP(t, fixture.server, "/api/v1/git/cumulative-diff?base="+base, &result)
				if !result.Success || result.BaseCommit != base || result.HeadCommit != head || len(result.Files) != 1 || result.TotalCommits != 1 || result.TruncatedFilesCount != 0 {
					t.Fatalf("cumulative = %+v", result)
				}
				assertStatusMetadataHTTPFile(t, result.Files, plainHTTPPath, plainHTTPPath, "modified", 1, 1, patch)
			})
		})
	}
}

func plainHTTPRepositoryState(t *testing.T, repo string) string {
	t.Helper()
	configBytes, err := os.ReadFile(filepath.Join(repo, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	return string(configBytes) + runGitAPI(t, repo, "rev-parse", "HEAD") +
		runGitAPI(t, repo, "show-ref") + runGitAPI(t, repo, "ls-files", "--stage", "-z") +
		runGitAPI(t, repo, "--no-optional-locks", "status", "--porcelain=v1", "-z") +
		runGitAPI(t, repo, "diff", "--no-color", "--binary", "HEAD")
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.6
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.7
func TestGitComparisonPlainOutputMultiRepoHTTP(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	colorConfig := "[color]\n\tui = always\n\tdiff = always\n"
	configPath := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(configPath, []byte(colorConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		data, err := os.ReadFile(configPath)
		if err != nil || string(data) != colorConfig {
			t.Errorf("comparison changed global color config: %q, err = %v", data, err)
		}
	})
	root := t.TempDir()
	alpha := seedStatusMetadataHTTPRepo(t, root, "alpha", plainHTTPPath, false)
	beta := seedStatusMetadataHTTPRepo(t, root, "beta", plainHTTPPath, true)
	alpha.patch = runGitAPI(t, alpha.dir, "diff", "--no-color", "--src-prefix=a/", "--dst-prefix=b/", alpha.base)
	beta.patch = runGitAPI(t, beta.dir, "diff", "--no-color", "--src-prefix=a/", "--dst-prefix=b/", beta.base)
	for _, repo := range []statusMetadataHTTPRepo{alpha, beta} {
		runGitAPI(t, repo.dir, "config", "color.ui", "always")
		runGitAPI(t, repo.dir, "config", "color.diff", "always")
		before := plainHTTPRepositoryState(t, repo.dir)
		t.Cleanup(func() {
			if plainHTTPRepositoryState(t, repo.dir) != before {
				t.Errorf("comparison changed repository %s", repo.name)
			}
		})
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.InstanceConfig{WorkDir: root, AgentEnv: os.Environ(), BaseBranches: map[string]string{"alpha": "main", "beta": "main"}}
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	server := NewServer(cfg, manager, nil, nil, log)
	for _, repo := range []statusMetadataHTTPRepo{alpha, beta} {
		t.Run(repo.name, func(t *testing.T) { assertPlainHTTPSelected(t, server, repo) })
	}
	t.Run("aggregate", func(t *testing.T) {
		var result process.CumulativeDiffResult
		readStatusMetadataHTTP(t, server, "/api/v1/git/cumulative-diff?base="+alpha.base, &result)
		if !result.Success || len(result.Files) != 2 || result.TotalCommits != 2 || result.TruncatedFilesCount != 0 {
			t.Fatalf("aggregate = %+v", result)
		}
		for _, repo := range []statusMetadataHTTPRepo{alpha, beta} {
			entry := assertStatusMetadataHTTPFile(t, result.Files, repo.name+"\x00"+plainHTTPPath, plainHTTPPath, repo.status, repo.additions, repo.deletions, repo.patch)
			if entry["repository_name"] != repo.name || entry["base_ref"] != repo.base {
				t.Errorf("repository identity = %#v", entry)
			}
			if _, present := entry["is_submodule"]; present {
				t.Errorf("ordinary repository marked as submodule: %#v", entry)
			}
		}
	})
}

func assertPlainHTTPSelected(t *testing.T, server *Server, repo statusMetadataHTTPRepo) {
	t.Helper()
	t.Run("commit", func(t *testing.T) {
		var result process.CommitDiffResult
		readStatusMetadataHTTP(t, server, "/api/v1/git/commit/"+repo.head+"?repo="+repo.name, &result)
		if !result.Success || result.CommitSHA != repo.head || result.FilesChanged != 1 || len(result.Files) != 1 || result.Insertions != repo.additions || result.Deletions != repo.deletions {
			t.Fatalf("selected commit = %+v", result)
		}
		assertStatusMetadataHTTPFile(t, result.Files, plainHTTPPath, plainHTTPPath, repo.status, repo.additions, repo.deletions, repo.patch)
	})
	t.Run("cumulative", func(t *testing.T) {
		var result process.CumulativeDiffResult
		readStatusMetadataHTTP(t, server, "/api/v1/git/cumulative-diff?base="+repo.base+"&repo="+repo.name, &result)
		if !result.Success || result.BaseCommit != repo.base || result.HeadCommit != repo.head || len(result.Files) != 1 || result.TotalCommits != 1 {
			t.Fatalf("selected cumulative = %+v", result)
		}
		assertStatusMetadataHTTPFile(t, result.Files, plainHTTPPath, plainHTTPPath, repo.status, repo.additions, repo.deletions, repo.patch)
	})
}
