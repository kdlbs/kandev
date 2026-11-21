package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"io"
	"net/http"
)

func (c *Client) InferenceClosure(ctx context.Context, request utility.InferenceClosureRequest) (*utility.InferenceClosureResponse, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/inference/closure", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusNotFound {
			return nil, ErrInferenceOwnershipUnsupported
		}
		return nil, fmt.Errorf("inference closure unavailable (status %d)", response.StatusCode)
	}
	var result utility.InferenceClosureResponse
	if err = json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&result); err != nil {
		return nil, err
	}
	if err = c.validateInferenceSnapshot(&result.Snapshot); err != nil {
		return nil, err
	}
	if request.Action == "probe" {
		if !result.Snapshot.Known || len(result.State) == 0 {
			return nil, utility.ErrInferenceClosureIncomplete
		}
		return &result, nil
	}
	closure := result.Snapshot.Closure
	if closure == nil || closure.OperationID != request.OperationID || len(result.State) == 0 {
		return nil, utility.ErrInferenceClosureIncomplete
	}
	return &result, nil
}

type inferenceClosureAdoptionKey struct{}

// WithInferenceClosureAdoption carries an already validated internal operation.
func WithInferenceClosureAdoption(ctx context.Context, operation string) context.Context {
	return context.WithValue(ctx, inferenceClosureAdoptionKey{}, operation)
}

func InferenceClosureAdoptionFromContext(ctx context.Context) string {
	operation, _ := ctx.Value(inferenceClosureAdoptionKey{}).(string)
	return operation
}
