package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/ledger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestCoordinatorMalformedJSONUsesStableClientError(t *testing.T) {
	h, _, _, c := newListActivityTestHandlers(t, true)
	ctx := getItemPrincipalContext(c.WorkspaceID, c.ID)
	cases := []struct {
		action string
		handle func(context.Context, *ws.Message) (*ws.Message, error)
	}{
		{coordinator.ActionProposeTask, h.handleProposeTask},
		{coordinator.ActionProposeResume, h.proposeKindHandler(coordinator.ProposalKindResume)},
		{coordinator.ActionListTurns, h.handleListCoordinatorTurns},
		{coordinator.ActionListActivity, h.handleListCoordinatorActivity},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			response, err := tc.handle(ctx, &ws.Message{ID: "bad-json", Action: tc.action, Payload: json.RawMessage(`{"private":"value"`)})
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeError, response.Type)
			var payload ws.ErrorPayload
			require.NoError(t, json.Unmarshal(response.Payload, &payload))
			require.Equal(t, ws.ErrorCodeBadRequest, payload.Code)
			require.Equal(t, "Invalid JSON payload", payload.Message)
		})
	}
}

func TestCoordinatorActivityRejectsExplicitEmptyCursor(t *testing.T) {
	_, err := coordinatorToolBefore(map[string]json.RawMessage{"before": json.RawMessage(`""`)})
	require.Error(t, err)
	value, err := coordinatorToolBefore(map[string]json.RawMessage{})
	require.NoError(t, err)
	require.Empty(t, value)
}

func TestCoordinatorRecordedStallTargetUsesItsItemID(t *testing.T) {
	h, c := listTurnsHandlers(t, false, nil)
	ledger := &recordingLedger{}
	h.coordinatorSvc.SetTurnLedger(ledger)
	ctx := getItemPrincipalContext(c.WorkspaceID, c.ID)
	for _, kind := range []string{"stall", "proposal"} {
		h.recordCoordinatorCall(ctx, makeWSMessage(t, coordinator.ActionGetItem,
			map[string]interface{}{"kind": kind, "id": "target"}), true)
	}
	require.Len(t, ledger.calls, 2)
	require.Equal(t, "target", ledger.calls[0].target)
	require.Empty(t, ledger.calls[1].target)
}

func TestCoordinatorProposalNotFoundErrorsRemainNotFound(t *testing.T) {
	h, _, _, _ := newListActivityTestHandlers(t, true)
	msg := makeWSMessage(t, coordinator.ActionProposeTask, map[string]interface{}{})
	missing := errors.Join(coordinator.ErrNotFound, errors.New("private lookup detail"))
	cases := []func() (*ws.Message, error){
		func() (*ws.Message, error) { return h.proposeTaskErrorResponse(msg, missing) },
		func() (*ws.Message, error) {
			return h.proposeKindErrorResponse(msg, coordinator.ProposalKindResume, missing)
		},
	}
	for _, call := range cases {
		response, err := call()
		require.NoError(t, err)
		var payload ws.ErrorPayload
		require.NoError(t, json.Unmarshal(response.Payload, &payload))
		require.Equal(t, ws.ErrorCodeNotFound, payload.Code)
		require.NotContains(t, payload.Message, "private")
	}
}

func TestCoordinatorMalformedJSONThroughDispatcherUsesStableError(t *testing.T) {
	reader := &turnReaderStub{page: &ledger.Page{Turns: []ledger.TurnView{}}}
	h, c := listTurnsHandlers(t, true, reader)
	d := ws.NewDispatcher()
	h.RegisterHandlers(d)
	response, err := d.Dispatch(getItemPrincipalContext(c.WorkspaceID, c.ID), &ws.Message{ID: "malformed", Type: ws.MessageTypeRequest, Action: coordinator.ActionListTurns, Payload: json.RawMessage(`{"private":"value"`)})
	require.NoError(t, err)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.Equal(t, ws.ErrorCodeBadRequest, payload.Code)
	require.Equal(t, "Invalid JSON payload", payload.Message)
	require.Empty(t, reader.coordinatorID)
}
