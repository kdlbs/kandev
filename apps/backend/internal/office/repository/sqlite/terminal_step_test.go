package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// seedStep inserts a workflow_steps row with the given workflow, position,
// and name — the three columns IsTaskWorkflowStepTerminal reads.
func seedStep(t *testing.T, repo *sqlite.Repository, ctx context.Context, id, workflowID string, position int, name string) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_steps (id, workflow_id, position, name)
		VALUES (?, ?, ?, ?)
	`, id, workflowID, position, name); err != nil {
		t.Fatalf("insert workflow_step %s: %v", id, err)
	}
}

func TestIsTaskWorkflowStepTerminal(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	// office-default shape: Backlog(0) -> Work(1) -> Review(2) -> Approval(3) -> Done(4).
	seedStep(t, repo, ctx, "step-backlog", "wf-office", 0, "Backlog")
	seedStep(t, repo, ctx, "step-work", "wf-office", 1, "Work")
	seedStep(t, repo, ctx, "step-review", "wf-office", 2, "Review")
	seedStep(t, repo, ctx, "step-approval", "wf-office", 3, "Approval")
	seedStep(t, repo, ctx, "step-done", "wf-office", 4, "Done")

	// A single-step workflow whose one step is misleadingly named "Done"
	// but is not last-by-position relative to a later step in the SAME
	// workflow — exercises the position half of the check independent of
	// the name half.
	seedStep(t, repo, ctx, "step-early-done", "wf-mixed", 0, "Done")
	seedStep(t, repo, ctx, "step-mixed-last", "wf-mixed", 1, "Custom")

	seedParticipantTask(t, repo, "task-backlog", "step-backlog")
	seedParticipantTask(t, repo, "task-work", "step-work")
	seedParticipantTask(t, repo, "task-review", "step-review")
	seedParticipantTask(t, repo, "task-approval", "step-approval")
	seedParticipantTask(t, repo, "task-done", "step-done")
	seedParticipantTask(t, repo, "task-early-done", "step-early-done")
	seedParticipantTask(t, repo, "task-mixed-last", "step-mixed-last")
	seedParticipantTask(t, repo, "task-no-step", "")

	cases := []struct {
		name   string
		taskID string
		want   bool
	}{
		{"backlog: not last position", "task-backlog", false},
		{"work: not last position", "task-work", false},
		{"review: not last position (this is the live bug's step)", "task-review", false},
		{"approval: not last position", "task-approval", false},
		{"done: last position and terminal name", "task-done", true},
		{"named Done but not last position", "task-early-done", false},
		{"last position but non-terminal name", "task-mixed-last", false},
		{"no workflow_step_id", "task-no-step", false},
		{"missing task", "task-missing", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.IsTaskWorkflowStepTerminal(ctx, tc.taskID)
			if err != nil {
				t.Fatalf("IsTaskWorkflowStepTerminal: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
