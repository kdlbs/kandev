// @covers AC-TASKS-PLAN-COMMENTS-001.8
package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresPlanCommentCollectionLimitRollsBack(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	taskID, planID := "task-plan-comments-limit-pg", "plan-comments-limit-pg"
	seedPostgresTask(t, repo, taskID)
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{ID: planID, TaskID: taskID, Content: "Plan"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-comments-limit-pg", TaskID: taskID, PlanID: planID,
		Body:         strings.Repeat("x", (1<<20)+1),
		SelectedText: "Plan", AnchorFrom: 0, AnchorTo: 4,
	}); err == nil {
		t.Fatalf("aggregate overflow error = %v", err)
	}
	snapshot, err := repo.ListTaskPlanComments(ctx, taskID)
	if err != nil || len(snapshot.Comments) != 0 || snapshot.Revision != 0 {
		t.Fatalf("snapshot after rejected create = %#v, err=%v", snapshot, err)
	}
}
