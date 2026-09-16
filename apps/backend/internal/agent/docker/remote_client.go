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

// NewRemoteClient creates a Docker client whose Engine API requests travel
// through dial rather than a local socket or a TCP address.
func NewRemoteClient(dial DialContextFunc, log *logger.Logger) (*Client, error) {
	return newRemoteClientWithOptionOrder(dial, log, false)
}

// newRemoteClientWithOptionOrder builds the remote client, optionally
// reversing the host/dialer option order. Only a test reverses it, to prove
// the ordering below is the thing that makes the dialer effective.
func newRemoteClientWithOptionOrder(dial DialContextFunc, log *logger.Logger, reversed bool) (*Client, error) {
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
		cli:     cli,
		storage: cli,
		remover: cli,
		builder: cli,
		logger:  log,
		config:  config.DockerConfig{Host: remoteEngineHost},
	}, nil
}
