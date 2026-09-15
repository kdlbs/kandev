package kubernetes

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	gwebsocket "github.com/gorilla/websocket"
	"k8s.io/apimachinery/pkg/util/httpstream"
	streamspdy "k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/apimachinery/pkg/util/httpstream/wsstream"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	portforwardconstants "k8s.io/apimachinery/pkg/util/portforward"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport"
	clientwebsocket "k8s.io/client-go/transport/websocket"
)

// NewPortForwardDialer builds the maintained client-go WebSocket-primary,
// legacy-SPDY-fallback dialer. Each upgrade attempt is bounded by ctx and
// handshakeTimeout; successful upgraded connections are not tied to that
// request deadline.
func NewPortForwardDialer(
	ctx context.Context,
	config *rest.Config,
	targetURL *url.URL,
	handshakeTimeout time.Duration,
) (httpstream.Dialer, context.CancelFunc, error) {
	if config == nil || targetURL == nil {
		return nil, nil, errors.New("kubernetes port-forward dialer configuration is incomplete")
	}
	if handshakeTimeout <= 0 {
		return nil, nil, errors.New("kubernetes port-forward handshake timeout must be positive")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	dialContext, cancel := context.WithCancel(ctx)
	dialConfig := rest.CopyConfig(config)
	dialConfig.NextProtos = []string{"http/1.1"}
	dialConfig.Wrap(func(next http.RoundTripper) http.RoundTripper {
		return &portForwardContextRoundTripper{
			ctx: dialContext, timeout: handshakeTimeout, next: next,
		}
	})
	websocketDialer, err := newContextWebSocketDialer(targetURL, dialConfig)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	spdyTransport, err := rest.TransportFor(dialConfig)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	spdyDialer := &contextSPDYDialer{client: &http.Client{Transport: spdyTransport}, targetURL: targetURL}
	return portforward.NewFallbackDialer(
		websocketDialer, spdyDialer, ShouldFallbackStream,
	), cancel, nil
}

type contextWebSocketDialer struct {
	targetURL *url.URL
	transport http.RoundTripper
	holder    clientwebsocket.ConnectionHolder
}

func newContextWebSocketDialer(targetURL *url.URL, config *rest.Config) (httpstream.Dialer, error) {
	transportConfig, err := config.TransportConfig()
	if err != nil {
		return nil, err
	}
	tlsConfig, err := transport.TLSConfigFor(transportConfig)
	if err != nil {
		return nil, err
	}
	proxy := config.Proxy
	if proxy == nil {
		proxy = utilnet.NewProxierWithNoProxyCIDR(http.ProxyFromEnvironment)
	}
	dialContext := (&net.Dialer{}).DialContext
	if transportConfig.DialHolder != nil && transportConfig.DialHolder.Dial != nil {
		dialContext = transportConfig.DialHolder.Dial
	}
	holder := &contextWebSocketRoundTripper{
		TLSConfig:   tlsConfig,
		Proxier:     proxy,
		DialContext: dialContext,
	}
	wrapper, err := transport.HTTPWrappersForConfig(transportConfig, holder)
	if err != nil {
		return nil, err
	}
	return &contextWebSocketDialer{targetURL: targetURL, transport: wrapper, holder: holder}, nil
}

func (d *contextWebSocketDialer) Dial(protocols ...string) (httpstream.Connection, string, error) {
	request, err := http.NewRequest(http.MethodGet, d.targetURL.String(), nil)
	if err != nil {
		return nil, "", err
	}
	tunnelingProtocols := make([]string, 0, len(protocols))
	for _, protocol := range protocols {
		tunnelingProtocols = append(
			tunnelingProtocols, portforwardconstants.WebsocketsSPDYTunnelingPrefix+protocol,
		)
	}
	connection, err := clientwebsocket.Negotiate(
		d.transport, d.holder, request, tunnelingProtocols...,
	)
	if err != nil {
		return nil, "", err
	}
	protocol := strings.TrimPrefix(
		connection.Subprotocol(), portforwardconstants.WebsocketsSPDYTunnelingPrefix,
	)
	tunnel := portforward.NewTunnelingConnection("client", connection)
	spdyConnection, err := streamspdy.NewClientConnectionWithPings(
		tunnel, portforward.PingPeriod,
	)
	if err != nil {
		_ = tunnel.Close()
		return nil, "", err
	}
	return spdyConnection, protocol, nil
}

type contextWebSocketRoundTripper struct {
	TLSConfig   *tls.Config
	Proxier     func(*http.Request) (*url.URL, error)
	DialContext func(context.Context, string, string) (net.Conn, error)
	conn        *gwebsocket.Conn
}

func (r *contextWebSocketRoundTripper) TLSClientConfig() *tls.Config { return r.TLSConfig }
func (r *contextWebSocketRoundTripper) DataBufferSize() int          { return 32 * 1024 }
func (r *contextWebSocketRoundTripper) Connection() *gwebsocket.Conn { return r.conn }

func (r *contextWebSocketRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	protocols := request.Header[wsstream.WebSocketProtocolHeader]
	delete(request.Header, wsstream.WebSocketProtocolHeader)
	requestURL, err := webSocketUpgradeURL(request.URL)
	if err != nil {
		return nil, err
	}
	var handshakeConn *detachableHandshakeConn
	dialer := r.handshakeDialer(protocols, &handshakeConn)
	connection, response, err := dialer.DialContext(
		request.Context(), requestURL.String(), request.Header,
	)
	if err != nil {
		return nil, webSocketDialError(request.Context(), handshakeConn, response, err)
	}
	if err := finishWebSocketHandshake(connection, response, handshakeConn, protocols); err != nil {
		return nil, err
	}
	r.conn = connection
	return response, nil
}

func webSocketUpgradeURL(source *url.URL) (url.URL, error) {
	requestURL := *source
	switch requestURL.Scheme {
	case "https":
		requestURL.Scheme = "wss"
	case "http":
		requestURL.Scheme = "ws"
	default:
		return url.URL{}, fmt.Errorf("unknown websocket URL scheme %q", requestURL.Scheme)
	}
	return requestURL, nil
}

func (r *contextWebSocketRoundTripper) handshakeDialer(
	protocols []string,
	handshakeConn **detachableHandshakeConn,
) gwebsocket.Dialer {
	dialContext := r.DialContext
	if dialContext == nil {
		dialContext = (&net.Dialer{}).DialContext
	}
	return gwebsocket.Dialer{
		Proxy:           r.Proxier,
		TLSClientConfig: r.TLSConfig,
		Subprotocols:    protocols,
		ReadBufferSize:  r.DataBufferSize() + 1024,
		WriteBufferSize: r.DataBufferSize() + 1024,
		NetDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			connection, err := dialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			*handshakeConn = newDetachableHandshakeConn(ctx, connection)
			return *handshakeConn, nil
		},
	}
}

func webSocketDialError(
	ctx context.Context,
	handshakeConn *detachableHandshakeConn,
	response *http.Response,
	dialErr error,
) error {
	if handshakeConn != nil {
		handshakeConn.close()
	}
	if contextErr := ctx.Err(); contextErr != nil {
		closeHTTPResponse(response)
		return contextErr
	}
	if !errors.Is(dialErr, gwebsocket.ErrBadHandshake) {
		closeHTTPResponse(response)
		return dialErr
	}
	cause := dialErr
	if response != nil {
		cause = fmt.Errorf("%w (%s)", dialErr, response.Status)
	}
	closeHTTPResponse(response)
	return &httpstream.UpgradeFailureError{Cause: cause}
}

func finishWebSocketHandshake(
	connection *gwebsocket.Conn,
	response *http.Response,
	handshakeConn *detachableHandshakeConn,
	protocols []string,
) error {
	if !slicesContain(protocols, connection.Subprotocol()) {
		closeWebSocketHandshake(connection, response, handshakeConn)
		return &httpstream.UpgradeFailureError{Cause: fmt.Errorf(
			"invalid websocket protocol, expected one of %q, got %q",
			protocols, connection.Subprotocol(),
		)}
	}
	if handshakeConn == nil {
		closeWebSocketHandshake(connection, response, nil)
		return errors.New("websocket handshake completed without a tracked connection")
	}
	if err := handshakeConn.detach(); err != nil {
		closeWebSocketHandshake(connection, response, nil)
		return err
	}
	return nil
}

func slicesContain(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func closeWebSocketHandshake(
	connection *gwebsocket.Conn,
	response *http.Response,
	handshakeConn *detachableHandshakeConn,
) {
	if connection != nil {
		_ = connection.Close()
	}
	if handshakeConn != nil {
		handshakeConn.close()
	}
	closeHTTPResponse(response)
}

func closeHTTPResponse(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}

type detachableHandshakeConn struct {
	net.Conn
	ctx             context.Context
	mu              sync.Mutex
	detached        bool
	closedByContext bool
	stop            func() bool
}

func newDetachableHandshakeConn(ctx context.Context, connection net.Conn) *detachableHandshakeConn {
	guarded := &detachableHandshakeConn{Conn: connection, ctx: ctx}
	guarded.stop = context.AfterFunc(ctx, guarded.closeIfAttached)
	return guarded
}

func (c *detachableHandshakeConn) closeIfAttached() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.detached {
		c.closedByContext = true
		_ = c.Close()
	}
}

func (c *detachableHandshakeConn) detach() error {
	c.mu.Lock()
	if c.closedByContext {
		err := c.ctx.Err()
		if err == nil {
			err = context.Canceled
		}
		c.mu.Unlock()
		return err
	}
	if err := c.ctx.Err(); err != nil {
		c.closedByContext = true
		_ = c.Close()
		c.mu.Unlock()
		return err
	}
	c.detached = true
	stop := c.stop
	c.mu.Unlock()
	if stop != nil {
		stop()
	}
	return nil
}

func (c *detachableHandshakeConn) close() {
	c.mu.Lock()
	if c.stop != nil {
		c.stop()
	}
	c.detached = false
	_ = c.Close()
	c.mu.Unlock()
}

// ShouldFallbackStream keeps the WebSocket-to-SPDY compatibility policy shared
// by exec and port-forward callers.
func ShouldFallbackStream(err error) bool {
	if httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err) {
		return true
	}
	var timeoutErr net.Error
	return errors.As(err, &timeoutErr) && timeoutErr.Timeout()
}

type portForwardContextRoundTripper struct {
	ctx     context.Context
	timeout time.Duration
	next    http.RoundTripper
}

func (r *portForwardContextRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	requestContext := newDetachablePortForwardContext(r.ctx, r.timeout)
	response, err := r.next.RoundTrip(request.Clone(requestContext))
	if err == nil && response != nil && response.StatusCode == http.StatusSwitchingProtocols {
		requestContext.detach()
	} else {
		requestContext.cancelNow()
	}
	return response, err
}

type detachablePortForwardContext struct {
	parent     context.Context
	inner      context.Context
	cancel     context.CancelCauseFunc
	deadline   time.Time
	timer      *time.Timer
	stopParent func() bool
	finishOnce sync.Once
	stateMu    sync.RWMutex
	err        error
	detached   bool
}

func newDetachablePortForwardContext(parent context.Context, timeout time.Duration) *detachablePortForwardContext {
	inner, cancel := context.WithCancelCause(context.Background())
	initialized := make(chan struct{})
	ctx := &detachablePortForwardContext{
		parent: parent, inner: inner, cancel: cancel, deadline: time.Now().Add(timeout),
	}
	ctx.stopParent = context.AfterFunc(parent, func() {
		<-initialized
		ctx.finish(context.Cause(parent), false)
	})
	ctx.timer = time.AfterFunc(timeout, func() {
		<-initialized
		ctx.finish(context.DeadlineExceeded, false)
	})
	close(initialized)
	return ctx
}

func (c *detachablePortForwardContext) Deadline() (time.Time, bool) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.detached {
		return time.Time{}, false
	}
	return c.deadline, true
}
func (c *detachablePortForwardContext) Done() <-chan struct{} { return c.inner.Done() }
func (c *detachablePortForwardContext) Err() error {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.err
}
func (c *detachablePortForwardContext) Value(key any) any { return c.parent.Value(key) }
func (c *detachablePortForwardContext) detach()           { c.finish(nil, true) }
func (c *detachablePortForwardContext) cancelNow()        { c.finish(context.Canceled, false) }
func (c *detachablePortForwardContext) finish(cause error, detach bool) {
	c.finishOnce.Do(func() {
		if c.stopParent != nil {
			c.stopParent()
		}
		if c.timer != nil {
			c.timer.Stop()
		}
		c.stateMu.Lock()
		c.detached = detach
		if !detach {
			c.err = context.Canceled
			if errors.Is(cause, context.DeadlineExceeded) {
				c.err = context.DeadlineExceeded
			}
		}
		err := c.err
		c.stateMu.Unlock()
		if !detach {
			c.cancel(err)
		}
	})
}

// contextSPDYDialer uses the standard client-go REST transport for the HTTP
// upgrade. Unlike client-go's legacy SPDY dialer, that transport terminates a
// blocked response-header read when the request context is canceled. The
// upgraded byte stream is still handled by Kubernetes' maintained SPDY stack.
type contextSPDYDialer struct {
	client        *http.Client
	targetURL     *url.URL
	newConnection func(net.Conn, time.Duration) (httpstream.Connection, error)
}

func (d *contextSPDYDialer) Dial(protocols ...string) (httpstream.Connection, string, error) {
	request, err := http.NewRequest(http.MethodPost, d.targetURL.String(), nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set(httpstream.HeaderConnection, httpstream.HeaderUpgrade)
	request.Header.Set(httpstream.HeaderUpgrade, streamspdy.HeaderSpdy31)
	for _, protocol := range protocols {
		request.Header.Add(httpstream.HeaderProtocolVersion, protocol)
	}
	response, err := d.client.Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("send SPDY upgrade request: %w", err)
	}
	if !validSPDYUpgrade(response) {
		defer func() { _ = response.Body.Close() }()
		message, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		return nil, "", &httpstream.UpgradeFailureError{Cause: fmt.Errorf(
			"SPDY upgrade rejected (%s): %s", response.Status, strings.TrimSpace(string(message)),
		)}
	}
	stream, ok := response.Body.(io.ReadWriteCloser)
	if !ok {
		_ = response.Body.Close()
		return nil, "", errors.New("SPDY upgrade response is not a writable stream")
	}
	newConnection := d.newConnection
	if newConnection == nil {
		newConnection = streamspdy.NewClientConnectionWithPings
	}
	connection, err := newConnection(
		&upgradedResponseConn{ReadWriteCloser: stream, remote: d.targetURL.Host}, 5*time.Second,
	)
	if err != nil {
		_ = stream.Close()
		return nil, "", err
	}
	return connection, response.Header.Get(httpstream.HeaderProtocolVersion), nil
}

func validSPDYUpgrade(response *http.Response) bool {
	if response == nil || response.StatusCode != http.StatusSwitchingProtocols {
		return false
	}
	connection := strings.ToLower(response.Header.Get(httpstream.HeaderConnection))
	upgrade := strings.ToLower(response.Header.Get(httpstream.HeaderUpgrade))
	return strings.Contains(connection, strings.ToLower(httpstream.HeaderUpgrade)) &&
		strings.Contains(upgrade, strings.ToLower(streamspdy.HeaderSpdy31))
}

type upgradedResponseConn struct {
	io.ReadWriteCloser
	remote string
}

func (c *upgradedResponseConn) LocalAddr() net.Addr              { return portForwardAddr("local") }
func (c *upgradedResponseConn) RemoteAddr() net.Addr             { return portForwardAddr(c.remote) }
func (c *upgradedResponseConn) SetDeadline(time.Time) error      { return nil }
func (c *upgradedResponseConn) SetReadDeadline(time.Time) error  { return nil }
func (c *upgradedResponseConn) SetWriteDeadline(time.Time) error { return nil }

type portForwardAddr string

func (a portForwardAddr) Network() string { return "tcp" }
func (a portForwardAddr) String() string  { return string(a) }
