// Package sandboxmcp exposes one captured MCP transport through a private route.
package sandboxmcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

// Forward is a core-owned transport bound to one execution and generation.
// Implementations must authenticate the control peer and must not follow redirects.
type Forward func(context.Context, string, http.Header, []byte) (*http.Response, error)

type Relay struct {
	ctx           context.Context
	cancel        context.CancelFunc
	address, path string
	server        *http.Server
	forward       Forward
	paths         map[string]string
	slots         chan struct{}
	closeOnce     sync.Once
	closeErr      error
}

func OpenWithPaths(parent context.Context, forward Forward, paths []string) (*Relay, error) {
	if forward == nil {
		return nil, http.ErrNotSupported
	}
	canonical, err := contract.CanonicalMCPPaths(paths)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		_ = listener.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	relay := &Relay{ctx: ctx, cancel: cancel, address: listener.Addr().String(), path: "/" + hex.EncodeToString(secret) + "/mcp", forward: forward, slots: make(chan struct{}, 16)}
	relay.paths = make(map[string]string, len(canonical))
	for _, path := range canonical {
		relay.paths[relay.path+strings.TrimPrefix(path, contract.MCPRootPath)] = path
	}
	relay.server = &http.Server{Handler: relay, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, MaxHeaderBytes: 8192, ErrorLog: log.New(io.Discard, "", 0)}
	relay.server.SetKeepAlivesEnabled(false)
	go func() { _ = relay.server.Serve(listener); cancel() }()
	go func() { <-ctx.Done(); _ = relay.Close() }()
	return relay, nil
}

func (r *Relay) URL() string     { return "http://" + r.address + r.path }
func (r *Relay) Address() string { return r.address }
func (r *Relay) Close() error {
	r.closeOnce.Do(func() { r.cancel(); r.closeErr = r.server.Close() })
	return r.closeErr
}
