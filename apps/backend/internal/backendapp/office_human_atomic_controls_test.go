package backendapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// @covers AC-OFFICE-HUMAN-DONE-002.1 AC-OFFICE-HUMAN-DONE-002.5
func TestOfficeHumanCompletionSessionApprovalRace(t *testing.T) {
	var window *officeAdmissionWindow
	f := newOfficeAtomicFixture(t, func(r *tasksqlite.Repository) taskrepo.TaskRepository {
		window = &officeAdmissionWindow{Repository: r}
		return window
	})
	writer := officeAtomicSecondWriter(t, f.h)
	ctx := context.Background()
	require.NoError(t, f.h.repo.CreateTaskSession(ctx, &models.TaskSession{ID: "race-session", TaskID: "atomic-task", State: models.TaskSessionStateWaitingForInput, IsPrimary: true, ReviewStatus: models.ReviewStatusPending}))
	window.before = func() {
		_, err := writer.UpsertTaskParticipant(ctx, "step-home", "atomic-task", "approver", "racing-agent")
		require.NoError(t, err)
	}
	before, err := f.h.repo.GetTask(ctx, "atomic-task")
	require.NoError(t, err)
	session, err := f.h.repo.GetTaskSession(ctx, "race-session")
	require.NoError(t, err)
	var mu sync.Mutex
	var emitted []string
	sub, err := f.h.eventBus.Subscribe(events.TaskStateChanged, func(_ context.Context, e *bus.Event) error {
		mu.Lock()
		emitted = append(emitted, e.Type)
		mu.Unlock()
		return nil
	})
	require.NoError(t, err)
	defer func() { _ = sub.Unsubscribe() }()
	router, dispatcher := gin.New(), ws.NewDispatcher()
	taskhandlers.RegisterTaskRoutes(router, dispatcher, f.svc, nil, f.h.repo, nil, f.log)
	invoke := func() int {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/race-session/approve", strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, request)
		return rec.Code
	}
	require.Equal(t, http.StatusConflict, invoke())
	after, err := f.h.repo.GetTask(ctx, "atomic-task")
	require.NoError(t, err)
	require.Equal(t, before, after)
	afterSession, err := f.h.repo.GetTaskSession(ctx, "race-session")
	require.NoError(t, err)
	require.Equal(t, session, afterSession)
	mu.Lock()
	observed := append([]string(nil), emitted...)
	mu.Unlock()
	require.Empty(t, observed)
	pending, err := f.office.ListTaskParticipantsAtStep(ctx, "atomic-task", "atomic-done", "approver")
	require.NoError(t, err)
	require.Len(t, pending, 1)
	window.before = func() {}
	require.NoError(t, writer.DeleteTaskParticipant(ctx, "step-home", "atomic-task", "approver", "racing-agent"))
	require.Equal(t, http.StatusOK, invoke())
	mu.Lock()
	observed = append([]string(nil), emitted...)
	mu.Unlock()
	require.Equal(t, []string{events.TaskStateChanged}, observed)
	approved, err := f.h.repo.GetTaskSession(ctx, "race-session")
	require.NoError(t, err)
	require.Equal(t, models.ReviewStatusApproved, approved.ReviewStatus)
}

func TestOfficeHumanCompletionQueuedApprovalsDoNotRefuse(t *testing.T) {
	f := newOfficeAtomicFixture(t, nil)
	ctx := context.Background()
	step, err := f.workflow.GetStep(ctx, "atomic-done")
	require.NoError(t, err)
	step.WIPLimit = 1
	require.NoError(t, f.workflow.UpdateStep(ctx, step))
	require.NoError(t, f.h.repo.CreateTask(ctx, &models.Task{ID: "occupant", WorkspaceID: "ws-1", WorkflowID: "wf-home", WorkflowStepID: "atomic-done", State: v1.TaskStateTODO, Title: "Occupant"}))
	_, err = f.workflow.UpsertTaskParticipant(ctx, "step-home", "atomic-task", "approver", "pending")
	require.NoError(t, err)
	result, err := f.svc.MoveTaskWithOptions(ctx, "atomic-task", "wf-home", "atomic-done", 0, taskservice.MoveTaskOptions{StepHistoryActor: wfmodels.StepTransitionActorHuman})
	require.NoError(t, err)
	require.Equal(t, "atomic-done", result.Task.QueuedForStepID)
	require.Equal(t, v1.TaskStateTODO, result.Task.State)
}

func TestOfficeHumanCompletionEngineAndNonOfficeControls(t *testing.T) {
	for _, kind := range []string{"system", "agent", "nonOffice human"} {
		t.Run(kind, func(t *testing.T) {
			f := newOfficeAtomicFixture(t, nil)
			ctx := context.Background()
			_, err := f.workflow.UpsertTaskParticipant(ctx, "step-home", "atomic-task", "approver", "pending")
			require.NoError(t, err)
			actor := wfmodels.StepTransitionActor(kind)
			if kind == "nonOffice human" {
				actor = wfmodels.StepTransitionActorHuman
				_, err = f.h.db.Exec(`UPDATE workspaces SET office_workflow_id='' WHERE id='ws-1'`)
				require.NoError(t, err)
			} else {
				ctx = models.WithOfficeHumanCompletionIntent(ctx, models.OfficeHumanCompletionIntent{TaskID: "another-task", Kind: "office_done"})
			}
			result, err := f.svc.MoveTaskWithOptions(ctx, "atomic-task", "wf-home", "atomic-done", 0, taskservice.MoveTaskOptions{StepHistoryActor: actor})
			require.NoError(t, err)
			require.Equal(t, v1.TaskStateCompleted, result.Task.State)
		})
	}
}

func TestOfficeHumanCompletionConcurrentStateSurvives(t *testing.T) {
	for _, kind := range []string{"arrival", "reorder", "fresh failure"} {
		t.Run(kind, func(t *testing.T) {
			var window *officeAdmissionWindow
			f := newOfficeAtomicFixture(t, func(r *tasksqlite.Repository) taskrepo.TaskRepository {
				window = &officeAdmissionWindow{Repository: r}
				return window
			})
			ctx := context.Background()
			before := func() { require.NoError(t, f.h.repo.UpdateTaskState(ctx, "atomic-task", v1.TaskStateFailed)) }
			window.before = before
			target := "atomic-done"
			if kind == "reorder" {
				target = "step-home"
			}
			if kind == "fresh failure" {
				before()
				window.before = func() {}
			}
			// For arrival, the writer runs after state preparation and before admission.
			_, err := f.svc.MoveTaskWithOptions(ctx, "atomic-task", "wf-home", target, 77, taskservice.MoveTaskOptions{StepHistoryActor: wfmodels.StepTransitionActorHuman})
			if kind != "fresh failure" {
				require.ErrorIs(t, err, repoerrors.ErrWorkflowChangeConflict)
			} else {
				require.NoError(t, err)
			}
			after, err := f.h.repo.GetTask(ctx, "atomic-task")
			require.NoError(t, err)
			require.Equal(t, v1.TaskStateFailed, after.State)
			if kind != "fresh failure" {
				require.Equal(t, "step-home", after.WorkflowStepID)
			}
		})
	}
}

func TestOfficeHumanCompletionExplicitStateKeepsStepPolicy(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(map[bool]string{false: "clear", true: "pending"}[pending], func(t *testing.T) {
			f := newOfficeAtomicFixture(t, nil)
			ctx := context.Background()
			require.NoError(t, f.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "explicit-work", WorkflowID: "wf-home", Name: "Work", Position: 5}))
			if pending {
				_, err := f.workflow.UpsertTaskParticipant(ctx, "explicit-work", "atomic-task", "approver", "pending")
				require.NoError(t, err)
			}
			before, err := f.h.repo.GetTask(ctx, "atomic-task")
			require.NoError(t, err)
			state, step := v1.TaskStateCompleted, "explicit-work"
			result, err := f.svc.UpdateTaskAsHuman(ctx, "atomic-task", &taskservice.UpdateTaskRequest{State: &state, WorkflowStepID: &step})
			if pending {
				require.ErrorAs(t, err, new(*taskservice.ApprovalsPendingMoveError))
				after, err := f.h.repo.GetTask(ctx, "atomic-task")
				require.NoError(t, err)
				require.Equal(t, before, after)
			} else {
				require.NoError(t, err)
				require.Equal(t, v1.TaskStateCompleted, result.State)
				require.Equal(t, step, result.WorkflowStepID)
			}
		})
	}
}
