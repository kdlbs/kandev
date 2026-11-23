package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/agentctl/types"
	"io"
	"net/http"
)

var errFilePreviewUnavailable = errors.New("file preview unavailable")

func (c *Client) filePreviewRequest(ctx context.Context, operation string, payload any) (*http.Response, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, errFilePreviewUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/workspace/file-preview/"+operation, bytes.NewReader(data))
	if err != nil {
		return nil, errFilePreviewUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	client := *c.longRunningHTTPClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client.Do(req)
}

// CopyFilePreview streams bytes but reports success only after agentctl validates
// the final revision. Callers must withhold their completion marker on failure.
func (c *Client) CopyFilePreview(ctx context.Context, token string, size int64, w io.Writer) error {
	if size < 0 || size > 64<<20 {
		return errFilePreviewUnavailable
	}
	resp, err := c.filePreviewRequest(ctx, "read", map[string]string{"token": token})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/octet-stream" {
		return errFilePreviewUnavailable
	}
	n, err := io.Copy(w, io.LimitReader(resp.Body, size+1))
	if err != nil {
		return err
	}
	if n != size || resp.Trailer.Get("X-Kandev-Preview-Result") != "complete" {
		return errFilePreviewUnavailable
	}
	return nil
}

// PrepareFilePreview obtains metadata without transferring model bytes.
func (c *Client) PrepareFilePreview(ctx context.Context, path, repo string) (*types.FilePreviewMetadata, error) {
	resp, err := c.filePreviewRequest(ctx, "prepare", map[string]string{"path": path, "repo": repo})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusRequestEntityTooLarge {
		return nil, types.ErrFilePreviewLimit
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errFilePreviewUnavailable
	}
	var meta types.FilePreviewMetadata
	if json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&meta) != nil || meta.Token == "" || len(meta.Token) > 128 || meta.SizeBytes < 0 || meta.SizeBytes > 64<<20 {
		return nil, errFilePreviewUnavailable
	}
	return &meta, nil
}

// CancelFilePreview releases a pending selection on its original executor.
func (c *Client) CancelFilePreview(ctx context.Context, token string) error {
	resp, err := c.filePreviewRequest(ctx, "cancel", map[string]string{"token": token})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		return errFilePreviewUnavailable
	}
	return nil
}
