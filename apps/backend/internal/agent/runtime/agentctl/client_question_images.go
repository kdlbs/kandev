package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/kandev/kandev/internal/common/imagepreview"
)

// RequestQuestionImage uses the dedicated bounded endpoint, including on remote executors.
func (c *Client) RequestQuestionImage(ctx context.Context, path, repo string) (*imagepreview.Content, error) {
	query := url.Values{"path": {path}, "repo": {repo}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/workspace/image-preview?"+query.Encode(), nil)
	if err != nil {
		return nil, imagepreview.Unavailable
	}
	httpClient := *c.httpClient
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, imagepreview.Unavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		var failure struct {
			Code string `json:"code"`
		}
		if err = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&failure); err == nil && failure.Code != "" {
			return nil, imagepreview.ParseError(failure.Code)
		}
		if resp.StatusCode == http.StatusNotFound {
			return nil, imagepreview.Unsupported
		}
		return nil, imagepreview.Unavailable
	}
	mime := resp.Header.Get("Content-Type")
	if mime != "image/png" && mime != "image/jpeg" {
		return nil, imagepreview.InvalidImage
	}
	if resp.ContentLength > imagepreview.MaxBytes {
		return nil, imagepreview.TooLarge
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, imagepreview.MaxBytes+1))
	if err != nil {
		return nil, imagepreview.Unavailable
	}
	if len(data) > imagepreview.MaxBytes {
		return nil, imagepreview.TooLarge
	}
	return &imagepreview.Content{Bytes: data, MIME: mime}, nil
}
