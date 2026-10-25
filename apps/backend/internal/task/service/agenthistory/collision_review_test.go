package agenthistory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveClaudeKeepsUnownedEmptyCollidingProject(t *testing.T) {
	home := t.TempDir()
	chatDir := "/w/a-b"
	otherDir := "/w/a/b"
	if encodeClaudeProjectName(chatDir) != encodeClaudeProjectName(otherDir) {
		t.Fatal("fixture must exercise a real project-name collision")
	}
	project := filepath.Join(home, ".claude", "projects", encodeClaudeProjectName(otherDir))
	memory := filepath.Join(project, "memory")
	if err := os.MkdirAll(memory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Remove(context.Background(), Identity{AgentName: AgentClaudeACP, ResumeToken: tokenA, WorkDir: chatDir, HomeDir: home}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !exists(t, project) || !exists(t, memory) {
		t.Fatal("an empty colliding project was removed despite having no transcript or sidecar for this session")
	}
}
