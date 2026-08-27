package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestTaskCountsPullCandidatesQueueAndWorkflowPlacement(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-selection")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-selection", WorkspaceID: "workspace-selection", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.April, 5, 6, 7, 8, 0, time.UTC)
	for _, stepID := range []string{"step-feeder", "step-destination"} {
		if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO workflow_steps
			(id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`), stepID, "workflow-selection", stepID, 0, now, now); err != nil {
			t.Fatalf("insert step %s: %v", stepID, err)
		}
	}
	for _, task := range []*models.Task{
		{ID: "task-low", WorkspaceID: "workspace-selection", WorkflowID: "workflow-selection", WorkflowStepID: "step-feeder", Title: "Low", Priority: "low", Position: 1},
		{ID: "task-high", WorkspaceID: "workspace-selection", WorkflowID: "workflow-selection", WorkflowStepID: "step-feeder", Title: "High", Priority: "high", Position: 1},
		{ID: "task-first-position", WorkspaceID: "workspace-selection", WorkflowID: "workflow-selection", WorkflowStepID: "step-feeder", Title: "First", Priority: "none", Position: 0},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.ID, err)
		}
	}
	if count, err := repo.CountTasksByWorkflow(ctx, "workflow-selection"); err != nil || count != 3 {
		t.Fatalf("CountTasksByWorkflow = %d, %v", count, err)
	}
	if count, err := repo.CountTasksByWorkflowStep(ctx, "step-feeder"); err != nil || count != 3 {
		t.Fatalf("CountTasksByWorkflowStep = %d, %v", count, err)
	}
	if count, err := repo.CountTasksByWorkflowStepExcludingTask(ctx, "step-feeder", "task-low"); err != nil || count != 2 {
		t.Fatalf("CountTasksByWorkflowStepExcludingTask = %d, %v", count, err)
	}
	if count, err := repo.CountAdmittedTasksByWorkflowStep(ctx, "step-feeder"); err != nil || count != 3 {
		t.Fatalf("CountAdmittedTasksByWorkflowStep = %d, %v", count, err)
	}
	candidate, err := repo.NextPullCandidate(ctx, "step-feeder", "")
	if err != nil || candidate.ID != "task-first-position" {
		t.Fatalf("NextPullCandidate = %+v, %v", candidate, err)
	}
	candidate, err = repo.NextPullCandidateExcluding(ctx, "step-feeder", []string{"", "task-first-position", "task-high"})
	if err != nil || candidate.ID != "task-low" {
		t.Fatalf("NextPullCandidateExcluding = %+v, %v", candidate, err)
	}

	queuedAt := now.Add(time.Minute)
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE tasks SET queued_for_step_id = ?, queued_at = ?, wip_admitted = 0 WHERE id = ?`), "step-destination", queuedAt, "task-first-position"); err != nil {
		t.Fatal(err)
	}
	queued, err := repo.ListQueuedTasks(ctx)
	if err != nil || strings.Join(selectionTaskIDs(queued), ",") != "task-first-position" {
		t.Fatalf("ListQueuedTasks = %v, %v", selectionTaskIDs(queued), err)
	}
	if count, err := repo.CountAdmittedTasksByWorkflowStep(ctx, "step-feeder"); err != nil || count != 2 {
		t.Fatalf("admitted count after queue = %d, %v", count, err)
	}
	candidate, err = repo.NextQueuedTaskForStepExcluding(ctx, "step-feeder", "step-destination", nil)
	if err != nil || candidate.ID != "task-first-position" {
		t.Fatalf("NextQueuedTaskForStepExcluding = %+v, %v", candidate, err)
	}

	if err := repo.AddTaskToWorkflow(ctx, "task-first-position", "workflow-selection", "step-destination", 4); err != nil {
		t.Fatalf("AddTaskToWorkflow: %v", err)
	}
	placed, err := repo.GetTask(ctx, "task-first-position")
	if err != nil || placed.WorkflowStepID != "step-destination" || placed.Position != 4 {
		t.Fatalf("placed task = %+v, %v", placed, err)
	}
	if err := repo.RemoveTaskFromWorkflow(ctx, "task-first-position", "wrong-workflow"); err != nil {
		t.Fatalf("RemoveTaskFromWorkflow(wrong): %v", err)
	}
	stillPlaced, err := repo.GetTask(ctx, "task-first-position")
	if err != nil || stillPlaced.WorkflowID != "workflow-selection" {
		t.Fatalf("wrong-workflow removal mutated task: %+v, %v", stillPlaced, err)
	}
	if err := repo.RemoveTaskFromWorkflow(ctx, "task-first-position", "workflow-selection"); err != nil {
		t.Fatalf("RemoveTaskFromWorkflow: %v", err)
	}
	detached, err := repo.GetTask(ctx, "task-first-position")
	if err != nil || detached.WorkflowID != "" || detached.WorkflowStepID != "" || detached.Position != 0 {
		t.Fatalf("detached task = %+v, %v", detached, err)
	}
}

func selectionTaskIDs(tasks []*models.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func TestTaskAdmissionCapacityOverflowFeederAndPromotion(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-admission")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-admission", WorkspaceID: "workspace-admission", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	insertSelectionStep(t, repo, "workflow-admission", "step-target", 0)
	insertSelectionStep(t, repo, "workflow-admission", "step-feeder-admission", 1)

	first := &models.Task{ID: "admission-first", WorkspaceID: "workspace-admission", WorkflowID: "workflow-admission", WorkflowStepID: "step-target", Title: "First"}
	if err := repo.CreateTaskIfWorkflowStepHasCapacity(ctx, first, "step-target", 1); err != nil {
		t.Fatalf("CreateTaskIfWorkflowStepHasCapacity(first): %v", err)
	}
	blocked := &models.Task{ID: "admission-blocked", WorkspaceID: "workspace-admission", WorkflowID: "workflow-admission", WorkflowStepID: "step-target", Title: "Blocked"}
	if err := repo.CreateTaskIfWorkflowStepHasCapacity(ctx, blocked, "step-target", 1); err == nil {
		t.Fatal("capacity-limited create admitted a second occupant")
	} else {
		var limitErr *wfmodels.WIPLimitError
		if !errors.As(err, &limitErr) {
			t.Fatalf("capacity error = %T %v", err, err)
		}
	}

	overflow := &models.Task{ID: "admission-overflow", WorkspaceID: "workspace-admission", WorkflowID: "workflow-admission", Title: "Overflow", Metadata: map[string]any{models.MetaKeyDeferredLaunch: true}}
	if err := repo.CreateTaskWithWorkflowStepAdmission(ctx, overflow, "step-target", 1, "", 0); err != nil {
		t.Fatalf("CreateTaskWithWorkflowStepAdmission(overflow): %v", err)
	}
	if overflow.WorkflowStepID != "step-target" || overflow.WIPAdmitted || overflow.QueuedForStepID != "step-target" || overflow.QueuedAt == nil {
		t.Fatalf("destination overflow placement = %+v", overflow)
	}

	feeder := &models.Task{ID: "admission-feeder", WorkspaceID: "workspace-admission", WorkflowID: "workflow-admission", Title: "Feeder"}
	if err := repo.CreateTaskWithWorkflowStepAdmission(ctx, feeder, "step-target", 1, "step-feeder-admission", 2); err != nil {
		t.Fatalf("CreateTaskWithWorkflowStepAdmission(feeder): %v", err)
	}
	if feeder.WorkflowStepID != "step-feeder-admission" || !feeder.WIPAdmitted || feeder.QueuedForStepID != "step-target" {
		t.Fatalf("feeder placement = %+v", feeder)
	}
	feederBlocked := &models.Task{ID: "admission-feeder-blocked", WorkspaceID: "workspace-admission", WorkflowID: "workflow-admission", Title: "Feeder blocked"}
	if err := repo.CreateTaskWithWorkflowStepAdmission(ctx, feederBlocked, "step-target", 1, "step-feeder-admission", 1); err == nil {
		t.Fatal("full feeder accepted overflow")
	}
	sameStep := &models.Task{ID: "admission-same-step", WorkspaceID: "workspace-admission", WorkflowID: "workflow-admission", Title: "Same"}
	if err := repo.CreateTaskWithWorkflowStepAdmission(ctx, sameStep, "step-target", 1, "step-target", 1); err == nil {
		t.Fatal("same target and feeder accepted overflow")
	}

	first.Title = "Updated in target"
	if err := repo.UpdateTaskIfWorkflowStepHasCapacity(ctx, first, "step-target", first.ID, 1); err != nil {
		t.Fatalf("UpdateTaskIfWorkflowStepHasCapacity: %v", err)
	}
	feeder.WorkflowStepID = "step-target"
	feeder.WIPAdmitted = true
	feeder.QueuedForStepID = ""
	feeder.QueuedAt = nil
	promoted, err := repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, feeder, "step-feeder-admission", "step-target", 1)
	if err != nil || promoted {
		t.Fatalf("promotion into full target = %v, %v", promoted, err)
	}
	if err := repo.DeleteTask(ctx, "admission-first"); err != nil {
		t.Fatal(err)
	}
	promoted, err = repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, feeder, "step-feeder-admission", "step-target", 1)
	if err != nil || !promoted {
		t.Fatalf("promotion after capacity frees = %v, %v", promoted, err)
	}
}

func TestTaskMetadataWatcherFiltersDetachAndQuickChatExpiry(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-task-extra")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-task-extra", WorkspaceID: "workspace-task-extra", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	insertSelectionStep(t, repo, "workflow-task-extra", "step-task-extra", 0)
	parent := &models.Task{ID: "task-extra-parent", WorkspaceID: "workspace-task-extra", WorkflowID: "workflow-task-extra", WorkflowStepID: "step-task-extra", Title: "Parent", ProjectID: "project-one"}
	child := &models.Task{ID: "task-extra-child", WorkspaceID: "workspace-task-extra", WorkflowID: "workflow-task-extra", WorkflowStepID: "step-task-extra", Title: "Child", ParentID: parent.ID, ProjectID: "project-one", AssigneeAgentProfileID: "agent-one", Metadata: map[string]any{"workspace": map[string]any{"mode": "inherit_parent"}, "watch_id": "watch-one"}}
	for _, task := range []*models.Task{parent, child} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SetTaskMetadataKey(ctx, child.ID, "extra", map[string]any{"ok": true}); err != nil {
		t.Fatalf("SetTaskMetadataKey: %v", err)
	}
	wrote, err := repo.SetTaskMetadataKeyIfNotArchived(ctx, child.ID, "guarded", "yes")
	if err != nil || !wrote {
		t.Fatalf("SetTaskMetadataKeyIfNotArchived = %v, %v", wrote, err)
	}
	removed, err := repo.RemoveTaskMetadataKey(ctx, child.ID, "extra")
	if err != nil || !removed {
		t.Fatalf("RemoveTaskMetadataKey = %v, %v", removed, err)
	}
	removed, err = repo.RemoveTaskMetadataKey(ctx, child.ID, "missing")
	if err != nil || removed {
		t.Fatalf("RemoveTaskMetadataKey(missing) = %v, %v", removed, err)
	}
	count, err := repo.CountOpenWatcherCreatedTasks(ctx, "watch_id", "watch-one")
	if err != nil || count != 1 {
		t.Fatalf("CountOpenWatcherCreatedTasks = %d, %v", count, err)
	}
	if count, err := repo.CountOpenWatcherCreatedTasks(ctx, "watch-id", "watch-one"); err == nil || count != 0 {
		t.Fatalf("invalid watcher key = %d, %v", count, err)
	}
	if count, err := repo.CountOpenWatcherCreatedTasks(ctx, "watch_id", ""); err != nil || count != 0 {
		t.Fatalf("empty watcher id = %d, %v", count, err)
	}
	projectTasks, err := repo.ListTasksByProject(ctx, "project-one")
	if err != nil || len(projectTasks) != 2 {
		t.Fatalf("ListTasksByProject = %v, %v", selectionTaskIDs(projectTasks), err)
	}
	assigned, err := repo.ListTasksByAssignee(ctx, "agent-one")
	if err != nil || len(assigned) != 1 || assigned[0].ID != child.ID {
		t.Fatalf("ListTasksByAssignee = %v, %v", selectionTaskIDs(assigned), err)
	}
	detached, err := repo.DetachTask(ctx, child.ID)
	if err != nil || !detached {
		t.Fatalf("DetachTask = %v, %v", detached, err)
	}
	detached, err = repo.DetachTask(ctx, child.ID)
	if err != nil || detached {
		t.Fatalf("second DetachTask = %v, %v", detached, err)
	}
	if _, err := repo.DetachTask(ctx, "missing"); err == nil {
		t.Fatal("DetachTask accepted missing task")
	}
	got, err := repo.GetTask(ctx, child.ID)
	if err != nil || got.ParentID != "" {
		t.Fatalf("detached persisted task = %+v, %v", got, err)
	}

	cutoff := time.Date(2026, time.June, 7, 8, 9, 10, 0, time.UTC)
	for _, task := range []*models.Task{
		{ID: "quick-expired", WorkspaceID: "workspace-task-extra", Title: "Expired", IsEphemeral: true},
		{ID: "quick-fresh", WorkspaceID: "workspace-task-extra", Title: "Fresh", IsEphemeral: true},
		{ID: "quick-config", WorkspaceID: "workspace-task-extra", Title: "Config", IsEphemeral: true, Metadata: map[string]any{"config_mode": true}},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.db.Exec(`UPDATE tasks SET updated_at = ? WHERE id IN ('quick-expired', 'quick-config')`, cutoff.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`UPDATE tasks SET updated_at = ? WHERE id = 'quick-fresh'`, cutoff.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	expired, err := repo.ListExpiredQuickChatTasks(ctx, cutoff)
	if err != nil || len(expired) != 1 || expired[0].ID != "quick-expired" {
		t.Fatalf("ListExpiredQuickChatTasks = %v, %v", selectionTaskIDs(expired), err)
	}
	deleted, err := repo.DeleteExpiredQuickChatTask(ctx, "quick-fresh", cutoff)
	if err != nil || deleted {
		t.Fatalf("DeleteExpiredQuickChatTask(fresh) = %v, %v", deleted, err)
	}
	deleted, err = repo.DeleteExpiredQuickChatTask(ctx, "quick-expired", cutoff)
	if err != nil || !deleted {
		t.Fatalf("DeleteExpiredQuickChatTask(expired) = %v, %v", deleted, err)
	}
}

func insertSelectionStep(t *testing.T, repo *Repository, workflowID, stepID string, position int) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO workflow_steps
		(id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`), stepID, workflowID, stepID, position, now, now); err != nil {
		t.Fatalf("insert step %s: %v", stepID, err)
	}
}

func TestTaskSQLProjectionHelpers(t *testing.T) {
	for _, alias := range []string{"", "custom"} {
		predicate := IsFromOfficePredicate(alias)
		if !strings.Contains(predicate, "project_id") || !strings.Contains(predicate, "office_workflow_id") {
			t.Fatalf("IsFromOfficePredicate(%q) = %q", alias, predicate)
		}
	}
	if predicate := excludeConfigModePredicate("sqlite3", "metadata"); !strings.Contains(predicate, "config_mode") {
		t.Fatalf("excludeConfigModePredicate = %q", predicate)
	}
}
