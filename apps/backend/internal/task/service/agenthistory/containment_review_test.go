package agenthistory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveCodexRejectsSymlinkedDateComponents(t *testing.T) {
	for _, component := range []string{"2026", "2026/10", "2026/10/14"} {
		t.Run(component, func(t *testing.T) {
			home := t.TempDir()
			outside := t.TempDir()
			workDir := "/w/quick-chat/session"
			dateRoot := filepath.Join(home, ".codex", "sessions")
			link := filepath.Join(dateRoot, component)
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			suffix, err := filepath.Rel(component, "2026/10/14")
			if err != nil {
				t.Fatal(err)
			}
			victim := filepath.Join(outside, suffix, "rollout-x-"+codexThreadA+".jsonl")
			writeFile(t, victim, fmt.Sprintf(`{"type":"session_meta","payload":{"id":%q,"cwd":%q}}`+"\n", codexThreadA, workDir))
			if err := Remove(context.Background(), Identity{AgentName: AgentCodexACP, ResumeToken: codexThreadA, WorkDir: workDir, HomeDir: home}); err != nil {
				t.Fatalf("Remove: %v", err)
			}
			if !exists(t, victim) {
				t.Fatal("matching backup rollout outside the history root was removed")
			}
		})
	}
}

func TestRemoveCodexTreatsHomeAsLiteralPath(t *testing.T) {
	home := filepath.Join(t.TempDir(), "codex[1]")
	workDir := "/w/quick-chat/session"
	rollout := codexRollout(t, home, "2026/10/14", codexThreadA, codexThreadA, workDir)
	if err := Remove(context.Background(), Identity{AgentName: AgentCodexACP, ResumeToken: codexThreadA, WorkDir: workDir, HomeDir: home}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if exists(t, rollout) {
		t.Fatal("matching rollout in a literal bracketed home was retained")
	}
}

type historyBoundaryContext struct {
	context.Context
	checks int
	change func()
}

func (c *historyBoundaryContext) Err() error {
	c.checks++
	if c.checks == 2 {
		c.change()
	}
	return nil
}

func TestRemoveClaudeKeepsReplacementProject(t *testing.T) {
	home := t.TempDir()
	workDir := "/w/quick-chat/session"
	project := filepath.Join(home, ".claude", "projects", encodeClaudeProjectName(workDir))
	transcript := filepath.Join(project, tokenA+".jsonl")
	writeFile(t, transcript, "{}\n")
	backup := project + "-original"
	ctx := &historyBoundaryContext{Context: context.Background(), change: func() {
		if err := os.Rename(project, backup); err != nil {
			t.Fatal(err)
		}
		writeFile(t, transcript, "replacement must survive\n")
		writeFile(t, filepath.Join(project, tokenA, "subagents", "keep.jsonl"), "replacement sidecar\n")
	}}
	_ = Remove(ctx, Identity{AgentName: AgentClaudeACP, ResumeToken: tokenA, WorkDir: workDir, HomeDir: home})
	if ctx.checks < 2 {
		t.Fatal("replacement schedule was not exercised")
	}
	if !exists(t, transcript) || !exists(t, filepath.Join(project, tokenA, "subagents", "keep.jsonl")) {
		t.Fatal("a replacement project was removed after the original directory was checked")
	}
}
