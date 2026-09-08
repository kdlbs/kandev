package sqlite_test

import (
	"context"
	"testing"
)

func TestGetWorkflowStepStageType(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `CREATE TABLE workflow_steps (
		id TEXT PRIMARY KEY,
		stage_type TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create workflow_steps: %v", err)
	}
	if _, err := repo.ExecRaw(ctx,
		`INSERT INTO workflow_steps (id, stage_type) VALUES (?, ?)`,
		"step-approval", "approval"); err != nil {
		t.Fatalf("insert workflow step: %v", err)
	}

	got, err := repo.GetWorkflowStepStageType(ctx, "step-approval")
	if err != nil {
		t.Fatalf("stage type lookup: %v", err)
	}
	if got != "approval" {
		t.Fatalf("stage type = %q, want approval", got)
	}

	missing, err := repo.GetWorkflowStepStageType(ctx, "step-missing")
	if err != nil {
		t.Fatalf("missing stage type lookup: %v", err)
	}
	if missing != "" {
		t.Fatalf("missing stage type = %q, want empty", missing)
	}
}
