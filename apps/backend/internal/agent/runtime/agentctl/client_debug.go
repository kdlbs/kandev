package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const maxACPDebugExportBytes = int64(96 * 1024 * 1024)

// ExportACPDebug requests a bounded raw+normalized ACP ZIP for one exact
// agentctl ACP session. The response body belongs to the caller and must be
// closed, even when the caller chooses not to materialize it.
func (c *Client) ExportACPDebug(ctx context.Context, sessionID string, maxBytes int64) (io.ReadCloser, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("ACP session ID is required")
	}
	if maxBytes <= 0 || maxBytes > maxACPDebugExportBytes {
		maxBytes = maxACPDebugExportBytes
	}
	path := "/api/v1/debug/acp/" + url.PathEscape(sessionID) + "/export?max_bytes=" + strconv.FormatInt(maxBytes, 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("ACP debug export failed with status %d: %s", resp.StatusCode, string(body))
	}
	return resp.Body, nil
}
