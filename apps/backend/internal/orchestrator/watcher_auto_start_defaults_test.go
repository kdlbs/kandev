package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/sentry"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type watcherAutoStartOptions struct {
	workspaceAgentProfileID      string
	workflowAgentProfileID       string
	stepAgentProfileID           string
	sessionTargetSourceProfileID string
	executorProfileID            string
	launchErr                    error
	activeSibling                bool
	failFirstTaskRead            error
}

type watcherAutoStartFixture struct {
	svc       *Service
	repo      *sqliterepo.Repository
	steps     *mockStepGetter
	tasks     *mockTaskRepo
	agent     *mockAgentManager
	sentry    *fakeSentryService
	publisher *recordingTaskUpdatedPublisher
	creator   *persistedWatcherTaskCreator
	event     *sentry.NewSentryIssueEvent
}

type persistedWatcherTaskCreator struct {
	repo            *sqliterepo.Repository
	tasks           *mockTaskRepo
	activeSibling   bool
	createdTaskID   string
	createCallCount int
}

func (c *persistedWatcherTaskCreator) CreateIssueTask(ctx context.Context, req *IssueTaskRequest) (*models.Task, error) {
	c.createCallCount++
	now := time.Now().UTC()
	task := &models.Task{
		ID:             "watch-task",
		WorkspaceID:    req.WorkspaceID,
		WorkflowID:     req.WorkflowID,
		WorkflowStepID: req.WorkflowStepID,
		Title:          req.Title,
		Description:    req.Description,
		State:          v1.TaskStateTODO,
		Metadata:       req.Metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := c.repo.CreateTask(ctx, task); err != nil {
		return nil, err
	}
	c.createdTaskID = task.ID
	c.tasks.mu.Lock()
	c.tasks.tasks[task.ID] = &v1.Task{
		ID: task.ID, WorkspaceID: task.WorkspaceID, Title: task.Title,
		Description: task.Description, State: task.State,
	}
	c.tasks.mu.Unlock()
	if c.activeSibling {
		if err := c.repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: "watch-task-active-sibling", TaskID: task.ID,
			State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
		}); err != nil {
			return nil, err
		}
	}
	return task, nil
}

type failFirstWatcherTaskReadRepo struct {
	*sqliterepo.Repository
	err    error
	failed atomic.Bool
}

func (r *failFirstWatcherTaskReadRepo) GetTask(ctx context.Context, taskID string) (*models.Task, error) {
	if r.failed.CompareAndSwap(false, true) {
		return nil, r.err
	}
	return r.Repository.GetTask(ctx, taskID)
}

func newWatcherAutoStartFixture(t *testing.T, options watcherAutoStartOptions) *watcherAutoStartFixture {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedExecutorRow(t, repo, models.ExecutorIDLocal, models.ExecutorTypeLocal)
	now := time.Now().UTC()
	workspace := &models.Workspace{
		ID: "watch-workspace", Name: "Watcher workspace",
		DefaultExecutorID: strPtr(models.ExecutorIDLocal),
		CreatedAt:         now, UpdatedAt: now,
	}
	if options.workspaceAgentProfileID != "" {
		workspace.DefaultAgentProfileID = strPtr(options.workspaceAgentProfileID)
	}
	if err := repo.CreateWorkspace(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "watch-workflow", WorkspaceID: workspace.ID, Name: "Watch workflow",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if options.executorProfileID != "" {
		if err := repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
			ID: options.executorProfileID, ExecutorID: models.ExecutorIDLocal,
			Name: "Watch executor profile", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("create executor profile: %v", err)
		}
	}
	steps := newMockStepGetter()
	steps.workflowAgentProfileID = options.workflowAgentProfileID
	watchStep := &wfmodels.WorkflowStep{
		ID: "watch-step", WorkflowID: "watch-workflow", Position: 1, Name: "Watch step",
		AgentProfileID: options.stepAgentProfileID,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}
	if options.sessionTargetSourceProfileID != "" {
		steps.steps["source-step"] = &wfmodels.WorkflowStep{
			ID: "source-step", WorkflowID: "watch-workflow", Position: 0,
			AgentProfileID: options.sessionTargetSourceProfileID,
		}
		watchStep.SessionTarget = &wfmodels.WorkflowSessionTarget{
			Kind: wfmodels.WorkflowSessionTargetStep, StepID: "source-step",
		}
	}
	steps.steps["watch-step"] = watchStep
	tasks := newMockTaskRepo()
	var launchErr = options.launchErr
	agent := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			if launchErr != nil {
				return nil, launchErr
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: "watch-launch-" + req.SessionID}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, steps, tasks, agent)
	publisher := &recordingTaskUpdatedPublisher{}
	svc.SetTaskEventPublisher(publisher)
	sentryService := &fakeSentryService{reserveOK: true}
	svc.SetSentryService(sentryService)
	creator := &persistedWatcherTaskCreator{
		repo: repo, tasks: tasks, activeSibling: options.activeSibling,
	}
	svc.SetIssueTaskCreator(creator)
	if options.failFirstTaskRead != nil {
		svc.repo = &failFirstWatcherTaskReadRepo{Repository: repo, err: options.failFirstTaskRead}
	}
	event := sampleSentryEvent()
	event.WorkspaceID = workspace.ID
	event.WorkflowID = "watch-workflow"
	event.WorkflowStepID = "watch-step"
	event.AgentProfileID = ""
	event.ExecutorProfileID = options.executorProfileID
	event.IssueWatchID = "watch-source"
	return &watcherAutoStartFixture{
		svc: svc, repo: repo, steps: steps, tasks: tasks, agent: agent,
		sentry: sentryService, publisher: publisher, creator: creator, event: event,
	}
}

func (f *watcherAutoStartFixture) dispatch() {
	f.svc.watcherCoordinator.Dispatch(
		context.Background(), NewSentryWatcherSource(f.sentry, f.svc.logger), f.event,
	)
}

func assertWatcherTaskMarkedFailed(t *testing.T, f *watcherAutoStartFixture, want bool) {
	t.Helper()
	task, err := f.repo.GetTask(context.Background(), f.creator.createdTaskID)
	require.NoError(t, err)
	_, marked := task.Metadata[models.MetaKeyAutoStartFailed]
	require.Equal(t, want, marked)
	if want {
		require.Contains(t, f.publisher.updatedTaskIDs, task.ID)
	}
}

// @covers AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.1, AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.3
func TestWatcherAutoStart_UsesWorkspaceDefault(t *testing.T) {
	f := newWatcherAutoStartFixture(t, watcherAutoStartOptions{
		workspaceAgentProfileID: "workspace-agent",
	})
	f.dispatch()

	sessions, err := f.repo.ListTaskSessions(context.Background(), f.creator.createdTaskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1, "watch dispatch must start the task before it is opened")
	require.Equal(t, "workspace-agent", sessions[0].AgentProfileID)
	require.Equal(t, models.ExecutorIDLocal, sessions[0].ExecutorID)
	require.Empty(t, sessions[0].ExecutorProfileID)
	require.Equal(t, []string{"watch-source:PROJ-1:watch-task"}, f.sentry.gotAssign)
}

// @covers AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.2, AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.3
func TestWatcherAutoStart_PreservesConfiguredProfiles(t *testing.T) {
	const executorProfileID = "watch-executor-profile"
	cases := []struct {
		name                         string
		workspaceAgentProfileID      string
		watchAgentProfileID          string
		stepAgentProfileID           string
		workflowAgentProfileID       string
		sessionTargetSourceProfileID string
		validProfileID               string
		wantAgentProfileID           string
	}{
		{
			name:                "watch selection",
			watchAgentProfileID: "watch-agent", wantAgentProfileID: "watch-agent",
		},
		{
			name:                "step profile overrides watch selection",
			watchAgentProfileID: "watch-agent", stepAgentProfileID: "step-agent",
			workflowAgentProfileID: "workflow-agent", wantAgentProfileID: "step-agent",
		},
		{
			name:                "workflow profile overrides watch selection",
			watchAgentProfileID: "watch-agent", workflowAgentProfileID: "workflow-agent",
			wantAgentProfileID: "workflow-agent",
		},
		{
			name:                         "session target resolves earlier fixed source profile",
			workspaceAgentProfileID:      "unavailable-workspace-agent",
			sessionTargetSourceProfileID: "source-agent",
			validProfileID:               "source-agent",
			wantAgentProfileID:           "source-agent",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newWatcherAutoStartFixture(t, watcherAutoStartOptions{
				workspaceAgentProfileID:      tc.workspaceAgentProfileID,
				executorProfileID:            executorProfileID,
				stepAgentProfileID:           tc.stepAgentProfileID,
				workflowAgentProfileID:       tc.workflowAgentProfileID,
				sessionTargetSourceProfileID: tc.sessionTargetSourceProfileID,
			})
			if tc.validProfileID != "" {
				f.svc.SetProfileExecutionResolver(newExplicitOnlyProfileExecutionResolver(t, tc.validProfileID))
			}
			f.event.AgentProfileID = tc.watchAgentProfileID
			f.dispatch()

			sessions, err := f.repo.ListTaskSessions(context.Background(), f.creator.createdTaskID)
			require.NoError(t, err)
			require.Len(t, sessions, 1)
			require.Equal(t, tc.wantAgentProfileID, sessions[0].AgentProfileID)
			require.Equal(t, executorProfileID, sessions[0].ExecutorProfileID)
			assertWatcherTaskMarkedFailed(t, f, false)
		})
	}
}

// @covers AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.4, AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.5, AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.6, AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.8
func TestWatcherAutoStart_ReportsFailure(t *testing.T) {
	lookupErr := errors.New("temporary task lookup failure")
	launchErr := errors.New("agent launch failed")
	cases := []struct {
		name          string
		options       watcherAutoStartOptions
		wantMarker    bool
		wantNoSession bool
		fillCeiling   bool
	}{
		{name: "no resolved profile", wantMarker: true, wantNoSession: true},
		{name: "task lookup failure", options: watcherAutoStartOptions{failFirstTaskRead: lookupErr}, wantMarker: true},
		{name: "launch failure", options: watcherAutoStartOptions{launchErr: launchErr}, wantMarker: true},
		{name: "capacity deferral", options: watcherAutoStartOptions{workspaceAgentProfileID: "workspace-agent"}, fillCeiling: true},
		{name: "active sibling", options: watcherAutoStartOptions{activeSibling: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newWatcherAutoStartFixture(t, tc.options)
			if tc.name == "launch failure" {
				f.event.AgentProfileID = "watch-agent"
			}
			if tc.fillCeiling {
				f.svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
				decision := f.svc.sessionCeiling.admit(context.Background(), admissionRequest{
					taskID: "ceiling-occupier", sessionID: "ceiling-occupier-session",
					origin: launchOriginAutomatic, seam: "watcher-test",
				})
				require.True(t, decision.admitted)
			}
			f.dispatch()
			assertWatcherTaskMarkedFailed(t, f, tc.wantMarker)
			if tc.wantNoSession {
				sessions, err := f.repo.ListTaskSessions(context.Background(), f.creator.createdTaskID)
				require.NoError(t, err)
				require.Empty(t, sessions, "an unresolved profile must fail before session preparation")
			}
			if tc.name == "capacity deferral" {
				record, _, err := f.repo.GetTaskDeferredLaunch(context.Background(), f.creator.createdTaskID)
				require.NoError(t, err)
				require.NotNil(t, record, "capacity refusal must keep the existing deferred launch")
				require.Equal(t, true, record[models.CeilingDeferredKey])
			}
		})
	}
}
