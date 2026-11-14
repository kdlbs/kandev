package handlers

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/coordinator"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func decodeErrorPayload(t *testing.T, resp *ws.Message) ws.ErrorPayload {
	t.Helper()
	require.Equal(t, ws.MessageTypeError, resp.Type, "got: %s", resp.Payload)
	var ep ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &ep))
	return ep
}

func TestProposeKindErrorResponseAnswerMappings(t *testing.T) {
	h := &Handlers{logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, coordinator.ActionProposeAnswer, map[string]interface{}{})

	t.Run("refusal with field", func(t *testing.T) {
		resp, err := h.proposeKindErrorResponse(msg, coordinator.ProposalKindAnswer,
			fmt.Errorf("wrapped: %w", &coordinator.ProposeRefusal{Code: "empty_answer", Field: "draft"}))
		require.NoError(t, err)
		ep := decodeErrorPayload(t, resp)
		require.Equal(t, ws.ErrorCodeValidation, ep.Code)
		require.Equal(t, "empty_answer", ep.Message)
		require.Equal(t, "empty_answer", ep.Details["code"])
		require.Equal(t, "draft", ep.Details[fieldErrorDetailsKey])
	})

	t.Run("refusal without field is a bare message", func(t *testing.T) {
		resp, err := h.proposeKindErrorResponse(msg, coordinator.ProposalKindAnswer,
			&coordinator.ProposeRefusal{Code: "nothing_to_answer"})
		require.NoError(t, err)
		ep := decodeErrorPayload(t, resp)
		require.Equal(t, "nothing_to_answer", ep.Message)
		require.Equal(t, "", ep.Details[fieldErrorDetailsKey])
	})

	t.Run("denied class looks like an unknown tool", func(t *testing.T) {
		resp, err := h.proposeKindErrorResponse(msg, coordinator.ProposalKindAnswer,
			&coordinator.PolicyDeniedError{Action: coordinator.ActionAnswer})
		require.NoError(t, err)
		require.Equal(t, ws.ErrorCodeUnknownAction, decodeErrorPayload(t, resp).Code)
	})

	t.Run("target read failure is internal and generic", func(t *testing.T) {
		resp, err := h.proposeKindErrorResponse(msg, coordinator.ProposalKindAnswer,
			fmt.Errorf("%w: database is locked", coordinator.ErrAnswerTargetRead))
		require.NoError(t, err)
		ep := decodeErrorPayload(t, resp)
		require.Equal(t, ws.ErrorCodeInternalError, ep.Code)
		require.Equal(t, "could not read the task's current question", ep.Message)
	})
}

func TestHandleProposeAnswerRequiresCoordinatorPrincipal(t *testing.T) {
	h := &Handlers{logger: testLogger(t).WithFields()}
	resp, err := h.handleProposeAnswer(t.Context(), makeWSMessage(t, coordinator.ActionProposeAnswer, map[string]interface{}{}))
	require.NoError(t, err)
	require.Equal(t, ws.ErrorCodeForbidden, decodeErrorPayload(t, resp).Code)
}

func TestHandleProposeAnswerFlagOffIsUnknownAction(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	store, err := coordinator.NewStore(conn, conn)
	require.NoError(t, err)
	svc := coordinator.NewService(store, coordinator.NewValidator(nil, nil), nil, testLogger(t),
		coordinator.WithPhase2(true), coordinator.WithPhase3(true))
	c := &coordinator.Coordinator{WorkspaceID: "ws-answer", Name: "Ops", AgentProfileID: "agent-1", ExecutorProfileID: "executor-1"}
	require.NoError(t, store.CreateCoordinator(t.Context(), c))
	h := &Handlers{logger: testLogger(t).WithFields()}
	h.SetCoordinatorService(svc)

	resp, err := h.handleProposeAnswer(coordinatorPrincipalContext(c.ID),
		makeWSMessage(t, coordinator.ActionProposeAnswer, map[string]interface{}{"task_id": "t"}))
	require.NoError(t, err)
	require.Equal(t, ws.ErrorCodeUnknownAction, decodeErrorPayload(t, resp).Code)
}

func TestProposeAnswerActionIsOnTheCoordinatorSurfaceAndPrincipalOnly(t *testing.T) {
	_, onSurface := coordinatorSurfaceActions[coordinator.ActionProposeAnswer]
	_, principalOnly := coordinatorPrincipalOnlyActions[coordinator.ActionProposeAnswer]
	require.True(t, onSurface)
	require.True(t, principalOnly)
	require.True(t, isCoordinatorProposeAction(coordinator.ActionProposeAnswer))
}

func TestHandleProposeMergeRequiresCoordinatorPrincipal(t *testing.T) {
	h := &Handlers{logger: testLogger(t).WithFields()}
	resp, err := h.handleProposeMerge(t.Context(), makeWSMessage(t, coordinator.ActionProposeMerge, map[string]interface{}{}))
	require.NoError(t, err)
	require.Equal(t, ws.ErrorCodeForbidden, decodeErrorPayload(t, resp).Code)
}
