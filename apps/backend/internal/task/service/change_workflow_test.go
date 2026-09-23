package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
)

type workflowChangeExecutorValidator struct {
	executorID string
	profiles   []string
}

func (v *workflowChangeExecutorValidator) ValidateAgentProfileForExecutor(
	_ context.Context,
	profile *settingsmodels.AgentProfile,
	executor *models.Executor,
	_ *models.ExecutorProfile,
) error {
	v.profiles = append(v.profiles, profile.ID)
	if executor == nil || executor.ID != v.executorID {
		return errors.New("unexpected executor")
	}
	return nil
}

type workflowChangePreflightRecorder struct {
	candidate *models.Task
}

func (*workflowChangePreflightRecorder) PreflightWorkflowStepMove(
	context.Context,
	string,
	*models.TaskSession,
	*wfmodels.WorkflowStep,
) error {
	return nil
}

func (r *workflowChangePreflightRecorder) PreflightWorkflowStepChange(
	_ context.Context,
	candidate *models.Task,
	_ *models.TaskSession,
	_ *wfmodels.WorkflowStep,
) error {
	copy := *candidate
	r.candidate = &copy
	return nil
}

type workflowChangeFixture struct {
	svc       *Service
	repo      *sqliterepo.Repository
	task      *models.Task
	request   *models.WorkflowChangeRequest
	validator *workflowChangeExecutorValidator
}

func newWorkflowChangeFixture(t *testing.T) workflowChangeFixture {
	t.Helper()
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	workspaceDefaultExecutor := "executor-workspace-default"
	for _, executorID := range []string{workspaceDefaultExecutor, "executor-task"} {
		if err := repo.CreateExecutor(ctx, &models.Executor{
			ID: executorID, Name: executorID, Type: models.ExecutorTypeLocal,
			Status: models.ExecutorStatusActive, Resumable: true,
		}); err != nil {
			t.Fatalf("CreateExecutor(%s): %v", executorID, err)
		}
	}
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "workspace-change", Name: "Change workflow",
		DefaultExecutorID: &workspaceDefaultExecutor,
	}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	for _, workflow := range []*models.Workflow{
		{ID: "workflow-source", WorkspaceID: "workspace-change", Name: "Source"},
		{ID: "workflow-target", WorkspaceID: "workspace-change", Name: "Target"},
	} {
		if err := repo.CreateWorkflow(ctx, workflow); err != nil {
			t.Fatalf("CreateWorkflow(%s): %v", workflow.ID, err)
		}
	}
	wfDB := sqlx.NewDb(repo.DB(), "sqlite3")
	wfRepo, err := repository.NewWithDB(wfDB, wfDB, nil)
	if err != nil {
		t.Fatalf("New workflow repository: %v", err)
	}
	steps := []*wfmodels.WorkflowStep{
		{ID: "source-step", WorkflowID: "workflow-source", Name: "Review", Position: 0, AgentProfileID: "old-source-profile"},
		{ID: "target-analysis", WorkflowID: "workflow-target", Name: "Analysis", Position: 0, AgentProfileID: "source-profile"},
		{ID: "target-implement", WorkflowID: "workflow-target", Name: "Implement", Position: 1, AgentProfileID: "source-profile"},
		{ID: "target-review", WorkflowID: "workflow-target", Name: "Review", Position: 2, AgentProfileID: "source-profile", SessionTarget: &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetInitial}},
	}
	for _, step := range steps {
		if err := wfRepo.CreateStep(ctx, step); err != nil {
			t.Fatalf("CreateStep(%s): %v", step.ID, err)
		}
	}
	stepMap := make(map[string]*wfmodels.WorkflowStep, len(steps))
	for _, step := range steps {
		stepMap[step.ID] = step
	}
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: stepMap})
	oldOverrides, err := models.NewWorkflowAgentOverrides("workflow-source", []models.WorkflowAgentOverrideBinding{{
		StepID: "source-step", SourceProfileID: "old-source-profile", ReplacementProfileID: "old-replacement-profile",
	}})
	if err != nil {
		t.Fatalf("NewWorkflowAgentOverrides: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-workflow-change", WorkspaceID: "workspace-change",
		WorkflowID: "workflow-source", WorkflowStepID: "source-step",
		Title: "Preserved title", Description: "Preserved description",
		State: "TODO", Priority: "medium", Metadata: map[string]interface{}{"sentinel": "keep"},
		WorkflowAgentOverrides: oldOverrides,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-workflow-change", TaskID: "task-workflow-change",
		ExecutorID: "executor-task", State: models.TaskSessionStateWaitingForInput,
		StartedAt: now, UpdatedAt: now, IsPrimary: true,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	task, err := repo.GetTask(ctx, "task-workflow-change")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	validator := &workflowChangeExecutorValidator{executorID: "executor-task"}
	svc.agentProfiles = workflowAgentOverrideProfilesStub{profiles: map[string]*settingsmodels.AgentProfile{
		"source-profile":          {ID: "source-profile", Enabled: true, WorkspaceID: "workspace-change"},
		"replacement-profile":     {ID: "replacement-profile", Enabled: true, WorkspaceID: "workspace-change"},
		"old-source-profile":      {ID: "old-source-profile", Enabled: true, WorkspaceID: "workspace-change"},
		"old-replacement-profile": {ID: "old-replacement-profile", Enabled: true, WorkspaceID: "workspace-change"},
	}}
	svc.agentProfileExecutorValidator = validator
	return workflowChangeFixture{
		svc: svc, repo: repo, task: task, validator: validator,
		request: &models.WorkflowChangeRequest{
			ExpectedWorkflowID: task.WorkflowID,
			ExpectedStepID:     task.WorkflowStepID,
			ExpectedUpdatedAt:  task.UpdatedAt,
			AgentOverrides:     map[string]string{"source-profile": "replacement-profile"},
		},
	}
}

func TestMoveTaskWithWorkflowChangeReplacesOverridesAndPreservesTaskContext(t *testing.T) {
	fixture := newWorkflowChangeFixture(t)
	preflight := &workflowChangePreflightRecorder{}
	fixture.svc.SetWorkflowMovePreflight(preflight)

	result, err := fixture.svc.MoveTaskWithOptions(
		context.Background(), fixture.task.ID, "workflow-target", "target-analysis", 0,
		MoveTaskOptions{AllowActivePrimarySession: true, WorkflowChange: fixture.request},
	)
	if err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}
	if result.Task.ID != fixture.task.ID || result.Task.WorkflowID != "workflow-target" || result.Task.WorkflowStepID != "target-analysis" {
		t.Fatalf("moved task identity = %+v", result.Task)
	}
	if result.Task.Title != "Preserved title" || result.Task.Description != "Preserved description" || result.Task.Metadata["sentinel"] != "keep" {
		t.Fatalf("task context changed: %+v", result.Task)
	}
	if result.Task.WorkflowAgentOverrides == nil || result.Task.WorkflowAgentOverrides.WorkflowID != "workflow-target" {
		t.Fatalf("destination overrides = %+v", result.Task.WorkflowAgentOverrides)
	}
	for _, stepID := range []string{"target-analysis", "target-implement"} {
		if replacement, ok := result.Task.WorkflowAgentOverrides.ReplacementFor("workflow-target", stepID); !ok || replacement != "replacement-profile" {
			t.Fatalf("replacement for %s = %q, %v", stepID, replacement, ok)
		}
	}
	if _, ok := result.Task.WorkflowAgentOverrides.ReplacementFor("workflow-target", "target-review"); ok {
		t.Fatal("explicit initial-session target received a fixed-profile replacement")
	}
	if _, ok := result.Task.WorkflowAgentOverrides.ReplacementFor("workflow-source", "source-step"); ok {
		t.Fatal("source workflow override remained active after explicit change")
	}
	if preflight.candidate == nil || preflight.candidate.WorkflowID != "workflow-target" ||
		preflight.candidate.WorkflowAgentOverrides == nil ||
		preflight.candidate.WorkflowAgentOverrides.WorkflowID != "workflow-target" {
		t.Fatalf("preflight did not receive candidate destination state: %+v", preflight.candidate)
	}
	if len(fixture.validator.profiles) != 1 || fixture.validator.profiles[0] != "replacement-profile" {
		t.Fatalf("validated profiles = %v, want replacement on task executor", fixture.validator.profiles)
	}
}

func TestValidateWorkflowChangeValidatesDefaultsAndRejectsUnavailableAgent(t *testing.T) {
	fixture := newWorkflowChangeFixture(t)
	fixture.request.AgentOverrides = map[string]string{}
	overrides, err := fixture.svc.ValidateWorkflowChange(context.Background(), fixture.task.ID, "workflow-target", "target-analysis", fixture.request)
	if err != nil {
		t.Fatalf("ValidateWorkflowChange with defaults: %v", err)
	}
	if overrides != nil {
		t.Fatalf("default selections produced overrides: %+v", overrides)
	}
	if len(fixture.validator.profiles) != 1 || fixture.validator.profiles[0] != "source-profile" {
		t.Fatalf("default profile validation = %v, want destination workflow default", fixture.validator.profiles)
	}

	fixture.request.AgentOverrides = map[string]string{"source-profile": "disabled-profile"}
	_, err = fixture.svc.ValidateWorkflowChange(context.Background(), fixture.task.ID, "workflow-target", "target-analysis", fixture.request)
	var validationErr *WorkflowChangeValidationError
	if !errors.As(err, &validationErr) || validationErr.Code != WorkflowChangeErrorAgentInvalid || validationErr.SourceProfileID != "source-profile" {
		t.Fatalf("unavailable replacement error = %#v, want row validation for source-profile", err)
	}
}

func TestValidateWorkflowChangeRejectsStaleTaskSource(t *testing.T) {
	tests := map[string]func(*models.WorkflowChangeRequest){
		"workflow": func(request *models.WorkflowChangeRequest) { request.ExpectedWorkflowID = "another-workflow" },
		"step":     func(request *models.WorkflowChangeRequest) { request.ExpectedStepID = "another-step" },
		"version": func(request *models.WorkflowChangeRequest) {
			request.ExpectedUpdatedAt = request.ExpectedUpdatedAt.Add(-time.Second)
		},
	}
	for name, alter := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newWorkflowChangeFixture(t)
			alter(fixture.request)
			_, err := fixture.svc.ValidateWorkflowChange(
				context.Background(), fixture.task.ID, "workflow-target", "target-analysis", fixture.request,
			)
			if !errors.Is(err, ErrWorkflowChangeConflict) {
				t.Fatalf("ValidateWorkflowChange error = %v, want source conflict", err)
			}
			stored, loadErr := fixture.repo.GetTask(context.Background(), fixture.task.ID)
			if loadErr != nil {
				t.Fatalf("GetTask after source conflict: %v", loadErr)
			}
			if stored.WorkflowID != fixture.task.WorkflowID || stored.WorkflowStepID != fixture.task.WorkflowStepID {
				t.Fatalf("stale form changed task assignment: %s/%s", stored.WorkflowID, stored.WorkflowStepID)
			}
		})
	}
}

func TestMoveTaskWithWorkflowChangePreservesSessionGuards(t *testing.T) {
	t.Run("running primary remains allowed", func(t *testing.T) {
		fixture := newWorkflowChangeFixture(t)
		repo := fixture.repo
		if err := repo.UpdateTaskSessionState(context.Background(), "session-workflow-change", models.TaskSessionStateRunning, ""); err != nil {
			t.Fatalf("UpdateTaskSessionState: %v", err)
		}
		if _, err := fixture.svc.MoveTaskWithOptions(
			context.Background(), fixture.task.ID, "workflow-target", "target-analysis", 0,
			MoveTaskOptions{AllowActivePrimarySession: true, WorkflowChange: fixture.request},
		); err != nil {
			t.Fatalf("MoveTaskWithOptions with running primary: %v", err)
		}
	})

	t.Run("running sibling remains blocked", func(t *testing.T) {
		fixture := newWorkflowChangeFixture(t)
		repo := fixture.repo
		now := time.Now().UTC()
		if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
			ID: "session-workflow-change-sibling", TaskID: fixture.task.ID,
			ExecutorID: "executor-task", State: models.TaskSessionStateRunning,
			StartedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("CreateTaskSession sibling: %v", err)
		}
		_, err := fixture.svc.MoveTaskWithOptions(
			context.Background(), fixture.task.ID, "workflow-target", "target-analysis", 0,
			MoveTaskOptions{AllowActivePrimarySession: true, WorkflowChange: fixture.request},
		)
		if err == nil || !strings.Contains(err.Error(), "active session") {
			t.Fatalf("MoveTaskWithOptions error = %v, want active sibling conflict", err)
		}
		stored, loadErr := repo.GetTask(context.Background(), fixture.task.ID)
		if loadErr != nil {
			t.Fatalf("GetTask after blocked move: %v", loadErr)
		}
		if stored.WorkflowID != fixture.task.WorkflowID || stored.WorkflowStepID != fixture.task.WorkflowStepID {
			t.Fatalf("blocked move changed assignment: %s/%s", stored.WorkflowID, stored.WorkflowStepID)
		}
	})
}
