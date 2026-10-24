package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type retentionRetryManager struct {
	*durableDeliveryTestAgentManager
	repo        *sqliterepo.Repository
	accepted    []*models.AgentDeliverySubmission
	proofErrors []error
}

func (m *retentionRetryManager) PromptAgentWithDispatchCallback(
	ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment,
	dispatchOnly bool, onDispatched func(),
) (*executor.PromptResult, error) {
	return m.acceptRetry(ctx, executionID, prompt, attachments, dispatchOnly, nil, onDispatched, "")
}

func (m *retentionRetryManager) PromptAgentWithDispatchCallbackAndSubmissionID(
	ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment,
	dispatchOnly bool, onDispatched func(), submissionID string,
) (*executor.PromptResult, error) {
	return m.acceptRetry(ctx, executionID, prompt, attachments, dispatchOnly, nil, onDispatched, submissionID)
}

func (m *retentionRetryManager) PromptAgentWithSubmissionID(
	ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment,
	dispatchOnly bool, submissionID string,
) (*executor.PromptResult, error) {
	return m.acceptRetry(ctx, executionID, prompt, attachments, dispatchOnly, nil, nil, submissionID)
}

func (m *retentionRetryManager) PromptAgentWithAdmissionCallbackAndSubmissionID(
	ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment,
	dispatchOnly bool, beforeAdmission func(context.Context, string, []v1.MessageAttachment) error, onDispatched func(), submissionID string,
) (*executor.PromptResult, error) {
	return m.acceptRetry(ctx, executionID, prompt, attachments, dispatchOnly, beforeAdmission, onDispatched, submissionID)
}

func (m *retentionRetryManager) PromptAgentWithAdmissionCallback(
	ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment,
	dispatchOnly bool, beforeAdmission func(context.Context, string, []v1.MessageAttachment) error, onDispatched func(),
) (*executor.PromptResult, error) {
	return m.acceptRetry(ctx, executionID, prompt, attachments, dispatchOnly, beforeAdmission, onDispatched, "")
}

func (m *retentionRetryManager) acceptRetry(
	ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment,
	dispatchOnly bool, beforeAdmission func(context.Context, string, []v1.MessageAttachment) error, onDispatched func(), submissionID string,
) (*executor.PromptResult, error) {
	// Runtime preparation changes both text and attachment descriptors before admission.
	prompt += "\nprepared runtime context"
	attachments = append([]v1.MessageAttachment(nil), attachments...)
	for i := range attachments {
		attachments[i].Name = "bundle-2.zip"
		attachments[i].MimeType = "application/zip"
		attachments[i].SizeBytes = 16
		attachments[i].DeliveryMode = "prompt"
	}
	if beforeAdmission != nil {
		if err := beforeAdmission(ctx, prompt, attachments); err != nil {
			return nil, err
		}
	}
	stored, err := m.repo.GetAgentDeliverySubmission(ctx, submissionID)
	if err == nil {
		var payload agentDeliveryPromptPayload
		err = json.Unmarshal(stored.Payload, &payload)
		if err == nil && (!reflect.DeepEqual(payload.Attachments, attachments) || payload.Text != prompt || stored.State != models.DeliverySubmissionDispatching || stored.PayloadHash != journal.SubmissionHash(stored.Payload)) {
			err = fmt.Errorf("retry SQL proof does not describe accepted provider prompt")
		}
	}
	m.accepted = append(m.accepted, stored)
	m.proofErrors = append(m.proofErrors, err)
	m.currentPromptGeneration.Add(1)
	return m.mockAgentManager.PromptAgentWithDispatchCallback(ctx, executionID, prompt, attachments, dispatchOnly, onDispatched)
}

func TestDeliveryRetentionRetryPromptAdmission(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, scenario := range []string{"two_retries", "legacy", "unresolved_peer", "storage_failure"} {
			t.Run(engine+"/"+scenario, func(t *testing.T) { assertRetentionRetryPromptAdmission(t, engine, scenario) })
		}
	}
}

func assertRetentionRetryPromptAdmission(t *testing.T, engine, scenario string) {
	t.Helper()
	repo := retentionInitialPromptRepository(t, engine)
	const taskID, sessionID, executionID = "retention-retry-task", "retention-retry-session", "retention-retry-execution"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	session, err := repo.GetTaskSession(t.Context(), sessionID)
	require.NoError(t, err)
	session.AgentProfileID = "profile-1"
	session.AgentProfileSnapshot = map[string]any{"model": "mock-fast"}
	session.DownstreamACPSessionID = "retention-retry-native"
	session.TaskEnvironmentID = "retention-retry-workspace"
	require.NoError(t, repo.CreateTaskEnvironment(t.Context(), &models.TaskEnvironment{
		ID: session.TaskEnvironmentID, TaskID: taskID, ExecutorType: "local", Status: models.TaskEnvironmentStatusReady,
	}))
	require.NoError(t, repo.UpdateTaskSession(t.Context(), session))
	require.NoError(t, repo.CreateHarnessSessionGeneration(t.Context(), &models.HarnessSessionGeneration{
		SessionID: sessionID, IncarnationID: session.QueueIncarnationID, Generation: 1, NativeSessionID: session.DownstreamACPSessionID, CreationReason: "native_started",
	}))
	const originalID = "prompt:retention-retry-original"
	prepared, err := repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{
		ID: originalID, SessionID: sessionID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, OwnerGeneration: 1,
		PayloadHash: "original-hash", Payload: []byte("original failed attempt"), State: models.DeliverySubmissionFailed,
	})
	require.NoError(t, err)
	require.True(t, prepared)
	base := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true, currentPromptExecutionID: executionID,
		isAgentReadyFn: func(context.Context, string) bool { return true }}
	base.currentPromptGeneration.Store(7)
	manager := &retentionRetryManager{repo: repo, durableDeliveryTestAgentManager: &durableDeliveryTestAgentManager{
		mockAgentManager: base, advertised: scenario != "legacy",
		capability: agentruntime.DurableDeliveryCapability{Version: journal.CurrentVersion, Durable: true, Unresolved: scenario == "unresolved_peer"},
	}}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	service := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, manager)
	logCore, recordedLogs := observer.New(zap.DebugLevel)
	service.logger, err = logger.NewFromZap(zap.New(logCore))
	require.NoError(t, err)
	service.messageCreator = &mockMessageCreator{}
	t.Cleanup(service.cancelAllTransientRetries)
	if scenario == "storage_failure" {
		installRetentionRetryStorageFailure(t, repo, engine, originalID)
	}
	service.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "/transient retry original", attachments: []v1.MessageAttachment{{AttachmentID: "retry-attachment", Type: "resource", Name: "bundle.zip"}}})
	var ids []string
	for attempt := 1; attempt <= 2; attempt++ {
		require.NoError(t, repo.UpdateTaskSessionState(t.Context(), sessionID, models.TaskSessionStateWaitingForInput, ""))
		service.beginPromptAttempt(sessionID, executionID, base.currentPromptGeneration.Load(), false)
		current, readErr := repo.GetTaskSession(t.Context(), sessionID)
		require.NoError(t, readErr)
		entry := &transientRetryEntry{attempt: attempt, retryCtx: t.Context(), cancel: func() {}, retainedRuntime: &retainedRuntimeRetry{
			executionID: executionID, generation: base.currentPromptGeneration.Load(), profileID: "profile-1",
			nativeID: continuationNativeID(current), identity: continuationSessionIdentity(current),
		}}
		service.transientRetries.Store(sessionID, entry)
		require.Equal(t, retainedRuntimeRetryUsable, service.retainedRuntimeRetryDisposition(t.Context(), taskID, sessionID, entry), "fixture must exercise the live retained retry path")
		service.retryTransientPrompt(t.Context(), taskID, sessionID, executionID)
		if len(manager.accepted) == 0 {
			for _, logged := range recordedLogs.All() {
				t.Logf("retry diagnostic: %s %v", logged.Message, logged.ContextMap())
			}
		}
		if scenario == "unresolved_peer" || scenario == "storage_failure" {
			require.Empty(t, manager.accepted, "failed durable retry preparation must not reach provider acceptance")
			rows, readErr := repo.ListAgentDeliverySubmissions(t.Context(), sessionID)
			require.NoError(t, readErr)
			require.Len(t, rows, 1)
			return
		}
		require.Len(t, manager.accepted, attempt)
		if scenario == "legacy" {
			require.Error(t, manager.proofErrors[attempt-1])
			require.Nil(t, manager.accepted[attempt-1], "legacy retry must not create durable SQL state")
			continue
		}
		require.NoError(t, manager.proofErrors[attempt-1], "retry must prepare exact durable SQL identity before provider acceptance")
		stored := manager.accepted[attempt-1]
		require.NotNil(t, stored)
		require.NotEqual(t, originalID, stored.ID)
		require.Equal(t, sessionID, stored.SessionID)
		require.Equal(t, session.QueueIncarnationID, stored.IncarnationID)
		require.Equal(t, int64(1), stored.HarnessGeneration)
		require.Equal(t, int64(1), stored.OwnerGeneration)
		ids = append(ids, stored.ID)
	}
	if scenario == "legacy" {
		rows, readErr := repo.ListAgentDeliverySubmissions(t.Context(), sessionID)
		require.NoError(t, readErr)
		require.Len(t, rows, 1, "legacy retries preserve the original evidence without adding durable submissions")
	} else {
		require.NotEqual(t, ids[0], ids[1], "each authorized retry requires a fresh immutable submission")
	}
	require.Len(t, base.capturedPrompts, 2)
	original, err := repo.GetAgentDeliverySubmission(t.Context(), originalID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionFailed, original.State)
	require.Equal(t, []byte("original failed attempt"), original.Payload)
}

func installRetentionRetryStorageFailure(t *testing.T, repo *sqliterepo.Repository, engine, originalID string) {
	t.Helper()
	query := `CREATE TRIGGER retention_retry_prompt_failure BEFORE INSERT ON agent_delivery_submissions BEGIN SELECT RAISE(ABORT, 'retry fixture failure'); END`
	if engine == "Postgres" {
		query = `ALTER TABLE agent_delivery_submissions ADD CONSTRAINT retention_retry_prompt_failure CHECK (id = '` + originalID + `')`
	}
	_, err := repo.DB().Exec(query)
	require.NoError(t, err)
}
