package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const ceilingReplayRetainedFailureLog = "ceiling retry replay failed for a non-ceiling reason; will retry on a later sweep"

type launchFailureReplayFixture struct {
	svc    *Service
	repo   *tasksqlite.Repository
	logs   *observer.ObservedLogs
	taskID string

	mu       sync.Mutex
	launches int
}

func (f *launchFailureReplayFixture) launchCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.launches
}

func (f *launchFailureReplayFixture) ceilingDeferred(t *testing.T) bool {
	t.Helper()
	return models.HasCeilingDeferredIntent(&models.Task{Metadata: map[string]interface{}{
		models.MetaKeyDeferredLaunch: deferredLaunchOf(t, f.svc, f.taskID),
	}})
}

// newLaunchFailureReplayFixture queues an automatic workflow start behind the
// ceiling and makes every agent launch fail with launchErr.
func newLaunchFailureReplayFixture(t *testing.T, taskID string, launchErr error) *launchFailureReplayFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	repo := setupTestRepo(t)
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Test Workflow", CreatedAt: now, UpdatedAt: now})
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step-auto",
		Title: "launch failure", Description: "desc", State: v1.TaskStateScheduling,
		CreatedAt: now, UpdatedAt: now,
	}))

	steps := newMockStepGetter()
	steps.steps["step-auto"] = &wfmodels.WorkflowStep{
		ID: "step-auto", WorkflowID: "wf1", Name: "Auto",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
	}
	tasks := newMockTaskRepo()
	seedMockTaskState(tasks, taskID, v1.TaskStateScheduling)

	core, logs := observer.New(zapcore.WarnLevel)
	observed, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)

	f := &launchFailureReplayFixture{repo: repo, logs: logs, taskID: taskID}
	agent := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			f.mu.Lock()
			f.launches++
			f.mu.Unlock()
			return nil, launchErr
		},
	}
	f.svc = createTestServiceWithScheduler(repo, steps, tasks, agent)
	f.svc.logger = observed
	f.svc.sessionCeiling = newSessionCeilingController(unlimitedSessionCeiling, nil, nil)

	record := models.CeilingRecordKeys(models.CeilingDeferral{
		Kind: models.CeilingLaunchStart,
		Payload: map[string]interface{}{
			metaKeyPrompt:         "start the step",
			metaKeyAgentProfileID: "profile-1",
			metaKeyWorkflowStepID: "step-auto",
			"origin":              string(launchOriginAutomatic),
		},
		Origin:     string(launchOriginAutomatic),
		ReasonCode: ceilingReasonRefused,
		QueuedAt:   now,
	})
	_, _, err = repo.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, tasksqlite.AbsentDeferredLaunch(), record)
	require.NoError(t, err)
	return f
}

// @covers AC-AGENTS-SESSION-CEILING-001.11
func TestCeilingSweepDropsAStartWhoseLaunchFailsCommandValidation(t *testing.T) {
	ctx := context.Background()
	f := newLaunchFailureReplayFixture(t, "launch-invalid-task",
		&routingerr.BuiltCommandValidationError{Command: "agent", Cause: errors.New("agent command cannot be empty")})

	f.svc.drainDeferredCeilingLaunches(ctx)
	require.Equal(t, 1, f.launchCount(), "the first sweep replays the queued start once")
	require.False(t, f.ceilingDeferred(t),
		"a start whose launch fails command validation must be dropped, not kept for the next sweep")

	f.svc.drainDeferredCeilingLaunches(ctx)
	require.Equal(t, 1, f.launchCount(), "a dropped start must not launch again")
	sessions, err := f.repo.ListTaskSessions(ctx, f.taskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1, "the invalid command must leave one failed session, not one per sweep")
}

// @covers AC-AGENTS-SESSION-CEILING-001.5
func TestCeilingSweepKeepsATransientLaunchFailureAndLogsItsCause(t *testing.T) {
	ctx := context.Background()
	f := newLaunchFailureReplayFixture(t, "launch-transient-task",
		errors.New("connection reset by peer while validate agent command: was pending"))

	f.svc.drainDeferredCeilingLaunches(ctx)
	require.Equal(t, 1, f.launchCount())
	require.True(t, f.ceilingDeferred(t), "a transient launch failure keeps the record for a later sweep")

	entries := f.logs.FilterMessage(ceilingReplayRetainedFailureLog).All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, f.taskID, fields["task_id"])
	require.Contains(t, fields["error"], "connection reset by peer", "the warning must carry the failure")
	require.NotEmpty(t, fields["detail"], "the warning must say which replay step failed")
}
