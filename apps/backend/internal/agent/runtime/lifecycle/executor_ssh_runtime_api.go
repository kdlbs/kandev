package lifecycle

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
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
	listener    net.Listener
	localTarget string
	remotePort  string
	once        sync.Once
}

func openSSHRuntimeAPITunnelForRequest(
	client *ssh.Client,
	req *ExecutorCreateRequest,
) (*sshRuntimeAPITunnel, string, error) {
	if req == nil {
		return nil, "", fmt.Errorf("ssh runtime API tunnel: request is required")
	}
	rawURL := getMetadataString(req.Metadata, MetadataKeySSHRuntimeAPILocalURL)
	if rawURL == "" && req.Env != nil {
		rawURL = req.Env[envKeyKandevAPIURL]
	}
	requestedPort := getMetadataString(req.Metadata, MetadataKeySSHRuntimeAPIRemotePort)
	tunnel, runtimeURL, err := openSSHRuntimeAPITunnel(client, rawURL, requestedPort)
	if err != nil || tunnel == nil {
		return tunnel, runtimeURL, err
	}
	if req.Env == nil {
		req.Env = make(map[string]string)
	}
	req.Env[envKeyKandevAPIURL] = runtimeURL
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	req.Metadata[MetadataKeySSHRuntimeAPILocalURL] = rawURL
	req.Metadata[MetadataKeySSHRuntimeAPIRemotePort] = tunnel.RemotePort()
	return tunnel, runtimeURL, nil
}

func openSSHRuntimeAPITunnel(client *ssh.Client, rawURL string, requestedRemotePort ...string) (*sshRuntimeAPITunnel, string, error) {
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
	localPort := parsed.Port()
	if localPort == "" {
		localPort = defaultURLPort(parsed.Scheme)
	}
	if localPort == "" {
		return nil, rawURL, fmt.Errorf("ssh runtime API tunnel: URL must include a port")
	}
	remotePort := "0"
	if len(requestedRemotePort) > 0 && strings.TrimSpace(requestedRemotePort[0]) != "" {
		remotePort = strings.TrimSpace(requestedRemotePort[0])
		if _, err := parseSSHRuntimeAPIPort(remotePort, true); err != nil {
			return nil, rawURL, err
		}
	}
	remoteAddress := net.JoinHostPort("127.0.0.1", remotePort)
	listener, err := client.Listen("tcp", remoteAddress)
	if err != nil {
		return nil, rawURL, fmt.Errorf("ssh runtime API tunnel: listen on remote %s: %w", remoteAddress, err)
	}
	actualRemotePort, err := sshRuntimeAPIPortFromAddr(listener.Addr())
	if err != nil {
		_ = listener.Close()
		return nil, rawURL, err
	}
	tunnel := &sshRuntimeAPITunnel{
		listener:    listener,
		localTarget: net.JoinHostPort(parsed.Hostname(), localPort),
		remotePort:  actualRemotePort,
	}
	go tunnel.acceptLoop()

	parsed.Host = net.JoinHostPort("127.0.0.1", actualRemotePort)
	return tunnel, parsed.String(), nil
}

func parseSSHRuntimeAPIPort(raw string, allowZero bool) (int, error) {
	port, err := strconv.Atoi(raw)
	if err != nil || port < 0 || port > 65535 || (!allowZero && port == 0) {
		return 0, fmt.Errorf("ssh runtime API tunnel: invalid remote port %q", raw)
	}
	return port, nil
}

func sshRuntimeAPIPortFromAddr(addr net.Addr) (string, error) {
	if addr == nil {
		return "", fmt.Errorf("ssh runtime API tunnel: listener has no address")
	}
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "", fmt.Errorf("ssh runtime API tunnel: parse listener address %q: %w", addr.String(), err)
	}
	if _, err := parseSSHRuntimeAPIPort(port, false); err != nil {
		return "", err
	}
	return port, nil
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
	local, err := net.DialTimeout("tcp", t.localTarget, sshRuntimeAPIDialTimeout)
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

func (t *sshRuntimeAPITunnel) RemotePort() string {
	if t == nil {
		return ""
	}
	return t.remotePort
}

func (t *sshRuntimeAPITunnel) Close() error {
	if t == nil {
		return nil
	}
	var err error
	t.once.Do(func() { err = t.listener.Close() })
	return err
}
