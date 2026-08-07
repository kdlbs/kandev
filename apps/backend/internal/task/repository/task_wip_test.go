package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type workflowStepCapacityCreator interface {
	CreateTaskIfWorkflowStepHasCapacity(context.Context, *models.Task, string, int) error
}

type workflowStepAdmissionCreator interface {
	CreateTaskWithWorkflowStepAdmission(context.Context, *models.Task, string, int, string, int) error
}

type queuedTaskPromoter interface {
	PromoteQueuedTaskIfWorkflowStepHasCapacity(context.Context, *models.Task, string, string, int) (bool, error)
}

func TestUpdateTaskIfWorkflowStepHasCapacity_ReturnsTypedWIPError(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "wip-existing", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "wip-step", Title: "Existing", State: v1.TaskStateCreated,
	}); err != nil {
		t.Fatalf("seed existing task: %v", err)
	}
	candidate := &models.Task{
		ID: "wip-candidate", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "other-step", Title: "Candidate", State: v1.TaskStateCreated,
	}
	err := repo.UpdateTaskIfWorkflowStepHasCapacity(ctx, candidate, "wip-step", "wip-candidate", 1)
	if err == nil || !errors.Is(err, wfmodels.ErrWIPLimitExceeded) {
		t.Fatalf("error=%v, want typed WIP limit error", err)
	}
}

func TestCreateTaskIfWorkflowStepHasCapacity_Concurrent(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	creator, ok := any(repo).(workflowStepCapacityCreator)
	if !ok {
		t.Fatal("task repository does not implement atomic workflow-step capacity creation")
	}

	const (
		workerCount = 8
		stepID      = "wip-step"
	)
	ctx := context.Background()
	start := make(chan struct{})
	results := make(chan error, workerCount)
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results <- creator.CreateTaskIfWorkflowStepHasCapacity(ctx, &models.Task{
				ID:             fmt.Sprintf("wip-task-%d", index),
				WorkspaceID:    "wip-workspace",
				WorkflowID:     "wip-workflow",
				WorkflowStepID: stepID,
				Title:          fmt.Sprintf("Task %d", index),
				State:          v1.TaskStateCreated,
			}, stepID, 2)
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	created, rejected := 0, 0
	for err := range results {
		if err == nil {
			created++
			continue
		}
		if !strings.Contains(strings.ToLower(err.Error()), "wip limit exceeded") {
			t.Fatalf("unexpected create error: %v", err)
		}
		rejected++
	}
	if created != 2 || rejected != workerCount-2 {
		t.Fatalf("created=%d rejected=%d, want created=2 rejected=%d", created, rejected, workerCount-2)
	}

	occupants, err := repo.CountTasksByWorkflowStep(ctx, stepID)
	if err != nil {
		t.Fatalf("count occupants: %v", err)
	}
	if occupants != 2 {
		t.Fatalf("occupants=%d, want 2", occupants)
	}
}

func TestCreateTaskWithWorkflowStepAdmission_QueuesOverflowInPlace(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	creator, ok := any(repo).(workflowStepAdmissionCreator)
	if !ok {
		t.Fatal("task repository does not implement workflow-step admission")
	}

	ctx := context.Background()
	const stepID = "wip-step"
	for i := 0; i < 2; i++ {
		if err := repo.CreateTask(ctx, &models.Task{
			ID:             fmt.Sprintf("existing-%d", i),
			WorkspaceID:    "wip-workspace",
			WorkflowID:     "wip-workflow",
			WorkflowStepID: stepID,
			Title:          "Existing",
			State:          v1.TaskStateCreated,
		}); err != nil {
			t.Fatalf("seed existing task %d: %v", i, err)
		}
	}

	for i := 0; i < 5; i++ {
		task := &models.Task{
			ID:             fmt.Sprintf("queued-%d", i),
			WorkspaceID:    "wip-workspace",
			WorkflowID:     "wip-workflow",
			WorkflowStepID: stepID,
			Title:          "Queued",
			State:          v1.TaskStateCreated,
		}
		if err := creator.CreateTaskWithWorkflowStepAdmission(ctx, task, stepID, 2, "", 0); err != nil {
			t.Fatalf("create overflow task %d: %v", i, err)
		}
		if task.WorkflowStepID != stepID {
			t.Fatalf("task %s moved to step %q", task.ID, task.WorkflowStepID)
		}
		if task.WIPAdmitted {
			t.Fatalf("task %s unexpectedly admitted", task.ID)
		}
		if task.QueuedForStepID != stepID {
			t.Fatalf("task %s queued_for_step_id=%q, want %q", task.ID, task.QueuedForStepID, stepID)
		}
	}

	tasks, err := repo.ListTasksByWorkflowStep(ctx, stepID)
	if err != nil {
		t.Fatalf("list step tasks: %v", err)
	}
	if len(tasks) != 7 {
		t.Fatalf("resident tasks=%d, want 7", len(tasks))
	}
	admitted := 0
	for _, task := range tasks {
		if task.WIPAdmitted {
			admitted++
		}
	}
	if admitted != 2 {
		t.Fatalf("admitted tasks=%d, want 2", admitted)
	}
}

func TestCreateTaskWithWorkflowStepAdmission_UsesFeederAndStopsAtFullFeeder(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	creator, ok := any(repo).(workflowStepAdmissionCreator)
	if !ok {
		t.Fatal("task repository does not implement workflow-step admission")
	}
	ctx := context.Background()

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "target-existing", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "target", Title: "Target", State: v1.TaskStateCreated,
	}); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	queued := &models.Task{
		ID: "feeder-queued", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "target", Title: "Feeder queued", State: v1.TaskStateCreated,
	}
	if err := creator.CreateTaskWithWorkflowStepAdmission(ctx, queued, "target", 1, "feeder", 0); err != nil {
		t.Fatalf("create feeder overflow: %v", err)
	}
	if queued.WorkflowStepID != "feeder" || queued.QueuedForStepID != "target" || !queued.WIPAdmitted {
		t.Fatalf("unexpected feeder placement: step=%q queue=%q admitted=%t", queued.WorkflowStepID, queued.QueuedForStepID, queued.WIPAdmitted)
	}

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "feeder-existing", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "feeder", Title: "Feeder", State: v1.TaskStateCreated,
	}); err != nil {
		t.Fatalf("seed feeder: %v", err)
	}
	blocked := &models.Task{
		ID: "blocked", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "target", Title: "Blocked", State: v1.TaskStateCreated,
	}
	if err := creator.CreateTaskWithWorkflowStepAdmission(ctx, blocked, "target", 1, "feeder", 1); err == nil || !errors.Is(err, wfmodels.ErrWIPLimitExceeded) {
		t.Fatalf("error=%v, want typed full-feeder conflict", err)
	}
	if _, err := repo.GetTask(ctx, blocked.ID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("blocked task lookup error=%v, want task not found", err)
	}
}

func TestPromoteQueuedTaskIfWorkflowStepHasCapacity_ClaimsOnce(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	promoter, ok := any(repo).(queuedTaskPromoter)
	if !ok {
		t.Fatal("task repository does not implement atomic queued-task promotion")
	}
	ctx := context.Background()
	queued := &models.Task{
		ID: "queued-once", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "feeder", Title: "Queued", State: v1.TaskStateCreated,
		WIPAdmitted: true, QueuedForStepID: "target",
	}
	if err := repo.CreateTask(ctx, queued); err != nil {
		t.Fatalf("create queued task: %v", err)
	}
	queued.WorkflowID = "wip-workflow"
	queued.WorkflowStepID = "target"
	queued.QueuedForStepID = ""
	queued.QueuedAt = nil
	first, err := promoter.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, queued, "feeder", "target", 1)
	if err != nil || !first {
		t.Fatalf("first promotion claimed=%t err=%v, want claim", first, err)
	}
	second, err := promoter.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, queued, "feeder", "target", 1)
	if err != nil {
		t.Fatalf("second promotion: %v", err)
	}
	if second {
		t.Fatal("second promotion claimed the already-promoted task")
	}
	got, err := repo.GetTask(ctx, queued.ID)
	if err != nil {
		t.Fatalf("reload promoted task: %v", err)
	}
	if got.WorkflowStepID != "target" || got.QueuedForStepID != "" || !got.WIPAdmitted {
		t.Fatalf("promoted task state: step=%q queue=%q admitted=%t", got.WorkflowStepID, got.QueuedForStepID, got.WIPAdmitted)
	}
}

func TestTaskMetadataKeyHelpersRoundTripNestedValue(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	task := &models.Task{ID: "metadata-task", WorkspaceID: "metadata-workspace", Title: "Metadata"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.SetTaskMetadataKey(ctx, task.ID, models.MetaKeyDeferredLaunch, map[string]string{"agent_profile_id": "agent"}); err != nil {
		t.Fatalf("set metadata key: %v", err)
	}
	got, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("reload metadata task: %v", err)
	}
	intent, ok := got.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	if !ok || intent["agent_profile_id"] != "agent" {
		t.Fatalf("metadata intent = %#v, want nested agent profile", got.Metadata[models.MetaKeyDeferredLaunch])
	}
	removed, err := repo.RemoveTaskMetadataKey(ctx, task.ID, models.MetaKeyDeferredLaunch)
	if err != nil || !removed {
		t.Fatalf("remove metadata key removed=%t err=%v", removed, err)
	}
	got, err = repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("reload after metadata removal: %v", err)
	}
	if _, exists := got.Metadata[models.MetaKeyDeferredLaunch]; exists {
		t.Fatalf("deferred launch metadata still present: %#v", got.Metadata)
	}
}

// TestSetTaskMetadataKeyIfNotArchivedSkipsArchivedTasks pins the archive-atomic
// contract of the interrupted-marker write: an archive that commits between a
// guard read and the metadata write must not leave a marker on an archived
// task, so the conditional write itself must be the guard (the check and the
// write are one statement).
func TestSetTaskMetadataKeyIfNotArchivedSkipsArchivedTasks(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	live := &models.Task{ID: "live-task", WorkspaceID: "metadata-workspace", Title: "Live"}
	if err := repo.CreateTask(ctx, live); err != nil {
		t.Fatalf("create live task: %v", err)
	}
	archived := &models.Task{ID: "archived-task", WorkspaceID: "metadata-workspace", Title: "Archived"}
	if err := repo.CreateTask(ctx, archived); err != nil {
		t.Fatalf("create archived task: %v", err)
	}
	if err := repo.ArchiveTask(ctx, archived.ID); err != nil {
		t.Fatalf("archive task: %v", err)
	}

	changed, err := repo.SetTaskMetadataKeyIfNotArchived(ctx, live.ID, models.MetaKeyInterruptedAt, "2026-08-07T00:00:00Z")
	if err != nil || !changed {
		t.Fatalf("live task write changed=%t err=%v, want true/nil", changed, err)
	}
	changed, err = repo.SetTaskMetadataKeyIfNotArchived(ctx, archived.ID, models.MetaKeyInterruptedAt, "2026-08-07T00:00:00Z")
	if err != nil || changed {
		t.Fatalf("archived task write changed=%t err=%v, want false/nil", changed, err)
	}

	got, err := repo.GetTask(ctx, archived.ID)
	if err != nil {
		t.Fatalf("reload archived task: %v", err)
	}
	if _, marked := got.Metadata[models.MetaKeyInterruptedAt]; marked {
		t.Fatalf("archived task must not carry the interrupted marker: %#v", got.Metadata)
	}
	got, err = repo.GetTask(ctx, live.ID)
	if err != nil {
		t.Fatalf("reload live task: %v", err)
	}
	if _, marked := got.Metadata[models.MetaKeyInterruptedAt]; !marked {
		t.Fatalf("live task must carry the interrupted marker: %#v", got.Metadata)
	}
}
