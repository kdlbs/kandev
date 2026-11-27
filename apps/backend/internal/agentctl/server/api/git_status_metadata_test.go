package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.1
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
func TestGitDiffStatusMetadataHTTP(t *testing.T) {
	fixture := newGitAPIFixture(t)
	path := "new file mode deleted file mode rename from.txt"
	writeFileAPI(t, fixture.repo, path, "rename from unchanged\nnew file mode removed\nold\n")
	runGitAPI(t, fixture.repo, "add", ".")
	runGitAPI(t, fixture.repo, "commit", "-m", "tracked baseline")
	base := fixture.head(t)
	writeFileAPI(t, fixture.repo, path, "rename from unchanged\ndeleted file mode added\nnew\n")
	runGitAPI(t, fixture.repo, "add", ".")
	runGitAPI(t, fixture.repo, "commit", "-m", "ordinary modification")
	head := fixture.head(t)
	var commit process.CommitDiffResult
	readStatusMetadataHTTP(t, fixture.server, "/api/v1/git/commit/"+head, &commit)
	if !commit.Success || commit.CommitSHA != head || commit.FilesChanged != 1 || commit.Insertions != 2 || commit.Deletions != 2 {
		t.Fatalf("commit identity/totals = %+v", commit)
	}
	assertStatusMetadataHTTPFile(t, commit.Files, path, path, "modified", 2, 2, runGitAPI(t, fixture.repo, "show", "--format=", "-p", head))
	var cumulative process.CumulativeDiffResult
	readStatusMetadataHTTP(t, fixture.server, "/api/v1/git/cumulative-diff?base="+base, &cumulative)
	if !cumulative.Success || cumulative.BaseCommit != base || cumulative.HeadCommit != head || cumulative.TotalCommits != 1 || cumulative.TruncatedFilesCount != 0 {
		t.Fatalf("cumulative identity/totals = %+v", cumulative)
	}
	assertStatusMetadataHTTPFile(t, cumulative.Files, path, path, "modified", 2, 2, runGitAPI(t, fixture.repo, "diff", base))
	if len(commit.Files) != 1 || len(cumulative.Files) != 1 {
		t.Fatal("unexpected comparison membership")
	}
}

type statusMetadataHTTPRepo struct {
	name, dir, base, head, status, patch string
	additions, deletions                 int
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
func TestGitDiffStatusMetadataMultiRepoHTTP(t *testing.T) {
	root := t.TempDir()
	path := "new file mode.txt"
	alpha := seedStatusMetadataHTTPRepo(t, root, "alpha", path, false)
	beta := seedStatusMetadataHTTPRepo(t, root, "beta", path, true)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.InstanceConfig{WorkDir: root, BaseBranches: map[string]string{"alpha": "main", "beta": "main"}}
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	server := NewServer(cfg, manager, nil, nil, log)
	var commit process.CommitDiffResult
	readStatusMetadataHTTP(t, server, "/api/v1/git/commit/"+alpha.head+"?repo=alpha", &commit)
	if !commit.Success || commit.CommitSHA != alpha.head || commit.FilesChanged != 1 || commit.Insertions != 2 || commit.Deletions != 1 {
		t.Fatalf("selected commit = %+v", commit)
	}
	assertStatusMetadataHTTPFile(t, commit.Files, path, path, alpha.status, alpha.additions, alpha.deletions, alpha.patch)
	var selected process.CumulativeDiffResult
	readStatusMetadataHTTP(t, server, "/api/v1/git/cumulative-diff?base="+alpha.base+"&repo=alpha", &selected)
	if !selected.Success || selected.BaseCommit != alpha.base || selected.HeadCommit != alpha.head || len(selected.Files) != 1 {
		t.Fatalf("selected cumulative = %+v", selected)
	}
	assertStatusMetadataHTTPFile(t, selected.Files, path, path, alpha.status, alpha.additions, alpha.deletions, alpha.patch)
	var aggregate process.CumulativeDiffResult
	readStatusMetadataHTTP(t, server, "/api/v1/git/cumulative-diff?base="+alpha.base, &aggregate)
	if !aggregate.Success || len(aggregate.Files) != 2 || aggregate.TotalCommits != 2 || aggregate.TruncatedFilesCount != 0 {
		t.Fatalf("aggregate cumulative = %+v", aggregate)
	}
	for _, repo := range []statusMetadataHTTPRepo{alpha, beta} {
		key := repo.name + "\x00" + path
		entry := assertStatusMetadataHTTPFile(t, aggregate.Files, key, path, repo.status, repo.additions, repo.deletions, repo.patch)
		if entry["repository_name"] != repo.name || entry["base_ref"] != repo.base {
			t.Errorf("repository identity = %#v", entry)
		}
		if _, present := entry["is_submodule"]; present {
			t.Errorf("ordinary repository marked as submodule: %#v", entry)
		}
	}
}

func seedStatusMetadataHTTPRepo(t *testing.T, root, name, path string, deleted bool) statusMetadataHTTPRepo {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitAPI(t, dir, "init", "--initial-branch=main")
	runGitAPI(t, dir, "config", "user.name", "Test User")
	runGitAPI(t, dir, "config", "user.email", "test@test.com")
	runGitAPI(t, dir, "config", "core.hooksPath", os.DevNull)
	runGitAPI(t, dir, "config", "core.autocrlf", "false")
	writeFileAPI(t, dir, path, name+" baseline\n")
	runGitAPI(t, dir, "add", ".")
	runGitAPI(t, dir, "commit", "-m", "baseline")
	base := strings.TrimSpace(runGitAPI(t, dir, "rev-parse", "HEAD"))
	runGitAPI(t, dir, "checkout", "-b", "feature/metadata")
	status, additions, deletions := "modified", 2, 1
	if deleted {
		runGitAPI(t, dir, "rm", path)
		status, additions, deletions = "deleted", 0, 1
	} else {
		writeFileAPI(t, dir, path, name+" changed\nrename from marker\n")
		runGitAPI(t, dir, "add", ".")
	}
	runGitAPI(t, dir, "commit", "-m", "metadata change")
	return statusMetadataHTTPRepo{name, dir, base, strings.TrimSpace(runGitAPI(t, dir, "rev-parse", "HEAD")), status, runGitAPI(t, dir, "diff", base), additions, deletions}
}

func readStatusMetadataHTTP(t *testing.T, server *Server, route string, result interface{}) {
	t.Helper()
	rec := getGitAPI(t, server, route)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status %d, body %s", route, rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), result); err != nil {
		t.Fatal(err)
	}
}

func assertStatusMetadataHTTPFile(t *testing.T, files map[string]interface{}, key, path, status string, additions, deletions int, patch string) map[string]interface{} {
	t.Helper()
	entry, ok := files[key].(map[string]interface{})
	if !ok {
		t.Fatalf("missing file key %q: %#v", key, files)
	}
	if entry["path"] != path || entry["status"] != status || entry["additions"] != float64(additions) || entry["deletions"] != float64(deletions) || entry["diff"] != patch || entry["staged"] != false {
		t.Errorf("file %q = %#v; want %s +%d/-%d and raw Git patch", key, entry, status, additions, deletions)
	}
	if _, present := entry["diff_skip_reason"]; present {
		t.Errorf("small HTTP patch unexpectedly skipped: %#v", entry)
	}
	return entry
}
