package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

const MaxWorkspacePreviewContentBytes = agentctltypes.MaxWorkspacePreviewContentBytes

// WorkspacePreviewRequest is the current editor buffer to publish through
// agentctl.
type WorkspacePreviewRequest = agentctltypes.WorkspacePreviewRequest

// WorkspacePreviewResponse identifies the live agentctl preview server and
// the published entry document.
type WorkspacePreviewResponse = agentctltypes.WorkspacePreviewResponse

// PublishWorkspacePreview publishes one current HTML editor buffer.
func (c *Client) PublishWorkspacePreview(ctx context.Context, payload WorkspacePreviewRequest) (WorkspacePreviewResponse, error) {
	if len([]byte(payload.Content)) > MaxWorkspacePreviewContentBytes {
		return WorkspacePreviewResponse{}, fmt.Errorf("workspace preview content exceeds 5 MiB")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return WorkspacePreviewResponse{}, fmt.Errorf("marshal workspace preview request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/workspace/html-previews", bytes.NewReader(body))
	if err != nil {
		return WorkspacePreviewResponse{}, fmt.Errorf("create workspace preview request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return WorkspacePreviewResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := readResponseBody(resp)
	if err != nil {
		return WorkspacePreviewResponse{}, fmt.Errorf("read workspace preview response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return WorkspacePreviewResponse{}, fmt.Errorf(
			"workspace preview publish failed with status %d: %s",
			resp.StatusCode,
			truncateBody(responseBody),
		)
	}
	var result WorkspacePreviewResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return WorkspacePreviewResponse{}, fmt.Errorf("parse workspace preview response: %w", err)
	}
	if result.Port < 1024 || result.Port > 65535 || result.Version == 0 || !strings.HasPrefix(result.Path, "/") {
		return WorkspacePreviewResponse{}, fmt.Errorf("invalid workspace preview response")
	}
	return result, nil
}
