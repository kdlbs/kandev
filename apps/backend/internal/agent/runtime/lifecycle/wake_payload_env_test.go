package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpillLargeWakePayloadEnv_UsesFallbackFileIDWhenRunIDMissing(t *testing.T) {
	workspace := t.TempDir()
	env := map[string]string{
		envWakePayloadJSON: strings.Repeat("x", envWakePayloadInlineMax+1),
	}
	if err := spillLargeWakePayloadEnv(env, workspace, nil); err != nil {
		t.Fatalf("spillLargeWakePayloadEnv: %v", err)
	}
	if _, exists := env[envWakePayloadJSON]; exists {
		t.Fatalf("expected %s removed", envWakePayloadJSON)
	}
	relPath, exists := env[envWakePayloadPath]
	if !exists {
		t.Fatalf("expected %s set", envWakePayloadPath)
	}
	if got, want := filepath.ToSlash(relPath), filepath.ToSlash(filepath.Join(wakePayloadDirRel, defaultWakePayloadFileID+".json")); got != want {
		t.Fatalf("payload path = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(relPath))); err != nil {
		t.Fatalf("payload file missing: %v", err)
	}
}

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

func TestEnsureWakePayloadGitExclude_WorktreeInfoPathIsFile(t *testing.T) {
	workspace := t.TempDir()
	gitDir := filepath.Join(t.TempDir(), "worktrees", "task", ".git")
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		t.Fatalf("create git dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "info"), []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write git info file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o600); err != nil {
		t.Fatalf("write .git file: %v", err)
	}

	err := ensureWakePayloadGitExclude(workspace)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "git info path is not a directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureWakePayloadGitExclude_NormalGitDirWithoutInfo(t *testing.T) {
	workspace := t.TempDir()
	gitDir := filepath.Join(workspace, ".git")
	if err := os.Mkdir(gitDir, 0o700); err != nil {
		t.Fatalf("create .git dir: %v", err)
	}

	if err := ensureWakePayloadGitExclude(workspace); err != nil {
		t.Fatalf("ensureWakePayloadGitExclude: %v", err)
	}

	excludePath := filepath.Join(gitDir, "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if !strings.Contains(string(data), wakePayloadExcludeLine) {
		t.Fatalf("exclude missing %q: %s", wakePayloadExcludeLine, string(data))
	}
}

func TestSanitizeWakePayloadFileID(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", defaultWakePayloadFileID},
		{"spaces", "  ", defaultWakePayloadFileID},
		{"allowed", "run_01-id", "run_01-id"},
		{"specials", "a*b c#1", "abc1"},
		{"only-specials", " /\\*?? ", defaultWakePayloadFileID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeWakePayloadFileID(tc.in)
			if got != tc.want {
				t.Fatalf("sanitizeWakePayloadFileID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSpillLargeWakePayloadEnv_NormalizesLargePayload(t *testing.T) {
	workspace := t.TempDir()
	env := map[string]string{
		envWakePayloadJSON: strings.Repeat("x", envWakePayloadInlineMax-1),
		"KANDEV_RUN_ID":    "keep-inline",
	}
	if err := spillLargeWakePayloadEnv(env, workspace, nil); err != nil {
		t.Fatalf("spillLargeWakePayloadEnv: %v", err)
	}
	if _, exists := env[envWakePayloadPath]; exists {
		t.Fatalf("did not expect %s set for under-limit payload", envWakePayloadPath)
	}
	if _, err := os.Stat(filepath.Join(workspace, wakePayloadDirRel)); err == nil {
		t.Fatalf("expected %s not created for under-limit payload", wakePayloadDirRel)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat wake payload dir: %v", err)
	}
}

func TestSpillLargeWakePayloadEnv_UsesWorkspaceErrorForMissingPath(t *testing.T) {
	workspace := ""
	env := map[string]string{
		envWakePayloadJSON: strings.Repeat("x", envWakePayloadInlineMax+1),
		"KANDEV_RUN_ID":    "x",
	}
	err := spillLargeWakePayloadEnv(env, workspace, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "workspace path is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}
