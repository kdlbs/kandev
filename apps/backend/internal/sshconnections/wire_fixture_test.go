package sshconnections

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// wireHost executes only the production enrollment script, inside a disposable
// home. Forwarding accepts only the explicitly configured fixture destination.
type wireHost struct {
	home                   string
	listener               net.Listener
	config                 *ssh.ServerConfig
	fingerprint            string
	publicHostKey          ssh.PublicKey
	forwardAddress         string
	forwardPort            uint32
	passwordCalls          atomic.Int32
	keyCalls               atomic.Int32
	executions             atomic.Int32
	forwards               atomic.Int32
	rejectKey              atomic.Bool
	disconnectAfterInstall atomic.Bool
	mu                     sync.Mutex
	clients                map[net.Conn]bool
	keyFingerprints        []string
	accepted               chan struct{}
	workers                sync.WaitGroup
}

func newWireHost(t *testing.T, authorized string) *wireHost {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	h := &wireHost{home: t.TempDir(), listener: listener, fingerprint: ssh.FingerprintSHA256(signer.PublicKey()), publicHostKey: signer.PublicKey(), clients: make(map[net.Conn]bool), accepted: make(chan struct{})}
	h.writeKeys(t, authorized)
	h.config = &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		h.passwordCalls.Add(1)
		if string(password) != "fixture-bootstrap" {
			return nil, ErrForbidden
		}
		return nil, nil
	}, PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		h.keyCalls.Add(1)
		h.mu.Lock()
		h.keyFingerprints = append(h.keyFingerprints, ssh.FingerprintSHA256(key))
		h.mu.Unlock()
		if h.rejectKey.Load() {
			return nil, ErrForbidden
		}
		data, err := os.ReadFile(h.keysPath())
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			candidate, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
			if err == nil && string(candidate.Marshal()) == string(key.Marshal()) {
				return nil, nil
			}
		}
		return nil, ErrForbidden
	}}
	h.config.AddHostKey(signer)
	go h.accept()
	t.Cleanup(func() {
		_ = h.listener.Close()
		<-h.accepted
		h.mu.Lock()
		for client := range h.clients {
			_ = client.Close()
		}
		h.mu.Unlock()
		h.workers.Wait()
	})
	return h
}

func (h *wireHost) addECDSAHostKey(t *testing.T) {
	t.Helper()
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.config.AddHostKey(signer)
}

func (h *wireHost) keysPath() string { return filepath.Join(h.home, ".ssh", "authorized_keys") }
func (h *wireHost) writeKeys(t *testing.T, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(h.keysPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.keysPath(), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func (h *wireHost) keys(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(h.keysPath())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func (h *wireHost) port() int {
	return h.listener.Addr().(*net.TCPAddr).Port
}
func (h *wireHost) accept() {
	defer close(h.accepted)
	for {
		client, err := h.listener.Accept()
		if err != nil {
			return
		}
		h.mu.Lock()
		h.clients[client] = true
		h.mu.Unlock()
		h.workers.Add(1)
		go func() {
			defer h.workers.Done()
			defer func() { _ = client.Close(); h.mu.Lock(); delete(h.clients, client); h.mu.Unlock() }()
			_ = client.SetDeadline(time.Now().Add(15 * time.Second))
			h.mu.Lock()
			config := h.config
			h.mu.Unlock()
			conn, channels, requests, err := ssh.NewServerConn(client, config)
			if err != nil {
				return
			}
			defer func() { _ = conn.Close() }()
			go ssh.DiscardRequests(requests)
			for channel := range channels {
				switch channel.ChannelType() {
				case "session":
					h.session(channel, conn)
				case "direct-tcpip":
					h.forward(channel)
				default:
					_ = channel.Reject(ssh.Prohibited, "unsupported fixture channel")
				}
			}
		}()
	}
}
func (h *wireHost) session(pending ssh.NewChannel, conn *ssh.ServerConn) {
	channel, requests, err := pending.Accept()
	if err != nil {
		return
	}
	defer func() { _ = channel.Close() }()
	for request := range requests {
		var command struct{ Command string }
		if request.Type != "exec" || ssh.Unmarshal(request.Payload, &command) != nil || command.Command != enrollmentScript {
			_ = request.Reply(false, nil)
			continue
		}
		h.executions.Add(1)
		_ = request.Reply(true, nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, "sh", "-c", command.Command)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + h.home, "LC_ALL=C"}
		cmd.Stdin = channel
		cmd.Stdout = channel
		cmd.Stderr = channel.Stderr()
		err = cmd.Run()
		cancel()
		if h.disconnectAfterInstall.Load() {
			_ = conn.Close()
			return
		}
		status := uint32(0)
		if err != nil {
			status = 1
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				status = uint32(exit.ExitCode())
			}
		}
		_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
		return
	}
}
func (h *wireHost) forward(pending ssh.NewChannel) {
	var target struct {
		Host       string
		Port       uint32
		Origin     string
		OriginPort uint32
	}
	h.mu.Lock()
	address, port := h.forwardAddress, h.forwardPort
	h.mu.Unlock()
	if ssh.Unmarshal(pending.ExtraData(), &target) != nil || target.Host != "fixture-target.invalid" || target.Port != port {
		_ = pending.Reject(ssh.Prohibited, "unexpected fixture destination")
		return
	}
	upstream, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		_ = pending.Reject(ssh.ConnectionFailed, "fixture unavailable")
		return
	}
	channel, requests, err := pending.Accept()
	if err != nil {
		_ = upstream.Close()
		return
	}
	h.forwards.Add(1)
	go ssh.DiscardRequests(requests)
	h.workers.Add(1)
	go func() {
		defer h.workers.Done()
		defer func() { _ = channel.Close(); _ = upstream.Close() }()
		copied := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, channel); _ = upstream.Close(); close(copied) }()
		_, _ = io.Copy(channel, upstream)
		_ = channel.Close()
		<-copied
	}()
}
func (h *wireHost) setDestination(destination *wireHost) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.forwardAddress = net.JoinHostPort("127.0.0.1", strconv.Itoa(destination.port()))
	h.forwardPort = uint32(destination.port())
}

func (h *wireHost) lastKey() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.keyFingerprints) == 0 {
		return ""
	}
	return h.keyFingerprints[len(h.keyFingerprints)-1]
}

func (h *wireHost) changeHostKey(t *testing.T) string {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	config := &ssh.ServerConfig{
		PasswordCallback:  h.config.PasswordCallback,
		PublicKeyCallback: h.config.PublicKeyCallback,
	}
	config.AddHostKey(signer)
	h.config = config
	return ssh.FingerprintSHA256(signer.PublicKey())
}
