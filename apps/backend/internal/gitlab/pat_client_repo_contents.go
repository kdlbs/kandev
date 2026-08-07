package gitlab

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// maxRepoFileBytes bounds a single repository file read. Workflow definition
// files are a few KiB at most; the cap keeps a misconfigured path from pulling
// an arbitrarily large blob into memory.
const maxRepoFileBytes = 1 << 20

// ListRepoTree lists one repository directory at ref, non-recursively.
// Pagination is followed to completion — a truncated listing would silently
// drop workflow definitions from a sync.
func (c *PATClient) ListRepoTree(
	ctx context.Context, projectPath, path, ref string,
) ([]RepoTreeEntry, error) {
	query := url.Values{}
	query.Set("ref", ref)
	query.Set("recursive", "false")
	query.Set("per_page", fmt.Sprintf("%d", maxPageSize))
	if path != "" {
		query.Set("path", path)
	}
	endpoint := fmt.Sprintf("/projects/%s/repository/tree?%s", projectRef(projectPath), query.Encode())

	var entries []RepoTreeEntry
	for endpoint != "" {
		var page []RepoTreeEntry
		nextLink, err := c.getPaginated(ctx, endpoint, &page)
		if err != nil {
			return nil, fmt.Errorf("list repo tree: %w", err)
		}
		entries = append(entries, page...)
		endpoint = nextLink
	}
	return entries, nil
}

// GetRepoFileContent returns the raw bytes of a repository file at ref.
func (c *PATClient) GetRepoFileContent(
	ctx context.Context, projectPath, path, ref string,
) ([]byte, error) {
	query := url.Values{}
	query.Set("ref", ref)
	endpoint := fmt.Sprintf("/projects/%s/repository/files/%s/raw?%s",
		projectRef(projectPath), encodeSegment(path), query.Encode())
	content, err := c.getRaw(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("get repo file content: %w", err)
	}
	return content, nil
}

// getRaw fetches an endpoint whose response body is returned verbatim rather
// than JSON-decoded.
func (c *PATClient) getRaw(ctx context.Context, endpoint string) ([]byte, error) {
	u := c.host + apiPathPrefix + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, defaultErrBytes))
		return nil, &APIError{StatusCode: resp.StatusCode, Endpoint: endpoint, Body: string(body)}
	}
	// Read one byte past the cap so an oversized file is reported rather than
	// silently truncated into a confusing parse failure downstream.
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxRepoFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxRepoFileBytes {
		return nil, fmt.Errorf("file exceeds the %d byte limit", maxRepoFileBytes)
	}
	return content, nil
}
