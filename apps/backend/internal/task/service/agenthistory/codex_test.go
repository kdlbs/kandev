package agenthistory

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

const (
	codexThreadA = "01a109d5-1957-7462-9235-e91c49e45533"
	codexThreadB = "01a0cc8c-7ad1-78f1-93f9-ac2779935fdf"
)

func codexRollout(t *testing.T, home, day, thread, metaID, cwd string) string {
	t.Helper()
	path := filepath.Join(home, ".codex", "sessions", day, "rollout-2026-10-05T03-12-03-"+thread+".jsonl")
	meta := fmt.Sprintf(`{"timestamp":"2026-10-05T02:12:03.262Z","type":"session_meta","payload":{"id":%q,"cwd":%q}}`, metaID, cwd)
	writeFile(t, path, meta+"\n"+`{"type":"response_item","payload":{}}`+"\n")
	return path
}

func TestRemoveCodexRemovesOnlyMatchingRollout(t *testing.T) {
	home := t.TempDir()
	workDir := "/home/u/.kandev/quick-chat/session-1"
	target := codexRollout(t, home, "2026/10/05", codexThreadA, codexThreadA, workDir)
	otherThread := codexRollout(t, home, "2026/10/05", codexThreadB, codexThreadB, workDir)
	otherDay := codexRollout(t, home, "2026/10/06", codexThreadB, codexThreadB, "/home/u/project")

	if err := Remove(context.Background(), Identity{AgentName: AgentCodexACP, ResumeToken: codexThreadA, WorkDir: workDir, HomeDir: home}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if exists(t, target) {
		t.Fatal("matching rollout was not removed")
	}
	if !exists(t, otherThread) || !exists(t, otherDay) {
		t.Fatal("an unrelated rollout was removed")
	}
	if !exists(t, filepath.Dir(target)) {
		t.Fatal("rollout day directory was removed")
	}
}

func TestRemoveCodexKeepsRolloutWithMismatchedMeta(t *testing.T) {
	home := t.TempDir()
	workDir := "/home/u/.kandev/quick-chat/session-1"
	wrongCwd := codexRollout(t, home, "2026/10/05", codexThreadA, codexThreadA, "/home/u/project")
	wrongID := codexRollout(t, home, "2026/10/06", codexThreadA, codexThreadB, workDir)
	malformed := filepath.Join(home, ".codex", "sessions", "2026", "10", "07", "rollout-x-"+codexThreadA+".jsonl")
	writeFile(t, malformed, "not json\n")
	notMeta := filepath.Join(home, ".codex", "sessions", "2026", "10", "08", "rollout-x-"+codexThreadA+".jsonl")
	writeFile(t, notMeta, fmt.Sprintf(`{"type":"response_item","payload":{"id":%q,"cwd":%q}}`+"\n", codexThreadA, workDir))

	if err := Remove(context.Background(), Identity{AgentName: AgentCodexAppServer, ResumeToken: codexThreadA, WorkDir: workDir, HomeDir: home}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	for _, path := range []string{wrongCwd, wrongID, malformed, notMeta} {
		if !exists(t, path) {
			t.Fatalf("rollout %s was removed despite mismatched metadata", path)
		}
	}
}

func TestRemoveCodexHonorsCodexHomeEnv(t *testing.T) {
	home := t.TempDir()
	codexHome := filepath.Join(t.TempDir(), "codex-alt")
	workDir := "/w/quick-chat/s"
	defaultRollout := codexRollout(t, home, "2026/10/05", codexThreadA, codexThreadA, workDir)
	altRollout := filepath.Join(codexHome, "sessions", "2026", "10", "05", "rollout-x-"+codexThreadA+".jsonl")
	writeFile(t, altRollout, fmt.Sprintf(`{"type":"session_meta","payload":{"id":%q,"cwd":%q}}`+"\n", codexThreadA, workDir))

	id := Identity{AgentName: AgentCodexACP, ResumeToken: codexThreadA, WorkDir: workDir, HomeDir: home,
		Env: map[string]string{"CODEX_HOME": codexHome}}
	if err := Remove(context.Background(), id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if exists(t, altRollout) {
		t.Fatal("rollout under CODEX_HOME was not removed")
	}
	if !exists(t, defaultRollout) {
		t.Fatal("rollout under the default codex home was removed")
	}
}

func TestRemoveCodexMissingHomeIsNoOp(t *testing.T) {
	id := Identity{AgentName: AgentCodexACP, ResumeToken: codexThreadA, WorkDir: "/w", HomeDir: t.TempDir()}
	if err := Remove(context.Background(), id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}
