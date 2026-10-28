package github

import (
	"context"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// withWSExpectedGitHubHost preserves the resource origin across workspace host changes.
// Services authorize workspace access before comparing this origin to the connection.
func withWSExpectedGitHubHost(handler ws.HandlerFunc) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var payload struct {
			ExpectedHost string `json:"expected_host"`
		}
		if err := msg.ParsePayload(&payload); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, errMsgInvalidPayload, nil)
		}
		return handler(withExpectedGitHubHost(ctx, payload.ExpectedHost), msg)
	}
}
