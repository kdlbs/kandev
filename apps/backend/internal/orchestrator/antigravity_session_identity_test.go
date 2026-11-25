package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type rejectingACPIdentityResumeStore struct {
	*sqliterepo.Repository
	called bool
}

func (r *rejectingACPIdentityResumeStore) UpdateResumeTokenWithACPAdapterIdentity(
	context.Context,
	string,
	string,
	string,
	string,
	models.ACPAdapterIdentity,
) error {
	r.called = true
	return errors.New("injected adapter identity write failure")
}

func TestAntigravitySessionCreatedDoesNotAcceptTokenWhenIdentityWriteFails(t *testing.T) {
	svc, repo, event := freshStartCommitFixture(t)
	event.DeliveryHarnessGeneration = 0
	rejecting := &rejectingACPIdentityResumeStore{Repository: repo}
	svc.repo = rejecting

	svc.handleACPSessionCreated(t.Context(), event)
	svc.handleSessionStatusEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: event.TaskID, SessionID: event.SessionID, ExecutionID: event.AgentExecutionID,
		AgentType: "antigravity-acp",
		Data:      &lifecycle.AgentStreamEventData{Type: "session_status", ACPSessionID: event.ACPSessionID},
	})

	require.True(t, rejecting.called, "the native ID and runtime tag must share the token acceptance boundary")
	running, err := repo.GetExecutorRunningBySessionID(t.Context(), event.SessionID)
	require.NoError(t, err)
	require.Empty(t, running.ResumeToken, "a token without its runtime identity must not be resumable")
}

func TestAntigravitySessionCreatedPersistsLegacyRuntimeIdentityWithToken(t *testing.T) {
	svc, repo, event := freshStartCommitFixture(t)
	event.DeliveryHarnessGeneration = 0

	svc.handleACPSessionCreated(t.Context(), event)

	running, err := repo.GetExecutorRunningBySessionID(t.Context(), event.SessionID)
	require.NoError(t, err)
	require.Equal(t, event.ACPSessionID, running.ResumeToken)
	session, err := repo.GetTaskSession(t.Context(), event.SessionID)
	require.NoError(t, err)
	require.Equal(t, event.AdapterVersion,
		models.ACPAdapterVersionForNativeSession(session.Metadata, event.ACPSessionID))
}
