package acpv2

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestWirePermissionRepliesStayWithTheirRequest(t *testing.T) {
	fixture := newWireFixture(t)
	invocations := make(chan *types.PermissionRequest, 2)
	fixture.adapter.SetPermissionHandler(func(ctx context.Context, req *types.PermissionRequest) (*types.PermissionResponse, error) {
		invocations <- req
		if req.Title == "obsolete" {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return &types.PermissionResponse{OptionID: "approve"}, nil
	})
	fixture.initialize()
	fixture.newSession(fixtureSession)
	done, prompt := fixture.prompt()
	fixture.reply(prompt, map[string]any{"messageId": "user-message"})
	requestPermission := func(id, session, title string) {
		fixture.send(map[string]any{"id": id, "method": "session/request_permission", "params": map[string]any{
			"sessionId": session, "title": title,
			"options": []map[string]any{{"optionId": "approve", "name": "Approve", "kind": "allow_once"}},
		}})
	}
	requestPermission("wrong-id", "wrong-session", "wrong session")
	rejected := fixture.read("")
	require.JSONEq(t, `"wrong-id"`, string(rejected.ID))
	require.JSONEq(t, `{"outcome":{"outcome":"cancelled"}}`, string(rejected.Result))
	requestPermission("old-id", fixtureSession, "obsolete")
	select {
	case request := <-invocations:
		require.Equal(t, "obsolete", request.Title)
	case <-fixture.ctx.Done():
		t.Fatal("permission handler did not receive request")
	}
	fixture.send(map[string]any{"method": "$/cancel_request", "params": map[string]any{"requestId": "old-id"}})
	requestPermission("new-id", fixtureSession, "current")
	for {
		reply := fixture.read("")
		if string(reply.ID) == `"new-id"` {
			require.JSONEq(t, `{"outcome":{"outcome":"selected","optionId":"approve"}}`, string(reply.Result))
			break
		}
		require.JSONEq(t, `"old-id"`, string(reply.ID))
		require.JSONEq(t, `{"outcome":{"outcome":"cancelled"}}`, string(reply.Result))
	}
	fixture.state("running", "")
	fixture.state("idle", "end_turn")
	fixture.event(streams.EventTypeComplete)
	require.NoError(t, fixture.wait(done))
}

func TestWireElicitationReturnsTheOfferedFormValue(t *testing.T) {
	fixture := newWireFixture(t)
	fixture.adapter.SetUserInputRequestHandler(func(_ context.Context, req *types.UserInputRequest) (*types.UserInputResponse, error) {
		if req.ThreadID != fixtureSession || req.ItemID == "" || !req.ScopedCancellation || len(req.Questions) != 1 {
			t.Errorf("unexpected normalized questions: %+v", req)
		}
		return &types.UserInputResponse{Answers: map[string]types.UserInputAnswer{
			"choice": {OptionIDs: []string{"yes"}},
		}}, nil
	})
	fixture.initialize()
	fixture.newSession(fixtureSession)
	done, prompt := fixture.prompt()
	fixture.reply(prompt, map[string]any{"messageId": "user-message"})
	fixture.send(map[string]any{"id": "form-id", "method": "elicitation/create", "params": map[string]any{
		"sessionId": fixtureSession, "mode": "form", "message": "Continue?",
		"requestedSchema": json.RawMessage(`{"type":"object","properties":{"choice":{"type":"string","enum":["yes","no"]}}}`),
	}})
	reply := fixture.read("")
	require.JSONEq(t, `"form-id"`, string(reply.ID))
	require.JSONEq(t, `{"action":"accept","content":{"choice":"yes"}}`, string(reply.Result))
	fixture.state("running", "")
	fixture.state("idle", "end_turn")
	fixture.event(streams.EventTypeComplete)
	require.NoError(t, fixture.wait(done))
}
