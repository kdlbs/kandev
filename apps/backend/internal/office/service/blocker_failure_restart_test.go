package service_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/models"
	officepause "github.com/kandev/kandev/internal/office/pause"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/scheduler"
	"github.com/kandev/kandev/internal/office/service"
	officewakeup "github.com/kandev/kandev/internal/office/wakeup"
	runmodels "github.com/kandev/kandev/internal/runs/models"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	wfrepo "github.com/kandev/kandev/internal/workflow/repository"
	wfservice "github.com/kandev/kandev/internal/workflow/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const restartBlockerID = "dependent-prerequisite-0"

type blockerFailureRestartProcess struct {
	db          *sqlx.DB
	eb          *bus.MemoryEventBus
	tasksRepo   *tasksqlite.Repository
	workflows   *wfrepo.Repository
	workflowSvc *wfservice.Service
	repo        *officesqlite.Repository
	svc         *service.Service
	tasks       *taskservice.Service
	pause       *officepause.Service
}

func openBlockerFailureRestartProcess(t *testing.T, path string, seed, resumeHook bool) *blockerFailureRestartProcess {
	t.Helper()
	db, err := sqlx.Open("sqlite3", path+"?_foreign_keys=on")
	blockerMust(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	log := logger.Default()
	settings, _, err := settingsstore.Provide(db, db, log)
	blockerMust(t, err)
	tasksRepo, err := tasksqlite.NewWithDB(db, db, log)
	blockerMust(t, err)
	workflowsRepo, err := wfrepo.NewWithDB(db, db, log)
	blockerMust(t, err)
	officeRepo, err := officesqlite.NewWithDB(db, db, log)
	blockerMust(t, err)
	if seed {
		blockerMust(t, tasksRepo.CreateWorkspace(context.Background(), &taskmodels.Workspace{
			ID: "assignment-ws", Name: "Blocker restart", OfficeWorkflowID: "assignment-workflow",
		}))
		blockerMust(t, settings.CreateAgent(context.Background(), &settingsmodels.Agent{ID: "assignment-cli", Name: "Assignment CLI"}))
		for _, id := range []string{"assignment-worker", escalationRunner} {
			blockerMust(t, officeRepo.CreateAgentInstance(context.Background(), &models.AgentInstance{
				ID: id, AgentID: "assignment-cli", WorkspaceID: "assignment-ws", Name: id, Status: models.AgentStatusIdle,
			}))
		}
		blockerMust(t, tasksRepo.CreateWorkflow(context.Background(), &taskmodels.Workflow{
			ID: "assignment-workflow", WorkspaceID: "assignment-ws", Name: "Office",
		}))
		for position, id := range []string{"assignment-work", "assignment-backlog"} {
			step := &wfmodels.WorkflowStep{ID: id, WorkflowID: "assignment-workflow", Name: id, Position: position}
			if position == 0 {
				step.Events.OnEnter = []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}
			}
			blockerMust(t, workflowsRepo.CreateStep(context.Background(), step))
		}
	}

	eb := bus.NewMemoryEventBus(log)
	t.Cleanup(eb.Close)
	runs := runsservice.New(officeRepo.RunsRepository(), nil, log, nil)
	officeSvc := service.NewService(service.ServiceOptions{
		Repo: officeRepo, Logger: log, CfgLoader: configloader.NewConfigLoader(t.TempDir()),
	})
	officeSvc.SetRunsService(runs)
	officeSvc.SetWorkflowStepGetter(workflowsRepo)
	officeSvc.SetSyncHandlers(true)
	blockerMust(t, officeSvc.RegisterEventSubscribers(eb))
	wakeupDispatcher := officewakeup.NewDispatcher(officeRepo, officeRepo, log)
	wakeupDispatcher.SetRunQueuer(officeSvc)
	officeSvc.SetWakeupRequestDispatcher(wakeupDispatcher)
	taskSvc := taskservice.NewService(taskservice.Repos{
		Workspaces: tasksRepo, Tasks: tasksRepo, TaskRepos: tasksRepo, Workflows: tasksRepo,
		Sessions: tasksRepo, TaskEnvironments: tasksRepo, ResourceCleanups: tasksRepo, TaskActivity: tasksRepo,
	}, eb, log, taskservice.RepositoryDiscoveryConfig{})
	workflowSvc := wfservice.NewService(workflowsRepo, log)
	t.Cleanup(func() { _ = workflowSvc.Close() })
	taskSvc.SetWorkflowStepGetter(workflowSvc)
	taskSvc.SetBlockerRepository(officeRepo)
	schedulerSvc := scheduler.NewSchedulerService(officeRepo, log, officeSvc)
	schedulerSvc.SetRunsService(runs)
	schedulerSvc.SetWorkflowStepGetter(workflowsRepo)
	blockerMust(t, schedulerSvc.RegisterBlockerFailureSubscriber(eb))
	pauseSvc := officepause.NewService(officeRepo, officeSvc, taskSvc, log)
	officeSvc.SetPauseGate(pauseSvc)
	wakeupDispatcher.SetPauseGate(pauseSvc)
	if resumeHook {
		pauseSvc.SetAssignmentReplayer(officeSvc)
	}
	return &blockerFailureRestartProcess{
		db: db, eb: eb, tasksRepo: tasksRepo, workflows: workflowsRepo, repo: officeRepo,
		workflowSvc: workflowSvc, svc: officeSvc, tasks: taskSvc, pause: pauseSvc,
	}
}

func (p *blockerFailureRestartProcess) close(t *testing.T) {
	t.Helper()
	blockerMust(t, p.workflowSvc.Close())
	p.eb.Close()
	blockerMust(t, p.db.Close())
}

func (p *blockerFailureRestartProcess) seedFailure(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	blockerMust(t, p.tasksRepo.CreateTask(ctx, &taskmodels.Task{
		ID: escalationParent, WorkspaceID: "assignment-ws", WorkflowID: "assignment-workflow",
		WorkflowStepID: "assignment-work", Title: escalationParent, State: v1.TaskStateInProgress,
		AssigneeAgentProfileID: escalationRunner, Priority: "medium",
	}))
	blockerMust(t, p.tasksRepo.CreateTask(ctx, &taskmodels.Task{
		ID: restartBlockerID, WorkspaceID: "assignment-ws", WorkflowID: "assignment-workflow",
		WorkflowStepID: "assignment-work", Title: restartBlockerID, State: v1.TaskStateInProgress,
		Priority: "medium",
	}))
	blockerMust(t, p.tasksRepo.CreateTask(ctx, &taskmodels.Task{
		ID: "dependent", WorkspaceID: "assignment-ws", WorkflowID: "assignment-workflow",
		WorkflowStepID: "assignment-work", Title: "dependent", State: v1.TaskStateTODO,
		AssigneeAgentProfileID: "assignment-worker", Priority: "medium",
	}))
	blockerMust(t, p.repo.UpdateTaskParentID(ctx, restartBlockerID, escalationParent))
	blockerMust(t, p.repo.CreateTaskBlocker(ctx, &models.TaskBlocker{TaskID: "dependent", BlockerTaskID: restartBlockerID}))
}

func (p *blockerFailureRestartProcess) queuedBlockerRequests(t *testing.T) []*officesqlite.WakeupRequest {
	t.Helper()
	requests, err := p.repo.ListQueuedWakeupRequestsForWorkspaceAndSource(
		context.Background(), "assignment-ws", officewakeup.SourceBlockerFailure, 0,
	)
	blockerMust(t, err)
	return requests
}

func (p *blockerFailureRestartProcess) blockerFailureRuns(t *testing.T) []*runmodels.Run {
	t.Helper()
	runs, err := p.svc.ListRuns(context.Background(), "assignment-ws")
	blockerMust(t, err)
	var result []*runmodels.Run
	for _, run := range runs {
		if run.Reason == service.RunReasonTaskBlockerFailed {
			result = append(result, run)
		}
	}
	return result
}

// @covers AC-OFFICE-BLOCKER-POLICY-002.4 AC-OFFICE-BLOCKER-POLICY-002.6 AC-OFFICE-BLOCKER-POLICY-002.8
func TestOfficeBlockerFailureRestartReplay(t *testing.T) {
	for _, resumeHook := range []bool{true, false} {
		name := "resume_hook"
		if !resumeHook {
			name = "missed_resume_hook_recovery_tick"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "office.sqlite")
			first := openBlockerFailureRestartProcess(t, path, true, false)
			first.seedFailure(t)
			_, err := first.pause.Pause(context.Background(), "assignment-ws", "test", "user-1", "user")
			blockerMust(t, err)
			_, err = first.tasks.UpdateTaskState(context.Background(), restartBlockerID, v1.TaskStateFailed)
			blockerMust(t, err)
			requests, err := first.repo.ListQueuedWakeupRequestsForWorkspaceAndSource(
				context.Background(), "assignment-ws", officewakeup.SourceBlockerFailure, 0,
			)
			blockerMust(t, err)
			if len(requests) != 1 {
				t.Fatalf("durable queued blocker requests=%d, want 1", len(requests))
			}
			if got := len(first.blockerFailureRuns(t)); got != 0 {
				t.Fatalf("pre-restart blocker failure runs=%d, want 0 while paused", got)
			}
			var queuedPayload officewakeup.BlockerFailurePayload
			blockerMust(t, json.Unmarshal([]byte(requests[0].Payload), &queuedPayload))
			var occurrenceID string
			for key := range queuedPayload.Alerts {
				occurrenceID = key
			}
			if occurrenceID == "" || queuedPayload.Alerts[occurrenceID].BlockerTaskID != restartBlockerID {
				t.Fatalf("queued failure occurrence=%q payload=%+v", occurrenceID, queuedPayload)
			}
			first.close(t)

			second := openBlockerFailureRestartProcess(t, path, false, resumeHook)
			if resumeHook {
				_, err = second.pause.Resume(context.Background(), "assignment-ws", "restart replay", "user-1", "user")
				blockerMust(t, err)
			} else {
				// Resume committed before the restart, but its replay hook was absent.
				_, err = second.pause.Resume(context.Background(), "assignment-ws", "missed hook", "user-1", "user")
				blockerMust(t, err)
				second.close(t)
				second = openBlockerFailureRestartProcess(t, path, false, false)
			}
			recovery := service.NewOfficeRecoveryHandler(service.NewSchedulerIntegration(second.svc, 0))
			if !resumeHook {
				blockerMust(t, recovery.Tick(context.Background()))
			}
			runs := second.blockerFailureRuns(t)
			if len(runs) != 1 {
				t.Fatalf("recovered blocker failure runs=%d, want exactly one", len(runs))
			}
			alerts := decodeEscalationAlerts(t, runs[0])
			if len(alerts) != 1 || alerts[occurrenceID].BlockerTaskID != restartBlockerID {
				t.Fatalf("replayed alert occurrence %q was not retained: %+v", occurrenceID, alerts)
			}
			blockerMust(t, recovery.Tick(context.Background()))
			if got := len(second.blockerFailureRuns(t)); got != 1 {
				t.Fatalf("runs after repeated replay=%d, want one", got)
			}
			request, err := second.repo.GetWakeupRequest(context.Background(), requests[0].ID)
			blockerMust(t, err)
			if request.Status != officesqlite.WakeupStatusClaimed {
				t.Fatalf("replayed request status=%q, want claimed", request.Status)
			}
		})
	}
}

// @covers AC-OFFICE-BLOCKER-POLICY-002.4 AC-OFFICE-BLOCKER-POLICY-002.8
func TestOfficeBlockerFailureRestartStaleDiscard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "office.sqlite")
	first := openBlockerFailureRestartProcess(t, path, true, false)
	first.seedFailure(t)
	_, err := first.pause.Pause(context.Background(), "assignment-ws", "test", "user-1", "user")
	blockerMust(t, err)
	_, err = first.tasks.UpdateTaskState(context.Background(), restartBlockerID, v1.TaskStateFailed)
	blockerMust(t, err)
	requests, err := first.repo.ListQueuedWakeupRequestsForWorkspaceAndSource(
		context.Background(), "assignment-ws", officewakeup.SourceBlockerFailure, 0,
	)
	blockerMust(t, err)
	if len(requests) != 1 {
		t.Fatalf("durable queued blocker requests=%d, want 1", len(requests))
	}
	_, err = first.tasks.UpdateTaskState(context.Background(), restartBlockerID, v1.TaskStateInProgress)
	blockerMust(t, err)
	first.close(t)

	second := openBlockerFailureRestartProcess(t, path, false, false)
	_, err = second.pause.Resume(context.Background(), "assignment-ws", "missed hook", "user-1", "user")
	blockerMust(t, err)
	blockerMust(t, second.svc.ReplayPendingDeferredAssignments(context.Background()))
	if got := len(second.blockerFailureRuns(t)); got != 0 {
		t.Fatalf("stale blocker failure runs=%d, want 0 after blocker recovery", got)
	}
	request, err := second.repo.GetWakeupRequest(context.Background(), requests[0].ID)
	blockerMust(t, err)
	if request.Status != officesqlite.WakeupStatusSkipped {
		t.Fatalf("stale request status=%q, want skipped", request.Status)
	}
	if got := len(second.queuedBlockerRequests(t)); got != 0 {
		t.Fatalf("stale queued blocker requests=%d, want 0", got)
	}
}
