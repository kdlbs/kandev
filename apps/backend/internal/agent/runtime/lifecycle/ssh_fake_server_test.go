package lifecycle

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// This file provides an in-process SSH server so the SSH executor's remote
// paths can be exercised end to end without any real infrastructure. It is the
// SSH analogue of httptest.NewServer: a real protocol implementation speaking
// to a real *ssh.Client, with canned command results supplied by the test.
//
// It supports the three channel shapes the SSH executor actually uses:
//   - "session" + exec  — every runSSHCommand / runSSHCommandStdin call
//   - "direct-tcpip"    — StartPortForward and the HTTP-over-SSH control calls
//   - "sftp" subsystem  — sftpUploadBytes and sshFileUploader.WriteFile
//
// Every goroutine it starts is tracked on a WaitGroup and drained by the
// t.Cleanup hook, because this package runs goleak.VerifyTestMain.

// sshExecCall records one exec request observed by the fake SSH server.
type sshExecCall struct {
	Command string
	Stdin   string
}

// sshExecResult is the canned response a handler returns for one exec request.
type sshExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// sshExecHandler answers a single remote command.
type sshExecHandler func(command, stdin string) sshExecResult

type fakeSSHServer struct {
	t        *testing.T
	listener net.Listener
	signer   ssh.Signer

	wg        sync.WaitGroup
	closeOnce sync.Once

	mu      sync.Mutex
	calls   []sshExecCall
	handler sshExecHandler

	// sftpEnabled serves the "sftp" subsystem against the real filesystem, so
	// tests point remote paths at a t.TempDir() and assert on real bytes.
	sftpEnabled bool

	// tcpTarget maps a requested direct-tcpip port onto a real local address.
	// Returning "" rejects the channel.
	tcpTarget func(port uint32) string
}

// newFakeSSHServer starts a listener on 127.0.0.1 and serves SSH until the
// test finishes. handler may be nil, in which case every command succeeds with
// empty output.
func newFakeSSHServer(t *testing.T, handler sshExecHandler) *fakeSSHServer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("host signer: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeSSHServer{t: t, listener: listener, signer: signer, handler: handler}
	t.Cleanup(s.Close)
	s.wg.Add(1)
	go s.acceptLoop()
	return s
}

// enableSFTP turns on the "sftp" subsystem. The server serves the real
// filesystem, so tests must use paths under t.TempDir().
func (s *fakeSSHServer) enableSFTP() { s.sftpEnabled = true }

// forwardTo makes every direct-tcpip channel connect to addr regardless of the
// port the client asked for. Used to point remote "127.0.0.1:<port>" dials at
// an httptest server.
func (s *fakeSSHServer) forwardTo(addr string) {
	s.tcpTarget = func(uint32) string { return addr }
}

func (s *fakeSSHServer) addr() string { return s.listener.Addr().String() }

func (s *fakeSSHServer) port() int { return s.listener.Addr().(*net.TCPAddr).Port }

func (s *fakeSSHServer) hostKeyFingerprint() string {
	return ssh.FingerprintSHA256(s.signer.PublicKey())
}

// commands returns the command strings seen so far, in order.
func (s *fakeSSHServer) commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.calls))
	for _, call := range s.calls {
		out = append(out, call.Command)
	}
	return out
}

// execCalls returns the full recorded calls (command plus stdin), in order.
func (s *fakeSSHServer) execCalls() []sshExecCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sshExecCall(nil), s.calls...)
}

// lastCommandContaining returns the most recent command containing substr.
func (s *fakeSSHServer) lastCommandContaining(substr string) (sshExecCall, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.calls) - 1; i >= 0; i-- {
		if strings.Contains(s.calls[i].Command, substr) {
			return s.calls[i], true
		}
	}
	return sshExecCall{}, false
}

func (s *fakeSSHServer) Close() {
	s.closeOnce.Do(func() {
		_ = s.listener.Close()
	})
	s.wg.Wait()
}

// dial returns a client connected to the fake server. The client is closed by
// t.Cleanup so the SSH mux goroutines drain before goleak inspects them.
func (s *fakeSSHServer) dial(t *testing.T) *ssh.Client {
	t.Helper()
	client, err := ssh.Dial("tcp", s.addr(), &ssh.ClientConfig{
		User:            "kandev",
		HostKeyCallback: ssh.FixedHostKey(s.signer.PublicKey()),
	})
	if err != nil {
		t.Fatalf("dial fake ssh server: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func (s *fakeSSHServer) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go s.serveConn(conn)
	}
}

func (s *fakeSSHServer) serveConn(conn net.Conn) {
	defer s.wg.Done()
	defer func() { _ = conn.Close() }()

	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(s.signer)
	serverConn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer func() { _ = serverConn.Close() }()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ssh.DiscardRequests(reqs)
	}()

	for newChan := range chans {
		switch newChan.ChannelType() {
		case "session":
			s.wg.Add(1)
			go s.serveSession(newChan)
		case "direct-tcpip":
			s.wg.Add(1)
			go s.serveDirectTCPIP(newChan)
		default:
			_ = newChan.Reject(ssh.UnknownChannelType, newChan.ChannelType())
		}
	}
}

func (s *fakeSSHServer) serveSession(newChan ssh.NewChannel) {
	defer s.wg.Done()
	channel, requests, err := newChan.Accept()
	if err != nil {
		return
	}
	defer func() { _ = channel.Close() }()

	for req := range requests {
		switch req.Type {
		case "exec":
			var payload struct{ Command string }
			if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
				_ = req.Reply(false, nil)
				return
			}
			_ = req.Reply(true, nil)
			s.runExec(channel, payload.Command)
			return
		case "subsystem":
			var payload struct{ Name string }
			if err := ssh.Unmarshal(req.Payload, &payload); err != nil || payload.Name != "sftp" || !s.sftpEnabled {
				_ = req.Reply(false, nil)
				return
			}
			_ = req.Reply(true, nil)
			s.runSFTP(channel)
			return
		default:
			// pty-req / env / signal — accepted but ignored.
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
		}
	}
}

func (s *fakeSSHServer) runExec(channel ssh.Channel, command string) {
	stdin, _ := io.ReadAll(channel)

	s.mu.Lock()
	s.calls = append(s.calls, sshExecCall{Command: command, Stdin: string(stdin)})
	handler := s.handler
	s.mu.Unlock()

	result := sshExecResult{}
	if handler != nil {
		result = handler(command, string(stdin))
	}
	if result.Stdout != "" {
		_, _ = io.WriteString(channel, result.Stdout)
	}
	if result.Stderr != "" {
		_, _ = io.WriteString(channel.Stderr(), result.Stderr)
	}
	_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(result.ExitCode)}))
}

func (s *fakeSSHServer) runSFTP(channel ssh.Channel) {
	server, err := sftp.NewServer(channel)
	if err != nil {
		return
	}
	defer func() { _ = server.Close() }()
	if err := server.Serve(); err != nil && !errors.Is(err, io.EOF) {
		s.t.Logf("fake sftp server: %v", err)
	}
}

func (s *fakeSSHServer) serveDirectTCPIP(newChan ssh.NewChannel) {
	defer s.wg.Done()
	var payload struct {
		DestAddr string
		DestPort uint32
		OrigAddr string
		OrigPort uint32
	}
	if err := ssh.Unmarshal(newChan.ExtraData(), &payload); err != nil {
		_ = newChan.Reject(ssh.ConnectionFailed, "bad direct-tcpip payload")
		return
	}
	target := ""
	if s.tcpTarget != nil {
		target = s.tcpTarget(payload.DestPort)
	}
	if target == "" {
		_ = newChan.Reject(ssh.ConnectionFailed, "no forward target configured")
		return
	}
	upstream, err := net.Dial("tcp", target)
	if err != nil {
		_ = newChan.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	channel, requests, err := newChan.Accept()
	if err != nil {
		_ = upstream.Close()
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ssh.DiscardRequests(requests)
	}()

	var copies sync.WaitGroup
	copies.Add(2)
	go func() {
		defer copies.Done()
		_, _ = io.Copy(upstream, channel)
		if tcp, ok := upstream.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()
	go func() {
		defer copies.Done()
		_, _ = io.Copy(channel, upstream)
		_ = channel.CloseWrite()
	}()
	copies.Wait()
	_ = upstream.Close()
	_ = channel.Close()
}

// sshScriptedHandler routes commands to canned results by substring match, in
// declaration order, and fails the test on an unmatched command so a changed
// remote command string surfaces as a test failure rather than silent success.
type sshScriptedHandler struct {
	t     *testing.T
	rules []sshScriptRule
}

type sshScriptRule struct {
	match  string
	result sshExecResult
}

func newSSHScriptedHandler(t *testing.T, rules ...sshScriptRule) *sshScriptedHandler {
	return &sshScriptedHandler{t: t, rules: rules}
}

func (h *sshScriptedHandler) handle(command, _ string) sshExecResult {
	for _, rule := range h.rules {
		if strings.Contains(command, rule.match) {
			return rule.result
		}
	}
	h.t.Errorf("fake ssh server: no rule matched command %q", command)
	return sshExecResult{ExitCode: 127, Stderr: "no rule matched"}
}

// unwrapLoginShell asserts that command is a `<shell> -lc '<script>'` wrapper
// and returns the script with shellQuote's `'\”` escaping undone, so tests can
// assert on the script the remote shell actually executes rather than on its
// quoted-on-the-wire spelling.
func unwrapLoginShell(t *testing.T, shell, command string) string {
	t.Helper()
	prefix := shell + " -lc '"
	if !strings.HasPrefix(command, prefix) || !strings.HasSuffix(command, "'") {
		t.Fatalf("command is not wrapped in %s -lc '...': %q", shell, command)
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(command, prefix), "'")
	return strings.ReplaceAll(inner, `'\''`, "'")
}

// sshOK is the canned success result for commands whose output is irrelevant.
var sshOK = sshExecResult{}

// sshOut builds a success result with the given stdout.
func sshOut(stdout string) sshExecResult { return sshExecResult{Stdout: stdout} }

// sshFail builds a failing result with the given stderr.
func sshFail(stderr string) sshExecResult {
	return sshExecResult{Stderr: stderr, ExitCode: 1}
}
