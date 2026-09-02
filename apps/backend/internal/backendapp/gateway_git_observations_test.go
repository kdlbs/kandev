package backendapp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/statussummary"
)

func TestTaskGitObservationConvertsSnapshotFields(t *testing.T) {
	session := &models.TaskSession{ID: "session-1"}
	snapshot := &models.GitSnapshot{
		SessionID: "session-1",
		Files:     map[string]interface{}{"one.go": true, "two.go": true},
		Ahead:     -2,
		Behind:    -3,
		Metadata: map[string]interface{}{
			"repository_name":  "org/repo",
			"branch_additions": json.Number("-7"),
			"branch_deletions": -4,
		},
	}

	observation, ok := taskGitObservation(session, snapshot)
	if !ok {
		t.Fatal("taskGitObservation returned ok=false")
	}
	if observation.Repository != "org/repo" {
		t.Fatalf("repository = %q, want org/repo", observation.Repository)
	}
	if observation.Summary.Additions != 0 || observation.Summary.Deletions != 0 {
		t.Fatalf("negative branch counts = %+v, want zero", observation.Summary)
	}
	if observation.Summary.Ahead != 0 || observation.Summary.Behind != 0 {
		t.Fatalf("negative ahead/behind = %+v, want zero", observation.Summary)
	}
	if observation.Summary.ChangedFiles != 2 {
		t.Fatalf("changed files = %d, want 2", observation.Summary.ChangedFiles)
	}

	fallback, ok := taskGitObservation(session, &models.GitSnapshot{})
	if !ok || fallback.Repository != session.ID {
		t.Fatalf("missing repository metadata fallback = %+v, ok=%v", fallback, ok)
	}
	if _, ok := taskGitObservation(session, nil); ok {
		t.Fatal("nil snapshot should be skipped")
	}
}

func TestTaskGitObservationUsesStableRootKeyWithoutRepositoryMetadata(t *testing.T) {
	observation, ok := taskGitObservation(nil, &models.GitSnapshot{
		TaskEnvironmentID: "environment-root",
	})
	if !ok {
		t.Fatal("taskGitObservation returned ok=false")
	}
	if observation.Repository != statussummary.RootRepositoryKey {
		t.Fatalf("root repository key = %q, want %q", observation.Repository, statussummary.RootRepositoryKey)
	}
}

func TestNonNegativeMetadataIntParsesAndNormalizesValues(t *testing.T) {
	metadata := map[string]interface{}{
		"number":   json.Number("7"),
		"negative": -3,
		"invalid":  "not-a-number",
	}
	for name, want := range map[string]int{"number": 7, "negative": 0, "invalid": 0, "missing": 0} {
		if got := nonNegativeMetadataInt(metadata, name); got != want {
			t.Errorf("nonNegativeMetadataInt(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestLoadTaskGitObservationsSkipsSessionsWithoutSnapshots(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	workflows, err := harness.taskSvc.ListWorkflows(ctx, workspaces[0].ID, true)
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	steps, err := harness.workflowSvc.ListStepsByWorkflow(ctx, workflows[0].ID)
	if err != nil {
		t.Fatalf("list workflow steps: %v", err)
	}
	taskResult, err := harness.taskSvc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:    workspaces[0].ID,
		WorkflowID:     workflows[0].ID,
		WorkflowStepID: steps[0].ID,
		Title:          "Git observation hydration",
	})
	task := taskResult.Task
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	const environmentID = "environment-git-observation"
	if err := harness.taskRepo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: task.ID, WorkspacePath: "/workspace/git-observation",
		Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create environment: %v", err)
	}
	for _, sessionID := range []string{"session-with-snapshot", "session-without-snapshot"} {
		if err := harness.taskRepo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: task.ID, TaskEnvironmentID: environmentID,
		}); err != nil {
			t.Fatalf("create session %s: %v", sessionID, err)
		}
	}
	if err := harness.taskRepo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID:          "snapshot-1",
		SessionID:   "session-with-snapshot",
		Files:       map[string]interface{}{"main.go": true},
		Metadata:    map[string]interface{}{"repository_name": "org/repo"},
		TriggeredBy: "agent_completed",
	}); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	observations, err := loadTaskGitObservations(ctx, harness.taskRepo, task.ID)
	if err != nil {
		t.Fatalf("load observations: %v", err)
	}
	if len(observations) != 1 || observations[0].Repository != "org/repo" {
		t.Fatalf("observations = %+v, want one org/repo observation", observations)
	}
}

func TestLoadTaskGitObservationsUsesNewestSnapshotOncePerSharedEnvironment(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	const (
		taskID        = "task-shared-git-observation"
		environmentID = "environment-shared-git-observation"
	)
	if err := harness.taskRepo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := harness.taskRepo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, WorkspacePath: "/workspace/shared-git-observation",
		Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create environment: %v", err)
	}
	oldAt := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	newAt := oldAt.Add(time.Minute)
	for _, session := range []*models.TaskSession{
		{ID: "session-shared-old", TaskID: taskID, TaskEnvironmentID: environmentID, StartedAt: oldAt, UpdatedAt: oldAt},
		{ID: "session-shared-new", TaskID: taskID, TaskEnvironmentID: environmentID, StartedAt: newAt, UpdatedAt: newAt},
	} {
		if err := harness.taskRepo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("create session %s: %v", session.ID, err)
		}
	}
	for _, snapshot := range []*models.GitSnapshot{
		{
			ID: "snapshot-shared-old", SessionID: "session-shared-old", TaskEnvironmentID: environmentID,
			Files: map[string]interface{}{"old.go": true}, CreatedAt: oldAt,
			Metadata: map[string]interface{}{"repository_name": "org/repo", "branch_additions": 1},
		},
		{
			ID: "snapshot-shared-new", SessionID: "session-shared-new", TaskEnvironmentID: environmentID,
			Files: map[string]interface{}{"new.go": true}, CreatedAt: newAt,
			Metadata: map[string]interface{}{"repository_name": "org/repo", "branch_additions": 9},
		},
	} {
		if err := harness.taskRepo.CreateGitSnapshot(ctx, snapshot); err != nil {
			t.Fatalf("create snapshot %s: %v", snapshot.ID, err)
		}
	}

	observations, err := loadTaskGitObservations(ctx, harness.taskRepo, taskID)
	if err != nil {
		t.Fatalf("load observations: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("observations = %+v, want one shared-environment observation", observations)
	}
	if observations[0].Repository != "org/repo" || observations[0].Summary.Additions != 9 {
		t.Fatalf("observation = %+v, want newest snapshot from shared environment", observations[0])
	}
}
