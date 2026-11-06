package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/handlers"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type remoteAbsenceIntegrationBackend struct {
	lifecycle.ExecutorBackend
	name        executor.Name
	recoveryIDs []string
	createCalls int
}

func (b *remoteAbsenceIntegrationBackend) Name() executor.Name { return b.name }

func (b *remoteAbsenceIntegrationBackend) HealthCheck(context.Context) error { return nil }

func (b *remoteAbsenceIntegrationBackend) CreateInstance(context.Context, *lifecycle.ExecutorCreateRequest) (*lifecycle.ExecutorInstance, error) {
	b.createCalls++
	return nil, fmt.Errorf("remote runtime creation is not part of this integration test")
}

func (b *remoteAbsenceIntegrationBackend) RecoverInstances(context.Context, []*models.ExecutorRunning) ([]*lifecycle.ExecutorInstance, error) {
	return nil, nil
}

func (b *remoteAbsenceIntegrationBackend) StopInstance(context.Context, *lifecycle.ExecutorInstance, bool) error {
	return nil
}

func (b *remoteAbsenceIntegrationBackend) GetInteractiveRunner() *process.InteractiveRunner {
	return nil
}

func (b *remoteAbsenceIntegrationBackend) RequiresCloneURL() bool { return false }

func (b *remoteAbsenceIntegrationBackend) ShouldApplyPreferredShell() bool { return false }

func (b *remoteAbsenceIntegrationBackend) IsAlwaysResumable() bool { return true }

func (b *remoteAbsenceIntegrationBackend) RecoverInstancesDetailed(
	_ context.Context,
	records []*models.ExecutorRunning,
) ([]*lifecycle.ExecutorInstance, map[string]lifecycle.RecoveryCandidateOutcome, error) {
	outcomes := make(map[string]lifecycle.RecoveryCandidateOutcome, len(records))
	for _, record := range records {
		if record == nil {
			continue
		}
		b.recoveryIDs = append(b.recoveryIDs, record.SessionID)
		outcomes[record.SessionID] = lifecycle.RecoveryOutcomeNoMatchingInstance
	}
	return nil, outcomes, nil
}

type remoteAbsenceIntegrationWorkspaceInfo struct {
	snapshot *lifecycle.RemoteRecoverySnapshot
}

func (p remoteAbsenceIntegrationWorkspaceInfo) GetWorkspaceInfoForSession(_ context.Context, taskID, sessionID string) (*lifecycle.WorkspaceInfo, error) {
	return &lifecycle.WorkspaceInfo{TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: p.snapshot.TaskEnvironmentID}, nil
}

func (p remoteAbsenceIntegrationWorkspaceInfo) GetWorkspaceInfoForEnvironment(_ context.Context, environmentID string) (*lifecycle.WorkspaceInfo, error) {
	return &lifecycle.WorkspaceInfo{TaskEnvironmentID: environmentID}, nil
}

func (p remoteAbsenceIntegrationWorkspaceInfo) GetRemoteRecoverySnapshot(context.Context, string) (*lifecycle.RemoteRecoverySnapshot, error) {
	return p.snapshot, nil
}

func TestRemoteRecoveryAbsencePublicRecoveryActions(t *testing.T) {
	for _, action := range []string{"resume", "continue_from_history", "retry_connection", "stop"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
			require.NoError(t, err)
			harness := newBootStateTestHarness(t)
			eventBus := bus.NewMemoryEventBus(log)
			t.Cleanup(eventBus.Close)

			workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
			require.NoError(t, err)
			require.NotEmpty(t, workspaces)
			workflows, err := harness.taskSvc.ListWorkflows(ctx, workspaces[0].ID, true)
			require.NoError(t, err)
			require.NotEmpty(t, workflows)
			steps, err := harness.workflowSvc.ListStepsByWorkflow(ctx, workflows[0].ID)
			require.NoError(t, err)
			require.NotEmpty(t, steps)
			taskResult, err := harness.taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
				WorkspaceID: workspaces[0].ID, WorkflowID: workflows[0].ID,
				WorkflowStepID: steps[0].ID, Title: "Remote recovery absence action",
			})
			require.NoError(t, err)
			taskID := taskResult.Task.ID
			sessionID := "remote-absence-" + action
			incarnationID := "incarnation-" + action
			executionID := "execution-" + action
			environmentID := "environment-" + action
			executorID := "executor-" + action
			executorProfileID := "remote-profile-" + action
			submissionID := "prompt:uncertain-" + action
			streamID := "stream-" + action
			now := time.Now().UTC()
			require.NoError(t, harness.taskRepo.CreateExecutor(ctx, &models.Executor{
				ID: executorID, Name: "Remote recovery test", Type: models.ExecutorTypeRemoteDocker,
				Status: models.ExecutorStatusActive,
			}))
			require.NoError(t, harness.taskRepo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
				ID: executorProfileID, ExecutorID: executorID, Name: "Remote recovery test profile",
			}))
			require.NoError(t, harness.taskRepo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
				ID: environmentID, TaskID: taskID, ExecutorType: string(models.ExecutorTypeRemoteDocker),
				ExecutorID: executorID, ExecutorProfileID: executorProfileID,
				WorkspacePath: "/workspace/" + action, Status: models.TaskEnvironmentStatusReady,
			}))
			session := &models.TaskSession{
				ID: sessionID, TaskID: taskID, QueueIncarnationID: incarnationID,
				AgentProfileID: "mock-agent", ExecutionProfileID: "mock-agent",
				ExecutorID: executorID, ExecutorProfileID: executorProfileID, TaskEnvironmentID: environmentID,
				State: models.TaskSessionStateFailed, StartedAt: now,
			}
			require.NoError(t, harness.taskRepo.CreateTaskSession(ctx, session))
			require.NoError(t, harness.taskRepo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
				ExecutionProfileID: "mock-agent", ExecutorID: executorID,
				Runtime: agentruntime.RuntimeRemoteDocker, Status: models.ExecutorRunningStatusRunning,
				Resumable: true, ResumeToken: "native-conversation-" + action,
				WorktreePath: "/workspace/" + action, Metadata: map[string]interface{}{"owner": "preserved"},
			}))
			submission := &models.AgentDeliverySubmission{
				ID: submissionID, SessionID: sessionID, IncarnationID: incarnationID,
				HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "sha256-original-" + action,
				Payload: []byte("original uncertain prompt: " + action),
				State:   models.DeliverySubmissionInterruptedUnknown, Outcome: "unknown_prompt_outcome",
			}
			_, err = harness.taskRepo.PrepareAgentDeliverySubmission(ctx, submission)
			require.NoError(t, err)
			block := &models.SessionRecoveryBlock{
				SessionID: sessionID, IncarnationID: incarnationID, ExpectedGeneration: 1,
				Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
				ConsumerReference: "agent_delivery", DeliverySubmissionID: submissionID,
				DeliveryStreamID: streamID, DeliveryOutcome: "unknown_prompt_outcome",
			}
			recovery := &models.AgentDeliveryRecovery{
				Phase: models.AgentDeliveryRecoveryUncertain, SessionID: sessionID,
				AgentExecutionID: executionID, SubmissionID: submissionID, StreamID: streamID,
				IncarnationID: incarnationID, HarnessGeneration: 1, PromptGeneration: 1,
				Message: "original uncertain prompt: " + action,
			}
			updated, err := harness.taskRepo.UpsertAgentDeliveryRecovery(ctx, recovery, block)
			require.NoError(t, err)
			require.True(t, updated)
			turnID := "turn-original-" + action
			require.NoError(t, harness.taskRepo.CreateTurn(ctx, &models.Turn{
				ID: turnID, TaskSessionID: sessionID, TaskID: taskID,
			}))
			require.NoError(t, harness.taskRepo.CreateMessage(ctx, &models.Message{
				ID: "message-original-" + action, TaskSessionID: sessionID, TaskID: taskID,
				TurnID: turnID, AuthorType: models.MessageAuthorUser,
				Content: "original transcript: " + action, CreatedAt: now,
			}))

			queueRepository, err := messagequeue.NewSQLiteRepository(harness.db, harness.db)
			require.NoError(t, err)
			queue := messagequeue.NewService(queueRepository, messagequeue.DefaultMaxPerSession, log)
			queued, err := queue.QueueMessage(ctx, sessionID, taskID, "separate queued instruction", "", messagequeue.QueuedByUser, false, nil)
			require.NoError(t, err)
			reserved, exists := queue.ReserveQueued(ctx, sessionID)
			require.True(t, exists)
			require.Equal(t, queued.ID, reserved.ID)
			claims := queueRepository.(interface {
				ListPendingQueueDispatches(context.Context) ([]messagequeue.PendingQueueDispatch, error)
			})
			claimsBefore, err := claims.ListPendingQueueDispatches(ctx)
			require.NoError(t, err)
			require.Len(t, claimsBefore, 1)
			require.Equal(t, queued.ID, claimsBefore[0].Message.ID)

			workspaceSnapshot := &lifecycle.RemoteRecoverySnapshot{
				TaskID: taskID, SessionID: sessionID, SessionState: models.TaskSessionStateFailed,
				AgentExecutionID: executionID, Runtime: string(agentruntime.RuntimeRemoteDocker),
				TaskWorkspaceID: workspaces[0].ID, TaskEnvironmentID: environmentID,
				EnvironmentOwnerTaskID: taskID, EnvironmentOwnershipGen: 1,
				EnvironmentExecutorType: string(agentruntime.RuntimeRemoteDocker),
				EnvironmentExecutorID:   executorID, EnvironmentStatus: "ready",
				RecoverySourcePresent: true, RecoverableDeliveryPending: true,
				RecoveryExecutionID: executionID, RecoverySubmissionID: submissionID,
				RecoveryStreamID: streamID, RecoveryIncarnationID: incarnationID,
				RecoveryHarnessGeneration: 1, RecoveryRevision: recovery.Revision,
			}
			backend := &remoteAbsenceIntegrationBackend{name: executor.NameRemoteDocker}
			runtimes := lifecycle.NewExecutorRegistry(log)
			runtimes.Register(backend)
			agentRegistry := registry.NewRegistry(log)
			agentRegistry.LoadDefaults()
			mockAgent, ok := agentRegistry.Get("mock-agent")
			require.True(t, ok)
			mockAgent.(*agents.MockAgent).SetEnabled(true)
			manager := lifecycle.NewManager(agentRegistry, eventBus, runtimes, nil, nil, nil,
				lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
			manager.SetExecutorRunningWriter(harness.taskRepo)
			manager.SetExecutorProfileReader(harness.taskRepo)
			manager.SetWorkspaceInfoProvider(remoteAbsenceIntegrationWorkspaceInfo{snapshot: workspaceSnapshot})
			manager.SetPassthroughLookup(func(context.Context, string) (bool, bool) { return false, true })
			manager.SetRecoveryDeadline(time.Second)
			guard := lifecycle.NewRecoveryGuard()
			manager.SetRecoveryGuard(guard)
			t.Cleanup(func() { require.NoError(t, manager.Stop()) })
			require.NoError(t, manager.Start(ctx))
			require.Contains(t, backend.recoveryIDs, sessionID)
			require.NoError(t, guard.CheckLaunchAllowed(sessionID), "confirmed absence must release only the startup guard")
			require.Zero(t, backend.createCalls)

			adapter := newLifecycleAdapter(manager, nil, log)
			orchestratorService := orchestrator.NewService(
				orchestrator.DefaultServiceConfig(), eventBus, adapter,
				&taskRepositoryAdapter{repo: harness.taskRepo, svc: harness.taskSvc},
				harness.taskRepo, nil, nil, queue, log,
			)
			orchestratorService.SetTurnService(newTurnServiceAdapter(harness.taskSvc))
			orchestratorService.SetTaskEventPublisher(harness.taskSvc)
			dispatcher := ws.NewDispatcher()
			handlers.NewHandlers(orchestratorService, log).RegisterHandlers(dispatcher)
			route := ws.ActionSessionRecover
			payload := map[string]interface{}{"task_id": taskID, "session_id": sessionID, "action": action}
			if action == "stop" {
				route = ws.ActionSessionStop
				payload = map[string]interface{}{"session_id": sessionID, "reason": "integration test"}
			}
			request, err := ws.NewRequest("remote-absence-"+action, route, payload)
			require.NoError(t, err)
			response, err := dispatcher.Dispatch(ctx, request)
			require.NoError(t, err)
			switch action {
			case "resume":
				require.Equal(t, ws.MessageTypeError, response.Type)
				var errorPayload ws.ErrorPayload
				require.NoError(t, json.Unmarshal(response.Payload, &errorPayload))
				require.Equal(t, ws.ErrorCodeInternalError, errorPayload.Code)
				require.Contains(t, errorPayload.Message, "remote runtime creation is not part of this integration test")
			case "retry_connection":
				require.Equal(t, ws.MessageTypeResponse, response.Type)
				var result orchestrator.SessionDeliveryRecoveryResponse
				require.NoError(t, json.Unmarshal(response.Payload, &result))
				require.Equal(t, orchestrator.SessionDeliveryRecoveryUnavailable, result.Outcome)
				require.Equal(t, "recovery_unavailable", result.Reason)
			case "continue_from_history":
				require.Equal(t, ws.MessageTypeError, response.Type)
				var errorPayload ws.ErrorPayload
				require.NoError(t, json.Unmarshal(response.Payload, &errorPayload))
				require.Contains(t, errorPayload.Message, "remote runtime creation is not part of this integration test")
			case "stop":
				require.Equal(t, ws.MessageTypeResponse, response.Type)
			}
			expectedCreateCalls := 0
			if action == "resume" || action == "continue_from_history" {
				expectedCreateCalls = 1
			}
			require.Equal(t, expectedCreateCalls, backend.createCalls,
				"only the explicit launch actions may request a replacement runtime")

			storedSubmission, err := harness.taskRepo.GetAgentDeliverySubmission(ctx, submissionID)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, storedSubmission.State)
			require.Equal(t, submission.PayloadHash, storedSubmission.PayloadHash)
			require.Equal(t, submission.Payload, storedSubmission.Payload)
			require.Equal(t, "unknown_prompt_outcome", storedSubmission.Outcome)
			storedSession, err := harness.taskRepo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			storedRecovery, exists := models.LoadAgentDeliveryRecovery(storedSession.Metadata)
			require.True(t, exists)
			require.Equal(t, models.AgentDeliveryRecoveryUncertain, storedRecovery.Phase)
			require.Equal(t, submissionID, storedRecovery.SubmissionID)
			storedBlock, err := harness.taskRepo.GetSessionRecoveryBlock(ctx, block.ID)
			require.NoError(t, err)
			require.Equal(t, models.RecoveryBlockOpen, storedBlock.State)
			messages, err := harness.taskRepo.ListMessages(ctx, sessionID)
			require.NoError(t, err)
			require.Len(t, messages, 1)
			require.Equal(t, "original transcript: "+action, messages[0].Content)
			claimsAfter, err := claims.ListPendingQueueDispatches(ctx)
			require.NoError(t, err)
			require.Equal(t, claimsBefore, claimsAfter)
			currentRow, err := harness.taskRepo.GetExecutorRunningBySessionID(ctx, sessionID)
			require.NoError(t, err)
			require.Equal(t, models.ExecutorRunningStatusStopped, currentRow.Status)
			require.Equal(t, "native-conversation-"+action, currentRow.ResumeToken)
			require.Equal(t, "/workspace/"+action, currentRow.WorktreePath)
			require.Equal(t, "preserved", currentRow.Metadata["owner"])
		})
	}
}
