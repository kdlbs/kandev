package client

import (
	"bytes"
	"context"
	"net/http"
	"strconv"

	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/common/sandboxmcp"
)

// SandboxMCPForward binds a relay to this authenticated execution transport.
// RoundTrip preserves endpoint leases and never follows a Location response.
func (c *Client) SandboxMCPForward(attemptID string, generation uint64, digest string) sandboxmcp.Forward {
	return func(ctx context.Context, method string, headers http.Header, body []byte) (*http.Response, error) {
		path := headers.Get(sandboxmcp.MCPPathHeader)
		if path == "" {
			path = contract.MCPRootPath
		}
		if path != contract.MCPRootPath {
			if _, err := contract.CanonicalMCPPaths([]string{contract.MCPRootPath, path}); err != nil {
				return nil, ErrSandboxLaunchUnsupported
			}
		}
		request, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v1/agent/sandbox"+path, bytes.NewReader(body))
		if err != nil {
			return nil, ErrSandboxLaunchUnsupported
		}
		request.Header = headers.Clone()
		request.Header.Del(sandboxmcp.MCPPathHeader)
		request.Header.Set("X-Kandev-Sandbox-Attempt", attemptID)
		request.Header.Set("X-Kandev-Sandbox-Generation", strconv.FormatUint(generation, 10))
		request.Header.Set("X-Kandev-Sandbox-Policy", digest)
		return c.ProxyTransport().RoundTrip(request)
	}
}
