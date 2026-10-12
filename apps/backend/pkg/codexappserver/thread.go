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

// ListBackgroundTerminals lists active background terminals on a thread.
func (c *Client) ListBackgroundTerminals(ctx context.Context, params BackgroundTerminalsListParams) (*BackgroundTerminalsListResponse, error) {
	if params.ThreadID == "" {
		return nil, errors.New("thread/backgroundTerminals/list requires a thread ID")
	}
	var response BackgroundTerminalsListResponse
	if err := c.Call(ctx, MethodBackgroundTerminalsList, params, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// TerminateBackgroundTerminal terminates a background terminal process on a thread.
func (c *Client) TerminateBackgroundTerminal(ctx context.Context, threadID, processID string) (bool, error) {
	if threadID == "" || processID == "" {
		return false, errors.New("thread/backgroundTerminals/terminate requires thread ID and process ID")
	}
	params := BackgroundTerminalTerminateParams{
		ThreadID:  threadID,
		ProcessID: processID,
	}
	var resp struct {
		Terminated *bool `json:"terminated"`
		Success    *bool `json:"success"`
	}
	if err := c.Call(ctx, MethodBackgroundTerminalsTerminate, params, &resp); err != nil {
		return false, err
	}
	if resp.Terminated != nil {
		return *resp.Terminated, nil
	}
	if resp.Success != nil {
		return *resp.Success, nil
	}
	return true, nil
}

// InterruptTurn interrupts a running turn on a thread.
func (c *Client) InterruptTurn(ctx context.Context, threadID, turnID string) error {
	if threadID == "" {
		return errors.New("turn/interrupt requires thread ID")
	}
	params := TurnInterruptParams{
		ThreadID: threadID,
		TurnID:   turnID,
	}
	return c.Call(ctx, MethodTurnInterrupt, params, nil)
}
