package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/kandev/kandev/internal/agentctl/types"
)

const builtinMCPCredentialResponseLimit = 4096

// BuiltinMCPCredential reads the instance's built-in MCP caller credential
// over the control-authenticated instance API. An agentctl without the route
// predates caller authentication and enforces nothing, so it yields an empty
// credential and no error. Any other failure is returned, so callers never
// fall back to header-less entries for an instance that may enforce.
func (c *Client) BuiltinMCPCredential(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+types.BuiltinMCPCredentialPath, nil)
	if err != nil {
		return "", err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("read built-in MCP credential: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("read built-in MCP credential: HTTP %d", response.StatusCode)
	}
	var result types.BuiltinMCPCredentialResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, builtinMCPCredentialResponseLimit)).Decode(&result); err != nil {
		return "", fmt.Errorf("decode built-in MCP credential: %w", err)
	}
	return result.Credential, nil
}
