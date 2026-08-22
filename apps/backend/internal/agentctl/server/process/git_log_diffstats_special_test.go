package process

// Special-path and section-boundary parser regressions for
// parseCommitDiffWithOptions. Kept in their own file (per the backend
// convention that new tests go in a new file rather than growing an
// already-large test file): the cases cover git's C-quoted paths (tab,
// non-ASCII octal, embedded quotes), ` b/`- and ` and b/`-containing names,
// binary/rename/mode-only sections, and header tokens inside patch bodies.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseCommitDiffWithOptions_SpecialPaths guards path extraction against
// git's quoting and separator rules. git C-quotes paths containing tabs,
// non-ASCII bytes, quotes, or backslashes (core.quotePath, default on) —
// a naive first-` b/` split drops them entirely — and a path containing the
// literal sequence ` b/` mis-splits the header. The parser must read the
// path off the per-file `+++ b/<path>` line, which git writes once per
// section and is unambiguous.
func TestParseCommitDiffWithOptions_SpecialPaths(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantPath string
	}{
		{
			name: "tab in path is C-quoted by git",
			output: "diff --git \"a/a\\tb.txt\" \"b/a\\tb.txt\"\n" +
				"new file mode 100644\n" +
				"index 0000000..e69de29\n" +
				"--- /dev/null\n" +
				"+++ \"b/a\\tb.txt\"\n" +
				"@@ -0,0 +1 @@\n" +
				"+content\n",
			wantPath: "a\tb.txt",
		},
		{
			name: "non-ASCII path is C-quoted with octal escapes",
			output: "diff --git \"a/caf\\303\\251.txt\" \"b/caf\\303\\251.txt\"\n" +
				"new file mode 100644\n" +
				"--- /dev/null\n" +
				"+++ \"b/caf\\303\\251.txt\"\n" +
				"@@ -0,0 +1 @@\n" +
				"+caf\u00e9\n",
			wantPath: "caf\u00e9.txt",
		},
		{
			name: "quote char in path is C-quoted",
			output: "diff --git \"a/has\\\"quote.txt\" \"b/has\\\"quote.txt\"\n" +
				"new file mode 100644\n" +
				"--- /dev/null\n" +
				"+++ \"b/has\\\"quote.txt\"\n" +
				"@@ -0,0 +1 @@\n" +
				"+content\n",
			wantPath: "has\"quote.txt",
		},
		{
			name: "path containing literal b-slash splits via the +++ line",
			output: "diff --git a/name b/dir.txt b/name b/dir.txt\n" +
				"new file mode 100644\n" +
				"--- /dev/null\n" +
				"+++ b/name b/dir.txt\n" +
				"@@ -0,0 +1 @@\n" +
				"+content\n",
			wantPath: "name b/dir.txt",
		},
		{
			name: "trailing-whitespace path keeps the tab marker stripped",
			output: "diff --git a/trail.txt  b/trail.txt \n" +
				"index 111..222 100644\n" +
				"--- a/trail.txt \t\n" +
				"+++ b/trail.txt \t\n" +
				"@@ -1 +1 @@\n" +
				"-old\n" +
				"+new\n",
			wantPath: "trail.txt ",
		},
		{
			name: "quoted path ending in whitespace strips the outer marker",
			output: "diff --git \"a/weird \\\"quote.txt \" \"b/weird \\\"quote.txt \"\n" +
				"index 111..222 100644\n" +
				"--- \"a/weird \\\"quote.txt \"\t\n" +
				"+++ \"b/weird \\\"quote.txt \"\t\n" +
				"@@ -1 +1 @@\n" +
				"-old\n" +
				"+new\n",
			wantPath: "weird \"quote.txt ",
		},
		{
			name: "renamed file keeps the new path from the +++ line",
			output: "diff --git a/apps/old/file.go b/apps/new/file.go\n" +
				"similarity index 90%\n" +
				"rename from apps/old/file.go\n" +
				"rename to apps/new/file.go\n" +
				"--- a/apps/old/file.go\n" +
				"+++ b/apps/new/file.go\n" +
				"@@ -1,2 +1,3 @@\n" +
				" package p\n" +
				"+added\n",
			wantPath: "apps/new/file.go",
		},
		{
			name: "pure rename without hunks keeps the rename to line path",
			output: "diff --git a/old b.txt b/new b.txt\n" +
				"similarity index 100%\n" +
				"rename from old b.txt\n" +
				"rename to new b.txt\n",
			wantPath: "new b.txt",
		},
		{
			name: "binary section with b-slash path parses the Binary files line",
			output: "diff --git a/name b/binary.bin b/name b/binary.bin\n" +
				"index 6735744..d7bf111 100644\n" +
				"Binary files a/name b/binary.bin and b/name b/binary.bin differ\n",
			wantPath: "name b/binary.bin",
		},
		{
			name: "quoted binary section unquotes the Binary files line",
			output: "diff --git \"a/caf\\303\\251 b.bin\" \"b/caf\\303\\251 b.bin\"\n" +
				"index a6a3e7f..073ea92 100644\n" +
				"Binary files \"a/caf\\303\\251 b.bin\" and \"b/caf\\303\\251 b.bin\" differ\n",
			wantPath: "caf\u00e9 b.bin",
		},
		{
			name: "mode-only change on a quoted path parses the quoted header",
			output: "diff --git \"a/caf\\303\\251 b/mode.sh\" \"b/caf\\303\\251 b/mode.sh\"\n" +
				"old mode 100644\n" +
				"new mode 100755\n",
			wantPath: "caf\u00e9 b/mode.sh",
		},
		{
			name: "mode-only change with escaped quote in path parses the quoted header",
			output: "diff --git \"a/has\\\"quote.sh\" \"b/has\\\"quote.sh\"\n" +
				"old mode 100644\n" +
				"new mode 100755\n",
			wantPath: "has\"quote.sh",
		},
		{
			name: "mode-only change with b-slash path splits on the equal-paths separator",
			output: "diff --git a/mode name b/dir.sh b/mode name b/dir.sh\n" +
				"old mode 100644\n" +
				"new mode 100755\n",
			wantPath: "mode name b/dir.sh",
		},
		{
			name: "mode-only change with quote and b-slash in path",
			output: "diff --git \"a/weird\\\"quote b/file.sh\" \"b/weird\\\"quote b/file.sh\"\n" +
				"old mode 100644\n" +
				"new mode 100755\n",
			wantPath: "weird\"quote b/file.sh",
		},
		{
			name: "binary path containing and b-slash splits on the equal-paths separator",
			output: "diff --git a/old and b/thing.bin b/old and b/thing.bin\n" +
				"index 6735744..d7bf111 100644\n" +
				"Binary files a/old and b/thing.bin and b/old and b/thing.bin differ\n",
			wantPath: "old and b/thing.bin",
		},
		{
			name: "binary rename with and b-slash in new path keeps the new path",
			output: "diff --git a/x.bin b/old and b/thing.bin\n" +
				"similarity index 50%\n" +
				"rename from x.bin\n" +
				"rename to old and b/thing.bin\n" +
				"Binary files a/x.bin and b/old and b/thing.bin differ\n",
			wantPath: "old and b/thing.bin",
		},
	}

	gitOp := &GitOperator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := gitOp.parseCommitDiffWithOptions(tt.output, parseCommitDiffOptions{})
			entry, ok := files[tt.wantPath].(map[string]interface{})
			if !ok {
				t.Fatalf("no entry for %q; got keys %v", tt.wantPath, keysOf(files))
			}
			if got, _ := entry["path"].(string); got != tt.wantPath {
				t.Errorf("entry path = %#v, want %#v", got, tt.wantPath)
			}
		})
	}
}

// TestGetCumulativeDiff_ModeOnlyBSlashPath covers the round-5 blocker with
// real git output: a chmod-only change on a path containing the literal
// sequence ` b/` produces an unquoted header with no ---/+++ lines
// (`diff --git a/mode name b/dir.sh b/mode name b/dir.sh` + old mode/new
// mode). The first ` b/` lies inside the old path, so the equal-paths
// separator rule must select the correct split.
func TestGetCumulativeDiff_ModeOnlyBSlashPath(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	if err := os.MkdirAll(filepath.Join(repoDir, "mode name b"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, repoDir, "mode name b/dir.sh", "#!/bin/sh\necho hi\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "seed: add script")
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	// Pin core.filemode so the chmod below reliably produces a mode-only
	// diff. Git's default (auto) disables executable-bit tracking on
	// filesystems without chmod support; on those the test cannot produce
	// the fixture it exists to prove, so skip with an explicit reason rather
	// than fail.
	runGit(t, repoDir, "config", "core.filemode", "true")
	if err := os.Chmod(filepath.Join(repoDir, "mode name b/dir.sh"), 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if diff := runGit(t, repoDir, "diff"); !strings.Contains(diff, "old mode") {
		t.Skipf("filesystem does not track executable bits; skipping mode-only regression (diff=%q)", diff)
	}

	gitOp := NewGitOperator(repoDir, newTestLogger(t), nil)
	result, err := gitOp.GetCumulativeDiff(context.Background(), base)
	if err != nil {
		t.Fatalf("GetCumulativeDiff returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("GetCumulativeDiff failed: %s", result.Error)
	}

	const wantPath = "mode name b/dir.sh"
	if _, ok := result.Files[wantPath].(map[string]interface{}); !ok {
		t.Fatalf("no entry for %q; got keys %v", wantPath, keysOf(result.Files))
	}
}

func TestGetCumulativeDiff_StablePrefixesIgnoreGitDiffConfig(t *testing.T) {
	for _, diffConfig := range []struct {
		name  string
		key   string
		value string
	}{
		{name: "no prefix", key: "diff.noprefix", value: "true"},
		{name: "mnemonic prefix", key: "diff.mnemonicPrefix", value: "true"},
	} {
		t.Run(diffConfig.name, func(t *testing.T) {
			repoDir, cleanup := setupTestRepo(t)
			t.Cleanup(cleanup)

			if err := os.MkdirAll(filepath.Join(repoDir, "src"), 0o755); err != nil {
				t.Fatalf("mkdir src: %v", err)
			}
			writeFile(t, repoDir, "src/config.txt", "before\n")
			runGit(t, repoDir, "add", ".")
			runGit(t, repoDir, "commit", "-m", "seed config file")
			base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
			runGit(t, repoDir, "config", diffConfig.key, diffConfig.value)
			writeFile(t, repoDir, "src/config.txt", "after\n")

			result, err := NewGitOperator(repoDir, newTestLogger(t), nil).GetCumulativeDiff(
				context.Background(), base,
			)
			if err != nil {
				t.Fatalf("GetCumulativeDiff returned error: %v", err)
			}
			if !result.Success {
				t.Fatalf("GetCumulativeDiff failed: %s", result.Error)
			}
			if _, ok := result.Files["src/config.txt"].(map[string]interface{}); !ok {
				t.Fatalf("no entry for src/config.txt; got keys %v", keysOf(result.Files))
			}
		})
	}
}

// TestParseCommitDiffWithOptions_HeaderTokenInsideBody guards the section
// boundary against a patch body (or filename) containing the literal
// `diff --git ` byte sequence. git only emits that token at the start of a
// section header line, so the split must be line-aware: an added line like
// `+diff --git a/example b/example` must not truncate the real section or
// fabricate a bogus one.
func TestParseCommitDiffWithOptions_HeaderTokenInsideBody(t *testing.T) {
	output := "diff --git a/real.txt b/real.txt\n" +
		"index 111..222 100644\n" +
		"--- a/real.txt\n" +
		"+++ b/real.txt\n" +
		"@@ -1 +1,2 @@\n" +
		"-old\n" +
		"+diff --git a/example b/example\n" +
		"+new\n"

	gitOp := &GitOperator{}
	files := gitOp.parseCommitDiffWithOptions(output, parseCommitDiffOptions{})
	if len(files) != 1 {
		t.Fatalf("got %d files, want exactly real.txt (%v)", len(files), keysOf(files))
	}
	entry, ok := files["real.txt"].(map[string]interface{})
	if !ok {
		t.Fatalf("no entry for real.txt; got keys %v", keysOf(files))
	}
	if got, _ := entry["path"].(string); got != "real.txt" {
		t.Errorf("entry path = %#v, want real.txt", got)
	}
	diff, _ := entry["diff"].(string)
	if !strings.Contains(diff, "+diff --git a/example b/example") || !strings.Contains(diff, "+new") {
		t.Errorf("real.txt diff truncated by header-token line: %q", diff)
	}
	if got, _ := entry["additions"].(int); got != 2 {
		t.Errorf("real.txt additions = %d, want 2 (the added header-token line counts)", got)
	}
	if _, ok := files["example"]; ok {
		t.Errorf("bogus example entry created from patch-body token: %v", keysOf(files))
	}
}
