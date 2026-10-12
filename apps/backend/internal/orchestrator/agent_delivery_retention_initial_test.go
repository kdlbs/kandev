package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type retentionInitialPromptManager struct {
	*durableDeliveryTestAgentManager
	boundID string
	bindErr error
}

func (m *retentionInitialPromptManager) SetInitialDeliverySubmissionID(_ context.Context, _, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bindErr != nil {
		return m.bindErr
	}
	m.boundID = id
	return nil
}

type retentionInitialPromptObservation struct {
	submission *models.AgentDeliverySubmission
	prompt     string
	boundID    string
	err        error
}

func TestDeliveryRetentionInitialPromptAdmission(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, scenario := range []string{"fresh", "generation_3", "explicit_identity", "legacy", durableDeliveryStorageNotDurable, "unresolved_peer", "storage_failure", "binding_failure"} {
			t.Run(engine+"/"+scenario, func(t *testing.T) {
				assertRetentionInitialPromptAdmission(t, engine, scenario)
			})
		}
	}
}

func assertRetentionInitialPromptAdmission(t *testing.T, engine, scenario string) {
	t.Helper()
	repo := retentionInitialPromptRepository(t, engine)
	const taskID, sessionID, executionID = "task-initial-retention", "session-initial-retention", "exec-initial-retention"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCreated)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	session, err := repo.GetTaskSession(t.Context(), sessionID)
	require.NoError(t, err)
	generation := int64(1)
	if scenario == "generation_3" {
		generation = 3
		require.NoError(t, repo.CreateHarnessSessionGeneration(t.Context(), &models.HarnessSessionGeneration{
			SessionID: sessionID, IncarnationID: session.QueueIncarnationID, Generation: generation,
			NativeSessionID: "existing-native-generation-3", CreationReason: "native_started",
		}))
	}
	expectedID := fmt.Sprintf("prompt:initial:%s:%d:%s", sessionID, generation, executionID)
	if scenario == "explicit_identity" {
		expectedID = "prompt:explicit-initial-message"
	}
	manager := &retentionInitialPromptManager{durableDeliveryTestAgentManager: &durableDeliveryTestAgentManager{
		mockAgentManager: &mockAgentManager{repoForExecutionLookup: repo},
		advertised:       scenario != "legacy",
		capability:       agentruntime.DurableDeliveryCapability{Version: journal.CurrentVersion, Durable: true, Unresolved: scenario == "unresolved_peer"},
	}}
	if scenario == durableDeliveryStorageNotDurable {
		manager.capability.Durable = false
		manager.capability.Reason = durableDeliveryStorageNotDurable
	}
	if scenario == "storage_failure" {
		installRetentionInitialPromptFailure(t, repo, engine, expectedID)
	}
	if scenario == "binding_failure" {
		manager.bindErr = errors.New("initial identity handoff unavailable")
	}
	observations := make(chan retentionInitialPromptObservation, 1)
	manager.startAgentProcessFunc = func(ctx context.Context, id string) error {
		manager.mu.Lock()
		calls := append([]promptCall(nil), manager.setExecutionDescriptionCalls...)
		boundID := manager.boundID
		manager.mu.Unlock()
		observation := retentionInitialPromptObservation{boundID: boundID}
		if id != executionID || len(calls) != 1 {
			observation.err = fmt.Errorf("initial provider dispatch owner/prompt: execution=%q descriptions=%d", id, len(calls))
		} else {
			observation.prompt = calls[0].Prompt
			observation.submission, observation.err = repo.GetAgentDeliverySubmission(ctx, expectedID)
		}
		observations <- observation
		return nil
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	service := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, manager)
	service.messageCreator = &mockMessageCreator{}
	var execution *executor.TaskExecution
	if scenario == "explicit_identity" {
		execution, err = service.StartCreatedSessionWithDeliverySubmission(t.Context(), taskID, sessionID, "profile-1", "/capacity-after-tools",
			DirectPromptStartOptions{AutoStart: true, DeliverySubmissionID: "explicit-initial-message"})
	} else {
		execution, err = service.StartCreatedSession(t.Context(), taskID, sessionID, "profile-1", "/capacity-after-tools", false, false, true, nil, nil)
	}
	if scenario == "unresolved_peer" || scenario == "storage_failure" || scenario == "binding_failure" {
		require.ErrorIs(t, err, ErrSessionRecoveryRequired, "unprepared durable initial prompt must fail before provider dispatch")
		manager.mu.Lock()
		starts := len(manager.startAgentProcessCalls)
		manager.mu.Unlock()
		require.Zero(t, starts)
		block, blockErr := service.GetOpenSessionRecoveryBlock(t.Context(), sessionID)
		require.NoError(t, blockErr)
		require.NotNil(t, block)
		if scenario == "unresolved_peer" {
			require.Equal(t, durableDeliveryUnresolvedReason, block.Reason)
		} else {
			require.Equal(t, "backend_delivery_admission_failed", block.Reason)
		}
		submissions, readErr := repo.ListAgentDeliverySubmissions(t.Context(), sessionID)
		require.NoError(t, readErr)
		if scenario == "binding_failure" {
			require.Len(t, submissions, 1)
			require.Equal(t, expectedID, submissions[0].ID)
			require.Equal(t, models.DeliverySubmissionDispatching, submissions[0].State)
		} else {
			require.Empty(t, submissions)
		}
		return
	}
	require.NoError(t, err)
	require.NotNil(t, execution)
	select {
	case observed := <-observations:
		manager.mu.Lock()
		starts := len(manager.startAgentProcessCalls)
		manager.mu.Unlock()
		require.Equal(t, 1, starts, "initial launch must dispatch exactly one provider prompt")
		if scenario == "legacy" || scenario == durableDeliveryStorageNotDurable {
			require.Error(t, observed.err)
			require.Nil(t, observed.submission, "legacy launch must not create durable SQL state")
			require.Contains(t, observed.prompt, "/capacity-after-tools")
			submissions, readErr := repo.ListAgentDeliverySubmissions(t.Context(), sessionID)
			require.NoError(t, readErr)
			require.Empty(t, submissions)
			return
		}
		require.NoError(t, observed.err, "durable initial prompt must have SQL proof before provider acceptance")
		require.NotNil(t, observed.submission)
		require.Equal(t, expectedID, observed.submission.ID)
		require.Equal(t, sessionID, observed.submission.SessionID)
		require.Equal(t, session.QueueIncarnationID, observed.submission.IncarnationID)
		require.Equal(t, generation, observed.submission.HarnessGeneration)
		require.Equal(t, generation, observed.submission.OwnerGeneration)
		require.Equal(t, models.DeliverySubmissionDispatching, observed.submission.State)
		var payload agentDeliveryPromptPayload
		require.NoError(t, json.Unmarshal(observed.submission.Payload, &payload))
		require.Equal(t, observed.prompt, payload.Text, "SQL payload must describe the actual system-wrapped provider prompt")
		require.Contains(t, payload.Text, "/capacity-after-tools")
		require.Equal(t, journal.SubmissionHash(observed.submission.Payload), observed.submission.PayloadHash)
		if scenario == "explicit_identity" {
			require.Equal(t, "explicit-initial-message", observed.boundID)
		} else {
			require.Equal(t, expectedID, observed.boundID, "generated canonical identity must be bound before native initial dispatch")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("initial provider dispatch was not observed")
	}
}

func retentionInitialPromptRepository(t *testing.T, engine string) *sqliterepo.Repository {
	t.Helper()
	if engine == "SQLite" {
		return setupTestRepo(t)
	}
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := sqliterepo.NewWithDB(database, database, nil)
	require.NoError(t, err)
	return repo
}

func installRetentionInitialPromptFailure(t *testing.T, repo *sqliterepo.Repository, engine, id string) {
	t.Helper()
	quotedID := "'" + strings.ReplaceAll(id, "'", "''") + "'"
	query := `CREATE TRIGGER retention_initial_prompt_failure BEFORE INSERT ON agent_delivery_submissions
 WHEN NEW.id = ` + quotedID + ` BEGIN SELECT RAISE(ABORT, 'initial prompt fixture failure'); END`
	if engine == "Postgres" {
		query = `ALTER TABLE agent_delivery_submissions ADD CONSTRAINT retention_initial_prompt_failure CHECK (id <> ` + quotedID + `)`
	}
	_, err := repo.DB().Exec(query)
	require.NoError(t, err)
}
