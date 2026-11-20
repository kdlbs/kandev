package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestGetTaskSessionStatusPreparedLaunchResumeTiming(t *testing.T) {
	for _, test := range []struct {
		name       string
		launchAge  time.Duration
		wantResume bool
	}{
		{name: "wait while launch is within grace", launchAge: time.Second, wantResume: false},
		{name: "recover after launch grace expires", launchAge: executor.AgentStartupGracePeriod + time.Second, wantResume: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

			session, err := repo.GetTaskSession(ctx, "session1")
			if err != nil {
				t.Fatalf("load session: %v", err)
			}
			session.AgentProfileID = "profile1"
			if err := repo.UpdateTaskSession(ctx, session); err != nil {
				t.Fatalf("update session: %v", err)
			}
			updatedAt := time.Now().UTC().Add(-test.launchAge)
			if _, err := repo.DB().ExecContext(ctx,
				`UPDATE task_sessions SET updated_at = ? WHERE id = ?`, updatedAt, session.ID,
			); err != nil {
				t.Fatalf("set session launch age: %v", err)
			}

			now := time.Now().UTC()
			if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID:               "runtime1",
				SessionID:        session.ID,
				TaskID:           session.TaskID,
				AgentExecutionID: "execution1",
				Status:           models.ExecutorRunningStatusPrepared,
				Resumable:        true,
				CreatedAt:        now,
				UpdatedAt:        now,
			}); err != nil {
				t.Fatalf("upsert prepared runtime: %v", err)
			}

			taskRepo := newMockTaskRepo()
			taskRepo.tasks["task1"] = &v1.Task{ID: "task1", State: v1.TaskStateInProgress}
			agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
			svc.executor = withTestCapabilities(executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{}))

			status, err := svc.GetTaskSessionStatus(ctx, "task1", "session1")
			if err != nil {
				t.Fatalf("GetTaskSessionStatus: %v", err)
			}
			if status.NeedsResume != test.wantResume {
				t.Fatalf("NeedsResume = %v, want %v", status.NeedsResume, test.wantResume)
			}
		})
	}
}
