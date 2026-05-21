package gitlab

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// connectivityTimeout bounds the host-reachability probe.
const connectivityTimeout = 5 * time.Second

// validateHost parses and validates a GitLab base URL, returning the parsed
// URL with only the fields we'll use copied across. Splitting validation
// from request construction makes both the unit tests and the SSRF surface
// tractable: anything that escapes this function has a known-good scheme
// and a non-empty host.
func validateHost(host string) (*url.URL, error) {
	if host == "" {
		host = DefaultHost
	}
	parsed, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("host must use http or https scheme")
	}
	if parsed.Host == "" {
		return nil, errors.New("host is missing a hostname")
	}
	if parsed.Opaque != "" || parsed.User != nil {
		return nil, errors.New("host must not contain userinfo or be an opaque URL")
	}
	return &url.URL{
		Scheme: parsed.Scheme,
		Host:   parsed.Host,
		Path:   strings.TrimRight(parsed.Path, "/"),
	}, nil
}

// CheckHost reports whether the given GitLab host is reachable. It issues
// an unauthenticated GET to /api/v4/version and treats any HTTP response
// (including 401) as "reachable" — only network-level errors count as a
// failure. Used by the settings page when the user enters a self-managed
// host URL, before they configure a token.
//
// The host is parsed and its scheme verified to be http/https before the
// request is built. The probe URL is composed via net/url rather than string
// concatenation so user input cannot smuggle in a different host or path —
// this is what makes the call safe against SSRF.
func CheckHost(ctx context.Context, host string) error {
	base, err := validateHost(host)
	if err != nil {
		return err
	}
	probe := *base
	probe.Path = base.Path + apiPathPrefix + "/version"
	cctx, cancel := context.WithTimeout(ctx, connectivityTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, probe.String(), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}
