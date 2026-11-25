package client

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/clarification/protocol"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestNativeQuestionCancellationTargetsOneRegisteredWaiter(t *testing.T) {
	ctx, done := context.WithTimeout(t.Context(), time.Second)
	defer done()
	var dispatcher nativeQuestionDispatch
	start := func(action string, scope protocol.NativeQuestionScope) <-chan context.Context {
		msg, err := ws.NewRequest(uuid.NewString(), action, scope)
		require.NoError(t, err)
		mcporigin.AttachTrustedInternalCall(msg)
		entered := make(chan context.Context, 1)
		dispatcher.start(ctx, msg, func(ctx context.Context) {
			entered <- ctx
			if action == ws.ActionMCPAskUserQuestion {
				<-ctx.Done()
			}
		})
		return entered
	}
	first := protocol.NativeQuestionScope{SessionID: "session", NativeRequestID: uuid.NewString()}
	second := protocol.NativeQuestionScope{SessionID: first.SessionID, NativeRequestID: uuid.NewString()}
	firstWaiter := start(ws.ActionMCPAskUserQuestion, first)
	secondWaiter := start(ws.ActionMCPAskUserQuestion, second)
	start(ws.ActionMCPCancelNativeClarification, first)
	require.ErrorIs(t, context.Cause(<-firstWaiter), protocol.ErrNativeQuestionCancelled)
	require.NoError(t, (<-secondWaiter).Err())
}
