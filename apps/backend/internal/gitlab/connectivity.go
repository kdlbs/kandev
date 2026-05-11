package gitlab

import (
	"context"
	"net/http"
	"time"
)

// connectivityTimeout bounds the host-reachability probe.
const connectivityTimeout = 5 * time.Second

// CheckHost reports whether the given GitLab host is reachable. It issues
// an unauthenticated GET to /api/v4/version and treats any HTTP response
// (including 401) as "reachable" — only network-level errors count as a
// failure. Used by the settings page when the user enters a self-managed
// host URL, before they configure a token.
func CheckHost(ctx context.Context, host string) error {
	if host == "" {
		host = DefaultHost
	}
	cctx, cancel := context.WithTimeout(ctx, connectivityTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, host+apiPathPrefix+"/version", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: connectivityTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}
