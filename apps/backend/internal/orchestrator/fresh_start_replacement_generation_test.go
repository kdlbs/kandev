package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

const (
	freshStartTaskID      = "task-fresh-start-generation"
	freshStartSessionID   = "session-fresh-start-generation"
	freshStartIncarnation = "incarnation-fresh-start"
)

func seedCommittedFirstGeneration(t *testing.T) (*Service, *sqliterepo.Repository, watcher.ACPSessionEventData) {
	t.Helper()
	svc, repo, data := freshStartCommitFixture(t)
	require.NoError(t, repo.UpdateResumeToken(t.Context(), data.SessionID, data.AgentExecutionID, data.ACPSessionID, ""))
	return svc, repo, data
}

func freshStartSessionCreated(generation uint64, replaces bool) watcher.ACPSessionEventData {
	return watcher.ACPSessionEventData{
		TaskID:                             freshStartTaskID,
		SessionID:                          freshStartSessionID,
		AgentExecutionID:                   "fresh-start-execution",
		ACPSessionID:                       "replacement-native",
		DeliveryIncarnationID:              freshStartIncarnation,
		DeliveryHarnessGeneration:          generation,
		DeliveryReplacesNativeConversation: replaces,
	}
}

func TestFreshStartSessionCreatedCommitsReplacementGeneration(t *testing.T) {
	svc, repo, data := seedCommittedFirstGeneration(t)

	svc.persistInitialHarnessGeneration(context.Background(), data, true)

	current, err := repo.GetCurrentHarnessSessionGeneration(context.Background(), freshStartSessionID, data.DeliveryIncarnationID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Generation != 2 || current.PredecessorGeneration != 1 ||
		current.NativeSessionID != "replacement-native" || current.CreationReason != "native_replaced" {
		t.Fatalf("current generation = %+v, want committed replacement 2 after 1", current)
	}
}

func TestSessionCreatedWithoutReplacementKeepsCommittedGeneration(t *testing.T) {
	cases := []struct {
		name string
		data watcher.ACPSessionEventData
	}{
		{"unmarked successor", freshStartSessionCreated(2, false)},
		{"non-adjacent replacement", freshStartSessionCreated(3, true)},
		{"same-generation replacement", freshStartSessionCreated(1, true)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, data := seedCommittedFirstGeneration(t)

			tc.data.DeliveryIncarnationID = data.DeliveryIncarnationID
			svc.persistInitialHarnessGeneration(context.Background(), tc.data, true)

			current, err := repo.GetCurrentHarnessSessionGeneration(context.Background(), freshStartSessionID, data.DeliveryIncarnationID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Generation != 1 || current.NativeSessionID != "previous-native" {
				t.Fatalf("current generation = %+v, want untouched generation 1", current)
			}
		})
	}
}

func TestRecoverSessionFreshStartLaunchesReplacementGeneration(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "fresh-start-generation-task"
		sessionID = "fresh-start-generation-session"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateFailed)
	task, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	task.WorkspaceID = "ws1"
	require.NoError(t, repo.UpdateTask(ctx, task))
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID = "fresh-start-profile"
	session.TaskEnvironmentID = "fresh-start-generation-environment"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	incarnationID := session.QueueIncarnationID
	require.NotEmpty(t, incarnationID)
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: session.TaskEnvironmentID, TaskID: taskID,
		ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
	}))
	seedExecutorRunning(t, repo, sessionID, taskID, "fresh-start-previous-execution")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyInitialPromptSubmission, map[string]interface{}{
		"version": 1, "content": "original request", "state": "pending",
	}))
	now := time.Now().UTC()
	committed, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: sessionID, IncarnationID: incarnationID, Generation: 1,
		NativeSessionID: "previous-native", CreationReason: "native_started", CreatedAt: now, CommittedAt: now,
	}, 0)
	require.NoError(t, err)
	require.True(t, committed)

	launched := make(chan *executor.LaunchAgentRequest, 1)
	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
		isAgentReadyFn:         func(context.Context, string) bool { return true },
		launchAgentFunc: func(_ context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			copied := *request
			launched <- &copied
			return &executor.LaunchAgentResponse{AgentExecutionID: "fresh-start-generation-execution"}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, WorkspaceID: "ws1", Title: "Fresh start task", State: v1.TaskStateInProgress}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.messageCreator = &mockMessageCreator{}
	svc.turnService = &repoTurnService{repo: repo}

	recoveryDone := make(chan error, 1)
	go func() {
		_, recoverErr := svc.RecoverSessionWithOptions(ctx, taskID, sessionID, "fresh_start", RecoverSessionOptions{})
		recoveryDone <- recoverErr
	}()
	var request *executor.LaunchAgentRequest
	select {
	case request = <-launched:
	case err := <-recoveryDone:
		t.Fatalf("fresh start ended before launch: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the fresh-start launch")
	}
	require.True(t, request.ReplaceNativeConversation)
	require.Empty(t, request.ACPSessionID, "fresh start must not load the replaced native conversation")
	require.EqualValues(t, 2, request.DeliveryHarnessGeneration)
	require.Equal(t, incarnationID+":g2", request.DeliveryStreamID)

	svc.invalidateResumeAttempt(sessionID)
	select {
	case <-recoveryDone:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled fresh start did not settle")
	}
}
