package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
)

// The upload DTOs and the conflict sentinel live in internal/agentctl/types so
// higher-level callers can consume them without importing this runtime-tier
// package (ARCH-RUNTIME-IMPORT).
type (
	// UploadedWorkspaceFile is the path agentctl actually wrote.
	UploadedWorkspaceFile = agentctltypes.UploadedWorkspaceFile
	// WorkspaceUploadConflict is one destination that already exists.
	WorkspaceUploadConflict = agentctltypes.WorkspaceUploadConflict
)

// ErrWorkspaceUploadConflict reports that the destination already exists and the
// caller supplied no resolution. Callers surface this as a prompt rather than an
// error, so it is a sentinel instead of a status string.
var ErrWorkspaceUploadConflict = agentctltypes.ErrWorkspaceUploadConflict

// WorkspaceUploadParams describes one file destined for the task workspace.
type WorkspaceUploadParams = agentctltypes.WorkspaceUploadParams

// PreflightWorkspaceUpload reports which of the candidate destinations already
// exist, so the caller can resolve every conflict before sending any bytes.
func (c *Client) PreflightWorkspaceUpload(
	ctx context.Context,
	dir, repo string,
	paths []string,
) ([]WorkspaceUploadConflict, error) {
	payload, err := json.Marshal(map[string]any{"dir": dir, "repo": repo, "paths": paths})
	if err != nil {
		return nil, fmt.Errorf("encode upload preflight: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/api/v1/workspace/file/upload-preflight",
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("create upload preflight request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.longRunningHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload preflight: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkWorkspaceUploadStatus(resp); err != nil {
		return nil, err
	}

	var result struct {
		Conflicts []WorkspaceUploadConflict `json:"conflicts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode upload preflight: %w", err)
	}
	return result.Conflicts, nil
}

// UploadWorkspaceFile streams one file into the task workspace. The body is
// multipart and piped, so no layer holds the whole file in memory and nothing
// is base64-encoded.
func (c *Client) UploadWorkspaceFile(
	ctx context.Context,
	upload WorkspaceUploadParams,
	content io.Reader,
) (*UploadedWorkspaceFile, error) {
	if strings.TrimSpace(upload.RelativePath) == "" || content == nil {
		return nil, fmt.Errorf("upload path and content are required")
	}
	if upload.SizeBytes < 0 || upload.SizeBytes > models.MaxMessageAttachmentBytes {
		return nil, fmt.Errorf("upload size exceeds maximum")
	}

	resp, err := c.doMultipartRequest(ctx, c.baseURL+"/api/v1/workspace/file/upload", func(writer *multipart.Writer) error {
		return writeWorkspaceUploadMultipart(writer, upload, content)
	})
	if err != nil {
		return nil, fmt.Errorf("upload workspace file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkWorkspaceUploadStatus(resp); err != nil {
		return nil, err
	}

	var result UploadedWorkspaceFile
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode workspace upload: %w", err)
	}
	if strings.TrimSpace(result.Path) == "" {
		return nil, fmt.Errorf("workspace upload returned no path")
	}
	return &result, nil
}

// checkWorkspaceUploadStatus converts a non-2xx response into an error,
// preserving the conflict case as a sentinel so callers can prompt instead of
// failing.
func checkWorkspaceUploadStatus(resp *http.Response) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	message := strings.TrimSpace(string(body))
	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("%w: %s", ErrWorkspaceUploadConflict, message)
	}
	return fmt.Errorf("workspace upload failed with status %d: %s", resp.StatusCode, message)
}

func writeWorkspaceUploadMultipart(
	writer *multipart.Writer,
	upload WorkspaceUploadParams,
	content io.Reader,
) error {
	fields := [...]struct {
		key   string
		value string
	}{
		{key: "dir", value: upload.Dir},
		{key: "repo", value: upload.Repo},
		{key: "relative_path", value: upload.RelativePath},
		{key: "resolution", value: upload.Resolution},
		{key: "size_bytes", value: strconv.FormatInt(upload.SizeBytes, 10)},
	}
	for _, field := range fields {
		if err := writer.WriteField(field.key, field.value); err != nil {
			return fmt.Errorf("write upload field %s: %w", field.key, err)
		}
	}
	part, err := writer.CreateFormFile("file", upload.RelativePath)
	if err != nil {
		return fmt.Errorf("create upload file part: %w", err)
	}
	if _, err := io.Copy(part, content); err != nil {
		return fmt.Errorf("stream upload content: %w", err)
	}
	return writer.Close()
}
