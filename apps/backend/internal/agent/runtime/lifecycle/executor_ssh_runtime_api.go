package lifecycle

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const sshRuntimeAPIDialTimeout = 10 * time.Second

const urlSchemeHTTP = "http"

// sshRuntimeAPITunnel exposes a control-plane loopback API through the SSH
// connection. The remote agent can then use the same API URL shape as a local
// launch without assuming that the SSH host can route to the developer's
// machine.
type sshRuntimeAPITunnel struct {
	listener net.Listener
	target   string
	once     sync.Once
}

func openSSHRuntimeAPITunnel(client *ssh.Client, rawURL string) (*sshRuntimeAPITunnel, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, rawURL, fmt.Errorf("ssh runtime API URL: %w", err)
	}
	if !isLoopbackURL(parsed) {
		return nil, rawURL, nil
	}
	if client == nil {
		return nil, rawURL, fmt.Errorf("ssh runtime API tunnel: SSH client is required")
	}
	port := parsed.Port()
	if port == "" {
		port = defaultURLPort(parsed.Scheme)
	}
	if port == "" {
		return nil, rawURL, fmt.Errorf("ssh runtime API tunnel: URL must include a port")
	}
	target := net.JoinHostPort("127.0.0.1", port)
	listener, err := client.Listen("tcp", target)
	if err != nil {
		return nil, rawURL, fmt.Errorf("ssh runtime API tunnel: listen on remote %s: %w", target, err)
	}
	tunnel := &sshRuntimeAPITunnel{listener: listener, target: target}
	go tunnel.acceptLoop()

	parsed.Host = net.JoinHostPort("127.0.0.1", port)
	return tunnel, parsed.String(), nil
}

func isLoopbackURL(parsed *url.URL) bool {
	if parsed == nil || parsed.Hostname() == "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "::1" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func defaultURLPort(scheme string) string {
	switch strings.ToLower(scheme) {
	case urlSchemeHTTP:
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func (t *sshRuntimeAPITunnel) acceptLoop() {
	for {
		remote, err := t.listener.Accept()
		if err != nil {
			return
		}
		go t.proxy(remote)
	}
}

func (t *sshRuntimeAPITunnel) proxy(remote net.Conn) {
	local, err := net.DialTimeout("tcp", t.target, sshRuntimeAPIDialTimeout)
	if err != nil {
		_ = remote.Close()
		return
	}
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(local, remote)
		close(done)
	}()
	go func() {
		_, _ = io.Copy(remote, local)
		close(done)
	}()
	<-done
	_ = local.Close()
	_ = remote.Close()
	<-done
}

func (t *sshRuntimeAPITunnel) Close() error {
	if t == nil {
		return nil
	}
	var err error
	t.once.Do(func() { err = t.listener.Close() })
	return err
}
