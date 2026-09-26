package codexappserver

import (
	"context"
	"errors"
)

// ForkThread creates a provider-native child thread through the requested
// completed turn. Callers own idempotency and must not retry after ambiguity.
func (c *Client) ForkThread(ctx context.Context, params ThreadForkParams) (*Thread, error) {
	if params.ThreadID == "" {
		return nil, errors.New("thread/fork requires a source thread ID")
	}
	var response ThreadForkResponse
	if err := c.Call(ctx, MethodThreadFork, params, &response); err != nil {
		return nil, err
	}
	if response.Thread.ID == "" {
		return nil, errors.New("thread/fork response omitted thread ID")
	}
	return &response.Thread, nil
}
