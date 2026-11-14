package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/usage"
)

func (c *Client) ConfigureAgentWithUsage(ctx context.Context, command string, args []string, env map[string]string, continueCommand string, continueArgs []string, policy *usage.ExecutionUsagePolicy) error {
	return c.configureAgent(ctx, command, args, env, continueCommand, continueArgs, false, policy, nil, nil)
}

func (c *Client) ReadExecutionUsage(ctx context.Context, model string) (*usage.ExecutionUsageReport, error) {
	return c.readExecutionUsage(ctx, model, false)
}

func (c *Client) PeekExecutionUsage(ctx context.Context) (*usage.ExecutionUsageReport, error) {
	return c.readExecutionUsage(ctx, "", true)
}

func (c *Client) readExecutionUsage(ctx context.Context, model string, cachedOnly bool) (*usage.ExecutionUsageReport, error) {
	var report usage.ExecutionUsageReport
	path := "/api/v1/agent/usage?model=" + url.QueryEscape(model)
	if cachedOnly {
		path = "/api/v1/agent/usage/cached"
	}
	request, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := readResponseBody(response)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusNotFound && strings.HasPrefix(response.Header.Get("Content-Type"), "text/plain") && strings.TrimSpace(string(body)) == "404 page not found" {
		return &usage.ExecutionUsageReport{Reason: "native_usage_unsupported", Model: model}, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("execution usage read failed: status %d", response.StatusCode)
	}
	if err := json.Unmarshal(body, &report); err != nil {
		return nil, fmt.Errorf("invalid execution usage report: %w", err)
	}
	return &report, nil
}

func executionUsageAdmissionError(report *usage.ExecutionUsageReport) error {
	if report == nil {
		return fmt.Errorf("execution usage observation unavailable")
	}
	if report.BindingChanged {
		return &usage.ExecutionUsageBindingError{}
	}
	if !report.Limited {
		return nil
	}
	return &routingerr.Error{Code: routingerr.CodeQuotaLimited, Class: routingerr.ClassHard, CatalogueVersion: routingerr.CatalogueVersion, Confidence: routingerr.ConfHigh, Phase: routingerr.PhasePromptSend, FallbackAllowed: true, AutoRetryable: report.ResetAt != nil, ResetHint: report.ResetAt, ClassifierRule: "native_measured_usage_limit"}
}

func (c *Client) CheckExecutionUsage(ctx context.Context, model string) error {
	report, err := c.ReadExecutionUsage(ctx, model)
	if err != nil {
		return err
	}
	return executionUsageAdmissionError(report)
}
