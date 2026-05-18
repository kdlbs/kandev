package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// StartProcessRequest contains the parameters for starting a process.
type StartProcessRequest struct {
	SessionID      string            `json:"session_id"`
	Kind           ProcessKind       `json:"kind"`
	ScriptName     string            `json:"script_name,omitempty"`
	Command        string            `json:"command"`
	WorkingDir     string            `json:"working_dir"`
	Env            map[string]string `json:"env,omitempty"`
	BufferMaxBytes int64             `json:"buffer_max_bytes,omitempty"`
}

// ProcessOutputChunk is a single chunk of process output.
type ProcessOutputChunk struct {
	Stream    string    `json:"stream"`
	Data      string    `json:"data"`
	Timestamp time.Time `json:"timestamp"`
}

// ProcessInfo contains information about a running or completed process.
type ProcessInfo struct {
	ID         string               `json:"id"`
	SessionID  string               `json:"session_id"`
	Kind       ProcessKind          `json:"kind"`
	ScriptName string               `json:"script_name,omitempty"`
	Command    string               `json:"command"`
	WorkingDir string               `json:"working_dir"`
	Status     ProcessStatus        `json:"status"`
	ExitCode   *int                 `json:"exit_code,omitempty"`
	StartedAt  time.Time            `json:"started_at"`
	UpdatedAt  time.Time            `json:"updated_at"`
	Output     []ProcessOutputChunk `json:"output,omitempty"`
}

type startProcessResponse struct {
	Process *ProcessInfo `json:"process,omitempty"`
	Error   string       `json:"error,omitempty"`
}

type stopProcessResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// StartProcess starts a new process via HTTP POST
func (c *Client) StartProcess(ctx context.Context, req StartProcessRequest) (*ProcessInfo, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/processes/start", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("start process failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result startProcessResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse start process response: %w", err)
	}
	if result.Process == nil {
		if result.Error != "" {
			return nil, fmt.Errorf("start process failed: %s", result.Error)
		}
		return nil, fmt.Errorf("start process failed: no process returned")
	}
	return result.Process, nil
}

// StopProcess stops a running process via HTTP POST
func (c *Client) StopProcess(ctx context.Context, processID string) error {
	body, err := json.Marshal(map[string]string{"process_id": processID})
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/processes/stop", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("stop process failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result stopProcessResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse stop process response: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("stop process failed: %s", result.Error)
	}
	return nil
}

// ListProcesses lists processes, optionally filtered by session ID
func (c *Client) ListProcesses(ctx context.Context, sessionID string) ([]ProcessInfo, error) {
	reqURL := c.baseURL + "/api/v1/processes"
	if sessionID != "" {
		reqURL = reqURL + "?session_id=" + sessionID
	}
	httpReq, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list processes failed with status %d", resp.StatusCode)
	}
	var result []ProcessInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetProcess retrieves a specific process by ID
func (c *Client) GetProcess(ctx context.Context, id string, includeOutput bool) (*ProcessInfo, error) {
	reqURL := c.baseURL + "/api/v1/processes/" + id
	if includeOutput {
		reqURL += "?include_output=true"
	}
	httpReq, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get process failed with status %d", resp.StatusCode)
	}
	var result ProcessInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
