// Package sandboxproxy enforces exact, captured TCP destinations for one attempt.
package sandboxproxy

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/sandbox"
)

type Proxy struct {
	ctx         context.Context
	cancel      context.CancelFunc
	address     string
	allowed     map[string]bool
	server      *http.Server
	slots       chan struct{}
	mu          sync.Mutex
	connections map[net.Conn]bool
	closed      bool
}

func Open(parent context.Context, grants []sandbox.NetworkGrant, protectedPorts ...uint16) (*Proxy, error) {
	canonical, err := sandbox.Canonicalize(sandbox.Grants{Network: grants}, nil)
	if err != nil || len(canonical.Network) == 0 {
		return nil, sandbox.ErrInvalidPolicy
	}
	allowed := make(map[string]bool, len(grants))
	for _, grant := range canonical.Network {
		if grant.Protocol != "tcp" || slices.Contains(protectedPorts, grant.Port) {
			return nil, sandbox.ErrInvalidPolicy
		}
		allowed[net.JoinHostPort(grant.Host, strconv.Itoa(int(grant.Port)))] = true
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	proxy := &Proxy{ctx: ctx, cancel: cancel, address: listener.Addr().String(), allowed: allowed, slots: make(chan struct{}, 64), connections: map[net.Conn]bool{}}
	proxy.server = &http.Server{Handler: proxy, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 5 * time.Second, MaxHeaderBytes: 8192, ErrorLog: log.New(io.Discard, "", 0)}
	proxy.server.SetKeepAlivesEnabled(false)
	go func() { _ = proxy.server.Serve(listener); cancel() }()
	go func() { <-ctx.Done(); _ = proxy.Close() }()
	return proxy, nil
}

func (p *Proxy) Address() string { return p.address }

func (p *Proxy) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	for conn := range p.connections {
		_ = conn.Close()
	}
	p.mu.Unlock()
	p.cancel()
	return p.server.Close()
}

func (p *Proxy) retain(connections ...net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.ctx.Err() != nil {
		return false
	}
	for _, conn := range connections {
		p.connections[conn] = true
	}
	return true
}

func (p *Proxy) release(connections ...net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
		delete(p.connections, conn)
	}
}
