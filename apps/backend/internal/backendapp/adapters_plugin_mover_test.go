package backendapp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/steptelemetry"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// TestPluginsTaskWriter_MoveAttributesToPluginIntegrationActor pins AC-003.1
// and AC-003.3: a plugin move is recorded under the integration actor kind
// with the plugin's own "plugin:<id>" provenance, under the dedicated
// plugin_move trigger — never Human, never the bulk/manual/mcp triggers.
func TestPluginsTaskWriter_MoveAttributesToPluginIntegrationActor(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	a := pluginsTaskWriterAdapter{svc: svc}
	wf := "wf-1"

	_, err := a.MoveTask(context.Background(), plugins.TaskMoveInput{
		TaskID: "task-1", WorkflowStepID: "step-2", WorkflowID: &wf, Source: "plugin:acme",
	})
	require.NoError(t, err)
	require.NotNil(t, svc.lastMoveCtx)
	attr := steptelemetry.FromContext(svc.lastMoveCtx)
	require.Equal(t, steptelemetry.TriggerPluginMove, attr.Trigger)
	require.Equal(t, steptelemetry.ActorIntegration, attr.ActorKind)
	require.Equal(t, "plugin:acme", attr.ActorID)
}

// TestPluginsTaskWriter_MoveStepHistoryOptions pins that the session-level
// history is written under plugin_move too (AC-003.2) and never under the
// Human actor (AC-003.3's "must not be Human" — see design's rationale that
// Human is what stamps an authenticated user id onto an unattended move).
func TestPluginsTaskWriter_MoveStepHistoryOptions(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	a := pluginsTaskWriterAdapter{svc: svc}
	wf := "wf-1"

	_, err := a.MoveTask(context.Background(), plugins.TaskMoveInput{
		TaskID: "task-1", WorkflowStepID: "step-2", WorkflowID: &wf,
	})
	require.NoError(t, err)
	require.Equal(t, wfmodels.StepTransitionTriggerPluginMove, svc.lastMoveOpts.StepHistoryTrigger)
	require.NotEqual(t, wfmodels.StepTransitionActorHuman, svc.lastMoveOpts.StepHistoryActor)
	require.False(t, svc.lastMoveOpts.AllowActivePrimarySession,
		"a plugin move must take the agent-shaped option and reject a live session (AC-001.8), not the board's AllowActivePrimarySession")
}

// TestPluginsTaskWriter_MoveExplicitWorkflowIDPassesThrough pins that a
// caller-supplied workflow_id is used as-is and never overridden by the
// task's current workflow.
func TestPluginsTaskWriter_MoveExplicitWorkflowIDPassesThrough(t *testing.T) {
	svc := &fakePluginTaskWriteService{getTaskResult: &taskmodels.Task{ID: "task-1", WorkflowID: "wf-current"}}
	a := pluginsTaskWriterAdapter{svc: svc}
	wf := "wf-explicit"

	_, err := a.MoveTask(context.Background(), plugins.TaskMoveInput{TaskID: "task-1", WorkflowStepID: "step-2", WorkflowID: &wf})
	require.NoError(t, err)
	require.Equal(t, "wf-explicit", svc.lastMoveWfID)
	require.Equal(t, "", svc.lastGetTaskID, "an explicit workflow_id needs no task read to resolve it")
}

// TestPluginsTaskWriter_MoveNilWorkflowIDInheritsCurrent pins AC-005.4: an
// omitted workflow_id uses the task's current workflow.
func TestPluginsTaskWriter_MoveNilWorkflowIDInheritsCurrent(t *testing.T) {
	svc := &fakePluginTaskWriteService{getTaskResult: &taskmodels.Task{ID: "task-1", WorkflowID: "wf-current"}}
	a := pluginsTaskWriterAdapter{svc: svc}

	_, err := a.MoveTask(context.Background(), plugins.TaskMoveInput{TaskID: "task-1", WorkflowStepID: "step-2"})
	require.NoError(t, err)
	require.Equal(t, "task-1", svc.lastGetTaskID)
	require.Equal(t, "wf-current", svc.lastMoveWfID)
}

// TestPluginsTaskWriter_MoveNilWorkflowIDSetsExpectedWorkflowIDForCAS pins the
// SEC-001 fix (Review round 2): when workflow_id is inherited via the
// resolvePluginMoveWorkflowID pre-read rather than named explicitly, MoveTask
// must pass that resolved value as MoveTaskOptions.ExpectedWorkflowID so
// MoveTaskWithOptions can reject the write if a concurrent reassignment lands
// before it — see TestConcurrentReassignmentSurvivesMatchingStepStaleMove for
// the mechanism itself.
func TestPluginsTaskWriter_MoveNilWorkflowIDSetsExpectedWorkflowIDForCAS(t *testing.T) {
	svc := &fakePluginTaskWriteService{getTaskResult: &taskmodels.Task{ID: "task-1", WorkflowID: "wf-current"}}
	a := pluginsTaskWriterAdapter{svc: svc}

	_, err := a.MoveTask(context.Background(), plugins.TaskMoveInput{TaskID: "task-1", WorkflowStepID: "step-2"})
	require.NoError(t, err)
	require.NotNil(t, svc.lastMoveOpts.ExpectedWorkflowID, "an inherited workflow id must be CAS-guarded against a concurrent reassignment")
	require.Equal(t, "wf-current", *svc.lastMoveOpts.ExpectedWorkflowID)
}

// TestPluginsTaskWriter_MoveExplicitWorkflowIDSkipsExpectedWorkflowID pins the
// other half of the SEC-001 fix: a plugin that explicitly names a target
// workflow_id is intentionally requesting a cross-workflow move, so that
// request must never be CAS-guarded against the task's current workflow —
// doing so would reject every legitimate explicit reassignment.
func TestPluginsTaskWriter_MoveExplicitWorkflowIDSkipsExpectedWorkflowID(t *testing.T) {
	svc := &fakePluginTaskWriteService{getTaskResult: &taskmodels.Task{ID: "task-1", WorkflowID: "wf-current"}}
	a := pluginsTaskWriterAdapter{svc: svc}
	wf := "wf-explicit"

	_, err := a.MoveTask(context.Background(), plugins.TaskMoveInput{TaskID: "task-1", WorkflowStepID: "step-2", WorkflowID: &wf})
	require.NoError(t, err)
	require.Nil(t, svc.lastMoveOpts.ExpectedWorkflowID, "an explicit workflow_id is an intentional target and must not be CAS-guarded")
}

// TestPluginsTaskWriter_MoveNoWorkflowNoneNamedDefersToSharedPath pins the
// AC-005.11/precedence-ladder fix: when the task has no current workflow and
// none was named, resolution must NOT short-circuit with its own
// InvalidArgument before MoveTaskWithOptions' archived/active-session checks
// (AC-001.7/001.8) get a chance to run. The empty workflow id is passed
// through so the shared path's own destination check (which the ladder places
// AFTER the task-state checks) is what ultimately answers InvalidArgument —
// see resolvePluginMoveWorkflowID's doc for why a local short-circuit here
// would answer the wrong code for an archived task with no workflow.
func TestPluginsTaskWriter_MoveNoWorkflowNoneNamedDefersToSharedPath(t *testing.T) {
	svc := &fakePluginTaskWriteService{getTaskResult: &taskmodels.Task{ID: "task-1", WorkflowID: ""}}
	a := pluginsTaskWriterAdapter{svc: svc}

	_, err := a.MoveTask(context.Background(), plugins.TaskMoveInput{TaskID: "task-1", WorkflowStepID: "step-2"})
	require.NoError(t, err, "the fake's MoveTaskWithOptions succeeds unconditionally; this test only pins that resolution reached it")
	require.Equal(t, "", svc.lastMoveWfID, "empty workflow id must reach MoveTaskWithOptions, not be rejected before it")
}

// TestPluginsTaskWriter_MoveNoWorkflowNoneNamedRejectsAfterSharedValidation
// pins the InvalidArgument end state for AC-005.11 once the shared path's own
// "workflow not found" (from GetWorkflow("")) surfaces, going through
// classifyPluginMoveError like every other destination-classification case.
func TestPluginsTaskWriter_MoveNoWorkflowNoneNamedRejectsAfterSharedValidation(t *testing.T) {
	svc := &fakePluginTaskWriteService{
		getTaskResult: &taskmodels.Task{ID: "task-1", WorkflowID: ""},
		moveErr:       fmt.Errorf("failed to get target workflow: workflow not found: "),
	}
	a := pluginsTaskWriterAdapter{svc: svc}

	_, err := a.MoveTask(context.Background(), plugins.TaskMoveInput{TaskID: "task-1", WorkflowStepID: "step-2"})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

// TestPluginsTaskWriter_MoveGetTaskNotFoundDuringResolution pins that a
// missing task surfaces NotFound (AC-005.6) even when discovered during
// workflow-inheritance resolution, before MoveTaskWithOptions is ever called.
func TestPluginsTaskWriter_MoveGetTaskNotFoundDuringResolution(t *testing.T) {
	svc := &fakePluginTaskWriteService{getTaskErr: fmt.Errorf("%w: task-missing", repoerrors.ErrTaskNotFound)}
	a := pluginsTaskWriterAdapter{svc: svc}

	_, err := a.MoveTask(context.Background(), plugins.TaskMoveInput{TaskID: "task-missing", WorkflowStepID: "step-2"})
	require.Equal(t, codes.NotFound, status.Code(err))
	require.Equal(t, 0, svc.moveCalls, "MoveTaskWithOptions must not be reached when the task can't be loaded")
}

// TestPluginsTaskWriter_MoveUnexpectedWorkflowReadErrorIsSanitized pins the
// plugin boundary contract for inherited-workflow resolution: an unexpected
// repository failure must not expose storage details or task identifiers to a
// plugin caller.
func TestPluginsTaskWriter_MoveUnexpectedWorkflowReadErrorIsSanitized(t *testing.T) {
	internalErr := errors.New("database is locked while reading task task-secret")
	svc := &fakePluginTaskWriteService{getTaskErr: internalErr}
	a := pluginsTaskWriterAdapter{svc: svc}

	_, err := a.MoveTask(context.Background(), plugins.TaskMoveInput{
		TaskID: "task-secret", WorkflowStepID: "step-2",
	})

	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, "failed to move task", status.Convert(err).Message())
	require.NotContains(t, status.Convert(err).Message(), internalErr.Error())
	require.Equal(t, 0, svc.moveCalls, "MoveTaskWithOptions must not run after workflow resolution fails")
}

// TestPluginsTaskWriter_MovePositionCastAndResultMapping pins that Position
// is forwarded as int and the response's Transitioned/FromStepID come
// straight from MoveTaskResult (not re-derived).
func TestPluginsTaskWriter_MovePositionCastAndResultMapping(t *testing.T) {
	svc := &fakePluginTaskWriteService{
		moveResult: &taskservice.MoveTaskResult{
			Task:         &taskmodels.Task{ID: "task-1"},
			Transitioned: true,
			FromStepID:   "step-origin",
		},
	}
	a := pluginsTaskWriterAdapter{svc: svc}
	wf := "wf-1"

	result, err := a.MoveTask(context.Background(), plugins.TaskMoveInput{
		TaskID: "task-1", WorkflowStepID: "step-2", WorkflowID: &wf, Position: 7,
	})
	require.NoError(t, err)
	require.Equal(t, 7, svc.lastMovePos)
	require.True(t, result.Transitioned)
	require.Equal(t, "step-origin", result.FromStepID)
}

// TestClassifyPluginMoveError pins every row of the binding error-mapping
// table this classifier is responsible for (the rows reachable from
// MoveTaskWithOptions' bare validation errors; capability/request-shape rows
// are classified earlier, in internal/plugins/host_write.go).
//
// SEC-002 (Review round 2): the classifier's whole job is to strip the real
// validator's internal text off the gRPC boundary. Pinning only the status
// code (as this table did before) lets a future change keep the code right
// while silently re-forwarding err.Error() into the message — reopening the
// exact info-disclosure surface SEC-002 closed. Every case therefore also
// asserts the exact fixed message, and every input error below embeds an
// internal-only fragment (an id, a raw validator sentence) that must NOT
// appear in the output message.
func TestClassifyPluginMoveError(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		want    codes.Code
		wantMsg string
	}{
		{"nil is nil", nil, codes.OK, ""},
		{"archived task, AC-001.7", errors.New("archived tasks cannot be moved"), codes.FailedPrecondition, "task is archived and cannot be moved"},
		{"active session, AC-001.8", errors.New("task has an active session (starting)"), codes.FailedPrecondition, "task has an active session and cannot be moved"},
		{"unknown workflow, AC-001.6", errors.New("failed to get target workflow: workflow not found: wf-x"), codes.InvalidArgument, "invalid move_task request: unknown or mismatched workflow, step, or workspace"},
		{"unknown step, AC-001.6", errors.New("failed to get target workflow step: workflow step not found: step-x"), codes.InvalidArgument, "invalid move_task request: unknown or mismatched workflow, step, or workspace"},
		{"different workspace, AC-001.6", errors.New("target workflow is in a different workspace"), codes.InvalidArgument, "invalid move_task request: unknown or mismatched workflow, step, or workspace"},
		{"step not in workflow, AC-001.6", errors.New("target workflow step does not belong to target workflow"), codes.InvalidArgument, "invalid move_task request: unknown or mismatched workflow, step, or workspace"},
		{"task not found sentinel, AC-005.6", fmt.Errorf("%w: task-1", repoerrors.ErrTaskNotFound), codes.NotFound, "task not found"},
		{"context canceled", fmt.Errorf("read task: %w", context.Canceled), codes.Canceled, "move task canceled"},
		{"context deadline", fmt.Errorf("read task: %w", context.DeadlineExceeded), codes.DeadlineExceeded, "move task deadline exceeded"},
		{"workflow resolution conflict, SEC-001", fmt.Errorf("%w: resolved %q, task is now in %q", taskservice.ErrWorkflowResolutionConflict, "wf-home", "wf-away"), codes.Aborted, "task workflow changed concurrently, retry the move"},
		{"unmapped error falls back to Internal", errors.New("something unexpected"), codes.Internal, "failed to move task"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyPluginMoveError(tc.err)
			if tc.err == nil {
				require.NoError(t, got)
				return
			}
			st := status.Convert(got)
			require.Equal(t, tc.want, st.Code())
			require.Equal(t, tc.wantMsg, st.Message())
			require.NotContains(t, st.Message(), "wf-x", "message must not leak the internal error's workflow/step id")
			require.NotContains(t, st.Message(), "step-x", "message must not leak the internal error's workflow/step id")
			require.NotContains(t, st.Message(), "task-1", "message must not leak the internal error's task id")
			require.NotContains(t, st.Message(), "wf-home", "message must not leak the internal error's workflow id")
			require.NotContains(t, st.Message(), "wf-away", "message must not leak the internal error's workflow id")
		})
	}
}
