package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// restJSONReader is implemented by clients that can read an arbitrary REST
// resource. A non-2xx answer is a *GitHubAPIError carrying the status.
type restJSONReader interface {
	GetRESTJSON(ctx context.Context, endpoint string) (json.RawMessage, error)
}

// GetRESTJSON reads one REST resource through the PAT credential.
func (c *PATClient) GetRESTJSON(ctx context.Context, endpoint string) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// GetRESTJSON reads one REST resource through the gh CLI.
func (c *GHClient) GetRESTJSON(ctx context.Context, endpoint string) (json.RawMessage, error) {
	out, err := c.run(ctx, "api", endpoint)
	if err != nil {
		if code, ok := ghMergeStatusCode(err); ok {
			return nil, &GitHubAPIError{StatusCode: code, Endpoint: endpoint, Body: err.Error()}
		}
		return nil, fmt.Errorf("read %s: %w", endpoint, err)
	}
	if !json.Valid([]byte(out)) {
		return nil, &GitHubAPIError{StatusCode: http.StatusBadGateway, Endpoint: endpoint, Body: "response is not JSON"}
	}
	return json.RawMessage(out), nil
}
