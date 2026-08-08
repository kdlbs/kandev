package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestCountDiffLines_Table(t *testing.T) {
	tests := []struct {
		name    string
		diff    string
		wantAdd int
		wantDel int
	}{
		{
			// GitLab's /changes payload puts `--- a/<path>` / `+++ b/<path>`
			// ahead of the first hunk, so the old prefix test could not tell them
			// apart from a removed `-- seed data` or an added `++counter;`.
			name: "content lines starting with dashes and pluses",
			diff: "--- a/seed.sql\n" +
				"+++ b/seed.sql\n" +
				"@@ -1,2 +1,2 @@\n" +
				"--- seed data\n" +
				" counter;\n" +
				"+++counter;\n",
			wantAdd: 1,
			wantDel: 1,
		},
		{
			name: "file headers are still excluded",
			diff: "--- a/foo.go\n" +
				"+++ b/foo.go\n" +
				"@@ -1,3 +1,3 @@\n" +
				"-old\n" +
				"+new1\n" +
				"+new2\n" +
				" unchanged\n",
			wantAdd: 2,
			wantDel: 1,
		},
		{
			name: "multiple hunks in one file patch",
			diff: "@@ -1,2 +1,2 @@\n" +
				"-a\n" +
				"+b\n" +
				"@@ -10,2 +10,3 @@\n" +
				" ctx\n" +
				"+c\n",
			wantAdd: 2,
			wantDel: 1,
		},
		{
			name: "no newline at end of file marker",
			diff: "@@ -1 +1 @@\n" +
				"-old\n" +
				"\\ No newline at end of file\n" +
				"+new\n" +
				"\\ No newline at end of file\n",
			wantAdd: 1,
			wantDel: 1,
		},
		{name: "empty diff (binary or renamed-only change)", diff: "", wantAdd: 0, wantDel: 0},
		{
			name:    "no hunk header at all",
			diff:    "--- a/foo.png\n+++ b/foo.png\n",
			wantAdd: 0,
			wantDel: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			add, del := countDiffLines(tt.diff)
			if add != tt.wantAdd || del != tt.wantDel {
				t.Errorf("countDiffLines() = +%d -%d, want +%d -%d", add, del, tt.wantAdd, tt.wantDel)
			}
		})
	}
}

// TestPATClient_ListMRFiles_CountsDashAndPlusPrefixedContent walks the live path
// so a regression in countDiffLines shows up in the MR file stats the UI renders.
func TestPATClient_ListMRFiles_CountsDashAndPlusPrefixedContent(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"changes": []map[string]any{{
				"old_path": "seed.sql",
				"new_path": "seed.sql",
				"diff": "--- a/seed.sql\n" +
					"+++ b/seed.sql\n" +
					"@@ -1,2 +1,2 @@\n" +
					"--- seed data\n" +
					" counter;\n" +
					"+++counter;\n",
			}},
		})
	}))
	defer stop()

	files, err := NewPATClient(host, "tok").ListMRFiles(context.Background(), "group/project", 7)
	if err != nil {
		t.Fatalf("ListMRFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Additions != 1 || files[0].Deletions != 1 {
		t.Errorf("seed.sql = +%d -%d, want +1 -1", files[0].Additions, files[0].Deletions)
	}
}
