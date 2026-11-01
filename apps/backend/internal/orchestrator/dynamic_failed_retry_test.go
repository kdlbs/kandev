package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	runtimekind "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// A dynamic session whose launch failed keeps its route claim "starting" with
// no owner. Normal cleanup leaves a stopped executors_running row behind for
// conversation and workspace resume. An explicit Retry must turn that stranded
// claim into a resumable route instead of answering "recovery pending" forever.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.24
func TestApplyRouteActionRetryRecoversFailedStartingRoute(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-failed-start-retry"
		sessionID   = "session-failed-start-retry"
		executionID = "execution-failed-start-retry"
	)
	tests := []struct {
		name        string
		running     bool
		noRow       bool
		neverSeen   bool
		rowStatus   string
		wantPending bool
	}{
		{name: "stopped local history", rowStatus: models.ExecutorRunningStatusStopped},
		// A launch that failed before agentctl reported an endpoint or PID
		// persists its terminal row without a last_seen_at stamp.
		{name: "stopped local history never seen", neverSeen: true, rowStatus: models.ExecutorRunningStatusStopped},
		{name: "live runtime", running: true, rowStatus: models.ExecutorRunningStatusStopped, wantPending: true},
		{name: "runtime record not stopped", rowStatus: "ready", wantPending: true},
		// Without an executor row the manual relaunch cold-resumes and is
		// refused for the missing record, so Retry must keep the guard.
		{name: "executor row absent", noRow: true, wantPending: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateFailed)
			taskRepo := newMockTaskRepo()
			seedMockTaskState(taskRepo, taskID, v1.TaskStateFailed)
			running := tt.running
			agentMgr := &mockAgentManager{isAgentRunningFn: func(context.Context, string) bool { return running }}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
			engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
			svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
			decision := seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, executionID)
			seenAt := time.Now().UTC()
			lastSeenAt := &seenAt
			if tt.neverSeen {
				lastSeenAt = nil
			}
			if !tt.noRow {
				if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
					ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
					ExecutorID: models.ExecutorIDWorktree, Runtime: runtimekind.RuntimeStandalone,
					Status: tt.rowStatus, LastSeenAt: lastSeenAt,
				}); err != nil {
					t.Fatalf("seed executors_running: %v", err)
				}
			}
			// The production Retry handler resumes the existing route state.
			svc.SetRouteActionHandler(func(ctx context.Context, request RouteActionRequest) (*RouteActionResult, error) {
				if _, err := engine.ResumePendingNow(ctx, request.SessionID, request.ExpectedGeneration); err != nil {
					return nil, err
				}
				return &RouteActionResult{SessionID: request.SessionID}, nil
			})

			_, err := svc.ApplyRouteAction(ctx, RouteActionRequest{
				SessionID: sessionID, Action: RouteActionRetry, ExpectedGeneration: decision.Generation,
			})

			if tt.wantPending {
				if !errors.Is(err, dynamicruntime.ErrRecoveryPending) {
					t.Fatalf("ApplyRouteAction error = %v, want recovery pending", err)
				}
				state, loadErr := repo.LoadRouteState(ctx, sessionID)
				if loadErr != nil || state == nil || state.Status != dynamicRouteStatusStarting {
					t.Fatalf("route state = %#v, %v; want the starting claim left untouched", state, loadErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ApplyRouteAction retry of a failed starting route: %v", err)
			}
			row, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
			if err != nil || row == nil || row.Status != models.ExecutorRunningStatusStopped {
				t.Fatalf("executors_running = %#v, %v; want the stopped history preserved", row, err)
			}
		})
	}
}

// After the route is resumed, the manual relaunch must be able to reopen the
// failed session for the retried prompt. Automatic relaunches and stale
// generations still refuse a terminal session.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.24
func TestResetDynamicRelaunchSessionForRecoveryReopensOnlyManualFailedRetry(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name        string
		target      models.TaskSessionState
		generation  int64
		allowFailed bool
		wantChanged bool
	}{
		{name: "manual retry", target: models.TaskSessionStateWaitingForInput, generation: 1, allowFailed: true, wantChanged: true},
		{name: "automatic relaunch", target: models.TaskSessionStateWaitingForInput, generation: 1},
		{name: "prepared relaunch", target: models.TaskSessionStateCreated, generation: 1, allowFailed: true},
		{name: "stale generation", target: models.TaskSessionStateWaitingForInput, generation: 2, allowFailed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)
			session, err := repo.GetTaskSession(ctx, "session1")
			if err != nil {
				t.Fatalf("GetTaskSession: %v", err)
			}
			session.RouteGeneration = 1
			if err := repo.UpdateTaskSession(ctx, session); err != nil {
				t.Fatalf("UpdateTaskSession: %v", err)
			}
			svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

			changed := svc.resetDynamicRelaunchSessionForRecovery(ctx, "session1", tt.target, tt.generation, tt.allowFailed)

			if changed != tt.wantChanged {
				t.Fatalf("reset changed = %v, want %v", changed, tt.wantChanged)
			}
		})
	}
}

// The retried prompt reschedules a task whose previous attempt failed.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.24
func TestScheduleTaskForSessionAdmitsRetryOfFailedTask(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-failed", "session-retry", models.TaskSessionStateWaitingForInput)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-failed", v1.TaskStateFailed)
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	if err := svc.scheduleTaskForSession(ctx, "task-failed", "session-retry"); err != nil {
		t.Fatalf("scheduleTaskForSession: %v", err)
	}
	task, err := taskRepo.GetTask(ctx, "task-failed")
	if err != nil || task == nil || task.State != v1.TaskStateScheduling {
		t.Fatalf("task = %#v, %v; want SCHEDULING", task, err)
	}
}
