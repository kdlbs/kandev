package unidiff

import (
	"context"
	"strings"
	"testing"
)

func TestBuildUntrackedDiff(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		content   string
		additions int
		want      string
	}{
		{
			name:      "one line",
			path:      "one.txt",
			content:   "one\n",
			additions: 1,
			want: "diff --git a/one.txt b/one.txt\nnew file mode 100644\nindex 0000000..0000000\n" +
				"--- /dev/null\n+++ b/one.txt\n@@ -0,0 +1,1 @@\n+one\n",
		},
		{
			name:      "multiple lines without final newline",
			path:      `new[one] file "a".txt`,
			content:   "first\nsecond",
			additions: 2,
			want: "diff --git a/new[one] file \"a\".txt b/new[one] file \"a\".txt\n" +
				"new file mode 100644\nindex 0000000..0000000\n--- /dev/null\n" +
				"+++ b/new[one] file \"a\".txt\n@@ -0,0 +1,2 @@\n+first\n+second\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, additions, truncated, err := BuildUntrackedDiff(context.Background(), tt.path, []byte(tt.content), 1024)
			if err != nil {
				t.Fatalf("BuildUntrackedDiff: %v", err)
			}
			if got != tt.want || additions != tt.additions || truncated {
				t.Fatalf("BuildUntrackedDiff = (%q, %d, %v), want (%q, %d, false)", got, additions, truncated, tt.want, tt.additions)
			}
		})
	}
}

func TestBuildUntrackedDiffTruncatesAtOutputLimit(t *testing.T) {
	const limit = 24
	got, additions, truncated, err := BuildUntrackedDiff(context.Background(), "a.txt", []byte(strings.Repeat("x", 32)), limit)
	if err != nil {
		t.Fatalf("BuildUntrackedDiff: %v", err)
	}
	if len(got) != limit || !truncated || additions != 1 {
		t.Fatalf("BuildUntrackedDiff = (%d bytes, %d additions, %v), want (%d bytes, 1, true)", len(got), additions, truncated, limit)
	}
}
