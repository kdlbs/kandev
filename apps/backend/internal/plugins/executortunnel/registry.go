// Package executortunnel carries host-opened byte streams to executor
// environments over a provider's OpenExecutorTunnel RPC. The host binds each
// stream to an installation, environment, ownership generation and one port;
// the provider can never open a stream to the host.
package executortunnel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"expvar"
	"fmt"
	"io"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/kdlbs/kandev/pluginsdk"
	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
	"google.golang.org/protobuf/proto"
)

const (
	// MaxStreamsPerEnvironment bounds open streams across both ports.
	MaxStreamsPerEnvironment = 64
	// MaxFrameBytes is the largest data payload of one frame in either direction.
	MaxFrameBytes = pluginsdk.MaxExecutorTunnelFrameBytes
	// AckTimeout is how long a stream may wait for the provider's binding ack.
	AckTimeout = 15 * time.Second

	reasonCancel        = "cancel"
	reasonRestart       = "restart"
	reasonLimit         = "limit"
	reasonAckTimeout    = "ack_timeout"
	reasonProviderError = "provider_error"
)

var (
	// ErrUnavailable means the stream could not be established or was lost;
	// the environment may still exist.
	ErrUnavailable = errors.New("executor tunnel unavailable")
	// ErrRateLimited is the `rate_limited` refusal of the 65th concurrent open.
	ErrRateLimited = errors.New("rate_limited")
	// ErrPortNotAllowed refuses a port other than the control port or the
	// recorded instance port before any stream opens.
	ErrPortNotAllowed = errors.New("executor tunnel port not allowed")
	// ErrStaleGeneration refuses an open bound to an older ownership generation.
	ErrStaleGeneration = errors.New("executor tunnel ownership generation is stale")
	// ErrStreamClosed is returned by reads and writes after the host closed the stream.
	ErrStreamClosed = errors.New("executor tunnel closed")
)

var (
	openGauge     = expvar.NewInt("executor_plugin_tunnel_open")
	closedCounter = expvar.NewMap("executor_plugin_tunnel_closed_total")
)

// ProviderError is the stable error code a provider closed a stream with.
type ProviderError struct{ Code string }

func (e *ProviderError) Error() string { return "executor tunnel closed by provider: " + e.Code }

// Stream is the host end of one provider tunnel stream.
type Stream interface {
	Send(*pluginsdk.ExecutorTunnelFrame) error
	Recv() (*pluginsdk.ExecutorTunnelFrame, error)
}

// Opener opens the provider stream. Cancelling ctx closes it and tells the
// provider the host is done with it.
type Opener func(ctx context.Context) (Stream, error)

// Key binds a stream to an installation, an environment and an ownership generation.
type Key struct {
	InstallationID string
	EnvironmentID  string
	Generation     uint64
}

// OpenRequest describes one tunnel to open.
type OpenRequest struct {
	Key  Key
	Port uint32
	// AllowedPorts are the control port and the recorded instance port.
	AllowedPorts []uint32
	// Header carries the trusted provider request context and resource; the
	// registry sets the port and the binding nonce.
	Header *pluginsdk.ExecutorTunnelHeader
	Open   Opener
}

type environmentKey struct{ installationID, environmentID string }

type environment struct {
	generation uint64
	tunnels    map[*tunnel]struct{}
}

// Registry owns every open tunnel stream of the host.
type Registry struct {
	mu         sync.Mutex
	envs       map[environmentKey]*environment
	ackTimeout time.Duration
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{envs: make(map[environmentKey]*environment), ackTimeout: AckTimeout}
}

// Active returns the number of open or opening streams of an environment.
func (r *Registry) Active(installationID, environmentID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if env := r.envs[environmentKey{installationID, environmentID}]; env != nil {
		return len(env.tunnels)
	}
	return 0
}

// CloseEnvironment closes every stream of an environment, as on release.
func (r *Registry) CloseEnvironment(installationID, environmentID string) {
	r.mu.Lock()
	var doomed []*tunnel
	if env := r.envs[environmentKey{installationID, environmentID}]; env != nil {
		doomed = collect(env)
	}
	r.mu.Unlock()
	closeAll(doomed, reasonCancel)
}

// CloseInstallation closes every stream of every environment of a plugin
// installation, as on plugin restart or disable.
func (r *Registry) CloseInstallation(installationID string) {
	r.mu.Lock()
	var doomed []*tunnel
	for key, env := range r.envs {
		if key.installationID == installationID {
			doomed = append(doomed, collect(env)...)
		}
	}
	r.mu.Unlock()
	closeAll(doomed, reasonRestart)
}

func collect(env *environment) []*tunnel {
	tunnels := make([]*tunnel, 0, len(env.tunnels))
	for t := range env.tunnels {
		tunnels = append(tunnels, t)
	}
	return tunnels
}

func closeAll(tunnels []*tunnel, reason string) {
	for _, t := range tunnels {
		t.finish(reason, false)
	}
}

// Open establishes a tunnel and returns a connection that carries no byte
// until the provider has acknowledged the binding.
func (r *Registry) Open(ctx context.Context, request OpenRequest) (net.Conn, error) {
	if request.Open == nil || request.Header == nil {
		return nil, fmt.Errorf("%w: incomplete open request", ErrUnavailable)
	}
	if !slices.Contains(request.AllowedPorts, request.Port) {
		return nil, ErrPortNotAllowed
	}
	nonce, err := newNonce()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	// The stream outlives the opening call; only release, a lifecycle
	// change or a tunnel error ends it.
	streamCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	t := &tunnel{registry: r, key: request.Key, cancel: cancel}
	if err := r.admit(t); err != nil {
		cancel()
		return nil, err
	}
	header := proto.Clone(request.Header).(*pluginsdk.ExecutorTunnelHeader)
	header.Port = request.Port
	header.BindingNonce = nonce
	stream, err := request.Open(streamCtx)
	if err != nil {
		t.finish(reasonProviderError, false)
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if err := stream.Send(&pluginsdk.ExecutorTunnelFrame{Kind: &pluginv1.ExecutorTunnelFrame_Header{Header: header}}); err != nil {
		t.finish(reasonProviderError, false)
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if err := r.awaitAck(ctx, t, stream, nonce); err != nil {
		return nil, err
	}
	return t.start(stream)
}

func (r *Registry) admit(t *tunnel) error {
	r.mu.Lock()
	key := environmentKey{t.key.InstallationID, t.key.EnvironmentID}
	env := r.envs[key]
	if env == nil {
		env = &environment{tunnels: make(map[*tunnel]struct{})}
		r.envs[key] = env
	}
	if t.key.Generation < env.generation {
		r.mu.Unlock()
		return ErrStaleGeneration
	}
	var stale []*tunnel
	if t.key.Generation > env.generation {
		stale = collect(env)
		env.generation = t.key.Generation
	}
	if len(env.tunnels)-len(stale) >= MaxStreamsPerEnvironment {
		r.mu.Unlock()
		closedCounter.Add(reasonLimit, 1)
		return ErrRateLimited
	}
	env.tunnels[t] = struct{}{}
	openGauge.Add(1)
	r.mu.Unlock()
	closeAll(stale, reasonRestart)
	return nil
}

func (r *Registry) remove(t *tunnel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := environmentKey{t.key.InstallationID, t.key.EnvironmentID}
	if env := r.envs[key]; env != nil {
		if _, ok := env.tunnels[t]; ok {
			delete(env.tunnels, t)
			openGauge.Add(-1)
		}
	}
}

type ackResult struct {
	frame *pluginsdk.ExecutorTunnelFrame
	err   error
}

func (r *Registry) awaitAck(ctx context.Context, t *tunnel, stream Stream, nonce string) error {
	received := make(chan ackResult, 1)
	go func() {
		frame, err := stream.Recv()
		received <- ackResult{frame: frame, err: err}
	}()
	timer := time.NewTimer(r.ackTimeout)
	defer timer.Stop()
	select {
	case result := <-received:
		if result.err != nil {
			t.finish(reasonProviderError, false)
			return fmt.Errorf("%w: %w", ErrUnavailable, result.err)
		}
		if ack := result.frame.GetAck(); ack == nil || ack.GetBindingNonce() != nonce {
			t.finish(reasonProviderError, false)
			return fmt.Errorf("%w: the first provider frame was not the binding ack", ErrUnavailable)
		}
		return nil
	case <-timer.C:
		t.finish(reasonAckTimeout, false)
		return fmt.Errorf("%w: the provider did not acknowledge the binding", ErrUnavailable)
	case <-ctx.Done():
		t.finish(reasonCancel, false)
		return ctx.Err()
	}
}

func newNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// tunnel is one admitted stream and the pipe that presents it as a net.Conn.
type tunnel struct {
	registry *Registry
	key      Key
	cancel   context.CancelFunc

	once   sync.Once
	errMu  sync.Mutex
	err    error
	local  net.Conn
	remote net.Conn
	done   bool
}

func (t *tunnel) start(stream Stream) (net.Conn, error) {
	t.errMu.Lock()
	if t.done {
		err := t.err
		t.errMu.Unlock()
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	local, remote := net.Pipe()
	t.local, t.remote = local, remote
	t.errMu.Unlock()
	go t.pumpToProvider(stream)
	go t.pumpFromProvider(stream)
	return &tunnelConn{Conn: local, t: t}, nil
}

func (t *tunnel) setErr(err error) {
	t.errMu.Lock()
	if t.err == nil {
		t.err = err
	}
	t.errMu.Unlock()
}

func (t *tunnel) readErr() error {
	t.errMu.Lock()
	defer t.errMu.Unlock()
	return t.err
}

// finish ends the tunnel once. An orderly end leaves the local side readable
// until the caller closes it so buffered data is still delivered.
func (t *tunnel) finish(reason string, orderly bool) {
	t.once.Do(func() {
		if !orderly {
			t.setErr(fmt.Errorf("%w: %s", ErrStreamClosed, reason))
		}
		t.cancel()
		t.registry.remove(t)
		closedCounter.Add(reason, 1)
		t.errMu.Lock()
		t.done = true
		local, remote := t.local, t.remote
		t.errMu.Unlock()
		if remote != nil {
			_ = remote.Close()
		}
		if local != nil && !orderly {
			_ = local.Close()
		}
	})
}

func (t *tunnel) pumpToProvider(stream Stream) {
	buf := make([]byte, MaxFrameBytes)
	for {
		n, err := t.remote.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			if sendErr := stream.Send(&pluginsdk.ExecutorTunnelFrame{Kind: &pluginv1.ExecutorTunnelFrame_Data{Data: chunk}}); sendErr != nil {
				t.finish(reasonProviderError, false)
				return
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
				_ = stream.Send(&pluginsdk.ExecutorTunnelFrame{Kind: &pluginv1.ExecutorTunnelFrame_Close{Close: &pluginsdk.ExecutorTunnelClose{}}})
			}
			t.finish(reasonCancel, false)
			return
		}
	}
}

func (t *tunnel) pumpFromProvider(stream Stream) {
	for {
		frame, err := stream.Recv()
		if err != nil {
			t.finish(reasonProviderError, false)
			return
		}
		switch kind := frame.GetKind().(type) {
		case *pluginv1.ExecutorTunnelFrame_Data:
			if len(kind.Data) > MaxFrameBytes {
				t.finish(reasonProviderError, false)
				return
			}
			if _, err := t.remote.Write(kind.Data); err != nil {
				t.finish(reasonCancel, false)
				return
			}
		case *pluginv1.ExecutorTunnelFrame_Close:
			if code := kind.Close.GetErrorCode(); code != "" {
				t.setErr(&ProviderError{Code: code})
				t.finish(reasonProviderError, false)
				return
			}
			t.finish(reasonCancel, true)
			return
		default:
			t.finish(reasonProviderError, false)
			return
		}
	}
}

// tunnelConn reports why a stream ended instead of a bare pipe error.
type tunnelConn struct {
	net.Conn
	t *tunnel
}

func (c *tunnelConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	return n, c.translate(err)
}

func (c *tunnelConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	return n, c.translate(err)
}

func (c *tunnelConn) translate(err error) error {
	if err == nil || (!errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe)) {
		return err
	}
	if reason := c.t.readErr(); reason != nil {
		return reason
	}
	return err
}

func (c *tunnelConn) Close() error {
	c.t.finish(reasonCancel, false)
	return c.Conn.Close()
}
