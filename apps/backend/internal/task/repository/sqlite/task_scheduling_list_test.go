package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ListSchedulingTasks feeds the startup pass for interrupted automatic starts
// (AC-TASKS-RESTART-ORPHAN-SESSIONS-003.1). It lists only active, visible,
// non-automation tasks in SCHEDULING, oldest update first.
func TestListSchedulingTasksReturnsOnlyActiveSchedulingTasks(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)
	create := func(id string, state v1.TaskState, offset time.Duration, mutate func(*models.Task)) {
		t.Helper()
		task := &models.Task{ID: id, Title: id, State: state, CreatedAt: base, UpdatedAt: base.Add(offset)}
		if mutate != nil {
			mutate(task)
		}
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask %s: %v", id, err)
		}
		if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET state = ?, updated_at = ? WHERE id = ?`),
			string(state), base.Add(offset), id); err != nil {
			t.Fatalf("pin %s: %v", id, err)
		}
	}
	create("scheduling-newer", v1.TaskStateScheduling, 2*time.Minute, nil)
	create("scheduling-older", v1.TaskStateScheduling, time.Minute, nil)
	create("in-progress", v1.TaskStateInProgress, 0, nil)
	create("ephemeral", v1.TaskStateScheduling, 0, func(task *models.Task) { task.IsEphemeral = true })
	create("automation", v1.TaskStateScheduling, 0, func(task *models.Task) { task.Origin = models.TaskOriginAutomationRun })
	create("archived", v1.TaskStateScheduling, 0, nil)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET archived_at = ? WHERE id = ?`), base, "archived"); err != nil {
		t.Fatalf("archive: %v", err)
	}

	tasks, err := repo.ListSchedulingTasks(ctx)
	if err != nil {
		t.Fatalf("ListSchedulingTasks: %v", err)
	}
	var ids []string
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	if len(ids) != 2 || ids[0] != "scheduling-older" || ids[1] != "scheduling-newer" {
		t.Fatalf("ListSchedulingTasks = %v, want [scheduling-older scheduling-newer]", ids)
	}
}
