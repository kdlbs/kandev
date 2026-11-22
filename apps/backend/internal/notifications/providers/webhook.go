package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/http/httpguts"
)

const (
	webhookTimeout         = 10 * time.Second
	webhookMaxResponseBody = 4 * 1024
	webhookPayloadVersion  = "1.0"
)

var (
	errWebhookDestinationBlocked = errors.New(
		"webhook destination must be a globally routable address",
	)
	webhookBlockedAddressRanges = mustParseWebhookPrefixes(
		"0.0.0.0/8",
		"10.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"192.88.99.0/24",
		"192.168.0.0/16",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"240.0.0.0/4",
		"::/128",
		"::1/128",
		"64:ff9b::/96",
		"64:ff9b:1::/48",
		"100::/64",
		"2001::/23",
		"2001:db8::/32",
		"2002::/16",
		"fc00::/7",
		"fe80::/10",
		"ff00::/8",
	)
)

// WebhookProvider delivers notification events to a configured HTTP endpoint.
type WebhookProvider struct {
	client               *http.Client
	allowPrivateNetworks bool
}

type webhookConfig struct {
	url     string
	headers map[string]string
	secret  string
}

type webhookPayload struct {
	Version string            `json:"version"`
	Event   string            `json:"event"`
	Title   string            `json:"title"`
	Message string            `json:"message"`
	Type    string            `json:"type"`
	Fields  map[string]string `json:"fields"`
}

// NewWebhookProvider builds a webhook provider with an explicit destination
// policy and a client that rejects cross-origin redirects.
func NewWebhookProvider(allowPrivateNetworks bool) *WebhookProvider {
	return &WebhookProvider{
		client:               newWebhookHTTPClient(allowPrivateNetworks),
		allowPrivateNetworks: allowPrivateNetworks,
	}
}

// Available reports that webhook delivery needs no host-side dependency.
func (p *WebhookProvider) Available() bool {
	return true
}

// Validate checks that config contains a usable URL, valid headers, and an
// optional secret without contacting the destination.
func (p *WebhookProvider) Validate(config map[string]interface{}) error {
	_, err := parseWebhookConfig(config, p.allowPrivateNetworks)
	return err
}

// Send posts a notification payload to the configured webhook endpoint.
func (p *WebhookProvider) Send(ctx context.Context, message Message) error {
	config, err := parseWebhookConfig(message.Config, p.allowPrivateNetworks)
	if err != nil {
		return err
	}
	body, err := json.Marshal(newWebhookPayload(message))
	if err != nil {
		return fmt.Errorf("webhook payload: %w", err)
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, webhookTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, config.url, bytes.NewReader(body))
	if err != nil {
		return errors.New("webhook request could not be created")
	}
	request.Header.Set("Content-Type", "application/json")
	for name, value := range config.headers {
		request.Header.Set(name, value)
	}
	if config.secret != "" && !hasHeader(config.headers, "Authorization") {
		request.Header.Set("Authorization", "Bearer "+config.secret)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return errors.New("webhook request failed")
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, webhookMaxResponseBody))
		return fmt.Errorf("webhook returned status %d", response.StatusCode)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, webhookMaxResponseBody))
	return nil
}

// newWebhookHTTPClient builds the outbound client with SSRF and redirect guards.
func newWebhookHTTPClient(allowPrivateNetworks bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// A proxy would hide the real destination from the dial-time address check.
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return webhookDialContext(ctx, network, address, allowPrivateNetworks)
	}
	return &http.Client{
		Timeout:       webhookTimeout,
		Transport:     transport,
		CheckRedirect: checkWebhookRedirect,
	}
}

// checkWebhookRedirect permits same-origin redirects only so configured
// credential headers never reach another origin.
func checkWebhookRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("webhook redirect limit exceeded")
	}
	if len(via) == 0 {
		return nil
	}
	if !sameWebhookOrigin(via[0].URL, request.URL) {
		return errors.New("webhook redirect to a different origin is not allowed")
	}
	return nil
}

// sameWebhookOrigin reports whether two URLs share a scheme, host, and
// effective port. Explicit default ports match their implicit form.
func sameWebhookOrigin(first, next *url.URL) bool {
	if first == nil || next == nil {
		return false
	}
	return strings.EqualFold(first.Scheme, next.Scheme) &&
		strings.EqualFold(first.Hostname(), next.Hostname()) &&
		webhookEffectivePort(first) == webhookEffectivePort(next)
}

// webhookEffectivePort returns the explicit port or the scheme default.
func webhookEffectivePort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	switch strings.ToLower(value.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

// webhookDialContext resolves and validates every connection at dial time so a
// DNS answer cannot change between validation and delivery.
func webhookDialContext(ctx context.Context, network, address string, allowPrivateNetworks bool) (net.Conn, error) {
	dialer := &net.Dialer{}
	if allowPrivateNetworks {
		return dialer.DialContext(ctx, network, address)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errors.New("webhook destination invalid")
	}
	if ip := net.ParseIP(host); ip != nil {
		address, ok := netip.AddrFromSlice(ip)
		if !ok || !isGlobalWebhookAddress(address) {
			return nil, errWebhookDestinationBlocked
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(address.Unmap().String(), port))
	}
	resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, errors.New("webhook destination could not be resolved")
	}
	if len(resolved) == 0 {
		return nil, errors.New("webhook destination could not be resolved")
	}
	var lastErr error
	for _, candidate := range resolved {
		address, ok := netip.AddrFromSlice(candidate.IP)
		if !ok || !isGlobalWebhookAddress(address) {
			continue
		}
		connection, dialErr := dialer.DialContext(
			ctx,
			network,
			net.JoinHostPort(address.Unmap().String(), port),
		)
		if dialErr == nil {
			return connection, nil
		}
		lastErr = dialErr
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errWebhookDestinationBlocked
}

// isGlobalWebhookAddress rejects loopback, private, link-local, multicast,
// unspecified, and reserved destination addresses.
func isGlobalWebhookAddress(address netip.Addr) bool {
	if !address.IsValid() || address.Zone() != "" {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() ||
		address.IsPrivate() ||
		address.IsLoopback() ||
		address.IsLinkLocalUnicast() ||
		address.IsLinkLocalMulticast() ||
		address.IsMulticast() ||
		address.IsUnspecified() {
		return false
	}
	for _, addressRange := range webhookBlockedAddressRanges {
		if addressRange.Contains(address) {
			return false
		}
	}
	return true
}

// mustParseWebhookPrefixes parses the blocked destination ranges at startup.
func mustParseWebhookPrefixes(values ...string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefixes = append(prefixes, netip.MustParsePrefix(value))
	}
	return prefixes
}

// newWebhookPayload builds the versioned payload sent to the endpoint.
func newWebhookPayload(message Message) webhookPayload {
	fields := map[string]string{}
	for name, value := range map[string]string{
		"task_id":         message.TaskID,
		"task_session_id": message.TaskSessionID,
		"occurrence_id":   message.OccurrenceID,
	} {
		if value != "" {
			fields[name] = value
		}
	}
	return webhookPayload{
		Version: webhookPayloadVersion,
		Event:   message.EventType,
		Title:   message.Title,
		Message: message.Body,
		Type:    "info",
		Fields:  fields,
	}
}

// parseWebhookConfig validates and normalizes a stored webhook configuration.
func parseWebhookConfig(config map[string]interface{}, allowPrivateNetworks bool) (webhookConfig, error) {
	if config == nil {
		return webhookConfig{}, fmt.Errorf("webhook config missing")
	}
	rawURL, ok := config["url"].(string)
	if !ok || strings.TrimSpace(rawURL) == "" {
		return webhookConfig{}, fmt.Errorf("webhook url required")
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return webhookConfig{}, errors.New("webhook url invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return webhookConfig{}, fmt.Errorf("webhook url must use http or https")
	}
	if parsed.Host == "" {
		return webhookConfig{}, fmt.Errorf("webhook url invalid: missing host")
	}
	if !allowPrivateNetworks {
		if err := validateWebhookDestination(parsed); err != nil {
			return webhookConfig{}, err
		}
	}
	headers, err := parseWebhookHeaders(config["headers"])
	if err != nil {
		return webhookConfig{}, err
	}
	secret, err := optionalString(config["secret"], "webhook secret")
	if err != nil {
		return webhookConfig{}, err
	}
	return webhookConfig{url: parsed.String(), headers: headers, secret: secret}, nil
}

// validateWebhookDestination rejects literal private addresses and localhost
// hostnames up front. Named hosts are resolved again at dial time.
func validateWebhookDestination(parsed *url.URL) error {
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if hostname == "" {
		return fmt.Errorf("webhook url invalid: missing host")
	}
	if address, err := netip.ParseAddr(hostname); err == nil {
		if !isGlobalWebhookAddress(address) {
			return errWebhookDestinationBlocked
		}
		return nil
	}
	if hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") {
		return errWebhookDestinationBlocked
	}
	return nil
}

// parseWebhookHeaders validates names and values and rejects case-insensitive
// duplicates so delivery never depends on map iteration order.
func parseWebhookHeaders(raw interface{}) (map[string]string, error) {
	if raw == nil {
		return nil, nil
	}
	headers := map[string]string{}
	seen := map[string]string{}
	add := func(name, value string) error {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil
		}
		value = strings.TrimSpace(value)
		if !httpguts.ValidHeaderFieldName(name) {
			return fmt.Errorf("webhook header name %q is invalid", name)
		}
		if !httpguts.ValidHeaderFieldValue(value) {
			return fmt.Errorf("webhook header %q has an invalid value", name)
		}
		key := strings.ToLower(name)
		if previous, ok := seen[key]; ok {
			return fmt.Errorf("webhook header %q duplicates %q", name, previous)
		}
		seen[key] = name
		headers[name] = value
		return nil
	}
	switch value := raw.(type) {
	case map[string]string:
		for name, item := range value {
			if err := add(name, item); err != nil {
				return nil, err
			}
		}
	case map[string]interface{}:
		for name, item := range value {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("webhook headers must map strings to strings")
			}
			if err := add(name, text); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("webhook headers must be an object")
	}
	return headers, nil
}

// optionalString reads and trims an optional string configuration value.
func optionalString(raw interface{}, label string) (string, error) {
	if raw == nil {
		return "", nil
	}
	text, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", label)
	}
	return strings.TrimSpace(text), nil
}

// hasHeader reports whether headers contains target case-insensitively.
func hasHeader(headers map[string]string, target string) bool {
	for name := range headers {
		if strings.EqualFold(name, target) {
			return true
		}
	}
	return false
}
