package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	p "github.com/kandev/kandev/internal/mcp/profile"
)

// PrepareBuiltinMCP binds an unused workspace helper before its agent starts.
// A successful HTTP response must confirm the exact prepared profile.
func (c *Client) PrepareBuiltinMCP(ctx context.Context, profile p.Context) error {
	body, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	return c.checkBuiltinMCP(ctx, http.MethodPut, bytes.NewReader(body), profile)
}

// VerifyBuiltinMCP compares the retained helper's registry profile with the
// execution binding without changing either side.
func (c *Client) VerifyBuiltinMCP(ctx context.Context, profile p.Context) error {
	return c.checkBuiltinMCP(ctx, http.MethodGet, nil, profile)
}

func (c *Client) checkBuiltinMCP(ctx context.Context, method string, body io.Reader, profile p.Context) error {
	expected, err := p.MarshalBuiltinBinding(profile)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v1/mcp/builtin-binding", body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound {
		return errors.New("agentctl cannot bind built-in MCP packages; upgrade the helper before starting this session")
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("prepare built-in MCP binding: HTTP %d", response.StatusCode)
	}
	var confirmed p.Context
	if err := json.NewDecoder(response.Body).Decode(&confirmed); err != nil {
		return err
	}
	actual, err := p.MarshalBuiltinBinding(confirmed)
	if err != nil {
		return err
	}
	if actual != expected {
		return errors.New("agentctl confirmed a different built-in MCP binding")
	}
	return nil
}
