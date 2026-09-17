package acp

import (
	"context"
	"encoding/json"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestHandleACPUpdateMockResponseRetry(t *testing.T) {
	var notification acpsdk.SessionNotification
	raw := []byte(`{"sessionId":"session-1","update":{"sessionUpdate":"session_info_update","_meta":{"kandevMock":{"responseAttemptReset":true}}}}`)
	if err := json.Unmarshal(raw, &notification); err != nil {
		t.Fatalf("decode notification: %v", err)
	}

	for _, test := range []struct {
		name      string
		agentID   string
		wantReset bool
	}{
		{name: "controlled mock", agentID: mockAgentID, wantReset: true},
		{name: "production dialect", agentID: codexAgentID},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := newTestAdapter()
			a.agentID = test.agentID
			a.dialect = newACPDialect(test.agentID)
			t.Cleanup(func() { _ = a.Close() })
			_, turn := a.registerPromptTurn(context.Background(), 7)
			t.Cleanup(func() { a.clearPromptTurn(turn) })

			a.handleACPUpdate(notification, 7)

			events := drainEvents(a)
			if test.wantReset {
				if len(events) != 2 || events[0].Type != streams.EventTypeResponseAttemptReset {
					t.Fatalf("events = %+v, want reset then session info", events)
				}
				return
			}
			if len(events) != 1 || events[0].Type != streams.EventTypeSessionInfo {
				t.Fatalf("production dialect events = %+v, want session info only", events)
			}
		})
	}
}
