package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureWakePayloadGitExclude_WorktreeGitFile(t *testing.T) {
	workspace := t.TempDir()
	gitDir := filepath.Join(t.TempDir(), "worktrees", "task", ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "info"), 0o700); err != nil {
		t.Fatalf("create git info dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o600); err != nil {
		t.Fatalf("write .git file: %v", err)
	}

	if err := ensureWakePayloadGitExclude(workspace); err != nil {
		t.Fatalf("ensureWakePayloadGitExclude: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(gitDir, "info", "exclude"))
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if !strings.Contains(string(data), wakePayloadExcludeLine) {
		t.Fatalf("exclude missing %q: %s", wakePayloadExcludeLine, string(data))
	}
}
