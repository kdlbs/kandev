package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type failingWorkflowOverrideTaskReadRepo struct {
	repoStore
	err error
}

func (r *failingWorkflowOverrideTaskReadRepo) GetTask(context.Context, string) (*models.Task, error) {
	return nil, r.err
}

func TestResolveStepAgentProfileForTaskKeepsThreeTasksIsolatedAfterWorkflowEdit(t *testing.T) {
	makeOverrides := func(replacement string) *models.WorkflowAgentOverrides {
		overrides, err := models.NewWorkflowAgentOverrides("workflow-1", []models.WorkflowAgentOverrideBinding{
			{StepID: "implement", SourceProfileID: "profile-a", ReplacementProfileID: replacement},
		})
		if err != nil {
			t.Fatalf("create overrides for %s: %v", replacement, err)
		}
		return overrides
	}

	service := &Service{}
	implement := &wfmodels.WorkflowStep{
		ID: "implement", WorkflowID: "workflow-1", AgentProfileID: "profile-a",
	}
	tasks := []*models.Task{
		{ID: "default", WorkflowID: "workflow-1"},
		{ID: "replacement-b", WorkflowID: "workflow-1", WorkflowAgentOverrides: makeOverrides("profile-b")},
		{ID: "replacement-c", WorkflowID: "workflow-1", WorkflowAgentOverrides: makeOverrides("profile-c")},
	}
	wantBeforeEdit := []string{"profile-a", "profile-b", "profile-c"}
	for i, task := range tasks {
		if got := service.resolveStepAgentProfileForTask(context.Background(), task, implement); got != wantBeforeEdit[i] {
			t.Fatalf("task %s profile before edit = %q, want %q", task.ID, got, wantBeforeEdit[i])
		}
	}

	implement.AgentProfileID = "profile-c"
	wantAfterEdit := []string{"profile-c", "profile-b", "profile-c"}
	for i, task := range tasks {
		if got := service.resolveStepAgentProfileForTask(context.Background(), task, implement); got != wantAfterEdit[i] {
			t.Fatalf("task %s profile after edit = %q, want %q", task.ID, got, wantAfterEdit[i])
		}
	}

	newStep := &wfmodels.WorkflowStep{
		ID: "later", WorkflowID: "workflow-1", AgentProfileID: "profile-c",
	}
	if got := service.resolveStepAgentProfileForTask(context.Background(), tasks[1], newStep); got != "profile-c" {
		t.Fatalf("new step inherited old replacement = %q, want profile-c", got)
	}
}

func TestResolveStepAgentProfileForTaskUsesTaskBindingBeforeWorkflowProfile(t *testing.T) {
	overrides, err := models.NewWorkflowAgentOverrides("workflow-1", []models.WorkflowAgentOverrideBinding{
		{StepID: "implement", SourceProfileID: "profile-luna", ReplacementProfileID: "profile-terra"},
	})
	if err != nil {
		t.Fatalf("create overrides: %v", err)
	}
	svc := &Service{}
	task := &models.Task{WorkflowID: "workflow-1", WorkflowAgentOverrides: overrides}
	step := &wfmodels.WorkflowStep{ID: "implement", WorkflowID: "workflow-1", AgentProfileID: "profile-luna"}

	if got := svc.resolveStepAgentProfileForTask(context.Background(), task, step); got != "profile-terra" {
		t.Fatalf("resolved profile = %q, want profile-terra", got)
	}
}

func TestResolveStepAgentProfileForTaskDoesNotApplyForeignWorkflowBinding(t *testing.T) {
	overrides, err := models.NewWorkflowAgentOverrides("workflow-1", []models.WorkflowAgentOverrideBinding{
		{StepID: "implement", SourceProfileID: "profile-luna", ReplacementProfileID: "profile-terra"},
	})
	if err != nil {
		t.Fatalf("create overrides: %v", err)
	}
	svc := &Service{}
	task := &models.Task{WorkflowID: "workflow-2", WorkflowAgentOverrides: overrides}
	step := &wfmodels.WorkflowStep{ID: "implement", WorkflowID: "workflow-2", AgentProfileID: "profile-sol"}

	if got := svc.resolveStepAgentProfileForTask(context.Background(), task, step); got != "profile-sol" {
		t.Fatalf("resolved foreign profile = %q, want profile-sol", got)
	}
}

func TestWorkflowOverrideTaskReadFailureStopsRoutingPreflightAndStart(t *testing.T) {
	ctx := context.Background()
	readErr := errors.New("task override read failed")
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-override-read", "session-override-read", "source")
	stepGetter := newMockStepGetter()
	target := &wfmodels.WorkflowStep{
		ID:             "target",
		WorkflowID:     "wf1",
		AgentProfileID: "replacement-profile",
	}
	stepGetter.steps[target.ID] = target
	svc := createTestService(repo, stepGetter, newMockTaskRepo())
	svc.repo = &failingWorkflowOverrideTaskReadRepo{repoStore: repo, err: readErr}
	current, err := repo.GetTaskSession(ctx, "session-override-read")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	source := &wfmodels.WorkflowStep{ID: "source", WorkflowID: "wf1", AgentProfileID: "source-profile"}

	if _, err := svc.resolveStepAgentProfileForTaskID(ctx, "task-override-read", target); !errors.Is(err, readErr) {
		t.Fatalf("routing error = %v, want task read error", err)
	}
	if _, switched, err := svc.prepareWorkflowStepSession(ctx, "task-override-read", current, target, source); !errors.Is(err, readErr) {
		t.Fatalf("prepare error = %v, switched = %t, want task read error", err, switched)
	} else if switched {
		t.Fatal("prepareWorkflowStepSession switched after the task override read failed")
	}
	if err := svc.preflightWorkflowStepCredentials(ctx, "task-override-read", current, target); !errors.Is(err, readErr) {
		t.Fatalf("preflight error = %v, want task read error", err)
	}
	if err := svc.StartSessionForWorkflowStep(ctx, "task-override-read", "session-override-read", target.ID); !errors.Is(err, readErr) {
		t.Fatalf("start error = %v, want task read error", err)
	}

	stored, err := repo.GetTaskSession(ctx, current.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if stored.State != current.State || stored.AgentProfileID != current.AgentProfileID {
		t.Fatalf("task read failure changed session: got state %q/profile %q, want state %q/profile %q", stored.State, stored.AgentProfileID, current.State, current.AgentProfileID)
	}
	messages, err := repo.ListMessages(ctx, current.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("task read failure recorded %d prompt messages", len(messages))
	}
}
