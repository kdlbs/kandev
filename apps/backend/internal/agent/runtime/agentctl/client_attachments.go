package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

// MaterializedAttachment is the agentctl-side filename selected for a
// descriptor. The name is relative to the session attachment directory.
type MaterializedAttachment struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
}

// MaterializeAttachment streams one claimed attachment to the agentctl
// session. The request body is multipart and remains bounded by the backend's
// descriptor limit; the client never base64-encodes the file.
func (c *Client) MaterializeAttachment(
	ctx context.Context,
	sessionID, attachmentID, name, mimeType string,
	sizeBytes int64,
	content io.Reader,
) (*MaterializedAttachment, error) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(attachmentID) == "" || content == nil {
		return nil, fmt.Errorf("attachment session, id, and content are required")
	}
	if sizeBytes < 0 || sizeBytes > models.MaxMessageAttachmentBytes {
		return nil, fmt.Errorf("attachment size exceeds maximum")
	}
	resp, err := c.doMultipartRequest(ctx, c.baseURL+"/api/v1/attachments/materialize", func(writer *multipart.Writer) error {
		return writeMaterializedAttachmentMultipart(writer, sessionID, attachmentID, name, mimeType, sizeBytes, content)
	})
	if err != nil {
		return nil, fmt.Errorf("materialize attachment: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		return nil, fmt.Errorf("materialize attachment failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result MaterializedAttachment
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode materialized attachment: %w", err)
	}
	if strings.TrimSpace(result.Name) == "" || result.SizeBytes != sizeBytes {
		return nil, fmt.Errorf("materialize attachment returned invalid metadata")
	}
	return &result, nil
}

func writeMaterializedAttachmentMultipart(
	writer *multipart.Writer,
	sessionID, attachmentID, name, mimeType string,
	sizeBytes int64,
	content io.Reader,
) error {
	fields := [...]struct {
		key   string
		value string
	}{
		{key: "session_id", value: sessionID},
		{key: "attachment_id", value: attachmentID},
		{key: "name", value: name},
		{key: "mime_type", value: mimeType},
		{key: "size_bytes", value: strconv.FormatInt(sizeBytes, 10)},
	}
	for _, field := range fields {
		if err := writer.WriteField(field.key, field.value); err != nil {
			return err
		}
	}
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, content); err != nil {
		return err
	}
	return writer.Close()
}
