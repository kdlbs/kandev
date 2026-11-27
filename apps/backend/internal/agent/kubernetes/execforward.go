package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/kandev/kandev/internal/agentctl/tcpstdio"
	"github.com/kandev/kandev/internal/common/securityutil"
)

// TCPStdioSubcommand is the agentctl subcommand that bridges its stdin and
// stdout to one TCP port on Pod loopback.
const TCPStdioSubcommand = tcpstdio.Subcommand

// ExecPortForwarder implements PortForwarder over pods/exec. It listens on a
// process-local loopback port exactly like the port-forward transport, and
// carries every accepted connection over its own exec stream into
// `<BridgePath> tcp-stdio --port <remote port>` inside the target container.
// The bridge dials only 127.0.0.1, so agentctl stays bound to Pod loopback.
//
// The two directions end independently: when the local client half-closes,
// the exec executor closes the stream's stdin and keeps copying stdout, and
// the bridge half-closes its TCP write side and keeps reading the response.
// A connection ends when the in-Pod server closes or the session closes.
type ExecPortForwarder struct {
	Executor   PodExecutor
	BridgePath string
	// Logger receives one debug line per connection open and close, and a
	// warning when an exec stream fails while the session is still open. Nil
	// disables logging.
	Logger *zap.Logger
}

// execBridgeStderrLimit bounds how much bridge stderr is kept for the
// failure warning. Only the tail is kept; it is credential-redacted before
// it is logged.
const execBridgeStderrLimit = 2 << 10

// Forward starts a loopback listener for the request. The returned session is
// ready as soon as the listener is bound; each connection opens its exec
// stream lazily, so an unreachable agentctl surfaces as a closed connection.
func (f *ExecPortForwarder) Forward(ctx context.Context, request PortForwardRequest) (PortForwardSession, error) {
	if f == nil || f.Executor == nil || f.BridgePath == "" {
		return nil, errors.New("exec port forwarder is not configured")
	}
	if len(validation.IsDNS1123Label(request.Container)) > 0 {
		return nil, fieldError("port_forward.container", "must name the Pod's main (agentctl) container for exec transport")
	}
	if request.RemotePort == 0 {
		return nil, fieldError("port_forward.remote_port", "must be non-zero")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	localAddress := request.LocalAddress
	if localAddress == "" {
		localAddress = loopbackAddress
	}
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(
		ctx, "tcp", net.JoinHostPort(localAddress, strconv.Itoa(int(request.LocalPort))),
	)
	if err != nil {
		return nil, err
	}
	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok || tcpAddress.Port <= 0 || tcpAddress.Port > 65535 {
		_ = listener.Close()
		return nil, errors.New("exec port forward did not allocate one local port")
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	session := &execForwardSession{
		parent: ctx, listener: listener, localPort: uint16(tcpAddress.Port),
		ready: make(chan struct{}), done: make(chan error, 1), cancel: cancel,
		finished: make(chan struct{}), conns: make(map[net.Conn]struct{}),
	}
	close(session.ready)
	go session.serve(sessionCtx, func(connCtx context.Context, conn net.Conn) {
		f.bridge(connCtx, request, conn)
	})
	return session, nil
}

func (f *ExecPortForwarder) bridge(ctx context.Context, request PortForwardRequest, conn net.Conn) {
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() { _ = conn.Close() }()
	log := f.connectionLogger(request, conn)
	log.Debug("exec transport connection opened")
	started := time.Now()
	stderr := &boundedTail{limit: execBridgeStderrLimit}
	err := f.Executor.Exec(connCtx, ExecRequest{
		Namespace: request.Namespace, Pod: request.Pod, Container: request.Container,
		Command: ExecBridgeCommand(f.BridgePath, request.RemotePort),
		Stdin:   conn, Stdout: conn, Stderr: stderr,
	})
	elapsed := time.Since(started)
	// A cancelled session context means Close or the parent ended the
	// stream on purpose; only failures of a live session are warnings.
	if err != nil && ctx.Err() == nil {
		log.Warn("exec transport stream failed",
			zap.Duration("duration", elapsed),
			zap.String("stderr", stderr.redacted()),
			zap.String("error", securityutil.SanitizeCredentials(err.Error())))
	}
	log.Debug("exec transport connection closed",
		zap.Duration("duration", elapsed), zap.Bool("stream_error", err != nil))
}

func (f *ExecPortForwarder) connectionLogger(request PortForwardRequest, conn net.Conn) *zap.Logger {
	if f.Logger == nil {
		return zap.NewNop()
	}
	peer := ""
	if address := conn.RemoteAddr(); address != nil {
		peer = address.String()
	}
	return f.Logger.With(
		zap.String("namespace", request.Namespace), zap.String("pod", request.Pod),
		zap.String("container", request.Container), zap.Uint16("remote_port", request.RemotePort),
		zap.String("local_peer", peer),
	)
}

// boundedTail keeps the last limit bytes written to it. Exec streams write
// stderr from their own goroutine, so it is safe for concurrent use.
type boundedTail struct {
	mu    sync.Mutex
	limit int
	data  []byte
	cut   bool
}

func (b *boundedTail) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if extra := len(b.data) - b.limit; extra > 0 {
		b.data = append(b.data[:0], b.data[extra:]...)
		b.cut = true
	}
	return len(p), nil
}

// redacted returns the kept tail as valid UTF-8 with credentials redacted.
func (b *boundedTail) redacted() string {
	b.mu.Lock()
	data := append([]byte(nil), b.data...)
	cut := b.cut
	b.mu.Unlock()
	for len(data) > 0 && !utf8.RuneStart(data[0]) {
		data = data[1:]
	}
	text := strings.TrimSpace(strings.ToValidUTF8(string(data), ""))
	if cut && text != "" {
		text = "…" + text
	}
	return securityutil.SanitizeCredentials(text)
}

// AgentctlContainer returns the Pod's main container: the one Kandev composed
// with the bootstrap command and the reserved kandev-agentctl port, and the
// only container that holds the uploaded agentctl. Template validation rejects
// that port on any other container and admission rejects a Pod that adds it
// elsewhere, so sidecars (service-mesh proxies, Docker engines) never match,
// whatever their position in the container list. A Pod without exactly one
// such container is not a Kandev session Pod and fails with an error.
func AgentctlContainer(pod *corev1.Pod) (string, error) {
	if pod == nil {
		return "", errors.New("exec transport: no Pod to locate the agentctl container in")
	}
	var found []string
	for i := range pod.Spec.Containers {
		for _, port := range pod.Spec.Containers[i].Ports {
			if port.Name == AgentctlContainerPort {
				found = append(found, pod.Spec.Containers[i].Name)
				break
			}
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", fmt.Errorf(
			"exec transport: Pod %s/%s has no main container with the %s port",
			pod.Namespace, pod.Name, AgentctlContainerPort,
		)
	default:
		return "", fmt.Errorf(
			"exec transport: Pod %s/%s has several containers with the %s port: %s",
			pod.Namespace, pod.Name, AgentctlContainerPort, strings.Join(found, ", "),
		)
	}
}

// ExecBridgeCommand returns the in-Pod command that bridges stdio to the
// given Pod-loopback port.
func ExecBridgeCommand(bridgePath string, remotePort uint16) []string {
	return []string{bridgePath, TCPStdioSubcommand, "--port", strconv.Itoa(int(remotePort))}
}

type execForwardSession struct {
	parent    context.Context
	listener  net.Listener
	localPort uint16
	ready     chan struct{}
	done      chan error
	cancel    context.CancelFunc
	finished  chan struct{}
	closeOnce sync.Once

	mu       sync.Mutex
	closed   bool
	conns    map[net.Conn]struct{}
	handlers sync.WaitGroup
}

func (s *execForwardSession) serve(ctx context.Context, handle func(context.Context, net.Conn)) {
	defer close(s.finished)
	stopListener := context.AfterFunc(ctx, func() { _ = s.listener.Close() })
	defer stopListener()
	var acceptErr error
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			acceptErr = err
			break
		}
		if !s.track(conn) {
			_ = conn.Close()
			continue
		}
		s.handlers.Add(1)
		go func() {
			defer s.handlers.Done()
			defer s.untrack(conn)
			handle(ctx, conn)
		}()
	}
	s.closeConnections()
	s.handlers.Wait()
	s.done <- s.finalError(acceptErr)
}

func (s *execForwardSession) finalError(acceptErr error) error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil
	}
	if err := s.parent.Err(); err != nil {
		return err
	}
	return acceptErr
}

func (s *execForwardSession) track(conn net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.conns[conn] = struct{}{}
	return true
}

func (s *execForwardSession) untrack(conn net.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
}

func (s *execForwardSession) closeConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for conn := range s.conns {
		_ = conn.Close()
	}
}

func (s *execForwardSession) LocalPort() uint16      { return s.localPort }
func (s *execForwardSession) Ready() <-chan struct{} { return s.ready }
func (s *execForwardSession) Done() <-chan error     { return s.done }

// Close stops accepting, cancels every in-flight exec stream, and waits for
// the connection handlers to return.
func (s *execForwardSession) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		s.cancel()
		_ = s.listener.Close()
	})
	<-s.finished
	return nil
}
