package unidiff

import "testing"

func TestCountLines(t *testing.T) {
	tests := []struct {
		name         string
		diff         string
		wantAddition int
		wantDeletion int
	}{
		{
			name: "content lines whose own text starts with dashes or pluses",
			// Removing `-- seed data` emits `--- seed data`; adding `++counter;`
			// emits `+++counter;`. git reports +1 -1 for exactly this diff.
			diff: "diff --git a/seed.sql b/seed.sql\n" +
				"index 64cf6a5..2559267 100644\n" +
				"--- a/seed.sql\n" +
				"+++ b/seed.sql\n" +
				"@@ -1,2 +1,2 @@\n" +
				"--- seed data\n" +
				" counter;\n" +
				"+++counter;\n",
			wantAddition: 1,
			wantDeletion: 1,
		},
		{
			name: "multi-file diff does not count file headers",
			diff: "diff --git a/a.go b/a.go\n" +
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
			wantAddition: 2,
			wantDeletion: 1,
		},
		{
			name: "no newline at end of file marker is not content",
			diff: "diff --git a/a.txt b/a.txt\n" +
				"--- a/a.txt\n" +
				"+++ b/a.txt\n" +
				"@@ -1 +1 @@\n" +
				"-old\n" +
				"\\ No newline at end of file\n" +
				"+new\n" +
				"\\ No newline at end of file\n",
			wantAddition: 1,
			wantDeletion: 1,
		},
		{
			name:         "empty diff",
			diff:         "",
			wantAddition: 0,
			wantDeletion: 0,
		},
		{
			name: "no hunk header at all",
			diff: "diff --git a/logo.png b/logo.png\n" +
				"index 111..222 100644\n" +
				"Binary files a/logo.png and b/logo.png differ\n",
			wantAddition: 0,
			wantDeletion: 0,
		},
		{
			name: "new file",
			diff: "diff --git a/new.txt b/new.txt\n" +
				"new file mode 100644\n" +
				"index 0000000..111\n" +
				"--- /dev/null\n" +
				"+++ b/new.txt\n" +
				"@@ -0,0 +1,2 @@\n" +
				"+one\n" +
				"+two\n",
			wantAddition: 2,
			wantDeletion: 0,
		},
		{
			name: "deleted file whose content starts with dashes",
			diff: "diff --git a/gone.sql b/gone.sql\n" +
				"deleted file mode 100644\n" +
				"index 111..0000000\n" +
				"--- a/gone.sql\n" +
				"+++ /dev/null\n" +
				"@@ -1,2 +0,0 @@\n" +
				"--- header comment\n" +
				"-select 1;\n",
			wantAddition: 0,
			wantDeletion: 2,
		},
		{
			name: "hunk header without line spans",
			diff: "--- a/a.txt\n" +
				"+++ b/a.txt\n" +
				"@@ -1 +1 @@\n" +
				"-old\n" +
				"+new\n",
			wantAddition: 1,
			wantDeletion: 1,
		},
		{
			name: "CRLF line endings",
			diff: "--- a/a.txt\r\n" +
				"+++ b/a.txt\r\n" +
				"@@ -1,2 +1,2 @@\r\n" +
				"--- old comment\r\n" +
				"+++new value\r\n",
			wantAddition: 1,
			wantDeletion: 1,
		},
		{
			// Real `git show --cc` output for a conflicted merge. The two marker
			// columns would break a one-column counter, so the hunk pattern
			// deliberately does not match `@@@` and the patch counts as zero.
			// No caller produces one (ShowCommit passes --first-parent), and
			// counting nothing beats counting wrongly.
			name: "combined merge diff is not counted",
			diff: "diff --cc f.txt\n" +
				"index 797be14,0c02ccc..db3c03a\n" +
				"--- a/f.txt\n" +
				"+++ b/f.txt\n" +
				"@@@ -1,3 -1,3 +1,3 @@@\n" +
				"  a\n" +
				"- Y\n" +
				" -X\n" +
				"++Z\n" +
				"  c\n",
			wantAddition: 0,
			wantDeletion: 0,
		},
		{
			name: "empty context line carrying no marker stays in the hunk",
			diff: "--- a/a.txt\n" +
				"+++ b/a.txt\n" +
				"@@ -1,4 +1,4 @@\n" +
				" first\n" +
				"\n" +
				"-old\n" +
				"+new\n",
			wantAddition: 1,
			wantDeletion: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			additions, deletions := CountLines(tt.diff)
			if additions != tt.wantAddition || deletions != tt.wantDeletion {
				t.Errorf("CountLines() = +%d -%d, want +%d -%d",
					additions, deletions, tt.wantAddition, tt.wantDeletion)
			}
		})
	}
}
