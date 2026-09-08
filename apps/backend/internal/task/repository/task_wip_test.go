package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

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

type workflowStepMoveAdmissionRepository interface {
	UpdateTaskWithWorkflowStepAdmission(context.Context, *models.Task, string, int) (bool, error)
}

type workflowStepMoveAdmissionWithStateRepository interface {
	UpdateTaskWithWorkflowStepAdmissionAndState(context.Context, *models.Task, string, int, *v1.TaskState, bool, string) (bool, error)
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

func TestUpdateTaskWithWorkflowStepAdmission_QueuesOverflowInPlace(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	mover, ok := any(repo).(workflowStepMoveAdmissionRepository)
	if !ok {
		t.Fatal("task repository does not implement atomic workflow-step move admission")
	}
	ctx := context.Background()
	const targetStepID = "move-target"

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "move-occupant", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: targetStepID, Title: "Occupant", State: v1.TaskStateCreated,
	}); err != nil {
		t.Fatalf("seed occupant: %v", err)
	}
	candidate := &models.Task{
		ID: "move-candidate", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "move-source", Title: "Candidate", State: v1.TaskStateCreated,
		WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, candidate); err != nil {
		t.Fatalf("seed candidate: %v", err)
	}

	admitted, err := mover.UpdateTaskWithWorkflowStepAdmission(ctx, candidate, targetStepID, 1)
	if err != nil {
		t.Fatalf("move candidate: %v", err)
	}
	if admitted {
		t.Fatal("full target unexpectedly admitted candidate")
	}
	if candidate.WorkflowStepID != targetStepID || candidate.WIPAdmitted || candidate.QueuedForStepID != targetStepID {
		t.Fatalf("candidate placement: step=%q admitted=%t queued=%q", candidate.WorkflowStepID, candidate.WIPAdmitted, candidate.QueuedForStepID)
	}
	if candidate.QueuedAt == nil {
		t.Fatal("queued candidate has no queue timestamp")
	}
	stored, err := repo.GetTask(ctx, candidate.ID)
	if err != nil {
		t.Fatalf("reload candidate: %v", err)
	}
	if stored.WorkflowStepID != targetStepID || stored.WIPAdmitted || stored.QueuedForStepID != targetStepID {
		t.Fatalf("stored candidate placement: step=%q admitted=%t queued=%q", stored.WorkflowStepID, stored.WIPAdmitted, stored.QueuedForStepID)
	}
}

func TestUpdateTaskWithWorkflowStepAdmission_UnlimitedClearsQueue(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	mover, ok := any(repo).(workflowStepMoveAdmissionRepository)
	if !ok {
		t.Fatal("task repository does not implement atomic workflow-step move admission")
	}
	ctx := context.Background()
	queuedAt := time.Now().UTC().Add(-time.Minute)
	candidate := &models.Task{
		ID: "move-unlimited", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "move-source", Title: "Candidate", State: v1.TaskStateCreated,
		WIPAdmitted: false, QueuedForStepID: "old-target", QueuedAt: &queuedAt,
	}
	if err := repo.CreateTask(ctx, candidate); err != nil {
		t.Fatalf("seed candidate: %v", err)
	}

	admitted, err := mover.UpdateTaskWithWorkflowStepAdmission(ctx, candidate, "unlimited-target", 0)
	if err != nil || !admitted {
		t.Fatalf("move candidate admitted=%t err=%v, want admitted", admitted, err)
	}
	if candidate.WorkflowStepID != "unlimited-target" || !candidate.WIPAdmitted || candidate.QueuedForStepID != "" || candidate.QueuedAt != nil {
		t.Fatalf("candidate placement: step=%q admitted=%t queued=%q queued_at=%v", candidate.WorkflowStepID, candidate.WIPAdmitted, candidate.QueuedForStepID, candidate.QueuedAt)
	}
	stored, err := repo.GetTask(ctx, candidate.ID)
	if err != nil {
		t.Fatalf("reload unlimited candidate: %v", err)
	}
	if stored.WorkflowStepID != "unlimited-target" || !stored.WIPAdmitted || stored.QueuedForStepID != "" || stored.QueuedAt != nil {
		t.Fatalf("stored candidate placement: step=%q admitted=%t queued=%q queued_at=%v", stored.WorkflowStepID, stored.WIPAdmitted, stored.QueuedForStepID, stored.QueuedAt)
	}
}

func TestUpdateTaskWithWorkflowStepAdmissionAndState_PersistsMoveLifecycleAtomically(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	mover, ok := any(repo).(workflowStepMoveAdmissionWithStateRepository)
	if !ok {
		t.Fatal("task repository does not implement atomic workflow-step move admission with state")
	}
	ctx := context.Background()
	const targetStepID = "atomic-move-target"
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "atomic-move-occupant", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: targetStepID, Title: "Occupant", State: v1.TaskStateCreated,
	}); err != nil {
		t.Fatalf("seed occupant: %v", err)
	}
	candidate := &models.Task{
		ID: "atomic-move-candidate", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "atomic-move-source", Title: "Candidate", State: v1.TaskStateTODO,
		WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, candidate); err != nil {
		t.Fatalf("seed candidate: %v", err)
	}
	admittedState := v1.TaskStateCompleted
	admitted, err := mover.UpdateTaskWithWorkflowStepAdmissionAndState(ctx, candidate, targetStepID, 1, &admittedState, true, "")
	if err != nil {
		t.Fatalf("move candidate: %v", err)
	}
	if admitted {
		t.Fatal("full target unexpectedly admitted candidate")
	}
	stored, err := repo.GetTask(ctx, candidate.ID)
	if err != nil {
		t.Fatalf("reload candidate: %v", err)
	}
	if stored.State != v1.TaskStateTODO {
		t.Fatalf("queued move state = %q, want original TODO", stored.State)
	}
	if _, ok := stored.Metadata[models.MetaKeyQueuedMoveExitPending]; !ok {
		t.Fatalf("queued move metadata = %#v, want %q marker", stored.Metadata, models.MetaKeyQueuedMoveExitPending)
	}

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "atomic-admitted-candidate", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "atomic-move-source", Title: "Admitted candidate", State: v1.TaskStateTODO,
		WIPAdmitted: true,
		Metadata:    map[string]interface{}{models.MetaKeyQueuedMoveExitPending: true},
	}); err != nil {
		t.Fatalf("seed unlimited candidate: %v", err)
	}
	admittedCandidate, err := repo.GetTask(ctx, "atomic-admitted-candidate")
	if err != nil {
		t.Fatalf("load unlimited candidate: %v", err)
	}
	admitted, err = mover.UpdateTaskWithWorkflowStepAdmissionAndState(ctx, admittedCandidate, "atomic-unlimited-target", 0, &admittedState, true, "")
	if err != nil || !admitted {
		t.Fatalf("unlimited move admitted=%t err=%v", admitted, err)
	}
	stored, err = repo.GetTask(ctx, admittedCandidate.ID)
	if err != nil {
		t.Fatalf("reload admitted candidate: %v", err)
	}
	if stored.State != v1.TaskStateCompleted {
		t.Fatalf("admitted move state = %q, want COMPLETED", stored.State)
	}
	if _, ok := stored.Metadata[models.MetaKeyQueuedMoveExitPending]; ok {
		t.Fatalf("admitted move retained queued lifecycle marker: %#v", stored.Metadata)
	}
}

func TestUpdateTaskWithWorkflowStepAdmission_ConcurrentLastSlot(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	mover, ok := any(repo).(workflowStepMoveAdmissionRepository)
	if !ok {
		t.Fatal("task repository does not implement atomic workflow-step move admission")
	}
	ctx := context.Background()
	const (
		targetStepID = "concurrent-move-target"
		workerCount  = 8
	)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "concurrent-move-occupant", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: targetStepID, Title: "Occupant", State: v1.TaskStateCreated,
	}); err != nil {
		t.Fatalf("seed occupant: %v", err)
	}
	candidates := make([]*models.Task, workerCount)
	for i := range candidates {
		candidates[i] = &models.Task{
			ID: fmt.Sprintf("concurrent-move-%d", i), WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
			WorkflowStepID: "concurrent-move-source", Title: "Candidate", State: v1.TaskStateCreated,
			WIPAdmitted: true,
		}
		if err := repo.CreateTask(ctx, candidates[i]); err != nil {
			t.Fatalf("seed candidate %d: %v", i, err)
		}
	}

	start := make(chan struct{})
	results := make(chan struct {
		admitted bool
		err      error
	}, workerCount)
	var wg sync.WaitGroup
	for _, candidate := range candidates {
		wg.Add(1)
		go func(task *models.Task) {
			defer wg.Done()
			<-start
			admitted, err := mover.UpdateTaskWithWorkflowStepAdmission(ctx, task, targetStepID, 2)
			results <- struct {
				admitted bool
				err      error
			}{admitted: admitted, err: err}
		}(candidate)
	}
	close(start)
	wg.Wait()
	close(results)

	admitted, queued := 0, 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent move: %v", result.err)
		}
		if result.admitted {
			admitted++
		} else {
			queued++
		}
	}
	if admitted != 1 || queued != workerCount-1 {
		t.Fatalf("admitted=%d queued=%d, want admitted=1 queued=%d", admitted, queued, workerCount-1)
	}
	occupants, err := repo.CountAdmittedTasksByWorkflowStep(ctx, targetStepID)
	if err != nil {
		t.Fatalf("count admitted occupants: %v", err)
	}
	if occupants != 2 {
		t.Fatalf("admitted occupants=%d, want 2", occupants)
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

func TestPromoteQueuedTaskIfWorkflowStepHasCapacity_SameStepConcurrentClaim(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	promoter, ok := any(repo).(queuedTaskPromoter)
	if !ok {
		t.Fatal("task repository does not implement atomic queued-task promotion")
	}
	ctx := context.Background()
	queued := &models.Task{
		ID: "same-step-queued", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "target", Title: "Same-step queued", State: v1.TaskStateCreated,
		WIPAdmitted: false, QueuedForStepID: "target",
	}
	if err := repo.CreateTask(ctx, queued); err != nil {
		t.Fatalf("create same-step queued task: %v", err)
	}

	// Both reconcilers select the same row before either attempts its atomic
	// claim. The database predicate, rather than caller timing, must decide the
	// winner.
	start := make(chan struct{})
	results := make(chan bool, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		candidate := *queued
		candidate.Metadata = map[string]interface{}{
			models.MetaKeyQueuePromotionPending: true,
		}
		candidate.WIPAdmitted = true
		candidate.QueuedForStepID = ""
		go func(task *models.Task) {
			<-start
			claimed, err := promoter.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, task, "target", "target", 1)
			results <- claimed
			errs <- err
		}(&candidate)
	}
	close(start)

	claimedCount := 0
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent promotion %d: %v", i, err)
		}
		if <-results {
			claimedCount++
		}
	}
	if claimedCount != 1 {
		t.Fatalf("concurrent same-step claims = %d, want exactly 1", claimedCount)
	}

	got, err := repo.GetTask(ctx, queued.ID)
	if err != nil {
		t.Fatalf("reload same-step promoted task: %v", err)
	}
	if got.WorkflowStepID != "target" || got.QueuedForStepID != "" || !got.WIPAdmitted {
		t.Fatalf("same-step promoted task: step=%q queue=%q admitted=%t", got.WorkflowStepID, got.QueuedForStepID, got.WIPAdmitted)
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
