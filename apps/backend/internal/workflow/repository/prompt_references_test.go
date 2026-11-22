package repository

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestListPromptReferenceTextsCoversWorkflowAndStepPromptsInAllWorkspaces(t *testing.T) {
	repo, writer, _ := openImprovePromptRepo(t, filepath.Join(t.TempDir(), "refs.db"))
	if _, err := writer.Exec(`ALTER TABLE workflows ADD COLUMN prompt TEXT NOT NULL DEFAULT ''`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, wf := range []struct{ id, workspace, prompt string }{
		{"wf-a", "ws-1", "Workflow uses @principles"},
		{"wf-b", "ws-2", ""},
	} {
		if _, err := writer.Exec(`INSERT INTO workflows (id, workspace_id, name, prompt, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			wf.id, wf.workspace, wf.id, wf.prompt, now, now); err != nil {
			t.Fatal(err)
		}
	}
	for _, step := range []struct {
		id, workflow string
		prompt       any
	}{
		{"s-1", "wf-a", "Step @review"},
		{"s-2", "wf-b", "Other workspace @review"},
		{"s-3", "wf-b", nil},
		{"s-4", "wf-b", ""},
	} {
		if _, err := writer.Exec(`INSERT INTO workflow_steps (id, workflow_id, name, position, prompt, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?, ?)`,
			step.id, step.workflow, step.id, step.prompt, now, now); err != nil {
			t.Fatal(err)
		}
	}

	texts, err := repo.ListPromptReferenceTexts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, text := range texts {
		got[text.WorkflowID+"|"+text.Text] = true
	}
	want := []string{"wf-a|Workflow uses @principles", "wf-a|Step @review", "wf-b|Other workspace @review"}
	if len(texts) != len(want) {
		t.Fatalf("texts = %+v, want %v", texts, want)
	}
	for _, key := range want {
		if !got[key] {
			t.Fatalf("missing %q in %+v", key, texts)
		}
	}
}
