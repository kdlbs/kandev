package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type freshStartRotatingTokenStore struct {
	*sqliterepo.Repository
	accepted bool
}

func (r *freshStartRotatingTokenStore) UpdateResumeToken(ctx context.Context, sessionID, executionID, token, uuid string) error {
	if err := r.Repository.UpdateResumeToken(ctx, sessionID, executionID, token, uuid); err != nil {
		return err
	}
	r.accepted = true
	running, err := r.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		return err
	}
	running.AgentExecutionID = "successor-execution"
	running.ResumeToken = "successor-native"
	return r.UpsertExecutorRunning(ctx, running)
}

func freshStartCommitFixture(t *testing.T) (*Service, *sqliterepo.Repository, watcher.ACPSessionEventData) {
	t.Helper()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, freshStartTaskID, freshStartSessionID, models.TaskSessionStateFailed)
	session, err := repo.GetTaskSession(t.Context(), freshStartSessionID)
	require.NoError(t, err)
	now := time.Now().UTC()
	committed, err := repo.CommitHarnessSessionGeneration(t.Context(), &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 1,
		NativeSessionID: "previous-native", CreationReason: "native_started", CreatedAt: now, CommittedAt: now,
	}, 0)
	require.NoError(t, err)
	require.True(t, committed)
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "fresh-start-execution")
	event := freshStartSessionCreated(2, true)
	event.DeliveryIncarnationID = session.QueueIncarnationID
	event.DeliveryStreamID = session.QueueIncarnationID + ":g2"
	return createTestService(repo, newMockStepGetter(), newMockTaskRepo()), repo, event
}

func TestFreshStartCommitRotationAfterAcceptedToken(t *testing.T) {
	svc, repo, event := freshStartCommitFixture(t)
	rotating := &freshStartRotatingTokenStore{Repository: repo}
	svc.repo = rotating
	svc.handleACPSessionCreated(t.Context(), event)
	require.True(t, rotating.accepted, "the real token CAS must succeed before rotation")
	running, err := repo.GetExecutorRunningBySessionID(t.Context(), event.SessionID)
	require.NoError(t, err)
	require.Equal(t, "successor-execution", running.AgentExecutionID)
	current, err := repo.GetCurrentHarnessSessionGeneration(t.Context(), event.SessionID, event.DeliveryIncarnationID)
	require.NoError(t, err)
	require.EqualValues(t, 1, current.Generation, "an obsolete accepted event must not commit after execution rotation")
	require.Equal(t, "previous-native", current.NativeSessionID)
	svc.repo = repo
	event.AgentExecutionID, event.ACPSessionID = "successor-execution", "successor-native"
	svc.handleACPSessionCreated(t.Context(), event)
	current, err = repo.GetCurrentHarnessSessionGeneration(t.Context(), event.SessionID, event.DeliveryIncarnationID)
	require.NoError(t, err)
	require.EqualValues(t, 2, current.Generation)
	require.Equal(t, "successor-native", current.NativeSessionID, "the current execution must remain able to commit")
}

func TestFreshStartCommitForeignIdentity(t *testing.T) {
	for _, identity := range []string{"task", "incarnation", "execution", "empty execution"} {
		t.Run(identity, func(t *testing.T) {
			svc, repo, event := freshStartCommitFixture(t)
			original := event
			switch identity {
			case "task":
				event.TaskID = "foreign-task"
			case "incarnation":
				event.DeliveryIncarnationID = "foreign-incarnation"
			case "execution":
				event.AgentExecutionID = "foreign-execution"
			case "empty execution":
				event.AgentExecutionID = ""
			}
			before, err := repo.GetExecutorRunningBySessionID(t.Context(), original.SessionID)
			require.NoError(t, err)
			svc.handleACPSessionCreated(t.Context(), event)
			after, err := repo.GetExecutorRunningBySessionID(t.Context(), original.SessionID)
			require.NoError(t, err)
			require.Equal(t, before.ResumeToken, after.ResumeToken, "foreign identity must not change the resume token")
			current, err := repo.GetCurrentHarnessSessionGeneration(t.Context(), original.SessionID, original.DeliveryIncarnationID)
			require.NoError(t, err)
			require.EqualValues(t, 1, current.Generation, "foreign identity must not advance the owning generation")
			if identity == "incarnation" {
				_, err = repo.GetCurrentHarnessSessionGeneration(t.Context(), event.SessionID, event.DeliveryIncarnationID)
				require.ErrorIs(t, err, models.ErrTaskSessionNotFound, "foreign incarnation must not gain its own committed row")
			}
			svc.handleACPSessionCreated(t.Context(), original)
			current, err = repo.GetCurrentHarnessSessionGeneration(t.Context(), original.SessionID, original.DeliveryIncarnationID)
			require.NoError(t, err)
			require.EqualValues(t, 2, current.Generation, "the unchanged owning event is the positive control")
		})
	}
}

func TestFreshStartCommitPreservesPredecessorBlock(t *testing.T) {
	svc, repo, event := freshStartCommitFixture(t)
	block := &models.SessionRecoveryBlock{ID: "fresh-start-predecessor-block", SessionID: event.SessionID,
		IncarnationID: event.DeliveryIncarnationID, ExpectedGeneration: 1, Reason: "unknown_prompt_outcome",
		ConsumerReference: "agent_delivery", State: models.RecoveryBlockOpen}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(t.Context(), block))
	svc.handleACPSessionCreated(t.Context(), event)
	current, err := repo.GetCurrentHarnessSessionGeneration(t.Context(), event.SessionID, event.DeliveryIncarnationID)
	require.NoError(t, err)
	require.EqualValues(t, 2, current.Generation, "the current execution must actually commit")
	stored, err := repo.GetSessionRecoveryBlock(t.Context(), block.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, stored.ExpectedGeneration, "fresh start retains the predecessor block as history")
	require.Equal(t, models.RecoveryBlockOpen, stored.State)
	require.Empty(t, stored.AuthorizedAction)
}

func TestFreshStartCommitFirstGenerationCompatibility(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, freshStartTaskID, freshStartSessionID, models.TaskSessionStateStarting)
	seedExecutorRunning(t, repo, freshStartSessionID, freshStartTaskID, "fresh-start-execution")
	session, err := repo.GetTaskSession(t.Context(), freshStartSessionID)
	require.NoError(t, err)
	event := freshStartSessionCreated(1, false)
	event.DeliveryIncarnationID = session.QueueIncarnationID
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.handleACPSessionCreated(t.Context(), event)
	current, err := repo.GetCurrentHarnessSessionGeneration(t.Context(), session.ID, session.QueueIncarnationID)
	require.NoError(t, err)
	require.EqualValues(t, 1, current.Generation)
	require.Equal(t, "native_started", current.CreationReason)
	require.Equal(t, event.ACPSessionID, current.NativeSessionID)
}

func TestFreshStartCommitStaleNativeAfterSuccessor(t *testing.T) {
	svc, repo, event := freshStartCommitFixture(t)
	manager := svc.agentManager.(*mockAgentManager)
	manager.getACPSessionIDForSessionFunc = func(string) (string, bool) { return "replacement-native", true }
	svc.handleACPSessionCreated(t.Context(), event)
	event.ACPSessionID = "previous-native"
	svc.handleACPSessionCreated(t.Context(), event)
	current, err := repo.GetCurrentHarnessSessionGeneration(t.Context(), event.SessionID, event.DeliveryIncarnationID)
	require.NoError(t, err)
	require.EqualValues(t, 2, current.Generation)
	require.Equal(t, "replacement-native", current.NativeSessionID)
	running, err := repo.GetExecutorRunningBySessionID(t.Context(), event.SessionID)
	require.NoError(t, err)
	require.Equal(t, "replacement-native", running.ResumeToken)
}
