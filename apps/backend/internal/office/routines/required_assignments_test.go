package routines_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routines"
	"github.com/stretchr/testify/require"
)

type requiredAssignmentCreator struct {
	fakeTaskCreator
	required int
}

type routineContextCreator struct {
	fakeTaskCreator
	options routines.RoutineTaskOptions
}

func (f *requiredAssignmentCreator) CreateOfficeTaskInWorkflowWithRequirements(ctx context.Context, ws, project, agent, workflow, title, description, routine string, required int) (string, error) {
	f.required = required
	return f.CreateOfficeTaskInWorkflow(ctx, ws, project, agent, workflow, title, description, routine)
}

func (f *routineContextCreator) CreateOfficeTaskInWorkflowWithContext(
	ctx context.Context, ws, agent, workflow, title, description, routine string, options routines.RoutineTaskOptions,
) (string, error) {
	f.options = options
	return f.CreateOfficeTaskInWorkflow(ctx, ws, options.ProjectID, agent, workflow, title, description, routine)
}

func TestHeavyRoutinePreservesRequiredAssignments(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	routine := &models.Routine{WorkspaceID: "ws-1", Name: "Research", Status: "active", AssigneeAgentProfileID: "agent-coord", ConcurrencyPolicy: "always_create", TaskTemplate: `{"title":"Discovery","description":"Record evidence and result references","required_subtask_count":3}`}
	require.NoError(t, svc.CreateRoutine(ctx, routine))
	creator := &requiredAssignmentCreator{}
	svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
	svc.SetTaskCreator(creator)
	run, err := svc.FireManual(ctx, routine.ID, nil)
	require.NoError(t, err)
	require.Equal(t, 3, creator.required)
	require.Equal(t, "task-routine-1", run.LinkedTaskID)
	require.Equal(t, "Record evidence and result references", creator.captured.description)
}

func TestHeavyRoutinePassesProjectBlockersAndRequiredAssignments(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	routine := &models.Routine{
		WorkspaceID: "ws-1", Name: "Research", Status: "active", AssigneeAgentProfileID: "agent-coord",
		ConcurrencyPolicy: "always_create",
		TaskTemplate:      `{"title":"Discovery","description":"Record evidence","project_id":"project-1","blocked_by":["task-blocker"],"required_subtask_count":3}`,
	}
	require.NoError(t, svc.CreateRoutine(ctx, routine))
	creator := &routineContextCreator{}
	svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
	svc.SetTaskCreator(creator)
	run, err := svc.FireManual(ctx, routine.ID, nil)
	require.NoError(t, err)
	require.Equal(t, "task-routine-1", run.LinkedTaskID)
	require.Equal(t, routines.RoutineTaskOptions{
		ProjectID: "project-1", BlockedBy: []string{"task-blocker"}, RequiredSubtaskCount: 3,
	}, creator.options)
}

func TestHeavyRoutineRejectsUnsupportedProjectAndBlockerContext(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	routine := &models.Routine{
		WorkspaceID: "ws-1", Name: "Research", Status: "active", AssigneeAgentProfileID: "agent-coord",
		ConcurrencyPolicy: "always_create",
		TaskTemplate:      `{"title":"Discovery","project_id":"project-1","blocked_by":["task-blocker"]}`,
	}
	require.NoError(t, svc.CreateRoutine(ctx, routine))
	creator := &fakeTaskCreator{}
	svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
	svc.SetTaskCreator(creator)
	_, err := svc.FireManual(ctx, routine.ID, nil)
	require.ErrorContains(t, err, "does not support declared task context")
	require.Empty(t, creator.captured.title)
}

func TestHeavyRoutineDoesNotSilentlyDropUnsupportedRequiredAssignments(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	routine := &models.Routine{WorkspaceID: "ws-1", Name: "Research", Status: "active", AssigneeAgentProfileID: "agent-coord", ConcurrencyPolicy: "always_create", TaskTemplate: `{"title":"Discovery","required_subtask_count":3}`}
	require.NoError(t, svc.CreateRoutine(ctx, routine))
	creator := &fakeTaskCreator{}
	svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
	svc.SetTaskCreator(creator)
	_, err := svc.FireManual(ctx, routine.ID, nil)
	require.ErrorContains(t, err, "required subtask assignments")
	require.Empty(t, creator.captured.title)
}
