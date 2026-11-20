package lifecycle

import (
	"context"
	"errors"
	"testing"

	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-PLAN-COMMENTS-002.9
func TestSendPromptFailureBeforeTransportIsNotDispatched(t *testing.T) {
	sm, execution, _, _ := initialDeliveryPersistenceFixture(t)
	execution.ACPSessionID = "native-session"
	reached := false
	sm.beforePromptDispatchHook = func() { reached = true }
	want := errors.New("canonical admission failed")

	_, err := sm.sendPrompt(t.Context(), execution, "follow-up", false, nil, true,
		sendPromptCallbacks{beforeAdmission: func(context.Context, string, []v1.MessageAttachment) error {
			return want
		}}, false)

	require.False(t, reached)
	require.ErrorIs(t, err, ErrPromptNotDispatched)
	require.ErrorIs(t, err, want, "the original cause must stay matchable")
	require.Equal(t, want.Error(), err.Error(), "classification must not change user-visible error text")
}

// @covers AC-TASKS-PLAN-COMMENTS-002.9
func TestSendPromptTransportFailureIsNotClassifiedUndispatched(t *testing.T) {
	sm, execution, _, mock := initialDeliveryPersistenceFixture(t)
	execution.ACPSessionID = "native-session"
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.prompt" {
			resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "prompt rejected", nil)
			return resp
		}
		return mock.defaultHandler(msg)
	}

	_, err := sm.sendPrompt(t.Context(), execution, "follow-up", false, nil, true, sendPromptCallbacks{}, false)

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrPromptNotDispatched)
}

func TestMarkPromptNotDispatched(t *testing.T) {
	require.NoError(t, MarkPromptNotDispatched(nil))
	cause := errors.New("ceiling admission refused")
	marked := MarkPromptNotDispatched(cause)
	require.ErrorIs(t, marked, ErrPromptNotDispatched)
	require.ErrorIs(t, marked, cause)
	require.Equal(t, cause.Error(), marked.Error())
	require.Same(t, marked, MarkPromptNotDispatched(marked), "marking is idempotent")
}
