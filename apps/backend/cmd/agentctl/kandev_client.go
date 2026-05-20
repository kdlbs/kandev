package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// kandevClient is a thin HTTP client for the kandev office API.
// It reads configuration from environment variables set by the kandev backend
// when launching agent containers or processes.
type kandevClient struct {
	apiURL      string // KANDEV_API_URL
	apiKey      string // KANDEV_API_KEY
	runID       string // KANDEV_RUN_ID
	agentID     string // KANDEV_AGENT_ID
	taskID      string // KANDEV_TASK_ID (default for --id/--task flags)
	workspaceID string // KANDEV_WORKSPACE_ID
	http        *http.Client
}

// newKandevClient creates a client from environment variables.
// Returns an error if required variables (KANDEV_API_URL, KANDEV_API_KEY) are missing.
func newKandevClient() (*kandevClient, error) {
	apiURL := os.Getenv("KANDEV_API_URL")
	apiKey := os.Getenv("KANDEV_API_KEY")
	if apiURL == "" || apiKey == "" {
		return nil, fmt.Errorf("KANDEV_API_URL and KANDEV_API_KEY must be set")
	}
	return &kandevClient{
		apiURL:      apiURL,
		apiKey:      apiKey,
		runID:       os.Getenv("KANDEV_RUN_ID"),
		agentID:     os.Getenv("KANDEV_AGENT_ID"),
		taskID:      os.Getenv("KANDEV_TASK_ID"),
		workspaceID: os.Getenv("KANDEV_WORKSPACE_ID"),
		http:        &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// isMutating returns true for methods that modify server state.
func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

// do sends an HTTP request to the kandev API and returns the response body,
// status code, and any error. It sets Authorization and, for mutating requests,
// X-Kandev-Run-Id headers automatically.
func (c *kandevClient) do(method, path string, body any) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := c.apiURL + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if isMutating(method) && c.runID != "" {
		req.Header.Set("X-Kandev-Run-Id", c.runID)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}
