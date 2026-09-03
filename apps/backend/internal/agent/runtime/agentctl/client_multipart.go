package client

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// doMultipartRequest owns the pipe and writer lifecycle for streaming agentctl
// requests. Callers only provide the endpoint and the fields/file writer.
func (c *Client) doMultipartRequest(
	ctx context.Context,
	url string,
	write func(*multipart.Writer) error,
) (*http.Response, error) {
	pipeReader, pipeWriter := io.Pipe()
	multipartWriter := multipart.NewWriter(pipeWriter)
	go func() {
		if err := write(multipartWriter); err != nil {
			_ = pipeWriter.CloseWithError(err)
			return
		}
		_ = pipeWriter.Close()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pipeReader)
	if err != nil {
		_ = pipeReader.Close()
		return nil, fmt.Errorf("create multipart request: %w", err)
	}
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	resp, err := c.longRunningHTTPClient.Do(req)
	if err != nil {
		_ = pipeReader.Close()
		return nil, err
	}
	return resp, nil
}
