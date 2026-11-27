package agenthistory

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRemoveCodexDoesNotGuessRootForRelativeOverride(t *testing.T) {
	home := t.TempDir()
	workDir := "/w/chat"
	otherStore := codexRollout(t, home, "2026/10/14", codexThreadA, codexThreadA, workDir)
	id := Identity{AgentName: AgentCodexACP, ResumeToken: codexThreadA, WorkDir: workDir,
		HomeDir: home, Env: map[string]string{"CODEX_HOME": "relative-store"}}
	if err := Remove(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if !exists(t, otherStore) {
		t.Fatal("an explicit unresolved config override caused deletion in the default store")
	}
}

func TestRemoveCodexKeepsLiteralConfigWhitespace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve trailing spaces in directory names")
	}
	home := t.TempDir()
	workDir := "/w/chat"
	configDir := filepath.Join(t.TempDir(), "codex ")
	name := filepath.Join("sessions", "2026", "10", "14", "rollout-x-"+codexThreadA+".jsonl")
	meta := fmt.Sprintf(`{"type":"session_meta","payload":{"id":%q,"cwd":%q}}`+"\n", codexThreadA, workDir)
	target := filepath.Join(configDir, name)
	otherStore := filepath.Join(filepath.Clean(configDir[:len(configDir)-1]), name)
	writeFile(t, target, meta)
	writeFile(t, otherStore, meta)
	id := Identity{AgentName: AgentCodexACP, ResumeToken: codexThreadA, WorkDir: workDir,
		HomeDir: home, Env: map[string]string{"CODEX_HOME": configDir}}
	if err := Remove(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if exists(t, target) || !exists(t, otherStore) {
		t.Fatal("config whitespace was changed, removing history from a different store")
	}
}
