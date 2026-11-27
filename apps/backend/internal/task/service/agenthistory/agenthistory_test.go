package agenthistory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	tokenA = "bc1f73b7-6b8d-4925-a13f-40e0db287db1"
	tokenB = "eb284f1d-e4ea-47ed-92c2-e883126fc2e7"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err == nil {
		return true
	}
	if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return false
}

func TestEncodeClaudeProjectName(t *testing.T) {
	got := encodeClaudeProjectName("/home/alex/.kandev/quick-chat/e02d6162-145f-445f-8744-724241de48f2")
	want := "-home-alex--kandev-quick-chat-e02d6162-145f-445f-8744-724241de48f2"
	if got != want {
		t.Fatalf("encodeClaudeProjectName = %q, want %q", got, want)
	}
	got = encodeClaudeProjectName("/home/alex/.kandev/tasks/clean-up-quick-chat_kwlyfka3/kdlbs-kandev")
	want = "-home-alex--kandev-tasks-clean-up-quick-chat-kwlyfka3-kdlbs-kandev"
	if got != want {
		t.Fatalf("encodeClaudeProjectName = %q, want %q", got, want)
	}
}

func TestRemoveClaudeRemovesOnlyTheSessionTranscript(t *testing.T) {
	home := t.TempDir()
	workDir := "/home/u/.kandev/quick-chat/session-1"
	projects := filepath.Join(home, ".claude", "projects")
	project := filepath.Join(projects, encodeClaudeProjectName(workDir))
	writeFile(t, filepath.Join(project, tokenA+".jsonl"), "{}\n")
	writeFile(t, filepath.Join(project, tokenA, "subagents", "a.jsonl"), "{}\n")
	writeFile(t, filepath.Join(project, tokenB+".jsonl"), "{}\n")
	other := filepath.Join(projects, "-home-u--kandev-tasks-clean-up-quick-chat-x-repo")
	writeFile(t, filepath.Join(other, tokenA+".jsonl"), "{}\n")

	err := Remove(context.Background(), Identity{AgentName: AgentClaudeACP, ResumeToken: tokenA, WorkDir: workDir, HomeDir: home})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if exists(t, filepath.Join(project, tokenA+".jsonl")) || exists(t, filepath.Join(project, tokenA)) {
		t.Fatal("session transcript or sidecar was not removed")
	}
	if !exists(t, filepath.Join(project, tokenB+".jsonl")) {
		t.Fatal("another session transcript in the same project was removed")
	}
	if !exists(t, filepath.Join(other, tokenA+".jsonl")) {
		t.Fatal("transcript in an unrelated project was removed")
	}
}

func TestRemoveClaudeRemovesEmptyProjectDir(t *testing.T) {
	home := t.TempDir()
	workDir := "/home/u/.kandev/quick-chat/session-1"
	project := filepath.Join(home, ".claude", "projects", encodeClaudeProjectName(workDir))
	writeFile(t, filepath.Join(project, tokenA+".jsonl"), "{}\n")
	if err := os.MkdirAll(filepath.Join(project, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Remove(context.Background(), Identity{AgentName: AgentClaudeACP, ResumeToken: tokenA, WorkDir: workDir, HomeDir: home}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if exists(t, project) {
		t.Fatal("empty project directory was not removed")
	}
	if !exists(t, filepath.Join(home, ".claude", "projects")) {
		t.Fatal("projects root was removed")
	}
}

func TestRemoveClaudeKeepsProjectWithMemoryContent(t *testing.T) {
	home := t.TempDir()
	workDir := "/home/u/.kandev/quick-chat/session-1"
	project := filepath.Join(home, ".claude", "projects", encodeClaudeProjectName(workDir))
	writeFile(t, filepath.Join(project, tokenA+".jsonl"), "{}\n")
	writeFile(t, filepath.Join(project, "memory", "note.md"), "keep")

	if err := Remove(context.Background(), Identity{AgentName: AgentClaudeACP, ResumeToken: tokenA, WorkDir: workDir, HomeDir: home}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !exists(t, filepath.Join(project, "memory", "note.md")) {
		t.Fatal("project memory content was removed")
	}
}

func TestRemoveClaudeHonorsConfigDirEnv(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(t.TempDir(), "claude-alt")
	workDir := "/w/quick-chat/s"
	transcript := filepath.Join(configDir, "projects", encodeClaudeProjectName(workDir), tokenA+".jsonl")
	writeFile(t, transcript, "{}\n")
	defaultTranscript := filepath.Join(home, ".claude", "projects", encodeClaudeProjectName(workDir), tokenA+".jsonl")
	writeFile(t, defaultTranscript, "{}\n")

	id := Identity{AgentName: AgentClaudeACP, ResumeToken: tokenA, WorkDir: workDir, HomeDir: home,
		Env: map[string]string{"CLAUDE_CONFIG_DIR": configDir}}
	if err := Remove(context.Background(), id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if exists(t, transcript) {
		t.Fatal("transcript under CLAUDE_CONFIG_DIR was not removed")
	}
	if !exists(t, defaultTranscript) {
		t.Fatal("transcript under the default config dir was removed")
	}
}

func TestRemoveClaudeDoesNotFollowSymlinks(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	workDir := "/w/quick-chat/s"
	target := filepath.Join(outside, "victim.jsonl")
	writeFile(t, target, "keep")
	targetDir := filepath.Join(outside, "victim-dir")
	writeFile(t, filepath.Join(targetDir, "keep.txt"), "keep")
	project := filepath.Join(home, ".claude", "projects", encodeClaudeProjectName(workDir))
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(project, tokenA+".jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetDir, filepath.Join(project, tokenA)); err != nil {
		t.Fatal(err)
	}

	if err := Remove(context.Background(), Identity{AgentName: AgentClaudeACP, ResumeToken: tokenA, WorkDir: workDir, HomeDir: home}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !exists(t, target) || !exists(t, filepath.Join(targetDir, "keep.txt")) {
		t.Fatal("symlink target was removed")
	}
}

func TestRemoveClaudeSkipsSymlinkedProjectDir(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	workDir := "/w/quick-chat/s"
	writeFile(t, filepath.Join(outside, tokenA+".jsonl"), "keep")
	projects := filepath.Join(home, ".claude", "projects")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(projects, encodeClaudeProjectName(workDir))); err != nil {
		t.Fatal(err)
	}

	if err := Remove(context.Background(), Identity{AgentName: AgentClaudeACP, ResumeToken: tokenA, WorkDir: workDir, HomeDir: home}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !exists(t, filepath.Join(outside, tokenA+".jsonl")) {
		t.Fatal("transcript behind a symlinked project dir was removed")
	}
}

func TestRemoveClaudeSkipsOverlongProjectName(t *testing.T) {
	home := t.TempDir()
	workDir := "/" + strings.Repeat("a", 250)
	project := filepath.Join(home, ".claude", "projects", encodeClaudeProjectName(workDir))
	writeFile(t, filepath.Join(project, tokenA+".jsonl"), "{}\n")

	if err := Remove(context.Background(), Identity{AgentName: AgentClaudeACP, ResumeToken: tokenA, WorkDir: workDir, HomeDir: home}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !exists(t, filepath.Join(project, tokenA+".jsonl")) {
		t.Fatal("transcript was removed for an overlong project name")
	}
}

func TestRemoveIsNoOpForMissingOrUnsafeInputs(t *testing.T) {
	home := t.TempDir()
	workDir := "/w/quick-chat/s"
	project := filepath.Join(home, ".claude", "projects", encodeClaudeProjectName(workDir))
	writeFile(t, filepath.Join(project, "..jsonl"), "keep")
	writeFile(t, filepath.Join(project, tokenB+".jsonl"), "keep")

	cases := []Identity{
		{AgentName: AgentClaudeACP, ResumeToken: tokenA, WorkDir: workDir, HomeDir: home},
		{AgentName: AgentClaudeACP, ResumeToken: tokenA, WorkDir: "/does/not/exist", HomeDir: t.TempDir()},
		{AgentName: AgentClaudeACP, ResumeToken: "", WorkDir: workDir, HomeDir: home},
		{AgentName: AgentClaudeACP, ResumeToken: ".", WorkDir: workDir, HomeDir: home},
		{AgentName: AgentClaudeACP, ResumeToken: "../" + tokenB, WorkDir: workDir, HomeDir: home},
		{AgentName: AgentClaudeACP, ResumeToken: tokenB, WorkDir: "", HomeDir: home},
		{AgentName: AgentClaudeACP, ResumeToken: tokenB, WorkDir: "relative/dir", HomeDir: home},
		{AgentName: AgentClaudeACP, ResumeToken: tokenB, WorkDir: workDir, HomeDir: ""},
		{AgentName: "gemini", ResumeToken: tokenB, WorkDir: workDir, HomeDir: home},
	}
	for _, id := range cases {
		if err := Remove(context.Background(), id); err != nil {
			t.Fatalf("Remove(%+v): %v", id, err)
		}
	}
	if !exists(t, filepath.Join(project, tokenB+".jsonl")) || !exists(t, filepath.Join(project, "..jsonl")) {
		t.Fatal("a no-op identity removed history")
	}
}

func TestRemoveStopsWhenContextIsCancelled(t *testing.T) {
	home := t.TempDir()
	workDir := "/w/quick-chat/s"
	transcript := filepath.Join(home, ".claude", "projects", encodeClaudeProjectName(workDir), tokenA+".jsonl")
	writeFile(t, transcript, "{}\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := Remove(ctx, Identity{AgentName: AgentClaudeACP, ResumeToken: tokenA, WorkDir: workDir, HomeDir: home}); err == nil {
		t.Fatal("Remove with a cancelled context returned nil")
	}
	if !exists(t, transcript) {
		t.Fatal("transcript was removed after cancellation")
	}
}
