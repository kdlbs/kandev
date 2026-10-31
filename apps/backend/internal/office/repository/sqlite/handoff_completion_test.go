package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	runsmodels "github.com/kandev/kandev/internal/runs/models"
)

func TestHasHandoffSourceRun_RequiresClaimedParticipation(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	seedAgentProfile(t, repo.Writer(), "worker-source", "ws-1")

	run := &runsmodels.Run{
		AgentProfileID: "worker-source",
		Reason:         "handoff_completed",
		Payload:        `{"task_id":"source-task","comment_id":"cm-1"}`,
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	matches, err := repo.HasHandoffSourceRun(ctx, "source-task", "session-1", "worker-source")
	if err != nil {
		t.Fatalf("has handoff source run: %v", err)
	}
	if matches {
		t.Fatal("a queued run must not count as historical participation")
	}

	if _, err := repo.ExecRaw(ctx,
		`UPDATE runs SET session_id = ?, claimed_at = ? WHERE id = ?`,
		"session-1", time.Now().UTC(), run.ID); err != nil {
		t.Fatalf("claim run: %v", err)
	}

	matches, err = repo.HasHandoffSourceRun(ctx, "source-task", "session-1", "worker-source")
	if err != nil {
		t.Fatalf("has handoff source run after claim: %v", err)
	}
	if !matches {
		t.Fatal("a claimed run in the source session must count")
	}

	matches, err = repo.HasHandoffSourceRun(ctx, "other-task", "session-1", "worker-source")
	if err != nil {
		t.Fatalf("has handoff source run for other task: %v", err)
	}
	if matches {
		t.Fatal("a run for another task must not match")
	}

	matches, err = repo.HasHandoffSourceRun(ctx, "source-task", "other-session", "worker-source")
	if err != nil {
		t.Fatalf("has handoff source run for other session: %v", err)
	}
	if matches {
		t.Fatal("a run in another session must not match")
	}
}

func TestCreateHandoffCompletionComment_ReusesDeterministicID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	first := &models.TaskComment{
		ID:         "handoff_completed:delivery-1:COMPLETED",
		TaskID:     "source-task",
		AuthorType: "agent",
		Body:       "first report",
		Source:     "handoff",
	}
	createdComment, created, err := repo.CreateHandoffCompletionComment(ctx, first)
	if err != nil {
		t.Fatalf("create handoff comment: %v", err)
	}
	if !created {
		t.Fatal("first insert must create the report")
	}
	if createdComment.ID != first.ID || createdComment.Body != first.Body {
		t.Fatalf("created comment = %+v", createdComment)
	}

	second := &models.TaskComment{
		ID:         first.ID,
		TaskID:     "source-task",
		AuthorType: "agent",
		Body:       "second report",
		Source:     "handoff",
	}
	existing, created, err := repo.CreateHandoffCompletionComment(ctx, second)
	if err != nil {
		t.Fatalf("redeliver handoff comment: %v", err)
	}
	if created {
		t.Fatal("redelivery must reuse the existing report")
	}
	if existing.Body != first.Body {
		t.Fatalf("existing report body = %q, want the original", existing.Body)
	}

	comments, err := repo.ListTaskComments(ctx, "source-task")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("comments = %d, want exactly 1", len(comments))
	}

	conflict := &models.TaskComment{
		ID:         first.ID,
		TaskID:     "other-task",
		AuthorType: "agent",
		Body:       "conflicting report",
		Source:     "handoff",
	}
	if _, _, err := repo.CreateHandoffCompletionComment(ctx, conflict); err == nil {
		t.Fatal("a colliding id for another task must be an error")
	}
}
