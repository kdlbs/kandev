package process

import (
	"context"
	"strings"
	"testing"
)

// fileStats pulls the additions/deletions a parsed file entry carries.
func fileStats(t *testing.T, files map[string]interface{}, path string) (int, int) {
	t.Helper()
	entry, ok := files[path].(map[string]interface{})
	if !ok {
		t.Fatalf("no file entry for %q (have %v)", path, keysOf(files))
	}
	additions, ok := entry["additions"].(int)
	if !ok {
		t.Fatalf("file %q: additions is not an int: %#v", path, entry["additions"])
	}
	deletions, ok := entry["deletions"].(int)
	if !ok {
		t.Fatalf("file %q: deletions is not an int: %#v", path, entry["deletions"])
	}
	return additions, deletions
}

func keysOf(files map[string]interface{}) []string {
	out := make([]string, 0, len(files))
	for k := range files {
		out = append(out, k)
	}
	return out
}

func TestNumstatByPath(t *testing.T) {
	tests := []struct {
		name  string
		block string
		want  map[string]numstatEntry
	}{
		{name: "empty block", block: "", want: map[string]numstatEntry{}},
		{
			name:  "single row",
			block: "3\t1\tapps/a.go\n",
			want:  map[string]numstatEntry{"apps/a.go": {path: "apps/a.go", additions: 3, deletions: 1}},
		},
		{
			name:  "binary file reports dashes and yields a zero pair",
			block: "-\t-\tassets/logo.png\n",
			want:  map[string]numstatEntry{"assets/logo.png": {path: "assets/logo.png"}},
		},
		{
			name: "stat table lines are ignored",
			// `git show --stat --numstat` emits both blocks; only the tab-separated
			// numstat rows are ours to read.
			block: "1\t1\tseed.sql\n schema.sql | 2 +-\n 1 file changed, 1 insertion(+), 1 deletion(-)\n",
			want:  map[string]numstatEntry{"seed.sql": {path: "seed.sql", additions: 1, deletions: 1}},
		},
		{
			name:  "rename row is keyed by its new-side path",
			block: "2\t0\tapps/{old => new}/file.go\n",
			want: map[string]numstatEntry{
				"apps/new/file.go": {path: "apps/new/file.go", additions: 2, deletions: 0},
			},
		},
		{
			name:  "two rows",
			block: "1\t1\ta.go\n0\t4\tb.go\n",
			want: map[string]numstatEntry{
				"a.go": {path: "a.go", additions: 1, deletions: 1},
				"b.go": {path: "b.go", additions: 0, deletions: 4},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := numstatByPath(tt.block)
			if len(got) != len(tt.want) {
				t.Fatalf("numstatByPath() = %+v, want %+v", got, tt.want)
			}
			for path, want := range tt.want {
				if got[path] != want {
					t.Errorf("row %q = %+v, want %+v", path, got[path], want)
				}
			}
		})
	}
}

// TestParseCommitDiffWithOptions_LineCounts covers both counting routes: the
// authoritative --numstat rows when git supplied them, and the in-hunk fallback
// when it did not.
func TestParseCommitDiffWithOptions_LineCounts(t *testing.T) {
	// Removing the SQL line `-- seed data` emits `--- seed data`; adding the C
	// line `++counter;` emits `+++counter;`. A prefix test drops both and reports
	// +0 -0 where git reports +1 -1.
	dashAndPlusPatch := "diff --git a/seed.sql b/seed.sql\n" +
		"index 64cf6a5..2559267 100644\n" +
		"--- a/seed.sql\n" +
		"+++ b/seed.sql\n" +
		"@@ -1,2 +1,2 @@\n" +
		"--- seed data\n" +
		" counter;\n" +
		"+++counter;\n"

	tests := []struct {
		name     string
		output   string
		wantFile string
		wantAdd  int
		wantDel  int
	}{
		{
			name:     "content starting with dashes and pluses, no numstat",
			output:   dashAndPlusPatch,
			wantFile: "seed.sql",
			wantAdd:  1,
			wantDel:  1,
		},
		{
			name:     "content starting with dashes and pluses, with numstat",
			output:   "1\t1\tseed.sql\n" + dashAndPlusPatch,
			wantFile: "seed.sql",
			wantAdd:  1,
			wantDel:  1,
		},
		{
			name: "file headers of a real multi-file diff are not counted",
			output: "diff --git a/a.go b/a.go\n" +
				"index 111..222 100644\n" +
				"--- a/a.go\n" +
				"+++ b/a.go\n" +
				"@@ -1,3 +1,3 @@\n" +
				" package a\n" +
				"-old\n" +
				"+new\n" +
				"diff --git a/b.go b/b.go\n" +
				"index 333..444 100644\n" +
				"--- a/b.go\n" +
				"+++ b/b.go\n" +
				"@@ -1,2 +1,3 @@\n" +
				" package b\n" +
				"+added\n",
			wantFile: "a.go",
			wantAdd:  1,
			wantDel:  1,
		},
		{
			name: "no newline at end of file marker is not content",
			output: "diff --git a/a.txt b/a.txt\n" +
				"--- a/a.txt\n" +
				"+++ b/a.txt\n" +
				"@@ -1 +1 @@\n" +
				"-old\n" +
				"\\ No newline at end of file\n" +
				"+new\n" +
				"\\ No newline at end of file\n",
			wantFile: "a.txt",
			wantAdd:  1,
			wantDel:  1,
		},
		{
			name: "binary file from numstat dashes",
			output: "-\t-\tlogo.png\n" +
				"diff --git a/logo.png b/logo.png\n" +
				"index 111..222 100644\n" +
				"Binary files a/logo.png and b/logo.png differ\n",
			wantFile: "logo.png",
			wantAdd:  0,
			wantDel:  0,
		},
		{
			name: "diff with no hunk header at all",
			output: "diff --git a/mode.sh b/mode.sh\n" +
				"old mode 100644\n" +
				"new mode 100755\n",
			wantFile: "mode.sh",
			wantAdd:  0,
			wantDel:  0,
		},
		{
			name: "numstat rows for other paths fall back to counting this file",
			output: "5\t5\tsomewhere-else.go\n" +
				"7\t7\tand-another.go\n" +
				dashAndPlusPatch,
			wantFile: "seed.sql",
			wantAdd:  1,
			wantDel:  1,
		},
		{
			name: "renamed file is paired via git's brace notation",
			output: "1\t0\tapps/{old => new}/file.go\n" +
				"diff --git a/apps/old/file.go b/apps/new/file.go\n" +
				"similarity index 90%\n" +
				"rename from apps/old/file.go\n" +
				"rename to apps/new/file.go\n" +
				"--- a/apps/old/file.go\n" +
				"+++ b/apps/new/file.go\n" +
				"@@ -1,2 +1,3 @@\n" +
				" package p\n" +
				"+added\n",
			wantFile: "apps/new/file.go",
			wantAdd:  1,
			wantDel:  0,
		},
	}

	gitOp := &GitOperator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := gitOp.parseCommitDiffWithOptions(tt.output, parseCommitDiffOptions{})
			additions, deletions := fileStats(t, files, tt.wantFile)
			if additions != tt.wantAdd || deletions != tt.wantDel {
				t.Errorf("%s: got +%d -%d, want +%d -%d",
					tt.wantFile, additions, deletions, tt.wantAdd, tt.wantDel)
			}
		})
	}
}

func TestParseCommitDiffWithOptions_EmptyDiff(t *testing.T) {
	gitOp := &GitOperator{}
	for _, output := range []string{"", "\n", "1\t1\tseed.sql\n"} {
		files := gitOp.parseCommitDiffWithOptions(output, parseCommitDiffOptions{})
		if len(files) != 0 {
			t.Errorf("parseCommitDiffWithOptions(%q) returned %d files, want 0", output, len(files))
		}
	}
}

// TestParseCommitDiffWithOptions_MultiFileTotals checks the rolled-up
// Insertions/Deletions that feed the commit-detail header.
func TestParseCommitDiffWithOptions_MultiFileTotals(t *testing.T) {
	output := "1\t1\ta.go\n" +
		"1\t0\tb.go\n" +
		"diff --git a/a.go b/a.go\n" +
		"--- a/a.go\n" +
		"+++ b/a.go\n" +
		"@@ -1,3 +1,3 @@\n" +
		" package a\n" +
		"-old\n" +
		"+new\n" +
		"diff --git a/b.go b/b.go\n" +
		"--- a/b.go\n" +
		"+++ b/b.go\n" +
		"@@ -1,2 +1,3 @@\n" +
		" package b\n" +
		"+added\n"

	gitOp := &GitOperator{}
	files := gitOp.parseCommitDiffWithOptions(output, parseCommitDiffOptions{})
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2 (%v)", len(files), keysOf(files))
	}
	insertions, deletions := sumFileDiffStats(files)
	if insertions != 2 || deletions != 1 {
		t.Errorf("sumFileDiffStats() = +%d -%d, want +2 -1", insertions, deletions)
	}
}

// seedDashAndPlusCommit writes the pathological file, commits a baseline, then
// rewrites it so the patch contains a removed line starting with "--" and an
// added line starting with "++". git reports +1 -1 for the resulting commit.
func seedDashAndPlusCommit(t *testing.T, repoDir string) string {
	t.Helper()
	writeFile(t, repoDir, "seed.sql", "-- seed data\ncounter;\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "seed: baseline")
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	writeFile(t, repoDir, "seed.sql", "counter;\n++counter;\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "seed: rewrite")
	return base
}

// TestShowCommit_CountsDashAndPlusPrefixedContent is the end-to-end guard on the
// commit-detail view: real git output, real numbers, compared against git's own
// --numstat for the same commit.
func TestShowCommit_CountsDashAndPlusPrefixedContent(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	seedDashAndPlusCommit(t, repoDir)
	head := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	gitOp := NewGitOperator(repoDir, newTestLogger(t), nil)
	result, err := gitOp.ShowCommit(context.Background(), head)
	if err != nil {
		t.Fatalf("ShowCommit returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("ShowCommit failed: %s", result.Error)
	}

	// git's own answer for this commit.
	wantNumstat := "1\t1\tseed.sql"
	if got := strings.TrimSpace(runGit(t, repoDir, "show", "--format=", "--numstat", head)); got != wantNumstat {
		t.Fatalf("test fixture drifted: git numstat = %q, want %q", got, wantNumstat)
	}

	additions, deletions := fileStats(t, result.Files, "seed.sql")
	if additions != 1 || deletions != 1 {
		t.Errorf("seed.sql = +%d -%d, want +1 -1", additions, deletions)
	}
	if result.Insertions != 1 || result.Deletions != 1 {
		t.Errorf("rolled-up stats = +%d -%d, want +1 -1", result.Insertions, result.Deletions)
	}
}

// TestGetCumulativeDiff_CountsDashAndPlusPrefixedContent covers the second
// caller, which builds its diff from `git diff <base>` rather than `git show`.
func TestGetCumulativeDiff_CountsDashAndPlusPrefixedContent(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	base := seedDashAndPlusCommit(t, repoDir)

	gitOp := NewGitOperator(repoDir, newTestLogger(t), nil)
	result, err := gitOp.GetCumulativeDiff(context.Background(), base)
	if err != nil {
		t.Fatalf("GetCumulativeDiff returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("GetCumulativeDiff failed: %s", result.Error)
	}

	additions, deletions := fileStats(t, result.Files, "seed.sql")
	if additions != 1 || deletions != 1 {
		t.Errorf("seed.sql = +%d -%d, want +1 -1", additions, deletions)
	}
}
