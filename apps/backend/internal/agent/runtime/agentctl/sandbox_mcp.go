package client

import (
	"bytes"
	"context"
	"net/http"
	"strconv"

	"github.com/kandev/kandev/internal/common/sandboxmcp"
)

// SandboxMCPForward binds a relay to this authenticated execution transport.
// RoundTrip preserves endpoint leases and never follows a Location response.
func (c *Client) SandboxMCPForward(attemptID string, generation uint64, digest string) sandboxmcp.Forward {
	return func(ctx context.Context, method string, headers http.Header, body []byte) (*http.Response, error) {
		request, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v1/agent/sandbox/mcp", bytes.NewReader(body))
		if err != nil {
			return nil, ErrSandboxLaunchUnsupported
		}
		request.Header = headers.Clone()
		request.Header.Set("X-Kandev-Sandbox-Attempt", attemptID)
		request.Header.Set("X-Kandev-Sandbox-Generation", strconv.FormatUint(generation, 10))
		request.Header.Set("X-Kandev-Sandbox-Policy", digest)
		return c.ProxyTransport().RoundTrip(request)
	}
}
