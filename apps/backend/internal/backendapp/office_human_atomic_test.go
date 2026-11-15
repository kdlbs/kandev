package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/dashboard"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The decorator places a real committed writer after service preflight and
// before production admission. It never substitutes an eligibility result.
type officeAdmissionWindow struct {
	*tasksqlite.Repository
	before func()
}

func (r *officeAdmissionWindow) UpdateTaskWithWorkflowStepAdmissionAndState(ctx context.Context, task *models.Task, source, target string, limit int, state *v1.TaskState, exit bool, expected string) (bool, error) {
	r.before()
	return r.Repository.UpdateTaskWithWorkflowStepAdmissionAndState(ctx, task, source, target, limit, state, exit, expected)
}

// @covers AC-OFFICE-HUMAN-DONE-002.5
func TestOfficeHumanCompletionWriterBeforeAdmission(t *testing.T) {
	h := newOfficeAtomicHarness(t)
	ctx := context.Background()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	office, err := officesqlite.NewWithDB(h.db, h.db, log)
	require.NoError(t, err)
	decisions, err := workflowrepo.NewWithDB(h.db, h.db, log)
	require.NoError(t, err)
	dash := dashboard.NewDashboardService(office, log, nil, nil, nil)
	dash.SetDecisionStore(decisions)
	configureOfficeHumanCompletionGuard(h.repo, office, decisions)
	require.NoError(t, h.workflowSvc.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "atomic-done", WorkflowID: "wf-home", Name: "Done", Position: 10, CompleteTaskOnEnter: true}))
	h.createTask(t, "atomic-task", false)
	_, err = h.db.Exec(`UPDATE workspaces SET office_workflow_id='wf-home' WHERE id='ws-1'`)
	require.NoError(t, err)
	before, err := h.repo.GetTask(ctx, "atomic-task")
	require.NoError(t, err)
	writer := officeAtomicSecondWriter(t, h)
	window := &officeAdmissionWindow{Repository: h.repo, before: func() {
		_, err := writer.UpsertTaskParticipant(ctx, "step-home", "atomic-task", string(wfmodels.ParticipantRoleApprover), "new-approver")
		require.NoError(t, err)
	}}
	svc := taskservice.NewService(taskservice.Repos{Tasks: window, Workspaces: h.repo, Workflows: h.repo, Sessions: h.repo, Reviews: h.repo, Messages: h.repo, Turns: h.repo, TaskRepos: h.repo, GitSnapshots: h.repo, RepoEntities: h.repo, Executors: h.repo, Environments: h.repo, TaskEnvironments: h.repo}, h.eventBus, log, taskservice.RepositoryDiscoveryConfig{})
	svc.SetWorkflowStepGetter(&workflowStepGetterAdapter{svc: h.workflowSvc})
	svc.SetOfficeApprovalGuard(officeApprovalGuardAdapter{reader: dash})
	_, err = svc.MoveTaskWithOptions(ctx, "atomic-task", "wf-home", "atomic-done", 0, taskservice.MoveTaskOptions{AllowActivePrimarySession: true, StepHistoryActor: wfmodels.StepTransitionActorHuman})
	require.ErrorAs(t, err, new(*taskservice.ApprovalsPendingMoveError), "new approver committed after preflight must refuse completion")
	after, err := h.repo.GetTask(ctx, "atomic-task")
	require.NoError(t, err)
	require.Equal(t, before, after, "refused completion must not write task/placement")
	seats, err := office.ListTaskParticipantsAtStep(ctx, "atomic-task", "atomic-done", string(wfmodels.ParticipantRoleApprover))
	require.NoError(t, err)
	require.Len(t, seats, 1, "legitimate concurrent writer survives")
}

// @covers AC-OFFICE-HUMAN-DONE-002.1
func TestOfficeHumanCompletionFinalStepRefusalLeavesState(t *testing.T) {
	h := newOfficeAtomicHarness(t)
	ctx := context.Background()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	office, err := officesqlite.NewWithDB(h.db, h.db, log)
	require.NoError(t, err)
	decisions, err := workflowrepo.NewWithDB(h.db, h.db, log)
	require.NoError(t, err)
	dash := dashboard.NewDashboardService(office, log, nil, nil, nil)
	dash.SetDecisionStore(decisions)
	dash.SetTaskFinalStepMover(officeFinalStepMoverAdapter{tasks: h.adapter.svc, steps: h.workflowSvc})
	configureOfficeHumanCompletionGuard(h.repo, office, decisions)
	h.adapter.svc.(*taskservice.Service).SetOfficeApprovalGuard(officeApprovalGuardAdapter{reader: dash})
	dash.SetTaskHumanDoneCompleter(officeHumanDoneCompleterAdapter{tasks: h.adapter.svc.(*taskservice.Service)})
	h.createTask(t, "atomic-final-task", false)
	_, err = h.db.Exec(`UPDATE workspaces SET office_workflow_id='wf-home' WHERE id='ws-1'`)
	require.NoError(t, err)
	_, err = decisions.UpsertTaskParticipant(ctx, "step-home", "atomic-final-task", "approver", "pending-final")
	require.NoError(t, err)
	before, err := h.repo.GetTask(ctx, "atomic-final-task")
	require.NoError(t, err)
	_, err = dash.UpdateTaskStatusAsHuman(ctx, dashboard.TaskStatusUpdateRequest{TaskID: "atomic-final-task", NewStatus: "done"})
	assert.ErrorAs(t, err, new(*dashboard.FinalStepMoveRefusedError))
	after, err := h.repo.GetTask(ctx, "atomic-final-task")
	require.NoError(t, err)
	assert.Equal(t, before, after, "human refusal must not redirect or write")
}

// @covers AC-OFFICE-HUMAN-DONE-002.1
func TestOfficeHumanCompletionSameStepStateRefusal(t *testing.T) {
	h := newOfficeAtomicHarness(t)
	ctx := context.Background()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	office, err := officesqlite.NewWithDB(h.db, h.db, log)
	require.NoError(t, err)
	decisions, err := workflowrepo.NewWithDB(h.db, h.db, log)
	require.NoError(t, err)
	dash := dashboard.NewDashboardService(office, log, nil, nil, nil)
	dash.SetDecisionStore(decisions)
	svc := h.adapter.svc.(*taskservice.Service)
	svc.SetOfficeApprovalGuard(officeApprovalGuardAdapter{reader: dash})
	configureOfficeHumanCompletionGuard(h.repo, office, decisions)
	h.createTask(t, "atomic-state-task", false)
	_, err = h.db.Exec(`UPDATE workspaces SET office_workflow_id='wf-home' WHERE id='ws-1'`)
	require.NoError(t, err)
	_, err = decisions.UpsertTaskParticipant(ctx, "step-home", "atomic-state-task", "approver", "pending-state")
	require.NoError(t, err)
	before, err := h.repo.GetTask(ctx, "atomic-state-task")
	require.NoError(t, err)
	state := v1.TaskStateCompleted
	_, err = svc.UpdateTaskAsHuman(ctx, "atomic-state-task", &taskservice.UpdateTaskRequest{State: &state})
	assert.ErrorAs(t, err, new(*taskservice.ApprovalsPendingMoveError))
	after, err := h.repo.GetTask(ctx, "atomic-state-task")
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

// @covers AC-OFFICE-HUMAN-DONE-002.5
func TestOfficeHumanCompletionFreshPolicyBeforeAdmission(t *testing.T) {
	for _, change := range []string{"completion flag", "new final step", "fresh capacity"} {
		t.Run(change, func(t *testing.T) {
			h := newOfficeAtomicHarness(t)
			ctx := context.Background()
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
			require.NoError(t, err)
			office, err := officesqlite.NewWithDB(h.db, h.db, log)
			require.NoError(t, err)
			workflow, err := workflowrepo.NewWithDB(h.db, h.db, log)
			require.NoError(t, err)
			dash := dashboard.NewDashboardService(office, log, nil, nil, nil)
			dash.SetDecisionStore(workflow)
			configureOfficeHumanCompletionGuard(h.repo, office, workflow)
			require.NoError(t, h.workflowSvc.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "atomic-done", WorkflowID: "wf-home", Name: "Done", Position: 10, CompleteTaskOnEnter: true}))
			h.createTask(t, "atomic-policy-task", false)
			_, err = h.db.Exec(`UPDATE workspaces SET office_workflow_id='wf-home' WHERE id='ws-1'`)
			require.NoError(t, err)
			if change == "fresh capacity" {
				require.NoError(t, h.repo.CreateTask(ctx, &models.Task{ID: "occupant", WorkspaceID: "ws-1", WorkflowID: "wf-home", WorkflowStepID: "atomic-done", Title: "Occupant", State: v1.TaskStateTODO}))
			}
			window := &officeAdmissionWindow{Repository: h.repo, before: func() {
				if change == "new final step" {
					require.NoError(t, workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "new-final", WorkflowID: "wf-home", Name: "Next", Position: 20}))
					return
				}
				step, err := workflow.GetStep(ctx, "atomic-done")
				require.NoError(t, err)
				if change == "completion flag" {
					step.CompleteTaskOnEnter = false
				} else {
					step.WIPLimit = 1
				}
				require.NoError(t, workflow.UpdateStep(ctx, step))
			}}
			svc := taskservice.NewService(taskservice.Repos{Tasks: window, Workspaces: h.repo, Workflows: h.repo, Sessions: h.repo, Reviews: h.repo, Messages: h.repo, Turns: h.repo, TaskRepos: h.repo, GitSnapshots: h.repo, RepoEntities: h.repo, Executors: h.repo, Environments: h.repo, TaskEnvironments: h.repo}, h.eventBus, log, taskservice.RepositoryDiscoveryConfig{})
			svc.SetWorkflowStepGetter(&workflowStepGetterAdapter{svc: h.workflowSvc})
			svc.SetOfficeApprovalGuard(officeApprovalGuardAdapter{reader: dash})
			before, err := h.repo.GetTask(ctx, "atomic-policy-task")
			require.NoError(t, err)
			result, err := svc.MoveTaskWithOptions(ctx, "atomic-policy-task", "wf-home", "atomic-done", 0, taskservice.MoveTaskOptions{AllowActivePrimarySession: true, StepHistoryActor: wfmodels.StepTransitionActorHuman})
			after, readErr := h.repo.GetTask(ctx, "atomic-policy-task")
			require.NoError(t, readErr)
			if change == "fresh capacity" {
				require.NoError(t, err)
				require.Equal(t, "atomic-done", result.Task.QueuedForStepID)
				require.Equal(t, before.State, after.State)
			} else {
				assert.ErrorIs(t, err, repoerrors.ErrWorkflowChangeConflict)
				assert.Equal(t, before, after)
			}
		})
	}
}

func (r *officeAdmissionWindow) UpdateTaskPreservingDeferredLaunch(ctx context.Context, task *models.Task) error {
	r.before()
	return r.Repository.UpdateTaskPreservingDeferredLaunch(ctx, task)
}

func TestOfficeHumanCompletionCanonicalReadFailureClosed(t *testing.T) {
	f := newOfficeAtomicFixture(t, nil)
	ctx := context.Background()
	seatID, err := f.workflow.UpsertTaskParticipant(ctx, "step-home", "atomic-task", "approver", "pending-agent")
	require.NoError(t, err)
	before, err := f.h.repo.GetTask(ctx, "atomic-task")
	require.NoError(t, err)
	ledger, err := f.h.repo.GetLatestTaskStepTransitionID(ctx, "atomic-task")
	require.NoError(t, err)
	published := officeAtomicPublishedEvents(t, f)
	// Break the real canonical decision query, rather than replacing the guard.
	_, err = f.h.db.Exec(`ALTER TABLE workflow_step_decisions RENAME TO atomic_unavailable_decisions`)
	require.NoError(t, err)
	_, err = f.svc.MoveTaskWithOptions(ctx, "atomic-task", "wf-home", "atomic-done", 0,
		taskservice.MoveTaskOptions{StepHistoryActor: wfmodels.StepTransitionActorHuman})
	require.ErrorContains(t, err, "workflow_step_decisions")
	after, err := f.h.repo.GetTask(ctx, "atomic-task")
	require.NoError(t, err)
	require.Equal(t, before, after)
	afterLedger, err := f.h.repo.GetLatestTaskStepTransitionID(ctx, "atomic-task")
	require.NoError(t, err)
	require.Equal(t, ledger, afterLedger)
	require.Empty(t, published())
	_, err = f.h.db.Exec(`ALTER TABLE atomic_unavailable_decisions RENAME TO workflow_step_decisions`)
	require.NoError(t, err)
	require.NoError(t, f.workflow.RecordStepDecision(ctx, &wfmodels.WorkflowStepDecision{
		TaskID: "atomic-task", StepID: "step-home", ParticipantID: seatID, Decision: "approved",
		DeciderType: "agent", DeciderID: "pending-agent", Role: "approver",
	}))
	_, err = f.svc.MoveTaskWithOptions(ctx, "atomic-task", "wf-home", "atomic-done", 0,
		taskservice.MoveTaskOptions{StepHistoryActor: wfmodels.StepTransitionActorHuman})
	require.NoError(t, err, "same fixture succeeds after the canonical reader is restored and approval exists")
	require.NotEmpty(t, published())
}
