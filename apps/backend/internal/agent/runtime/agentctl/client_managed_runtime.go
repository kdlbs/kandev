package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentctl/tracing"
)

// RepairManagedRuntimeCacheRequest is the authenticated agentctl maintenance
// request for one exact managed npm package specification.
type RepairManagedRuntimeCacheRequest struct {
	PackageSpec string `json:"package_spec"`
}

// RepairManagedRuntimeCacheResponse is the typed success response from the
// authenticated agentctl maintenance endpoint.
type RepairManagedRuntimeCacheResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// RepairManagedRuntimeCache asks the colocated agentctl process to resolve its
// npm cache and remove only the exact execution tree for packageSpec.
func (c *Client) RepairManagedRuntimeCache(ctx context.Context, packageSpec string) error {
	if err := managedruntime.ValidateExactPackageSpec(packageSpec); err != nil {
		return err
	}
	ctx, span := tracing.TraceHTTPRequest(ctx, http.MethodPost, "/api/v1/agent/managed-runtime/cache-repair", c.executionID)
	defer span.End()

	body, err := json.Marshal(RepairManagedRuntimeCacheRequest{PackageSpec: packageSpec})
	if err != nil {
		tracing.TraceHTTPResponse(span, 0, err)
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/agent/managed-runtime/cache-repair", bytes.NewReader(body))
	if err != nil {
		tracing.TraceHTTPResponse(span, 0, err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		tracing.TraceHTTPResponse(span, 0, err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := readResponseBody(resp)
	if err != nil {
		tracing.TraceHTTPResponse(span, resp.StatusCode, err)
		return fmt.Errorf("read managed runtime cache repair response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("managed runtime cache repair failed with status %d", resp.StatusCode)
		tracing.TraceHTTPResponse(span, resp.StatusCode, err)
		return err
	}
	var result RepairManagedRuntimeCacheResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		tracing.TraceHTTPResponse(span, resp.StatusCode, err)
		return fmt.Errorf("parse managed runtime cache repair response: %w", err)
	}
	if !result.Success {
		err := fmt.Errorf("managed runtime cache repair was not successful")
		tracing.TraceHTTPResponse(span, resp.StatusCode, err)
		return err
	}
	tracing.TraceHTTPResponse(span, resp.StatusCode, nil)
	return nil
}
