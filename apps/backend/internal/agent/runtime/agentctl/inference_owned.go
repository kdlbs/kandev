package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

// InferencePromptOwned prepares a one-use owner token, then executes that token.
// Old agentctl fallback preserves legacy execution, never known settlement.
// Capacity controllers must require supported observation before admitting work.
func (c *Client) InferencePromptOwned(ctx context.Context, req *utility.PromptRequest) (*utility.PromptResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("inference prompt request is nil")
	}
	request := *req
	if request.AttemptID == "" {
		prepared, err := c.PrepareInferenceAttempt(ctx, request.CallID)
		if errors.Is(err, ErrInferenceOwnershipUnsupported) {
			return c.InferencePrompt(ctx, req)
		}
		if err != nil {
			return nil, err
		}
		request.AttemptID = prepared.AttemptID
	}
	response, err := c.InferencePrompt(ctx, &request)
	if err == nil {
		return response, nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.httpClient.Timeout)
	defer cancel()
	if settled, cleanupErr := c.CancelPreparedInferenceAttempt(cleanupCtx, request.AttemptID); cleanupErr == nil {
		var failure *InferenceAttemptError
		if errors.As(err, &failure) {
			failure.Attempt = settled
		}
	}
	return nil, err
}
