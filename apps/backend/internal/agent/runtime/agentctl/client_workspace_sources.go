package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// MaterializeRepositoryRequest is the credential-free repository checkout
// request accepted by agentctl. It intentionally has no credential field;
// lifecycle wiring supplies Git credentials separately when that is needed.
type MaterializeRepositoryRequest struct {
	RepositoryURL  string `json:"repository_url"`
	Destination    string `json:"destination"`
	BaseBranch     string `json:"base_branch"`
	CheckoutBranch string `json:"checkout_branch,omitempty"`
}

// MaterializeRepositoryResponse reports the adopted workspace subdirectory.
type MaterializeRepositoryResponse struct {
	Destination string `json:"destination"`
	Reused      bool   `json:"reused,omitempty"`
	Error       string `json:"error,omitempty"`
}

// RemoveMaterializedRepositoryRequest identifies an owned checkout for
// rollback after a later item in a remote materialization batch fails.
type RemoveMaterializedRepositoryRequest struct {
	RepositoryURL string `json:"repository_url"`
	Destination   string `json:"destination"`
}

type removeMaterializedRepositoryResponse struct {
	Removed bool   `json:"removed"`
	Error   string `json:"error,omitempty"`
}

// WorkspaceMaterializationError preserves an actionable remote status without
// retaining or formatting the repository locator supplied by the caller.
type WorkspaceMaterializationError struct {
	StatusCode int
	Message    string
}

func (e *WorkspaceMaterializationError) Error() string {
	return fmt.Sprintf("workspace repository materialization failed (%d): %s", e.StatusCode, e.Message)
}

// MaterializeRepository asks the live agentctl instance to atomically clone
// and check out a repository under its current workspace root.
func (c *Client) MaterializeRepository(ctx context.Context, materialization MaterializeRepositoryRequest) (*MaterializeRepositoryResponse, error) {
	body, err := json.Marshal(materialization)
	if err != nil {
		return nil, fmt.Errorf("marshal workspace materialization request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/workspace/materialize-repository", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create workspace materialization request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send workspace materialization request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var response MaterializeRepositoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode workspace materialization response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || response.Error != "" {
		message := response.Error
		if message == "" {
			message = "remote agentctl rejected the request"
		}
		return &response, &WorkspaceMaterializationError{StatusCode: resp.StatusCode, Message: message}
	}
	return &response, nil
}

// RemoveMaterializedRepository removes only a checkout whose destination and
// origin match the supplied credential-free request. Nonexistent destinations
// are treated as a successful idempotent rollback by agentctl.
func (c *Client) RemoveMaterializedRepository(ctx context.Context, removal RemoveMaterializedRepositoryRequest) error {
	body, err := json.Marshal(removal)
	if err != nil {
		return fmt.Errorf("marshal workspace cleanup request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/workspace/materialize-repository/remove", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create workspace cleanup request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send workspace cleanup request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var response removeMaterializedRepositoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("decode workspace cleanup response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || response.Error != "" {
		message := response.Error
		if message == "" {
			message = "remote agentctl rejected the cleanup request"
		}
		return &WorkspaceMaterializationError{StatusCode: resp.StatusCode, Message: message}
	}
	return nil
}
