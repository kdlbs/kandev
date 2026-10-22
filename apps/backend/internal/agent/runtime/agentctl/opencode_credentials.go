package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/kandev/kandev/internal/agent/managedruntime"
)

const OpenCodeCredentialCapability = managedruntime.OpenCodeCredentialCapability

func (c *Client) OpenCodeCredentialStatus(parent context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/agent/opencode/credentials", nil)
	if err != nil {
		return "", errors.New("could not prepare OpenCode credential status request")
	}
	resp, err := c.longRunningHTTPClient.Do(req)
	if err != nil {
		return "", errors.New("could not inspect destination OpenCode credentials")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("destination OpenCode credential inspection failed")
	}
	body, err := readResponseBody(resp)
	var result struct {
		Version int    `json:"version"`
		State   string `json:"state"`
	}
	if err != nil || json.Unmarshal(body, &result) != nil || result.Version != 1 {
		return "", errors.New("invalid OpenCode credential status response")
	}
	switch result.State {
	case "empty", "present", "pending":
		return result.State, nil
	default:
		return "", errors.New("unknown OpenCode credential status")
	}
}

// CheckOpenCodeCredentialCapability must succeed before the controller exports accounts.
func (c *Client) CheckOpenCodeCredentialCapability(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/identity", nil)
	if err != nil {
		return errors.New("could not prepare OpenCode worker capability check")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.New("could not reach agentctl for OpenCode credential capability check")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return errors.New("update agentctl: OpenCode credential bootstrap capability is required")
	}
	if resp.StatusCode != http.StatusOK {
		return errors.New("agentctl OpenCode credential capability check failed")
	}
	body, err := readResponseBody(resp)
	var identity struct {
		Capabilities []string `json:"capabilities"`
	}
	if err != nil || json.Unmarshal(body, &identity) != nil {
		return errors.New("invalid agentctl credential capability response")
	}
	if !slices.Contains(identity.Capabilities, OpenCodeCredentialCapability) {
		return errors.New("update agentctl: OpenCode credential bootstrap capability is required")
	}
	return nil
}

// ImportOpenCodeCredentials sends credentials only in the authenticated request
// body. Peer response bodies are deliberately excluded from diagnostics.
func (c *Client) ImportOpenCodeCredentials(parent context.Context, data []byte) error {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	if err := c.CheckOpenCodeCredentialCapability(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/agent/opencode/credentials", bytes.NewReader(data))
	if err != nil {
		return errors.New("could not prepare OpenCode credential import")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.longRunningHTTPClient.Do(req)
	if err != nil {
		return errors.New("could not deliver OpenCode credentials")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return errors.New("OpenCode credential import failed; ensure agentctl and OpenCode support credential provisioning")
	}
	var result struct {
		Success bool `json:"success"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1024)).Decode(&result) != nil || !result.Success {
		return errors.New("OpenCode credential import was not acknowledged")
	}
	return nil
}
