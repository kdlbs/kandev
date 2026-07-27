package review

import "testing"

const sampleDiff = `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -1,4 +1,6 @@
 package main

+func added() {}
+
 func existing() {}
@@ -20,3 +22,4 @@ func other() {
 	call()
-	removed()
+	replaced()
 }
`

func TestExtractAnchorText_SingleAddedLine(t *testing.T) {
	// New-side numbering from the first hunk: 1 package main, 2 blank,
	// 3 func added() {}, 4 blank, 5 func existing() {}.
	if got := ExtractAnchorText(sampleDiff, 3, 3); got != "func added() {}" {
		t.Fatalf("expected the added line, got %q", got)
	}
}

func TestExtractAnchorText_Range(t *testing.T) {
	got := ExtractAnchorText(sampleDiff, 1, 3)
	want := "package main\n\nfunc added() {}"
	if got != want {
		t.Fatalf("range mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestExtractAnchorText_SkipsDeletionsWhenNumbering(t *testing.T) {
	// Second hunk starts at new line 22: 22 call(), 23 replaced(), 24 }.
	// The removed() deletion must not consume a new-side line number.
	// Indentation is real content and is preserved.
	if got := ExtractAnchorText(sampleDiff, 23, 23); got != "\treplaced()" {
		t.Fatalf("expected the replacement line with its indentation, got %q", got)
	}
}

func TestExtractAnchorText_PreservesLeadingSpaceInContent(t *testing.T) {
	// The marker is exactly one byte; a line whose own content starts with a
	// space must not lose it.
	diff := "@@ -1 +1,2 @@\n old\n+  two-space indent\n"
	if got := ExtractAnchorText(diff, 2, 2); got != "  two-space indent" {
		t.Fatalf("expected content indentation preserved, got %q", got)
	}
}

func TestExtractAnchorText_EndBeforeStartIsTreatedAsSingleLine(t *testing.T) {
	if got := ExtractAnchorText(sampleDiff, 3, 1); got != "func added() {}" {
		t.Fatalf("expected a single-line anchor, got %q", got)
	}
}

func TestExtractAnchorText_OutOfRangeIsEmpty(t *testing.T) {
	if got := ExtractAnchorText(sampleDiff, 999, 1000); got != "" {
		t.Fatalf("expected empty anchor for an unreached line, got %q", got)
	}
}

func TestExtractAnchorText_DegenerateInputs(t *testing.T) {
	if got := ExtractAnchorText("", 1, 1); got != "" {
		t.Fatalf("expected empty for an empty diff, got %q", got)
	}
	if got := ExtractAnchorText(sampleDiff, 0, 0); got != "" {
		t.Fatalf("expected empty for a non-positive line, got %q", got)
	}
	if got := ExtractAnchorText("no hunks here\njust text", 1, 2); got != "" {
		t.Fatalf("expected empty when there is no hunk header, got %q", got)
	}
}

func TestExtractAnchorText_IgnoresNoNewlineMarker(t *testing.T) {
	diff := "@@ -1 +1,2 @@\n old\n+new\n\\ No newline at end of file\n"
	if got := ExtractAnchorText(diff, 2, 2); got != "new" {
		t.Fatalf("expected the added line without the metadata marker, got %q", got)
	}
}

func TestExtractAnchorText_HandlesCRLF(t *testing.T) {
	diff := "@@ -1,1 +1,2 @@\r\n package main\r\n+added\r\n"
	if got := ExtractAnchorText(diff, 2, 2); got != "added" {
		t.Fatalf("expected CRLF diffs to work, got %q", got)
	}
}

func TestExtractAnchorText_TrailingNewlineIsNotAContextLine(t *testing.T) {
	// Splitting on "\n" leaves an empty element after a trailing newline. It
	// carries no diff marker, so counting it would let a range that reaches the
	// end of the hunk pick up a phantom blank line.
	diff := "@@ -1,2 +1,3 @@\n keep\n+added\n"
	if got := ExtractAnchorText(diff, 1, 3); got != "keep\nadded" {
		t.Fatalf("expected no phantom trailing line, got %q", got)
	}
}

func TestExtractAnchorText_InteriorBlankContextLineStillCounts(t *testing.T) {
	// A whitespace-stripped blank context line legitimately appears as "".
	// Interior empties must keep advancing the new-side counter.
	diff := "@@ -1,3 +1,4 @@\n first\n\n+added\n"
	if got := ExtractAnchorText(diff, 3, 3); got != "added" {
		t.Fatalf("expected an interior blank line to advance numbering, got %q", got)
	}
}
