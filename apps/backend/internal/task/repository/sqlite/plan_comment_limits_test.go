// @covers AC-TASKS-PLAN-COMMENTS-001.8
package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestPlanCommentRepositoryEnforcesCollectionLimitsAtomically(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	taskID, planID := "task-plan-comment-limits", "plan-comment-limits"
	seedTaskForDocs(t, repo, taskID)
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: planID, TaskID: taskID, Content: "Plan",
	}); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 100; index++ {
		_, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
			ID: fmt.Sprintf("comment-limit-%03d", index), TaskID: taskID, PlanID: planID,
			Body: "body", SelectedText: "Plan", AnchorFrom: 0, AnchorTo: 4,
		})
		if err != nil {
			t.Fatalf("create comment %d: %v", index, err)
		}
	}
	if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-limit-over", TaskID: taskID, PlanID: planID,
		Body: "body", SelectedText: "Plan", AnchorFrom: 0, AnchorTo: 4,
	}); err == nil {
		t.Fatalf("count overflow error = %v", err)
	}
	snapshot, err := repo.ListTaskPlanComments(ctx, taskID)
	if err != nil || len(snapshot.Comments) != 100 {
		t.Fatalf("snapshot after count rejection = %#v, err=%v", snapshot, err)
	}
	first := snapshot.Comments[0]
	if _, err := repo.UpdateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: first.ID, TaskID: taskID, PlanID: planID,
		Body: strings.Repeat("x", (1 << 20)),
	}, first.Version); err == nil {
		t.Fatalf("aggregate overflow error = %v", err)
	}
	after, err := repo.ListTaskPlanComments(ctx, taskID)
	if err != nil || after.Comments[0].Body != first.Body || after.Comments[0].Version != first.Version {
		t.Fatalf("aggregate rejection changed comment: %#v, err=%v", after.Comments[0], err)
	}
}
