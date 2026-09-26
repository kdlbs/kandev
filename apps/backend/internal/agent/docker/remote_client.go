package docker

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/moby/moby/client"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
)

// DialContextFunc establishes a connection carrying the Docker Engine API.
// The address arguments are ignored by remote transports: a dial-stdio
// transport has no address, it runs a command on an already-authenticated
// connection.
type DialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// remoteEngineHost is the URL the Engine API client uses for request lines and
// Host headers when the transport is a dialer rather than an address. It is
// never resolved: the `.invalid` TLD is reserved by RFC 2606 precisely so a
// lookup cannot succeed, which keeps a broken dialer from silently reaching a
// real host.
const remoteEngineHost = "http://docker.example.invalid"

// ErrNilRemoteDialer is returned when a remote client is constructed without a
// transport. Falling back to the default transport would dial the local
// daemon, so this fails instead of guessing.
var ErrNilRemoteDialer = errors.New("docker: remote client requires a dialer")

// RemoteTransport carries the Engine API to a remote daemon.
//
// Cause exists because the Engine API client does not preserve transport
// causes. When a request fails with an error whose text names a local socket
// dial, its request path substitutes a generic "cannot connect to the Docker
// daemon" error that wraps nothing. A remote daemon reports its own socket
// failures in that same wording, so the classified cause survives only where
// the transport kept it. Cause reports the most recent one, or nil.
type RemoteTransport struct {
	Dial  DialContextFunc
	Cause func() error
}

// NewRemoteClient creates a Docker client whose Engine API requests travel
// through transport rather than a local socket or a TCP address.
func NewRemoteClient(transport RemoteTransport, log *logger.Logger) (*Client, error) {
	return newRemoteClientWithOptionOrder(transport, log, false)
}

// newRemoteClientWithOptionOrder builds the remote client, optionally
// reversing the host/dialer option order. Only a test reverses it, to prove
// the ordering below is the thing that makes the dialer effective.
func newRemoteClientWithOptionOrder(transport RemoteTransport, log *logger.Logger, reversed bool) (*Client, error) {
	dial := transport.Dial
	if dial == nil {
		return nil, ErrNilRemoteDialer
	}

	// API-version negotiation is enabled by default in this client version,
	// so it is not requested explicitly.
	//
	// Order is load-bearing. client.WithHost calls
	// sockets.ConfigureTransport, whose default branch installs its own TCP
	// DialContext; applying WithDialContext first means that TCP dialer
	// replaces it and every request goes to the wrong daemon.
	hostOpt := client.WithHost(remoteEngineHost)
	dialOpt := client.WithDialContext(dial)
	opts := []client.Opt{hostOpt, dialOpt}
	if reversed {
		opts = []client.Opt{dialOpt, hostOpt}
	}
	cli, err := client.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create remote docker client: %w", err)
	}

	log.Info("Remote Docker client created", zap.String("transport", "dialer"))

	return &Client{
		cli:         cli,
		storage:     cli,
		remover:     cli,
		builder:     cli,
		logger:      log,
		config:      config.DockerConfig{Host: remoteEngineHost},
		remoteCause: transport.Cause,
	}, nil
}

// ExplainRemoteFailure adds the transport-level cause to err when the Engine
// API client replaced it with a generic connection failure that wraps nothing.
// Both stay in the result: the cause names the fix, and err is what this
// request actually hit. Any other error, and any error on a client with no
// remote transport, is returned unchanged.
//
// Only a connection failure is substituted: a daemon that answered and refused
// the request has already reported the cause the user needs.
func (c *Client) ExplainRemoteFailure(err error) error {
	if err == nil || c.remoteCause == nil {
		return err
	}
	if !client.IsErrConnectionFailed(err) {
		return err
	}
	cause := c.remoteCause()
	if cause == nil {
		return err
	}
	return fmt.Errorf("%w (%w)", cause, err)
}

// PingVersion pings the daemon and reports its API version, so a connection
// test can name the daemon that answered instead of only reporting success.
func (c *Client) PingVersion(ctx context.Context) (string, error) {
	result, err := c.cli.Ping(ctx, client.PingOptions{})
	if err != nil {
		return "", fmt.Errorf("docker ping failed: %w", c.ExplainRemoteFailure(err))
	}
	return result.APIVersion, nil
}
