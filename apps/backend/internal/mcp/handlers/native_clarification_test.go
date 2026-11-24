package handlers

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/clarification/protocol"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type nativeQuestionCanceller struct {
	SessionCanceller
	sessionID, pendingID string
}

func (c *nativeQuestionCanceller) ExpireRequestAndNotify(_ context.Context, sessionID, pendingID string) error {
	c.sessionID, c.pendingID = sessionID, pendingID
	return nil
}

func TestNativeClarificationCancellationChecksScope(t *testing.T) {
	scope := protocol.NativeQuestionScope{SessionID: "session", NativeRequestID: uuid.NewString()}
	msg, err := ws.NewRequest("cancel", ws.ActionMCPCancelNativeClarification, scope)
	require.NoError(t, err)
	canceller := &nativeQuestionCanceller{}
	provider := &Handlers{sessionCanceller: canceller}
	ctx := streams.WithMCPExecutionContext(t.Context(), streams.MCPExecutionContext{TaskID: "task", SessionID: scope.SessionID})
	response, err := provider.handleCancelNativeClarification(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Empty(t, canceller.pendingID)

	foreign := streams.WithMCPExecutionContext(t.Context(), streams.MCPExecutionContext{TaskID: "task", SessionID: "other-session"})
	response, err = provider.handleCancelNativeClarification(mcporigin.WithTrustedInternalCall(foreign), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Empty(t, canceller.pendingID)

	response, err = provider.handleCancelNativeClarification(mcporigin.WithTrustedInternalCall(ctx), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, scope.SessionID, canceller.sessionID)
	require.Equal(t, scope.PendingID(), canceller.pendingID)
}

func TestAskUserQuestionRejectsMixedNativeAndAsyncIdentity(t *testing.T) {
	fixture := newAsyncQuestionFixture(t)
	task, session := fixture.seedTask(t, t.Context(), "session-mixed-question", false, "")
	ctx := streams.WithMCPExecutionContext(t.Context(), streams.MCPExecutionContext{
		TaskID: task.ID, SessionID: session.ID,
	})
	ctx = mcporigin.WithTrustedInternalCall(ctx)
	payload := askPayload(session.ID, task.ID, true, "mixed-async-request")
	payload["native_request_id"] = uuid.NewString()

	response, err := fixture.handlers.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion, payload))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeValidation)

	page, err := fixture.repo.ListAsyncQuestions(ctx, models.ListAsyncQuestionsOptions{AuthorSessionID: session.ID})
	require.NoError(t, err)
	require.Empty(t, page.Questions)
	require.Empty(t, fixture.store.ListPending())
}
