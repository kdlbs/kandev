package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/kandev/kandev/internal/agentctl/types"
)

var ErrProviderReplacementUnsupported = errors.New("helper provider replacement is unsupported")
var ErrProviderReplacementConflict = errors.New("helper provider replacement conflicts with current state")

// ProviderReplacement sends one idempotent control action. A lost response is
// reconciled through status using the same operation ID, never a fresh start.
func (c *Client) ProviderReplacement(ctx context.Context, command types.ProviderReplacementRequest) (types.ProviderReplacementStatus, error) {
	body, err := json.Marshal(command)
	if err != nil {
		return types.ProviderReplacementStatus{}, err
	}
	status, err := c.providerReplacementRequest(ctx, http.MethodPost, "", bytes.NewReader(body))
	if err != nil {
		return status, err
	}
	if status.OperationID != command.OperationID || status.Source != command.Source {
		return status, errors.New("helper confirmed a different provider replacement operation")
	}
	if command.Scope.BindingID != "" && (status.Scope == nil || *status.Scope != command.Scope) {
		return status, errors.New("helper confirmed a different provider scope")
	}
	return status, nil
}

func (c *Client) GetProviderReplacementStatus(ctx context.Context, operationID string) (types.ProviderReplacementStatus, error) {
	query := ""
	if operationID != "" {
		query = "?operation_id=" + url.QueryEscape(operationID)
	}
	status, err := c.providerReplacementRequest(ctx, http.MethodGet, query, nil)
	if err != nil {
		return status, err
	}
	if operationID != "" && status.OperationID != operationID {
		return status, errors.New("helper returned another provider replacement operation")
	}
	return status, nil
}

func (c *Client) providerReplacementRequest(ctx context.Context, method, query string, body io.Reader) (types.ProviderReplacementStatus, error) {
	var status types.ProviderReplacementStatus
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v1/agent/provider-replacement"+query, body)
	if err != nil {
		return status, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return status, err
	}
	defer func() { _ = response.Body.Close() }()
	switch response.StatusCode {
	case http.StatusNotFound:
		return status, ErrProviderReplacementUnsupported
	case http.StatusConflict:
		return status, ErrProviderReplacementConflict
	case http.StatusOK:
	default:
		return status, fmt.Errorf("provider replacement control: HTTP %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 256<<10))
	if err := decoder.Decode(&status); err != nil {
		return status, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return status, errors.New("invalid provider replacement response suffix")
	}
	if status.Phase == "" {
		return status, errors.New("provider replacement response has no phase")
	}
	return status, nil
}
