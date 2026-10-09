package service

import (
	"context"
	"errors"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type workflowAgentOverrideProfilesStub struct {
	profiles map[string]*settingsmodels.AgentProfile
}

func (s workflowAgentOverrideProfilesStub) GetAgentProfile(_ context.Context, id string) (*settingsmodels.AgentProfile, error) {
	return s.profiles[id], nil
}

func TestNormalizeWorkflowAgentOverridesExpandsDistinctSourcesToFixedSteps(t *testing.T) {
	steps := []*wfmodels.WorkflowStep{
		{ID: "analysis", WorkflowID: "workflow-1", AgentProfileID: "profile-a"},
		{ID: "implement", WorkflowID: "workflow-1", AgentProfileID: "profile-a"},
		{ID: "review", WorkflowID: "workflow-1", AgentProfileID: "profile-a", SessionTarget: &wfmodels.WorkflowSessionTarget{}},
		{ID: "pr", WorkflowID: "workflow-1", AgentProfileID: "profile-b"},
	}
	profiles := workflowAgentOverrideProfilesStub{profiles: map[string]*settingsmodels.AgentProfile{
		"profile-b": {ID: "profile-b", Enabled: true, WorkspaceID: "workspace-1"},
		"profile-c": {ID: "profile-c", Enabled: true, WorkspaceID: "workspace-1"},
	}}

	overrides, err := normalizeWorkflowAgentOverrides(context.Background(), "workspace-1", "workflow-1", map[string]string{
		"profile-a": "profile-b",
		"profile-b": "profile-c",
	}, steps, profiles)
	if err != nil {
		t.Fatalf("normalizeWorkflowAgentOverrides: %v", err)
	}
	if len(overrides.Steps) != 3 {
		t.Fatalf("expanded bindings = %d, want 3", len(overrides.Steps))
	}
	if got, ok := overrides.ReplacementFor("workflow-1", "analysis"); !ok || got != "profile-b" {
		t.Fatalf("analysis replacement = %q, %v", got, ok)
	}
	if got, ok := overrides.ReplacementFor("workflow-1", "implement"); !ok || got != "profile-b" {
		t.Fatalf("implement replacement = %q, %v", got, ok)
	}
	if got, ok := overrides.ReplacementFor("workflow-1", "pr"); !ok || got != "profile-c" {
		t.Fatalf("pr replacement = %q, %v", got, ok)
	}
}

func TestNormalizeWorkflowAgentOverridesRejectsUnknownOrUnavailableChoices(t *testing.T) {
	steps := []*wfmodels.WorkflowStep{{ID: "implement", WorkflowID: "workflow-1", AgentProfileID: "profile-a"}}
	profiles := workflowAgentOverrideProfilesStub{profiles: map[string]*settingsmodels.AgentProfile{
		"profile-disabled": {ID: "profile-disabled", Enabled: false, WorkspaceID: "workspace-1"},
	}}
	for name, requested := range map[string]map[string]string{
		"unknown source":    {"profile-missing": "profile-disabled"},
		"disabled choice":   {"profile-a": "profile-disabled"},
		"empty source":      {"": "profile-disabled"},
		"empty replacement": {"profile-a": ""},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeWorkflowAgentOverrides(context.Background(), "workspace-1", "workflow-1", requested, steps, profiles); err == nil {
				t.Fatal("normalizeWorkflowAgentOverrides succeeded")
			}
		})
	}
}

type rejectingWorkflowAgentExecutorValidator struct {
	err   error
	calls []string
}

func (v *rejectingWorkflowAgentExecutorValidator) ValidateAgentProfileForExecutor(
	_ context.Context,
	profile *settingsmodels.AgentProfile,
	_ *models.Executor,
	_ *models.ExecutorProfile,
) error {
	v.calls = append(v.calls, profile.ID)
	return v.err
}

func TestCreateTaskRejectsIncompatibleWorkflowOverrideBeforePersistence(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	workspaceID := "workspace-override-compat"
	workflowID := "workflow-override-compat"
	executorID := "executor-override-compat"
	defaultExecutorID := executorID
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID:                workspaceID,
		Name:              "Override compatibility workspace",
		DefaultExecutorID: &defaultExecutorID,
	}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateExecutor(ctx, &models.Executor{
		ID:        executorID,
		Name:      "Override compatibility executor",
		Type:      models.ExecutorTypeSSH,
		Status:    models.ExecutorStatusActive,
		Resumable: true,
	}); err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID:          workflowID,
		WorkspaceID: workspaceID,
		Name:        "Override compatibility workflow",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"implement": {
			ID:             "implement",
			WorkflowID:     workflowID,
			AgentProfileID: "source-profile",
		},
	}})
	svc.agentProfiles = workflowAgentOverrideProfilesStub{profiles: map[string]*settingsmodels.AgentProfile{
		"replacement-profile": {
			ID:          "replacement-profile",
			Enabled:     true,
			WorkspaceID: workspaceID,
		},
	}}
	compatibilityErr := errors.New("replacement profile has no SSH credentials")
	validator := &rejectingWorkflowAgentExecutorValidator{err: compatibilityErr}
	svc.agentProfileExecutorValidator = validator

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: workspaceID,
		WorkflowID:  workflowID,
		Title:       "Must not be persisted",
		ExecutorID:  executorID,
		WorkflowAgentOverrides: map[string]string{
			"source-profile": "replacement-profile",
		},
	})
	if err == nil || !errors.Is(err, models.ErrInvalidWorkflowAgentOverrides) {
		t.Fatalf("CreateTask error = %v, want invalid workflow override", err)
	}
	if !errors.Is(err, compatibilityErr) {
		t.Fatalf("CreateTask error = %v, want compatibility error in chain", err)
	}
	if len(validator.calls) != 1 || validator.calls[0] != "replacement-profile" {
		t.Fatalf("compatibility validator calls = %v, want one replacement validation", validator.calls)
	}
	tasks, err := repo.ListTasks(ctx, workflowID)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("persisted %d task rows after compatibility rejection, want none", len(tasks))
	}
}
