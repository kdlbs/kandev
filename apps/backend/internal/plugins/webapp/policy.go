package webapp

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

var ErrInvalidNetworkOrigin = errors.New("webapp: invalid network origin")

// NormalizeNetworkOrigins accepts only exact HTTPS origins. Paths, query
// strings, credentials, wildcards, and non-HTTPS schemes cannot be granted.
func NormalizeNetworkOrigins(origins []string) ([]string, error) {
	seen := make(map[string]struct{}, len(origins))
	for _, raw := range origins {
		origin, err := normalizeOrigin(raw, "https")
		if err != nil {
			return nil, err
		}
		seen[origin] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for origin := range seen {
		result = append(result, origin)
	}
	sort.Strings(result)
	return result, nil
}

// BuildContentSecurityPolicy creates the complete response policy from
// normalized, user-approved network origins and the configured Kandev host
// origins. Manifest text is never copied directly into this header.
func BuildContentSecurityPolicy(networkOrigins, frameAncestors []string) (string, error) {
	network, err := NormalizeNetworkOrigins(networkOrigins)
	if err != nil {
		return "", err
	}
	frames, err := normalizeFrameAncestors(frameAncestors)
	if err != nil {
		return "", err
	}
	connect := append([]string{"'self'"}, network...)
	images := append([]string{"'self'", "data:"}, network...)
	fonts := append([]string{"'self'", "data:"}, network...)
	return strings.Join([]string{
		"sandbox allow-scripts allow-forms",
		"default-src 'none'",
		"form-action 'none'",
		"base-uri 'none'",
		"object-src 'none'",
		// Packaged canvases may be a single HTML file. Inline scripts are
		// allowed by the approved canvas contract, while the absence of
		// unsafe-eval and remote script sources keeps code execution bounded.
		"script-src 'self' 'unsafe-inline'",
		"style-src 'self' 'unsafe-inline'",
		"img-src " + strings.Join(images, " "),
		"font-src " + strings.Join(fonts, " "),
		"connect-src " + strings.Join(connect, " "),
		"frame-ancestors " + strings.Join(frames, " "),
	}, "; "), nil
}

func normalizeOrigin(raw, scheme string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != scheme || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: %q", ErrInvalidNetworkOrigin, raw)
	}
	if strings.ContainsAny(parsed.Host, " \t\r\n") || parsed.Hostname() == "" {
		return "", fmt.Errorf("%w: %q", ErrInvalidNetworkOrigin, raw)
	}
	return parsed.Scheme + "://" + strings.ToLower(parsed.Host), nil
}

func normalizeFrameAncestors(origins []string) ([]string, error) {
	seen := make(map[string]struct{}, len(origins))
	result := make([]string, 0, len(origins))
	for _, raw := range origins {
		trimmed := strings.TrimSpace(raw)
		parsed, err := url.Parse(trimmed)
		if err != nil || strings.Contains(trimmed, "*") || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("%w: frame ancestor %q", ErrInvalidNetworkOrigin, raw)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "tauri" {
			return nil, fmt.Errorf("%w: frame ancestor %q", ErrInvalidNetworkOrigin, raw)
		}
		origin := parsed.Scheme + "://" + strings.ToLower(parsed.Host)
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		result = append(result, origin)
	}
	return result, nil
}
