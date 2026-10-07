package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type pushedLogFixture struct {
	dir     string
	base    string
	feature string
	sideTip string
	merge   string
}

func newPushedLogFixture(t *testing.T) *pushedLogFixture {
	t.Helper()
	isolateTestGitEnv(t)
	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	runGit(t, root, "init", "--bare", "--initial-branch=main", bare)
	dir := filepath.Join(root, "work")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "init", "--initial-branch=main")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "core.hooksPath", filepath.Join(root, "no-hooks"))
	runGit(t, dir, "config", "core.autocrlf", "false")
	f := &pushedLogFixture{dir: dir}
	f.base = f.commit(t, "base.txt", "2025-12-01T00:00:00Z")
	runGit(t, dir, "remote", "add", "origin", localGitRemoteURL(bare))
	runGit(t, dir, "push", "-u", "origin", "main")
	runGit(t, dir, "checkout", "-b", "feature")
	runGit(t, dir, "push", "-u", "origin", "feature")
	return f
}

func runPushedFixtureGit(t *testing.T, dir, date string, args ...string) string {
	t.Helper()
	argv := append([]string{"-C", dir, "-c", "commit.gpgsign=false"}, args...)
	cmd := exec.Command("git", argv...)
	cmd.Env = append(filterTestGitEnv(os.Environ()), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (f *pushedLogFixture) commit(t *testing.T, name, date string) string {
	t.Helper()
	writeFile(t, f.dir, name, name+"\n")
	runGit(t, f.dir, "add", name)
	runPushedFixtureGit(t, f.dir, date, "commit", "-m", name)
	return strings.TrimSpace(runGit(t, f.dir, "rev-parse", "HEAD"))
}

func (f *pushedLogFixture) mergeSide(t *testing.T) {
	t.Helper()
	f.feature = f.commit(t, "feature.txt", "2026-01-01T00:00:00Z")
	runGit(t, f.dir, "checkout", "-b", "side", f.base)
	f.commit(t, "side1.txt", "2026-02-01T00:00:00Z")
	f.sideTip = f.commit(t, "side2.txt", "2026-03-01T00:00:00Z")
	runGit(t, f.dir, "checkout", "feature")
	runPushedFixtureGit(t, f.dir, "2026-04-01T00:00:00Z", "merge", "--no-ff", "-m", "Merge side", "side")
	f.merge = strings.TrimSpace(runGit(t, f.dir, "rev-parse", "HEAD"))
}

func (f *pushedLogFixture) log(t *testing.T, base string, limit int) *GitLogResult {
	t.Helper()
	op := NewGitOperator(f.dir, newTestLogger(t), nil)
	result, err := op.GetLog(context.Background(), base, limit)
	if err != nil || !result.Success {
		t.Fatalf("GetLog: %v, %+v", err, result)
	}
	return result
}

// @covers AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.1, .2, .5
func TestGetLog_PushedFirstParentMerge(t *testing.T) {
	f := newPushedLogFixture(t)
	f.mergeSide(t)
	result := f.log(t, f.base, 1)
	if len(result.Commits) != 2 {
		t.Fatalf("range rows = %d, want 2 (range ignores recent limit)", len(result.Commits))
	}
	merge, feature := result.Commits[0], result.Commits[1]
	if merge.CommitSHA != f.merge || merge.ParentSHA != f.feature ||
		feature.CommitSHA != f.feature || feature.ParentSHA != f.base {
		t.Fatalf("unexpected first-parent membership/order/parents: %+v, %+v", merge, feature)
	}
	if merge.FilesChanged != 2 || merge.Insertions != 2 || merge.Deletions != 0 ||
		feature.FilesChanged != 1 || feature.Insertions != 1 || feature.Deletions != 0 {
		t.Fatalf("unexpected first-parent statistics: %+v, %+v", merge, feature)
	}
	for _, c := range result.Commits {
		if c.Pushed {
			t.Errorf("local commit %s (%s) incorrectly pushed", c.CommitMessage, c.CommitSHA)
		}
	}
	runGit(t, f.dir, "push", "origin", "feature")
	for _, c := range f.log(t, f.base, 0).Commits {
		if !c.Pushed {
			t.Errorf("published commit %s incorrectly unpushed", c.CommitSHA)
		}
	}
}

func assertPushedRows(t *testing.T, result *GitLogResult, shas []string, published ...string) {
	t.Helper()
	if len(result.Commits) != len(shas) {
		t.Fatalf("rows = %d, want %d", len(result.Commits), len(shas))
	}
	for i, c := range result.Commits {
		if c.CommitSHA != shas[i] || c.Pushed != slices.Contains(published, c.CommitSHA) {
			t.Errorf("row %d = %s pushed=%v, want %s pushed=%v", i,
				c.CommitSHA, c.Pushed, shas[i], slices.Contains(published, shas[i]))
		}
	}
}

// @covers AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.1, .2
func TestGetLog_PushedMixedFirstParent(t *testing.T) {
	f := newPushedLogFixture(t)
	f.mergeSide(t)
	runGit(t, f.dir, "push", "origin", f.feature+":refs/heads/feature")
	latest := f.commit(t, "latest.txt", "2026-05-01T00:00:00Z")
	assertPushedRows(t, f.log(t, f.base, 0), []string{latest, f.merge, f.feature}, f.feature)
}

// @covers AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.1, .2, .5
func TestGetLog_PushedRecentFullGraph(t *testing.T) {
	f := newPushedLogFixture(t)
	f.mergeSide(t)
	runGit(t, f.dir, "push", "origin", "side:feature")
	side1 := strings.TrimSpace(runGit(t, f.dir, "rev-parse", f.sideTip+"^"))
	limited := f.log(t, "", 3)
	assertPushedRows(t, limited, []string{f.merge, f.sideTip, side1}, f.sideTip, side1)
	if limited.Commits[0].FilesChanged != 0 || limited.Commits[1].FilesChanged != 1 ||
		limited.Commits[1].Insertions != 1 || limited.Commits[1].ParentSHA != side1 {
		t.Fatalf("full-graph parent/stat semantics changed: %+v, %+v", limited.Commits[0], limited.Commits[1])
	}
	assertPushedRows(t, f.log(t, "", 0), []string{f.merge, f.sideTip, side1, f.feature, f.base},
		f.sideTip, side1, f.base)
}

// @covers AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.1, .2
func TestGetLog_PushedUpstreamSecondParent(t *testing.T) {
	f := newPushedLogFixture(t)
	f.mergeSide(t)
	runGit(t, f.dir, "checkout", "main")
	runPushedFixtureGit(t, f.dir, "2026-05-01T00:00:00Z", "merge", "--no-ff", "-m", "Publish feature", f.feature)
	runGit(t, f.dir, "push", "origin", "main:feature")
	runGit(t, f.dir, "checkout", "feature")
	assertPushedRows(t, f.log(t, f.base, 0), []string{f.merge, f.feature}, f.feature)
	runGit(t, f.dir, "checkout", "main")
	runPushedFixtureGit(t, f.dir, "2026-06-01T00:00:00Z", "merge", "--no-ff", "-m", "Publish side", "side")
	runGit(t, f.dir, "push", "origin", "main:feature")
	runGit(t, f.dir, "checkout", "feature")
	side1 := strings.TrimSpace(runGit(t, f.dir, "rev-parse", f.sideTip+"^"))
	assertPushedRows(t, f.log(t, "", 3), []string{f.merge, f.sideTip, side1}, f.sideTip, side1)
	assertPushedRows(t, f.log(t, f.base, 0), []string{f.merge, f.feature}, f.feature)
}

// @covers AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.3
func TestGetLog_PushedWithoutUpstream(t *testing.T) {
	f := newPushedLogFixture(t)
	local := f.commit(t, "local.txt", "2026-01-01T00:00:00Z")
	runGit(t, f.dir, "branch", "--unset-upstream")
	assertPushedRows(t, f.log(t, "", 0), []string{local, f.base})
	runGit(t, f.dir, "config", "branch.feature.remote", "origin")
	runGit(t, f.dir, "config", "branch.feature.merge", "refs/heads/missing")
	assertPushedRows(t, f.log(t, f.base, 0), []string{local})
}

// @covers AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.3, .6
func TestMarkPushedCommits_LookupFailure(t *testing.T) {
	f := newPushedLogFixture(t)
	result := f.log(t, "", 0)
	assertPushedRows(t, result, []string{f.base}, f.base)
	op := NewGitOperator(f.dir, newTestLogger(t), nil)
	if op.getUpstreamRef(context.Background()) != "origin/feature" {
		t.Fatal("fixture upstream did not resolve")
	}
	for _, tc := range []struct {
		name string
		base string
	}{
		{"invalid range after upstream resolution", strings.Repeat("f", 40)},
		{"cancelled evidence", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			if tc.base == "" {
				cancel()
			}
			rows := []*GitCommitInfo{{CommitSHA: f.base}}
			op.markPushedCommits(ctx, rows, tc.base)
			if rows[0].Pushed {
				t.Fatal("unavailable evidence falsely marked a commit pushed")
			}
		})
	}
}

// @covers AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.1, .6
func TestMarkPushedCommits_AnchoredLogTip(t *testing.T) {
	f := newPushedLogFixture(t)
	f.mergeSide(t)
	rangeRows := f.log(t, f.base, 0)
	recentRows := f.log(t, "", 3)
	f.commit(t, "new-head.txt", "2026-05-01T00:00:00Z")
	op := NewGitOperator(f.dir, newTestLogger(t), nil)
	op.markPushedCommits(context.Background(), rangeRows.Commits, f.base)
	op.markPushedCommits(context.Background(), recentRows.Commits, "")
	assertPushedRows(t, rangeRows, []string{f.merge, f.feature})
	side1 := strings.TrimSpace(runGit(t, f.dir, "rev-parse", f.sideTip+"^"))
	assertPushedRows(t, recentRows, []string{f.merge, f.sideTip, side1})
}
