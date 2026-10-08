package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/kandev/kandev/internal/common/turnchanges"
)

const turnChangesResponseLimit = 48 << 20

// TurnCheckpointClientError preserves the executor's structured capture reason.
type TurnCheckpointClientError struct {
	Status  int
	Reason  turnchanges.ReasonCode
	Message string
}

func (e *TurnCheckpointClientError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("turn checkpoint request failed with status %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("turn checkpoint request failed with status %d", e.Status)
}

func (e *TurnCheckpointClientError) CaptureReason() turnchanges.ReasonCode { return e.Reason }

type turnCheckpointWireError struct {
	Error  string                 `json:"error"`
	Reason turnchanges.ReasonCode `json:"reason"`
}

func (c *Client) CaptureTurnCheckpoint(ctx context.Context, request turnchanges.CheckpointRequest) (*turnchanges.CheckpointResult, error) {
	var result turnchanges.CheckpointResult
	if err := c.doTurnChangesJSON(ctx, "/api/v1/git/turn-changes/checkpoint", request, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteTurnCheckpoint(ctx context.Context, request turnchanges.CheckpointDeleteRequest) error {
	return c.doTurnChangesJSON(ctx, "/api/v1/git/turn-changes/delete", request, &struct{}{})
}

func (c *Client) CompareTurnCheckpoints(ctx context.Context, request turnchanges.CompareRequest) (*turnchanges.CheckpointComparison, error) {
	var result turnchanges.CheckpointComparison
	if err := c.doTurnChangesJSON(ctx, "/api/v1/git/turn-changes/compare", request, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ExportTurnCheckpoint(ctx context.Context, request turnchanges.ExportRequest) (*turnchanges.CheckpointExport, error) {
	var result turnchanges.CheckpointExport
	if err := c.doTurnChangesJSON(ctx, "/api/v1/git/turn-changes/export", request, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) TurnCheckpointRepositoryScopes(ctx context.Context) ([]string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/git/turn-changes/scopes", nil)
	if err != nil {
		return nil, err
	}
	response, err := c.longRunningHTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("turn checkpoint scope request failed: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	limited := io.LimitReader(response.Body, turnChangesResponseLimit+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read turn checkpoint scopes: %w", err)
	}
	if len(responseBody) > turnChangesResponseLimit {
		return nil, fmt.Errorf("turn checkpoint scopes exceed %d bytes", turnChangesResponseLimit)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("turn checkpoint scope request failed with status %d", response.StatusCode)
	}
	var result struct {
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse turn checkpoint scopes: %w", err)
	}
	if result.Scopes == nil {
		result.Scopes = []string{}
	}
	return result.Scopes, nil
}

func (c *Client) doTurnChangesJSON(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("failed to marshal turn checkpoint request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.longRunningHTTPClient.Do(request)
	if err != nil {
		return fmt.Errorf("turn checkpoint request failed: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	limited := io.LimitReader(response.Body, turnChangesResponseLimit+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("failed to read turn checkpoint response: %w", err)
	}
	if len(responseBody) > turnChangesResponseLimit {
		return fmt.Errorf("turn checkpoint response exceeds %d bytes", turnChangesResponseLimit)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var wireError turnCheckpointWireError
		_ = json.Unmarshal(responseBody, &wireError)
		if wireError.Error == "" {
			wireError.Error = truncateBody(responseBody)
		}
		return &TurnCheckpointClientError{Status: response.StatusCode, Reason: wireError.Reason, Message: wireError.Error}
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("failed to parse turn checkpoint response: %w", err)
	}
	return nil
}
