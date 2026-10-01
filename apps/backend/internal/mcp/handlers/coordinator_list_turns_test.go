package handlers

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/ledger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type turnReaderStub struct {
	coordinatorID string
	args          ledger.ListArgs
	page          *ledger.Page
	err           error
}

func (s *turnReaderStub) List(_ context.Context, coordinatorID string, args ledger.ListArgs) (*ledger.Page, error) {
	s.coordinatorID, s.args = coordinatorID, args
	return s.page, s.err
}

func listTurnsHandlers(t *testing.T, phase31 bool, reader CoordinatorTurnReader) (*Handlers, *coordinator.Coordinator) {
	t.Helper()
	h, _, _, c := newListActivityTestHandlers(t, false, coordinator.WithPhase31(phase31))
	if reader != nil {
		h.SetCoordinatorTurnReader(reader)
	}
	return h, c
}

func callListTurns(t *testing.T, h *Handlers, ctx context.Context, payload map[string]interface{}) *ws.Message {
	t.Helper()
	resp, err := h.handleListCoordinatorTurns(ctx, makeWSMessage(t, coordinator.ActionListTurns, payload))
	require.NoError(t, err)
	return resp
}

func TestHandleListCoordinatorTurns_BindsCoordinatorFromPrincipalAndForwardsArgs(t *testing.T) {
	stub := &turnReaderStub{page: &ledger.Page{Turns: []ledger.TurnView{{ID: "t1"}}}}
	h, c := listTurnsHandlers(t, true, stub)

	resp := callListTurns(t, h, getItemPrincipalContext(c.WorkspaceID, c.ID), map[string]interface{}{
		"coordinator_id": "someone-else", "since": "2026-09-01T00:00:00Z", "verdict": "acted", "trigger": "wake", "task": "task-1", "limit": 7, "before": "abc",
	})
	require.Equal(t, ws.MessageTypeResponse, resp.Type, "got: %s", resp.Payload)
	require.Equal(t, c.ID, stub.coordinatorID)
	require.Equal(t, "2026-09-01T00:00:00Z", stub.args.Since)
	require.Equal(t, "acted", stub.args.Verdict)
	require.Equal(t, "wake", stub.args.Trigger)
	require.Equal(t, "task-1", stub.args.Task)
	require.Equal(t, "abc", stub.args.Before)
	require.NotNil(t, stub.args.Limit)
	require.Equal(t, 7, *stub.args.Limit)
	var out struct {
		Turns []map[string]interface{} `json:"turns"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	require.Len(t, out.Turns, 1)
}

func TestHandleListCoordinatorTurns_ErrorMapping(t *testing.T) {
	cases := map[string]struct {
		err  error
		code string
	}{
		"field":       {&coordinator.FieldError{Field: "limit", Message: "limit must be between 1 and 50"}, ws.ErrorCodeBadRequest},
		"not found":   {coordinator.ErrNotFound, ws.ErrorCodeNotFound},
		"unavailable": {ledger.ErrUnavailable, ws.ErrorCodeUnavailable},
		"other":       {context.DeadlineExceeded, ws.ErrorCodeInternalError},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h, c := listTurnsHandlers(t, true, &turnReaderStub{err: tc.err})
			assertWSError(t, callListTurns(t, h, getItemPrincipalContext(c.WorkspaceID, c.ID), map[string]interface{}{}), tc.code)
		})
	}
}

func TestHandleListCoordinatorTurns_NamesTheMistypedArgument(t *testing.T) {
	h, c := listTurnsHandlers(t, true, &turnReaderStub{page: &ledger.Page{}})
	for field, payload := range map[string]map[string]interface{}{
		"limit":   {"limit": "ten"},
		"since":   {"since": 5},
		"verdict": {"verdict": []string{"a"}},
	} {
		resp := callListTurns(t, h, getItemPrincipalContext(c.WorkspaceID, c.ID), payload)
		assertWSError(t, resp, ws.ErrorCodeBadRequest)
		var ep ws.ErrorPayload
		require.NoError(t, json.Unmarshal(resp.Payload, &ep))
		require.Equal(t, field, ep.Details[fieldErrorDetailsKey], field)
	}
}

func TestHandleListCoordinatorTurns_RefusesWithoutACoordinator(t *testing.T) {
	h, _ := listTurnsHandlers(t, true, &turnReaderStub{page: &ledger.Page{}})
	assertWSError(t, callListTurns(t, h, context.Background(), map[string]interface{}{}), ws.ErrorCodeNotFound)
}

func TestListTurnsAction_AbsentWhileTheSurfaceIsOff(t *testing.T) {
	for name, tc := range map[string]struct {
		phase31 bool
		reader  bool
	}{"flag off": {false, true}, "no reader": {true, false}} {
		t.Run(name, func(t *testing.T) {
			var reader CoordinatorTurnReader
			if tc.reader {
				reader = &turnReaderStub{page: &ledger.Page{}}
			}
			h, c := listTurnsHandlers(t, tc.phase31, reader)
			d := ws.NewDispatcher()
			h.RegisterHandlers(d)
			resp, err := d.Dispatch(getItemPrincipalContext(c.WorkspaceID, c.ID), makeWSMessage(t, coordinator.ActionListTurns, map[string]interface{}{}))
			require.NoError(t, err)
			assertWSError(t, resp, ws.ErrorCodeUnknownAction)
		})
	}
}

func TestListTurnsAction_ServedWhenTheSurfaceIsOn(t *testing.T) {
	stub := &turnReaderStub{page: &ledger.Page{Turns: []ledger.TurnView{}}}
	h, c := listTurnsHandlers(t, true, stub)
	d := ws.NewDispatcher()
	h.RegisterHandlers(d)
	resp, err := d.Dispatch(getItemPrincipalContext(c.WorkspaceID, c.ID), makeWSMessage(t, coordinator.ActionListTurns, map[string]interface{}{}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, "got: %s", resp.Payload)
	require.Equal(t, c.ID, stub.coordinatorID)
}

type callRecord struct {
	session, action, target string
	allowed                 bool
}

type recordingLedger struct {
	mu    sync.Mutex
	calls []callRecord
}

func (l *recordingLedger) ActiveTurnID(string) string { return "" }
func (l *recordingLedger) Call(session, action, target string, allowed bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, callRecord{session, action, target, allowed})
}

func TestGuardedDispatch_RecordsAllowAndRefuseDecisionsForCoordinators(t *testing.T) {
	h, c := listTurnsHandlers(t, false, nil)
	led := &recordingLedger{}
	h.coordinatorSvc.SetTurnLedger(led)
	d := ws.NewDispatcher()
	h.RegisterHandlers(d)
	ctx := getItemPrincipalContext(c.WorkspaceID, c.ID)

	_, err := d.Dispatch(ctx, makeWSMessage(t, coordinator.ActionListActivity, map[string]interface{}{}))
	require.NoError(t, err)
	_, err = d.Dispatch(ctx, makeWSMessage(t, coordinator.ActionListActivity, map[string]interface{}{"task_id": "task-9"}))
	require.NoError(t, err)
	_, err = d.Dispatch(ctx, makeWSMessage(t, coordinator.ActionProposeImprovement, map[string]interface{}{}))
	require.NoError(t, err)

	require.Equal(t, []callRecord{
		{"coordinator-session", coordinator.ActionListActivity, "", true},
		{"coordinator-session", coordinator.ActionListActivity, "task-9", false},
		{"coordinator-session", coordinator.ActionProposeImprovement, "", false},
	}, led.calls)
}

func TestGuardedDispatch_NonCoordinatorCallersAreNotRecorded(t *testing.T) {
	h, _ := listTurnsHandlers(t, false, nil)
	led := &recordingLedger{}
	h.coordinatorSvc.SetTurnLedger(led)
	d := ws.NewDispatcher()
	h.RegisterHandlers(d)
	_, err := d.Dispatch(context.Background(), makeWSMessage(t, coordinator.ActionListActivity, map[string]interface{}{}))
	require.NoError(t, err)
	require.Empty(t, led.calls)
}
