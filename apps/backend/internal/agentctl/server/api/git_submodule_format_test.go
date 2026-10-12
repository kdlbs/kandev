package api

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

const submoduleHTTPPath = "vendor/lib"

type submoduleHTTPRepo struct {
	name, dir, child, base, head, old, next string
	parentPatch, childPatch                 string
}

func initSubmoduleHTTPRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitAPI(t, dir, "init", "--initial-branch=main")
	runGitAPI(t, dir, "config", "user.name", "Submodule HTTP Test")
	runGitAPI(t, dir, "config", "user.email", "submodule@test.invalid")
	runGitAPI(t, dir, "config", "core.hooksPath", os.DevNull)
	runGitAPI(t, dir, "config", "core.autocrlf", "false")
}

func submoduleHTTPPatch(t *testing.T, repo, base string) string {
	t.Helper()
	return runGitAPI(t, repo, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--submodule=short", "--src-prefix=a/", "--dst-prefix=b/", base)
}

func seedSubmoduleHTTPRepo(t *testing.T, root, name string) submoduleHTTPRepo {
	t.Helper()
	source := t.TempDir()
	initSubmoduleHTTPRepo(t, source)
	writeFileAPI(t, source, "README.md", name+" child before\n")
	runGitAPI(t, source, "add", ".")
	runGitAPI(t, source, "commit", "-m", name+" child baseline")
	old := strings.TrimSpace(runGitAPI(t, source, "rev-parse", "HEAD"))
	dir := filepath.Join(root, name)
	initSubmoduleHTTPRepo(t, dir)
	runGitAPI(t, dir, "-c", "protocol.file.allow=always", "submodule", "add", filepath.ToSlash(source), submoduleHTTPPath)
	runGitAPI(t, dir, "add", ".")
	runGitAPI(t, dir, "commit", "-m", name+" parent baseline")
	base := strings.TrimSpace(runGitAPI(t, dir, "rev-parse", "HEAD"))
	runGitAPI(t, dir, "checkout", "-b", "feature/submodule")
	child := filepath.Join(dir, filepath.FromSlash(submoduleHTTPPath))
	runGitAPI(t, child, "config", "user.name", "Submodule HTTP Test")
	runGitAPI(t, child, "config", "user.email", "submodule@test.invalid")
	runGitAPI(t, child, "config", "core.hooksPath", os.DevNull)
	runGitAPI(t, child, "config", "core.autocrlf", "false")
	writeFileAPI(t, child, "README.md", name+" child after\n")
	runGitAPI(t, child, "add", ".")
	runGitAPI(t, child, "commit", "-m", name+" child advance")
	next := strings.TrimSpace(runGitAPI(t, child, "rev-parse", "HEAD"))
	runGitAPI(t, dir, "add", submoduleHTTPPath)
	runGitAPI(t, dir, "commit", "-m", name+" parent advance")
	head := strings.TrimSpace(runGitAPI(t, dir, "rev-parse", "HEAD"))
	return submoduleHTTPRepo{name, dir, child, base, head, old, next, submoduleHTTPPatch(t, dir, base), submoduleHTTPPatch(t, child, old)}
}

func newSubmoduleHTTPServer(t *testing.T, root string) *Server {
	t.Helper()
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.InstanceConfig{WorkDir: root, AgentEnv: env, BaseBranches: map[string]string{"alpha": "main", "beta": "main"}}
	mgr := process.NewManager(cfg, log)
	t.Cleanup(func() { _ = mgr.StopForTeardown(context.Background()) })
	return NewServer(cfg, mgr, nil, nil, log)
}

func submoduleHTTPState(t *testing.T, repo string) string {
	t.Helper()
	configPath := strings.TrimSpace(runGitAPI(t, repo, "rev-parse", "--git-path", "config"))
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(repo, configPath)
	}
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(config) + runGitAPI(t, repo, "rev-parse", "HEAD") +
		runGitAPI(t, repo, "show-ref", "--head") + runGitAPI(t, repo, "ls-files", "--stage", "-z") +
		runGitAPI(t, repo, "--no-optional-locks", "status", "--porcelain=v1", "-z") +
		submoduleHTTPPatch(t, repo, "HEAD")
}

func guardSubmoduleHTTPReads(t *testing.T, repos []submoduleHTTPRepo) {
	t.Helper()
	for _, repo := range repos {
		for _, dir := range []string{repo.dir, repo.child} {
			before := submoduleHTTPState(t, dir)
			t.Cleanup(func() {
				if after := submoduleHTTPState(t, dir); after != before {
					t.Errorf("registered comparison changed repository state in %s", dir)
				}
			})
		}
	}
}

func submoduleHTTPFixtures(t *testing.T) (string, []submoduleHTTPRepo) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	root := t.TempDir()
	repos := []submoduleHTTPRepo{seedSubmoduleHTTPRepo(t, root, "alpha"), seedSubmoduleHTTPRepo(t, root, "beta")}
	if repos[0].base == repos[1].base || repos[0].old == repos[1].old || repos[0].next == repos[1].next {
		t.Fatal("fixture repositories must have independent parent bases and child commits")
	}
	return root, repos
}

func assertSubmoduleHTTPSelected(t *testing.T, server *Server, repo submoduleHTTPRepo) {
	t.Helper()
	t.Run("commit", func(t *testing.T) { assertSubmoduleHTTPCommit(t, server, repo) })
	t.Run("cumulative", func(t *testing.T) { assertSubmoduleHTTPCumulative(t, server, repo) })
}

func assertSubmoduleHTTPCommit(t *testing.T, server *Server, repo submoduleHTTPRepo) {
	t.Helper()
	var commit process.CommitDiffResult
	readStatusMetadataHTTP(t, server, "/api/v1/git/commit/"+repo.head+"?repo="+repo.name, &commit)
	if !commit.Success || commit.CommitSHA != repo.head || commit.Message != repo.name+" parent advance" || commit.Author != "Submodule HTTP Test <submodule@test.invalid>" || commit.Date == "" || len(commit.Files) != 1 || commit.FilesChanged != 1 || commit.Insertions != 1 || commit.Deletions != 1 {
		t.Errorf("selected commit metadata = %+v", commit)
	}
	assertStatusMetadataHTTPFile(t, commit.Files, submoduleHTTPPath, submoduleHTTPPath, "modified", 1, 1, repo.parentPatch)
}

func assertSubmoduleHTTPCumulative(t *testing.T, server *Server, repo submoduleHTTPRepo) {
	t.Helper()
	var cumulative process.CumulativeDiffResult
	readStatusMetadataHTTP(t, server, "/api/v1/git/cumulative-diff?base="+repo.base+"&repo="+repo.name, &cumulative)
	if !cumulative.Success || cumulative.BaseCommit != repo.base || cumulative.HeadCommit != repo.head || cumulative.TotalCommits != 1 || len(cumulative.Files) != 1 || cumulative.TruncatedFilesCount != 0 {
		t.Errorf("selected cumulative metadata = %+v", cumulative)
	}
	assertStatusMetadataHTTPFile(t, cumulative.Files, submoduleHTTPPath, submoduleHTTPPath, "modified", 1, 1, repo.parentPatch)
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.12
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.13
func TestGitComparisonSubmoduleFormatHTTP(t *testing.T) {
	root, repos := submoduleHTTPFixtures(t)
	server := newSubmoduleHTTPServer(t, root)
	for _, mode := range []string{"short", "log"} {
		t.Run(mode, func(t *testing.T) {
			for _, repo := range repos {
				runGitAPI(t, repo.dir, "config", "diff.submodule", mode)
			}
			guardSubmoduleHTTPReads(t, repos)
			for _, repo := range repos {
				t.Run(repo.name, func(t *testing.T) { assertSubmoduleHTTPSelected(t, server, repo) })
			}
		})
	}
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.12
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.13
func TestGitComparisonSubmoduleFormatMultiRepoHTTP(t *testing.T) {
	root, repos := submoduleHTTPFixtures(t)
	server := newSubmoduleHTTPServer(t, root)
	for _, mode := range []string{"short", "log"} {
		t.Run(mode, func(t *testing.T) {
			for _, repo := range repos {
				runGitAPI(t, repo.dir, "config", "diff.submodule", mode)
			}
			guardSubmoduleHTTPReads(t, repos)
			var result process.CumulativeDiffResult
			readStatusMetadataHTTP(t, server, "/api/v1/git/cumulative-diff?base="+repos[0].base, &result)
			if !result.Success || len(result.Files) != 4 || result.TotalCommits != 4 || result.TruncatedFilesCount != 0 {
				t.Errorf("aggregate metadata = %+v", result)
			}
			for _, repo := range repos {
				t.Run(repo.name, func(t *testing.T) { assertSubmoduleHTTPAggregate(t, result, repo) })
			}
		})
	}
}

func assertSubmoduleHTTPAggregate(t *testing.T, result process.CumulativeDiffResult, repo submoduleHTTPRepo) {
	t.Helper()
	parent := assertStatusMetadataHTTPFile(t, result.Files, repo.name+"\x00"+submoduleHTTPPath, submoduleHTTPPath, "modified", 1, 1, repo.parentPatch)
	if parent["repository_name"] != repo.name || parent["base_ref"] != repo.base {
		t.Errorf("parent scope = %#v", parent)
	}
	if _, present := parent["is_submodule"]; present {
		t.Errorf("parent gitlink must retain ordinary repository scope: %#v", parent)
	}
	childScope := repo.name + "/" + submoduleHTTPPath
	child := assertStatusMetadataHTTPFile(t, result.Files, childScope+"\x00README.md", "README.md", "modified", 1, 1, repo.childPatch)
	if child["repository_name"] != childScope || child["base_ref"] != repo.old || child["is_submodule"] != true {
		t.Errorf("child scope/parent-recorded anchor = %#v", child)
	}
	for _, id := range []string{repo.old, repo.next} {
		if !strings.Contains(repo.parentPatch, "Subproject commit "+id) {
			t.Errorf("raw parent patch missing expected child commit %s", id)
		}
	}
}
