package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/kandev/kandev/internal/common/sandbox"
)

func (c *Client) UtilitySandboxReceipt(ctx context.Context, launch *sandbox.Launch) (*sandbox.StartReceipt, error) {
	return c.sandboxReceipt(ctx, launch, "/api/v1/inference/sandbox/")
}
func (c *Client) SandboxStartReceipt(ctx context.Context, launch *sandbox.Launch) (*sandbox.StartReceipt, error) {
	return c.sandboxReceipt(ctx, launch, "/api/v1/agent/sandbox/")
}
func (c *Client) sandboxReceipt(ctx context.Context, launch *sandbox.Launch, prefix string) (*sandbox.StartReceipt, error) {
	if launch == nil {
		return nil, ErrSandboxLaunchUnsupported
	}
	endpoint := c.baseURL + prefix + url.PathEscape(launch.AttemptID) + "/receipt"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, ErrSandboxLaunchUnsupported
	}
	response, err := c.ProxyTransport().RoundTrip(request)
	if err != nil {
		return nil, ErrSandboxLaunchUnsupported
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusConflict {
		return nil, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, ErrSandboxLaunchUnsupported
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(raw) > 4096 {
		return nil, ErrSandboxLaunchUnsupported
	}
	var receipt sandbox.StartReceipt
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&receipt) != nil || decoder.Decode(&struct{}{}) != io.EOF || receipt.ValidateStopped(launch) != nil {
		return nil, ErrSandboxLaunchUnsupported
	}
	return &receipt, nil
}
