package handlers

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/coordinator"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func proposeWithNotes(t *testing.T, h *Handlers, coordinatorID string, noteIDs interface{}) *ws.Message {
	t.Helper()
	payload := proposeTaskPayload()
	if noteIDs != nil {
		payload["note_ids"] = noteIDs
	}
	msg := makeWSMessage(t, coordinator.ActionProposeTask, payload)
	ctx := h.withProposalNotes(coordinatorPrincipalContext(coordinatorID), msg)
	resp, err := h.handleProposeTask(ctx, msg)
	require.NoError(t, err)
	return resp
}

func TestProposeTaskNoteIDsRefusedWhileNotesAreOff(t *testing.T) {
	h, c, _ := newProposeTaskTestHandlers(t)
	resp := proposeWithNotes(t, h, c.ID, []string{"11111111-1111-1111-1111-111111111111"})
	assertWSError(t, resp, ws.ErrorCodeValidation)
	var errPayload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &errPayload))
	require.Equal(t, coordinator.NoteRefUnavailable, errPayload.Details["code"])
	require.Equal(t, "note_ids", errPayload.Details["field"])
}

func TestProposeTaskEmptyNoteIDsAreNoOps(t *testing.T) {
	h, c, _ := newProposeTaskTestHandlers(t)
	for _, v := range []interface{}{nil, []string{}} {
		resp := proposeWithNotes(t, h, c.ID, v)
		require.Equal(t, ws.MessageTypeResponse, resp.Type, "got: %s", resp.Payload)
	}
}

func TestProposeTaskMalformedNoteIDsAreRefused(t *testing.T) {
	h, c, _ := newProposeTaskTestHandlers(t)
	resp := proposeWithNotes(t, h, c.ID, "not-a-list")
	assertWSError(t, resp, ws.ErrorCodeValidation)
	var errPayload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &errPayload))
	require.Equal(t, coordinator.NoteRefMalformed, errPayload.Details["code"])
}

func TestWithProposalNotesIgnoresOtherActionsAndPrincipals(t *testing.T) {
	h, c, _ := newProposeTaskTestHandlers(t)
	ctx := coordinatorPrincipalContext(c.ID)
	other := makeWSMessage(t, "coordinator.list_activity", map[string]interface{}{"note_ids": []string{"x"}})
	require.Equal(t, ctx, h.withProposalNotes(ctx, other))
	propose := makeWSMessage(t, coordinator.ActionProposeTask, map[string]interface{}{"note_ids": []string{"x"}})
	bare := t.Context()
	require.Equal(t, bare, h.withProposalNotes(bare, propose))
}
