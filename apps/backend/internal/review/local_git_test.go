package review

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/utility/hash"
)

func TestCollectLocalChangesUntrackedDiffUsesCanonicalPatch(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init")
	run("config", "user.email", "review@example.test")
	run("config", "user.name", "Review Test")
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "base")

	const path = "new[one] file.txt"
	if err := os.WriteFile(filepath.Join(dir, path), []byte("first\nsecond\n"), 0600); err != nil {
		t.Fatal(err)
	}

	files, err := CollectLocalChanges(context.Background(), Source{WorktreePath: dir, BaseRef: "HEAD"})
	if err != nil {
		t.Fatalf("CollectLocalChanges: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %+v, want one untracked file", files)
	}
	want := "diff --git a/new[one] file.txt b/new[one] file.txt\n" +
		"new file mode 100644\n" +
		"index 0000000..0000000\n" +
		"--- /dev/null\n" +
		"+++ b/new[one] file.txt\n" +
		"@@ -0,0 +1,2 @@\n" +
		"+first\n+second"
	if files[0].Path != path || files[0].Diff != want {
		t.Fatalf("untracked file = %+v, want canonical patch %q", files[0], want)
	}
	if files[0].DiffHash != hash.DJB2(want) {
		t.Fatalf("diff hash = %q, want %q", files[0].DiffHash, hash.DJB2(want))
	}
}

func TestCollectLocalChangesSkipsOversizedStagedBlobWithSmallWorktreeFile(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init")
	run("config", "user.email", "review@example.test")
	run("config", "user.name", "Review Test")
	const path = "large.txt"
	const base = "small\n"
	if err := os.WriteFile(filepath.Join(dir, path), []byte(base), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", path)
	run("commit", "-m", "base")
	if err := os.WriteFile(filepath.Join(dir, path), []byte(strings.Repeat("x", localMaxDiffFileSize+1)), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", path)
	if err := os.WriteFile(filepath.Join(dir, path), []byte(base), 0600); err != nil {
		t.Fatal(err)
	}

	files, err := CollectLocalChanges(context.Background(), Source{WorktreePath: dir, BaseRef: "HEAD"})
	if err != nil {
		t.Fatalf("CollectLocalChanges: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("files = %+v, want oversized index blob skipped", files)
	}
}
