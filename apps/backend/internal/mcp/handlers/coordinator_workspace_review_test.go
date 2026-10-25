package handlers

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/coordinator/ledger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestCoordinatorReadersRejectPrincipalWorkspaceMismatch(t *testing.T) {
	reader := &turnReaderStub{page: &ledger.Page{Turns: []ledger.TurnView{{ID: "private-turn"}}}}
	h, c := listTurnsHandlers(t, true, reader)
	bad := getItemPrincipalContext("foreign-workspace", c.ID)
	for _, response := range []*ws.Message{
		callListTurns(t, h, bad, map[string]interface{}{}),
		callListActivity(t, h, bad, map[string]interface{}{}),
	} {
		require.Equal(t, ws.MessageTypeError, response.Type)
		var payload ws.ErrorPayload
		require.NoError(t, json.Unmarshal(response.Payload, &payload))
		require.Equal(t, ws.ErrorCodeNotFound, payload.Code)
		require.NotContains(t, string(response.Payload), "private-turn")
	}
	require.Empty(t, reader.coordinatorID, "foreign workspace reached ledger")
	good := callListTurns(t, h, getItemPrincipalContext(c.WorkspaceID, c.ID), map[string]interface{}{})
	require.Equal(t, ws.MessageTypeResponse, good.Type)
	require.Equal(t, c.ID, reader.coordinatorID)
}
