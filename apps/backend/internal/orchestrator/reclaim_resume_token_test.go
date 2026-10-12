package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// rowRuleCleanupAgentManager applies the lifecycle stale-execution cleanup
// rule to the executors_running row it releases: a row that holds a resume
// token or still claims a running execution is repaired to stopped, and any
// other row is deleted.
type rowRuleCleanupAgentManager struct {
	*mockAgentManager
	repo     *sqliterepo.Repository
	cleanups atomic.Int32
}

func (m *rowRuleCleanupAgentManager) CleanupStaleExecutionBySessionIDIfCurrent(
	ctx context.Context,
	sessionID, expectedExecutionID string,
	_ time.Time,
) error {
	m.cleanups.Add(1)
	row, err := m.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.AgentExecutionID != expectedExecutionID {
		return nil
	}
	if row.ResumeToken != "" || row.Status == models.ExecutorRunningStatusRunning {
		return m.repo.RepairExecutorRunningDead(ctx, sessionID)
	}
	return m.repo.DeleteExecutorRunningBySessionID(ctx, sessionID)
}

func seedIdleReclaimRow(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID, status, token string) {
	t.Helper()
	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID:               sessionID,
		SessionID:        sessionID,
		TaskID:           taskID,
		AgentExecutionID: "exec-" + sessionID,
		ResumeToken:      token,
		Resumable:        true,
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           status,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("upsert executor row: %v", err)
	}
}

// TestReclaimIdleSessionRowOutcomeByResumeToken covers the resume-token
// dimension of idle reclaim against the lifecycle cleanup row rule. A
// resumable session (WaitingForInput, Idle) must keep its executors_running
// row; the cleanup would delete a tokenless row whose status is not running,
// so reclaim leaves that runtime alone. A token-bearing or running row is
// repaired in place, and a tokenless Completed row may still be pruned.
func TestReclaimIdleSessionRowOutcomeByResumeToken(t *testing.T) {
	tests := []struct {
		name        string
		state       models.TaskSessionState
		status      string
		token       string
		wantCleanup bool
		wantRow     bool
		wantStatus  string
	}{
		{
			name:       "prepared waiting session without token keeps its runtime and row",
			state:      models.TaskSessionStateWaitingForInput,
			status:     models.ExecutorRunningStatusPrepared,
			wantRow:    true,
			wantStatus: models.ExecutorRunningStatusPrepared,
		},
		{
			name:       "idle office session without token keeps its runtime and row",
			state:      models.TaskSessionStateIdle,
			status:     models.ExecutorRunningStatusReady,
			wantRow:    true,
			wantStatus: models.ExecutorRunningStatusReady,
		},
		{
			name:        "waiting session with token is reclaimed and repaired",
			state:       models.TaskSessionStateWaitingForInput,
			status:      models.ExecutorRunningStatusReady,
			token:       "rt-waiting",
			wantCleanup: true,
			wantRow:     true,
			wantStatus:  models.ExecutorRunningStatusStopped,
		},
		{
			name:        "running waiting session without token is reclaimed and repaired",
			state:       models.TaskSessionStateWaitingForInput,
			status:      models.ExecutorRunningStatusRunning,
			wantCleanup: true,
			wantRow:     true,
			wantStatus:  models.ExecutorRunningStatusStopped,
		},
		{
			name:        "completed session without token is reclaimed and pruned",
			state:       models.TaskSessionStateCompleted,
			status:      models.ExecutorRunningStatusReady,
			wantCleanup: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task-reclaim", "session-reclaim", tt.state)
			seedIdleReclaimRow(t, repo, "task-reclaim", "session-reclaim", tt.status, tt.token)
			manager := &rowRuleCleanupAgentManager{
				mockAgentManager: &mockAgentManager{isAgentRunning: false},
				repo:             repo,
			}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			svc.turnService = &inactiveTurnService{}

			if err := svc.reclaimIdleSession(ctx, "session-reclaim"); err != nil {
				t.Fatalf("reclaimIdleSession: %v", err)
			}

			row, err := repo.GetExecutorRunningBySessionID(ctx, "session-reclaim")
			switch {
			case !tt.wantRow:
				if !errors.Is(err, models.ErrExecutorRunningNotFound) {
					t.Fatalf("row after reclaim: err = %v, want not found", err)
				}
			case err != nil:
				t.Fatalf("executor row lost by idle reclaim: %v", err)
			case row.Status != tt.wantStatus || row.ResumeToken != tt.token:
				t.Fatalf("row after reclaim: status=%q token=%q, want status=%q token=%q",
					row.Status, row.ResumeToken, tt.wantStatus, tt.token)
			}
			if got := manager.cleanups.Load() > 0; got != tt.wantCleanup {
				t.Fatalf("runtime cleanup called = %v, want %v", got, tt.wantCleanup)
			}
		})
	}
}

// TestPromptAfterIdleReclaimLaunchesPreparedSession follows a prepared
// session whose agent has never started (WaitingForInput, tokenless
// prepared row) through an idle reclaim and a later prompt. The prompt must
// reach an agent launch instead of being classified as waiting for a runtime
// launch that nothing will perform.
func TestPromptAfterIdleReclaimLaunchesPreparedSession(t *testing.T) {
	oldReadyTimeout := agentPromptReadyTimeout
	oldReadyInterval := agentPromptReadyInterval
	agentPromptReadyTimeout = 20 * time.Millisecond
	agentPromptReadyInterval = time.Millisecond
	t.Cleanup(func() {
		agentPromptReadyTimeout = oldReadyTimeout
		agentPromptReadyInterval = oldReadyInterval
	})

	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-prepared", "session-prepared", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, "session-prepared")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	seedIdleReclaimRow(t, repo, "task-prepared", "session-prepared", models.ExecutorRunningStatusPrepared, "")

	var launches atomic.Int32
	errLaunchRefused := errors.New("launch refused by test")
	manager := &rowRuleCleanupAgentManager{
		mockAgentManager: &mockAgentManager{
			repoForExecutionLookup: repo,
			isAgentRunningFn:       func(context.Context, string) bool { return false },
			isAgentReadyFn:         func(context.Context, string) bool { return false },
			launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				launches.Add(1)
				return nil, errLaunchRefused
			},
		},
		repo: repo,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	svc.turnService = &inactiveTurnService{}

	if err := svc.reclaimIdleSession(ctx, "session-prepared"); err != nil {
		t.Fatalf("reclaimIdleSession: %v", err)
	}

	_, err = svc.PromptTask(ctx, "task-prepared", "session-prepared", "hello", "", false, nil, false)
	if errors.Is(err, ErrSessionRuntimeUnavailable) {
		t.Fatalf("prompt after idle reclaim is parked as waiting for a runtime launch: %v", err)
	}
	if launches.Load() == 0 {
		t.Fatalf("prompt after idle reclaim did not attempt an agent launch (err: %v)", err)
	}
}
