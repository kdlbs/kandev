package lifecycle

import (
	"context"
	"fmt"
	"sync"
)

// containerPublishedPorts reads the host endpoint a container port is
// published on. It is the part of the Docker client the endpoint resolvers
// need, narrowed so both can be tested without a daemon.
type containerPublishedPorts interface {
	PublishedPort(ctx context.Context, containerID string, containerPort int) (string, int, error)
}

// containerEndpointResolver turns a container port into an address the backend
// can dial.
//
// With a local daemon the published host endpoint is already dialable. With a
// remote daemon it is not: the port is published on the remote host's
// loopback, so it has to be forwarded back to the backend's own loopback.
type containerEndpointResolver interface {
	Resolve(ctx context.Context, containerID string, containerPort int, fallbackHost string) (string, int, error)
	Close() error
}

// localEndpointResolver preserves the shipped behavior, including the
// container-IP fallback when the published port cannot be read.
type localEndpointResolver struct {
	ports containerPublishedPorts
}

func newLocalEndpointResolver(ports containerPublishedPorts) *localEndpointResolver {
	return &localEndpointResolver{ports: ports}
}

func (l *localEndpointResolver) Resolve(
	ctx context.Context, containerID string, containerPort int, fallbackHost string,
) (string, int, error) {
	host, port, err := l.ports.PublishedPort(ctx, containerID, containerPort)
	if err == nil {
		return host, port, nil
	}
	// The container IP is reachable from the backend only because the daemon
	// shares its network namespace. That is exactly what stops being true for
	// a remote daemon, which is why the remote resolver has no such fallback.
	return fallbackHost, containerPort, nil
}

func (l *localEndpointResolver) Close() error { return nil }

// portForwarder establishes a local listener that carries traffic to a port on
// the remote host, returning the local port and a close function.
type portForwarder interface {
	Forward(remotePort int) (localPort int, closeFn func() error, err error)
}

// remoteEndpointResolver forwards each distinct published port back to the
// backend's loopback, once, for the life of the session.
type remoteEndpointResolver struct {
	ports     containerPublishedPorts
	forwarder portForwarder

	mu       sync.Mutex
	forwards map[forwardKey]remoteForward
	closed   bool
}

// forwardKey scopes a forward to the container that published the port. One
// session resolver can serve more than one container -- a failed reconnect is
// followed by a fresh launch on the same session -- and a port-only key would
// hand the replacement the abandoned container's forward.
type forwardKey struct {
	containerID   string
	containerPort int
}

type remoteForward struct {
	localPort int
	closeFn   func() error
}

func newRemoteEndpointResolver(ports containerPublishedPorts, forwarder portForwarder) *remoteEndpointResolver {
	return &remoteEndpointResolver{
		ports:     ports,
		forwarder: forwarder,
		forwards:  map[forwardKey]remoteForward{},
	}
}

// remoteLoopbackHost is the address the backend dials. The forward listens
// here; the container's port lives on the remote host's own loopback.
const remoteLoopbackHost = "127.0.0.1"

func (r *remoteEndpointResolver) Resolve(
	ctx context.Context, containerID string, containerPort int, _ string,
) (string, int, error) {
	key := forwardKey{containerID: containerID, containerPort: containerPort}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return "", 0, fmt.Errorf("remote docker: endpoint resolver is closed")
	}
	if existing, ok := r.forwards[key]; ok {
		r.mu.Unlock()
		return remoteLoopbackHost, existing.localPort, nil
	}
	r.mu.Unlock()

	// No container-IP fallback: a remote container's IP is on the remote
	// daemon's network, so returning it would hand back an endpoint that
	// hangs on first use instead of reporting why it is unreachable.
	_, publishedPort, err := r.ports.PublishedPort(ctx, containerID, containerPort)
	if err != nil {
		return "", 0, fmt.Errorf("remote docker: read published port for container port %d: %w", containerPort, err)
	}

	localPort, closeFn, err := r.forwarder.Forward(publishedPort)
	if err != nil {
		return "", 0, fmt.Errorf("remote docker: forward remote port %d: %w", publishedPort, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		_ = closeFn()
		return "", 0, fmt.Errorf("remote docker: endpoint resolver is closed")
	}
	// A concurrent Resolve for the same port may have won the race; keep the
	// established forward and discard this one rather than leaking it.
	if existing, ok := r.forwards[key]; ok {
		_ = closeFn()
		return remoteLoopbackHost, existing.localPort, nil
	}
	r.forwards[key] = remoteForward{localPort: localPort, closeFn: closeFn}
	return remoteLoopbackHost, localPort, nil
}

// Close tears down every forward this resolver established. Forwards are
// session-scoped: leaving one open outlives the container it pointed at and
// leaks a listener for the backend's lifetime.
func (r *remoteEndpointResolver) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	forwards := r.forwards
	r.forwards = map[forwardKey]remoteForward{}
	r.mu.Unlock()

	var firstErr error
	for _, fwd := range forwards {
		if err := fwd.closeFn(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
