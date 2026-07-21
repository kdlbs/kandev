package client

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/system/metrics"
)

func (c *Client) SystemMetrics(ctx context.Context, metricIDs []string, diskPath string) (*metrics.SourceSnapshot, error) {
	values := url.Values{}
	if len(metricIDs) > 0 {
		values.Set("metrics", strings.Join(metricIDs, ","))
	}
	if diskPath != "" {
		values.Set("disk_path", diskPath)
	}
	path := "/api/v1/system/metrics"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var result metrics.SourceSnapshot
	status, err := c.fetchJSONResult(ctx, path, &result)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return &result, fmt.Errorf("system metrics failed with status %d", status)
	}
	return &result, nil
}
