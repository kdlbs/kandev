package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/common/logger"
)

const (
	connectionLeaseRefreshWindow = 30 * time.Second
	connectionLeaseHeaderLimit   = 16 * 1024
	httpsScheme                  = "https"
	httpScheme                   = "http"
)

var errCrossOriginRedirect = errors.New("remote executor redirect changed authority")

// ConnectionLease contains the short-lived address and credentials used to
// reach an agentctl instance in a plugin-managed environment.
type ConnectionLease struct {
	BaseURL               string
	HTTPHeaders           http.Header
	WebSocketHeaders      http.Header
	WebSocketSubprotocols []string
	ExpiresAt             time.Time
	Generation            string
}

// ConnectionLeaseResolver obtains a fresh provider-issued connection lease.
type ConnectionLeaseResolver func(context.Context) (*ConnectionLease, error)

// NewEndpointClient creates an agentctl client that uses provider-issued
// short-lived HTTPS connection leases. NewClient(host, port, ...) remains the
// compatibility path for local and built-in executors.
func NewEndpointClient(ctx context.Context, resolver ConnectionLeaseResolver, log *logger.Logger, opts ...ClientOption) (*Client, error) {
	return newEndpointClient(ctx, resolver, log, endpointTransportDependencies{}, nil, opts...)
}

func newEndpointClient(ctx context.Context, resolver ConnectionLeaseResolver, log *logger.Logger, dependencies endpointTransportDependencies, now func() time.Time, opts ...ClientOption) (*Client, error) {
	manager := newConnectionLeaseManager(resolver, now, nil)
	lease, err := manager.resolve(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve remote executor connection lease: %w", err)
	}
	initial, err := url.Parse(lease.BaseURL)
	if err != nil {
		return nil, errors.New("remote executor connection lease has an invalid endpoint")
	}
	initial.RawQuery = ""
	initial.ForceQuery = false
	client := newClient(strings.TrimSuffix(initial.String(), "/"), log, opts...)
	transport := newEndpointRoundTripper(manager, initial, client.authToken, client.executionID, dependencies)
	manager.onGeneration = transport.CloseIdleConnections
	client.endpointTransport = transport
	client.httpClient.Transport = transport
	client.longRunningHTTPClient.Transport = transport
	client.httpClient.CheckRedirect = endpointRedirectPolicy
	client.longRunningHTTPClient.CheckRedirect = endpointRedirectPolicy
	return client, nil
}

type connectionLeaseManager struct {
	resolveLease ConnectionLeaseResolver
	now          func() time.Time
	onGeneration func()

	mu         sync.Mutex
	cached     *ConnectionLease
	refreshing chan struct{}
}

func newConnectionLeaseManager(resolver ConnectionLeaseResolver, now func() time.Time, onGeneration func()) *connectionLeaseManager {
	if now == nil {
		now = time.Now
	}
	return &connectionLeaseManager{resolveLease: resolver, now: now, onGeneration: onGeneration}
}

func (m *connectionLeaseManager) resolve(ctx context.Context) (*ConnectionLease, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lease, wait, leader := m.beginRefresh()
		if lease != nil {
			return lease, nil
		}
		if !leader {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-wait:
				continue
			}
		}
		return m.refresh(ctx, wait)
	}
}

func (m *connectionLeaseManager) beginRefresh() (*ConnectionLease, chan struct{}, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cached != nil && m.cached.ExpiresAt.After(m.now().Add(connectionLeaseRefreshWindow)) {
		return cloneConnectionLease(m.cached), nil, false
	}
	if m.refreshing != nil {
		return nil, m.refreshing, false
	}
	wait := make(chan struct{})
	m.refreshing = wait
	return nil, wait, true
}

func (m *connectionLeaseManager) refresh(ctx context.Context, wait chan struct{}) (*ConnectionLease, error) {
	var lease *ConnectionLease
	var err error
	if m.resolveLease == nil {
		err = errors.New("remote executor connection resolver is unavailable")
	} else {
		lease, err = m.resolveLease(ctx)
	}
	if err == nil && lease == nil {
		err = errors.New("remote executor connection resolver returned no lease")
	}
	if err == nil {
		lease, err = validateConnectionLease(*lease, m.now())
	}

	m.mu.Lock()
	if err == nil {
		changed := m.cached == nil || m.cached.Generation != lease.Generation
		m.cached = lease
		if changed && m.onGeneration != nil {
			m.onGeneration()
		}
	}
	m.refreshing = nil
	close(wait)
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return cloneConnectionLease(lease), nil
}

func (m *connectionLeaseManager) currentAuthority() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cached == nil {
		return ""
	}
	parsed, err := url.Parse(m.cached.BaseURL)
	if err != nil {
		return ""
	}
	return authorityKey(parsed)
}

func cloneConnectionLease(lease *ConnectionLease) *ConnectionLease {
	if lease == nil {
		return nil
	}
	return &ConnectionLease{
		BaseURL:               lease.BaseURL,
		HTTPHeaders:           lease.HTTPHeaders.Clone(),
		WebSocketHeaders:      lease.WebSocketHeaders.Clone(),
		WebSocketSubprotocols: append([]string(nil), lease.WebSocketSubprotocols...),
		ExpiresAt:             lease.ExpiresAt,
		Generation:            lease.Generation,
	}
}

func validateConnectionLease(input ConnectionLease, now time.Time) (*ConnectionLease, error) {
	if _, err := parseLeaseEndpoint(input.BaseURL); err != nil {
		return nil, err
	}
	if err := validateLeaseLifetime(input, now); err != nil {
		return nil, err
	}
	httpHeaders, err := normalizeLeaseHeaders(input.HTTPHeaders)
	if err != nil {
		return nil, fmt.Errorf("invalid remote executor HTTP headers: %w", err)
	}
	webSocketHeaders, err := normalizeLeaseHeaders(input.WebSocketHeaders)
	if err != nil {
		return nil, fmt.Errorf("invalid remote executor WebSocket headers: %w", err)
	}
	if err := validateLeaseHeaderBudget(httpHeaders, webSocketHeaders, input.WebSocketSubprotocols); err != nil {
		return nil, err
	}
	input.HTTPHeaders = httpHeaders
	input.WebSocketHeaders = webSocketHeaders
	return cloneConnectionLease(&input), nil
}

func parseLeaseEndpoint(raw string) (*url.URL, error) {
	if len(raw) > 8192 {
		return nil, errors.New("remote executor endpoint is too long")
	}
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint == nil {
		return nil, errors.New("remote executor connection lease has an invalid endpoint")
	}
	if err := validateLeaseEndpointURL(raw, endpoint); err != nil {
		return nil, err
	}
	return endpoint, nil
}

func validateLeaseEndpointURL(raw string, endpoint *url.URL) error {
	if endpoint.Scheme != httpsScheme {
		return errors.New("remote executor endpoint must use HTTPS")
	}
	if endpoint.Host == "" || endpoint.Opaque != "" {
		return errors.New("remote executor endpoint is missing a hostname")
	}
	if endpoint.User != nil {
		return errors.New("remote executor endpoint cannot contain userinfo")
	}
	if endpoint.Fragment != "" || strings.Contains(raw, "#") {
		return errors.New("remote executor endpoint cannot contain a fragment")
	}
	if err := rejectCredentialQuery(endpoint.RawQuery); err != nil {
		return err
	}
	if err := validateEndpointPort(endpoint); err != nil {
		return err
	}
	if endpoint.Hostname() == "" || strings.Contains(endpoint.Hostname(), "%") {
		return errors.New("remote executor endpoint has an invalid hostname")
	}
	if ip := net.ParseIP(endpoint.Hostname()); ip != nil {
		if err := validateResolvedAddresses([]net.IPAddr{{IP: ip}}); err != nil {
			return errors.New("remote executor endpoint resolves to a prohibited address")
		}
	}
	return nil
}

func validateEndpointPort(endpoint *url.URL) error {
	port := endpoint.Port()
	if port == "" {
		return nil
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return errors.New("remote executor endpoint has an invalid port")
	}
	return nil
}

func validateLeaseLifetime(lease ConnectionLease, now time.Time) error {
	if lease.Generation == "" || len(lease.Generation) > 128 {
		return errors.New("remote executor connection lease has an invalid generation")
	}
	if !lease.ExpiresAt.After(now.Add(connectionLeaseRefreshWindow)) {
		return errors.New("remote executor connection lease is expired or expires too soon")
	}
	return nil
}

func validateLeaseHeaderBudget(httpHeaders, webSocketHeaders http.Header, protocols []string) error {
	bytesUsed := headerBytes(httpHeaders) + headerBytes(webSocketHeaders)
	for _, protocol := range protocols {
		if !isHTTPToken(protocol) {
			return errors.New("remote executor lease contains an invalid WebSocket subprotocol")
		}
		bytesUsed += len(protocol)
	}
	if bytesUsed > connectionLeaseHeaderLimit {
		return errors.New("remote executor connection headers exceed 16 KiB")
	}
	return nil
}

func rejectCredentialQuery(rawQuery string) error {
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return errors.New("remote executor endpoint has an invalid query")
	}
	for key := range query {
		normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
		switch normalized {
		case "token", "access_token", "refresh_token", "auth", "authorization", "api_key", "apikey", "key", "secret", "password", "signature", "sig", "credential", "credentials":
			return errors.New("remote executor endpoint query cannot contain credentials")
		}
		if strings.Contains(normalized, "token") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "signature") || strings.Contains(normalized, "credential") || strings.Contains(normalized, "password") {
			return errors.New("remote executor endpoint query cannot contain credentials")
		}
	}
	return nil
}

func normalizeLeaseHeaders(headers http.Header) (http.Header, error) {
	normalized := make(http.Header, len(headers))
	seen := make(map[string]struct{}, len(headers))
	for name, values := range headers {
		canonical := textproto.CanonicalMIMEHeaderKey(name)
		if canonical == "" || !isHTTPToken(canonical) {
			return nil, errors.New("header name is invalid")
		}
		lower := strings.ToLower(canonical)
		if lower == "authorization" || lower == "x-instance-id" || lower == "host" || lower == "cookie" || lower == "set-cookie" || lower == "sec-websocket-protocol" || isHopHeader(lower) || strings.HasPrefix(lower, "proxy-") {
			return nil, fmt.Errorf("provider cannot override %s", canonical)
		}
		if _, exists := seen[lower]; exists {
			return nil, errors.New("header name is duplicated")
		}
		seen[lower] = struct{}{}
		for _, value := range values {
			if !validHTTPHeaderValue(value) {
				return nil, fmt.Errorf("header %s has an invalid value", canonical)
			}
		}
		normalized[canonical] = append([]string(nil), values...)
	}
	return normalized, nil
}

func validHTTPHeaderValue(value string) bool {
	for _, char := range value {
		if (char < 32 && char != '\t') || char == 127 {
			return false
		}
	}
	return true
}

func isHopHeader(lower string) bool {
	switch lower {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func isHTTPToken(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char <= 32 || char >= 127 || strings.ContainsRune("()<>@,;:\\\"/[]?={} ", char) {
			return false
		}
	}
	return true
}

func headerBytes(headers http.Header) int {
	total := 0
	for name, values := range headers {
		for _, value := range values {
			total += len(name) + 2 + len(value) + 2
		}
	}
	return total
}

func validateResolvedAddresses(addresses []net.IPAddr) error {
	if len(addresses) == 0 {
		return errors.New("remote executor hostname did not resolve")
	}
	for _, address := range addresses {
		ip := address.IP
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return errors.New("remote executor hostname resolves to a prohibited address")
		}
	}
	return nil
}

type endpointTransportDependencies struct {
	lookupIP  func(context.Context, string) ([]net.IPAddr, error)
	dial      func(context.Context, string, string) (net.Conn, error)
	tlsConfig *tls.Config
	now       func() time.Time
}

type endpointRoundTripper struct {
	manager      *connectionLeaseManager
	initial      *url.URL
	authMu       sync.RWMutex
	authToken    string
	instanceID   string
	transport    *http.Transport
	dependencies endpointTransportDependencies
}

func (t *endpointRoundTripper) setAuthToken(token string) {
	t.authMu.Lock()
	t.authToken = token
	t.authMu.Unlock()
}

func (t *endpointRoundTripper) authTokenValue() string {
	t.authMu.RLock()
	defer t.authMu.RUnlock()
	return t.authToken
}

func newEndpointRoundTripper(manager *connectionLeaseManager, initial *url.URL, authToken, instanceID string, dependencies endpointTransportDependencies) *endpointRoundTripper {
	if dependencies.lookupIP == nil {
		dependencies.lookupIP = func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return net.DefaultResolver.LookupIPAddr(ctx, host)
		}
	}
	if dependencies.dial == nil {
		dependencies.dial = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	if dependencies.now == nil {
		dependencies.now = time.Now
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("remote executor connection has an invalid authority")
		}
		ips, err := dependencies.lookupIP(ctx, host)
		if err != nil {
			return nil, errors.New("remote executor hostname could not be resolved")
		}
		if err := validateResolvedAddresses(ips); err != nil {
			return nil, err
		}
		return dependencies.dial(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
	if dependencies.tlsConfig != nil {
		base.TLSClientConfig = dependencies.tlsConfig.Clone()
		if base.TLSClientConfig.InsecureSkipVerify {
			base.TLSClientConfig.InsecureSkipVerify = false
		}
	}
	return &endpointRoundTripper{manager: manager, initial: initial, authToken: authToken, instanceID: instanceID, transport: base, dependencies: dependencies}
}

func (t *endpointRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	if !t.allowsAuthority(request.URL) {
		return nil, errCrossOriginRedirect
	}
	lease, err := t.manager.resolve(request.Context())
	if err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(lease.BaseURL)
	if err != nil {
		return nil, errors.New("remote executor lease endpoint is invalid")
	}
	out := request.Clone(request.Context())
	out.Header = request.Header.Clone()
	if out.Header == nil {
		out.Header = make(http.Header)
	}
	if err := rewriteEndpointRequest(out, t.initial, endpoint); err != nil {
		return nil, err
	}
	for name, values := range lease.HTTPHeaders {
		out.Header[name] = append([]string(nil), values...)
	}
	if hasWebSocketUpgrade(out.Header) {
		addWebSocketProtocols(out.Header, lease.WebSocketSubprotocols)
		for name, values := range lease.WebSocketHeaders {
			out.Header[name] = append([]string(nil), values...)
		}
	}
	if authToken := t.authTokenValue(); authToken != "" {
		out.Header.Set("Authorization", "Bearer "+authToken)
	}
	if t.instanceID != "" {
		out.Header.Set("X-Instance-ID", t.instanceID)
	}
	out.Host = ""
	resp, err := t.transport.RoundTrip(out)
	if err != nil {
		return nil, err
	}
	if resp.Request == nil {
		resp.Request = out
	}
	stripProviderSubprotocol(resp.Header, lease.WebSocketSubprotocols)
	return resp, nil
}

// BootstrapHandshake uses the endpoint lease transport to exchange the
// one-time nonce for the agentctl token. The token remains in memory and is
// applied after provider lease headers on every authenticated request.
func (c *Client) BootstrapHandshake(ctx context.Context, nonce string) (string, error) {
	if c.endpointTransport == nil {
		return "", errors.New("agentctl bootstrap handshake requires an endpoint client")
	}
	body, err := json.Marshal(map[string]string{"nonce": nonce})
	if err != nil {
		return "", errors.New("agentctl bootstrap request could not be encoded")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth/handshake", bytes.NewReader(body))
	if err != nil {
		return "", errors.New("agentctl bootstrap request could not be created")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("agentctl bootstrap handshake failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("agentctl bootstrap handshake returned status %d", resp.StatusCode)
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.Token == "" {
		return "", errors.New("agentctl bootstrap handshake returned an invalid token")
	}
	c.endpointTransport.setAuthToken(result.Token)
	return result.Token, nil
}

func (t *endpointRoundTripper) CloseIdleConnections() {
	t.transport.CloseIdleConnections()
}

func (t *endpointRoundTripper) allowsAuthority(requestURL *url.URL) bool {
	if authorityKey(requestURL) == authorityKey(t.initial) {
		return true
	}
	return authorityKey(requestURL) == t.manager.currentAuthority()
}

func rewriteEndpointRequest(request *http.Request, initial, endpoint *url.URL) error {
	if request.URL == nil || initial == nil || endpoint == nil {
		return errors.New("remote executor request endpoint is unavailable")
	}
	initialPath := strings.TrimSuffix(initial.EscapedPath(), "/")
	requestPath := request.URL.EscapedPath()
	if initialPath != "" && initialPath != "/" && requestPath != initialPath && !strings.HasPrefix(requestPath, initialPath+"/") {
		return errors.New("remote executor request escaped its endpoint path")
	}
	suffix := requestPath
	if initialPath != "" && initialPath != "/" {
		suffix = strings.TrimPrefix(requestPath, initialPath)
	}
	target := *endpoint
	basePath := strings.TrimSuffix(endpoint.EscapedPath(), "/")
	if suffix == "" {
		suffix = "/"
	}
	joined := basePath + "/" + strings.TrimPrefix(suffix, "/")
	if joined == "" {
		joined = "/"
	}
	decoded, err := url.PathUnescape(joined)
	if err != nil {
		return errors.New("remote executor request path is invalid")
	}
	target.Path = decoded
	target.RawPath = joined
	target.RawQuery = mergeEndpointQuery(endpoint.RawQuery, request.URL.RawQuery)
	target.ForceQuery = endpoint.ForceQuery || request.URL.ForceQuery
	request.URL = &target
	request.RequestURI = ""
	return nil
}

func authorityKey(endpoint *url.URL) string {
	if endpoint == nil {
		return ""
	}
	return strings.ToLower(endpoint.Scheme) + "://" + strings.ToLower(endpoint.Host)
}

func hasWebSocketUpgrade(headers http.Header) bool {
	return strings.EqualFold(headers.Get("Upgrade"), "websocket")
}

func addWebSocketProtocols(headers http.Header, protocols []string) {
	if len(protocols) == 0 {
		return
	}
	values := make([]string, 0, len(protocols)+1)
	seen := make(map[string]struct{}, len(protocols)+1)
	for _, value := range strings.Split(headers.Get("Sec-WebSocket-Protocol"), ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	for _, value := range protocols {
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	headers.Set("Sec-WebSocket-Protocol", strings.Join(values, ", "))
}

func stripProviderSubprotocol(header http.Header, protocols []string) {
	selected := header.Get("Sec-WebSocket-Protocol")
	for _, protocol := range protocols {
		if selected == protocol {
			header.Del("Sec-WebSocket-Protocol")
			return
		}
	}
}

func joinEndpointPath(endpoint *url.URL, route string) (*url.URL, error) {
	routeURL, err := url.Parse(route)
	if err != nil || routeURL.IsAbs() || routeURL.Host != "" {
		return nil, errors.New("remote executor WebSocket route is invalid")
	}
	result := *endpoint
	escapedRoute := routeURL.EscapedPath()
	joinedPath := strings.TrimSuffix(endpoint.EscapedPath(), "/") + "/" + strings.TrimPrefix(escapedRoute, "/")
	if strings.HasSuffix(escapedRoute, "/") && !strings.HasSuffix(joinedPath, "/") {
		joinedPath += "/"
	}
	decodedPath, err := url.PathUnescape(joinedPath)
	if err != nil {
		return nil, errors.New("remote executor WebSocket route path is invalid")
	}
	result.Path = decodedPath
	result.RawPath = joinedPath
	result.RawQuery = mergeEndpointQuery(endpoint.RawQuery, routeURL.RawQuery)
	result.ForceQuery = endpoint.ForceQuery || routeURL.ForceQuery
	switch result.Scheme {
	case httpsScheme:
		result.Scheme = "wss"
	case httpScheme:
		result.Scheme = "ws"
	default:
		return nil, errors.New("remote executor WebSocket endpoint has an unsupported scheme")
	}
	return &result, nil
}

func mergeEndpointQuery(endpointQuery, requestQuery string) string {
	endpointValues, err := url.ParseQuery(endpointQuery)
	if err != nil {
		return requestQuery
	}
	requestValues, err := url.ParseQuery(requestQuery)
	if err != nil {
		if endpointQuery == "" {
			return requestQuery
		}
		return endpointQuery + "&" + requestQuery
	}
	for key, values := range requestValues {
		endpointValues[key] = append(endpointValues[key], values...)
	}
	return endpointValues.Encode()
}

func (t *endpointRoundTripper) dialWebSocket(ctx context.Context, endpoint *url.URL, lease *ConnectionLease, route string, callerHeaders http.Header, callerProtocols []string, instanceID string) (*websocket.Conn, *http.Response, error) {
	u, err := joinEndpointPath(endpoint, route)
	if err != nil {
		return nil, nil, err
	}
	dialer := *websocket.DefaultDialer
	dialer.Subprotocols = append([]string(nil), callerProtocols...)
	if lease != nil {
		dialer.NetDialContext = t.transport.DialContext
		dialer.NetDialTLSContext = nil
		if t.transport.TLSClientConfig != nil {
			dialer.TLSClientConfig = t.transport.TLSClientConfig.Clone()
		}
		dialer.Proxy = nil
		dialer.Subprotocols = appendUnique(dialer.Subprotocols, lease.WebSocketSubprotocols...)
		callerHeaders = mergeHeaders(callerHeaders, lease.WebSocketHeaders)
	}
	if callerHeaders == nil {
		callerHeaders = make(http.Header)
	}
	if authToken := t.authTokenValue(); authToken != "" {
		callerHeaders.Set("Authorization", "Bearer "+authToken)
	}
	if instanceID != "" {
		callerHeaders.Set("X-Instance-ID", instanceID)
	}
	return dialer.DialContext(ctx, u.String(), callerHeaders)
}

func mergeHeaders(base, override http.Header) http.Header {
	result := base.Clone()
	if result == nil {
		result = make(http.Header)
	}
	for name, values := range override {
		result[name] = append([]string(nil), values...)
	}
	return result
}

func appendUnique(existing []string, add ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(add))
	result := make([]string, 0, len(existing)+len(add))
	for _, value := range append(append([]string(nil), existing...), add...) {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (c *Client) dialWebSocket(ctx context.Context, route string, callerHeaders http.Header, callerProtocols ...string) (*websocket.Conn, *http.Response, error) {
	if c.endpointTransport != nil {
		lease, err := c.endpointTransport.manager.resolve(ctx)
		if err != nil {
			return nil, nil, err
		}
		endpoint, err := url.Parse(lease.BaseURL)
		if err != nil {
			return nil, nil, errors.New("remote executor lease endpoint is invalid")
		}
		return c.endpointTransport.dialWebSocket(ctx, endpoint, lease, route, callerHeaders, callerProtocols, c.executionID)
	}
	endpoint, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, nil, err
	}
	endpoint, err = joinEndpointPath(endpoint, route)
	if err != nil {
		return nil, nil, err
	}
	dialer := *websocket.DefaultDialer
	dialer.Subprotocols = append([]string(nil), callerProtocols...)
	if callerHeaders == nil {
		callerHeaders = make(http.Header)
	} else {
		callerHeaders = callerHeaders.Clone()
	}
	if c.authToken != "" {
		callerHeaders.Set("Authorization", "Bearer "+c.authToken)
	}
	if c.executionID != "" {
		callerHeaders.Set("X-Instance-ID", c.executionID)
	}
	return dialer.DialContext(ctx, endpoint.String(), callerHeaders)
}

// ConnectionGeneration resolves and returns the current provider connection
// generation. Gateway proxy caches use it to notice same-URL credential rotation.
func (c *Client) ConnectionGeneration(ctx context.Context) (string, error) {
	if c.endpointTransport == nil {
		return "", nil
	}
	lease, err := c.endpointTransport.manager.resolve(ctx)
	if err != nil {
		return "", err
	}
	return lease.Generation, nil
}

func endpointRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	if !sameAuthority(req.URL, via[len(via)-1].URL) {
		return errCrossOriginRedirect
	}
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	return nil
}

func sameAuthority(left, right *url.URL) bool {
	return left != nil && right != nil && strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}
