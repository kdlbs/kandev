package sshtransport

import (
	"context"
	"errors"
	"golang.org/x/crypto/ssh"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type unsupportedDeadlineConn struct{ net.Conn }

func (unsupportedDeadlineConn) SetDeadline(time.Time) error {
	return errors.New("deadlines are not supported")
}

func TestHandshakeWithDeadlineClosesConnectionsWhenDeadlinesAreUnsupported(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer func() { _ = serverConn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, _, err := handshakeWithDeadline(ctx, unsupportedDeadlineConn{Conn: clientConn}, "test", &ssh.ClientConfig{
			User:            "test",
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		})
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("handshake error = %v, want context deadline exceeded", err)
		}
	case <-time.After(500 * time.Millisecond):
		_ = clientConn.Close()
		t.Fatalf("handshake did not stop at the deadline, closing the connection returned %v", <-done)
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home dir: %v", err)
	}
	if got := expandHome("~"); got != home {
		t.Errorf("expandHome(~) = %q, want %q", got, home)
	}
	if got := expandHome("~/.ssh/id_ed25519"); !strings.HasPrefix(got, home+"/.ssh") {
		t.Errorf("expandHome(~/.ssh/...) = %q, want prefix %q/.ssh", got, home)
	}
	if got := expandHome("/abs/path"); got != "/abs/path" {
		t.Errorf("expandHome(/abs/path) = %q, want unchanged", got)
	}
}

func TestErrHostKeyMismatchMessage(t *testing.T) {
	e := &HostKeyMismatchError{Expected: "SHA256:aaa", Got: "SHA256:bbb"}
	msg := e.Error()
	for _, want := range []string{"host key changed", "expected SHA256:aaa", "got SHA256:bbb"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q missing %q", msg, want)
		}
	}
}

func TestParsePortString(t *testing.T) {
	cases := []struct {
		in      string
		wantN   int
		wantOK  bool
		comment string
	}{
		{"22", 22, true, "low canonical port"},
		{"1", 1, true, "min port"},
		{"65535", 65535, true, "max port"},
		{"0", 0, false, "zero is reserved"},
		{"65536", 0, false, "above 16-bit range"},
		{"-1", 0, false, "negative"},
		{"", 0, false, "empty"},
		{"abc", 0, false, "non-numeric"},
		{"22 ", 0, false, "trailing whitespace not stripped here"},
	}
	for _, c := range cases {
		t.Run(c.in+"/"+c.comment, func(t *testing.T) {
			n, ok := parsePortString(c.in)
			if ok != c.wantOK || n != c.wantN {
				t.Errorf("parsePortString(%q) = (%d, %v), want (%d, %v)", c.in, n, ok, c.wantN, c.wantOK)
			}
		})
	}
}

func TestParseBracketedHostPort(t *testing.T) {
	cases := []struct {
		in       string
		wantHost string
		wantPort int
		wantOK   bool
		comment  string
	}{
		{"[2001:db8::1]", "2001:db8::1", 0, true, "ipv6 no port"},
		{"[2001:db8::1]:22", "2001:db8::1", 22, true, "ipv6 with port"},
		{"[::1]:2200", "::1", 2200, true, "ipv6 loopback"},
		{"[host.example.com]:22", "host.example.com", 22, true, "hostname in brackets"},
		{"[2001:db8::1", "", 0, false, "missing close bracket"},
		{"[2001:db8::1]extra", "", 0, false, "junk after close bracket"},
		{"[2001:db8::1]:0", "", 0, false, "port out of range"},
		{"[2001:db8::1]:abc", "", 0, false, "non-numeric port"},
	}
	for _, c := range cases {
		t.Run(c.in+"/"+c.comment, func(t *testing.T) {
			host, port, ok := parseBracketedHostPort(c.in)
			if ok != c.wantOK || host != c.wantHost || port != c.wantPort {
				t.Errorf("parseBracketedHostPort(%q) = (%q, %d, %v), want (%q, %d, %v)",
					c.in, host, port, ok, c.wantHost, c.wantPort, c.wantOK)
			}
		})
	}
}

func TestParseProxyJumpHostPort(t *testing.T) {
	cases := []struct {
		in       string
		wantHost string
		wantPort int
		wantOK   bool
		comment  string
	}{
		{"bastion.example.com", "bastion.example.com", 0, true, "host only"},
		{"bastion.example.com:2222", "bastion.example.com", 2222, true, "host + port"},
		{"[2001:db8::1]:22", "2001:db8::1", 22, true, "bracketed ipv6 + port"},
		{"[2001:db8::1]", "2001:db8::1", 0, true, "bracketed ipv6 no port"},
		{"bastion.example.com:abc", "", 0, false, "bad port"},
		{"bastion.example.com:0", "", 0, false, "port out of range"},
	}
	for _, c := range cases {
		t.Run(c.in+"/"+c.comment, func(t *testing.T) {
			host, port, ok := parseProxyJumpHostPort(c.in)
			if ok != c.wantOK || host != c.wantHost || port != c.wantPort {
				t.Errorf("parseProxyJumpHostPort(%q) = (%q, %d, %v), want (%q, %d, %v)",
					c.in, host, port, ok, c.wantHost, c.wantPort, c.wantOK)
			}
		})
	}
}

// noInstallScriptAgent is a minimal agentIdentity stub for cases where we
// want to assert behavior with InstallScript() returning empty / Name()
// returning empty. The real agents.Agent interface is large; this satisfies
// only the slice formatMissingAgentBinaryError reads.
func TestParseLiteralProxyJump(t *testing.T) {
	cases := []struct {
		in       string
		wantUser string
		wantHost string
		wantPort int
		wantOK   bool
		comment  string
	}{
		{"jump.example.com", "", "", 0, false, "alias only — no @ or : — defers to alias path"},
		{"alice@jump.example.com", "alice", "jump.example.com", 0, true, "user + host"},
		{"alice@jump.example.com:2222", "alice", "jump.example.com", 2222, true, "user + host + port"},
		{"jump.example.com:2222", "", "jump.example.com", 2222, true, "host + port, no user"},
		{"alice@[2001:db8::1]:22", "alice", "2001:db8::1", 22, true, "user + bracketed ipv6 + port"},
		{"[2001:db8::1]:22", "", "2001:db8::1", 22, true, "bracketed ipv6 + port, no user"},
		{"[2001:db8::1]", "", "2001:db8::1", 0, true, "bracketed ipv6 no port"},
		{"alice@", "", "", 0, false, "empty host after @"},
		{"alice@jump:0", "", "", 0, false, "invalid port"},
		{"", "", "", 0, false, "empty"},
		{"   ", "", "", 0, false, "whitespace only"},
		// IPv6 ProxyJump regression guard — the bracketed-host parser must
		// strip brackets so callers can feed host straight into net.JoinHostPort
		// without producing `[[2001:db8::1]]:22`.
		{"deploy@[2001:db8:dead:beef::42]:2200", "deploy", "2001:db8:dead:beef::42", 2200, true, "ipv6 ProxyJump regression"},
	}
	for _, c := range cases {
		t.Run(c.in+"/"+c.comment, func(t *testing.T) {
			user, host, port, ok := parseLiteralProxyJump(c.in)
			if ok != c.wantOK || user != c.wantUser || host != c.wantHost || port != c.wantPort {
				t.Errorf("parseLiteralProxyJump(%q) = (%q, %q, %d, %v), want (%q, %q, %d, %v)",
					c.in, user, host, port, ok, c.wantUser, c.wantHost, c.wantPort, c.wantOK)
			}
		})
	}
}

func TestExpandIdentityAgentOpenSSHSyntax(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENT_DIR", filepath.Join(home, "agents"))
	t.Setenv("LEGACY_AGENT", filepath.Join(home, "legacy.sock"))
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(home, "env.sock"))
	target := &SSHTarget{
		Host: "resolved.internal", Port: 2222, User: "deploy",
		ProxyJump: "jump-alias", OriginalHost: "prod",
	}

	cases := map[string]string{
		"~/.ssh/agent.sock":                   filepath.Join(home, ".ssh", "agent.sock"),
		"${AGENT_DIR}/%n-%h-%p-%r-%j-%%.sock": filepath.Join(home, "agents", "prod-resolved.internal-2222-deploy-jump-alias-%.sock"),
		"$LEGACY_AGENT":                       filepath.Join(home, "legacy.sock"),
		"SSH_AUTH_SOCK":                       filepath.Join(home, "env.sock"),
		"@abstract-agent":                     "@abstract-agent",
	}
	for input, want := range cases {
		got, err := expandIdentityAgent(input, target)
		if err != nil {
			t.Errorf("expandIdentityAgent(%q): %v", input, err)
		} else if got != want {
			t.Errorf("expandIdentityAgent(%q) = %q, want %q", input, got, want)
		}
	}

	allTokens, err := expandIdentityAgent("%d-%i-%k-%L-%l-%u", target)
	if err != nil || allTokens == "" || !strings.Contains(allTokens, "prod") {
		t.Fatalf("all token expansion = %q, %v", allTokens, err)
	}
	for _, input := range []string{"${MISSING_IDENTITY_AGENT}", "%C", "%Z", "trailing%"} {
		if _, err := expandIdentityAgent(input, target); err == nil {
			t.Errorf("expandIdentityAgent(%q) unexpectedly succeeded", input)
		}
	}
}

func TestExpandIdentityAgentRejectsUnsetLegacyVariable(t *testing.T) {
	const variable = "MISSING_IDENTITY_AGENT"
	previous, wasSet := os.LookupEnv(variable)
	if err := os.Unsetenv(variable); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(variable, previous)
			return
		}
		_ = os.Unsetenv(variable)
	})
	t.Setenv("SSH_AUTH_SOCK", "/tmp/fallback.sock")

	got, err := expandIdentityAgent("$"+variable, &SSHTarget{Host: "example.com"})
	if err == nil {
		t.Fatalf("expandIdentityAgent() returned %q without an error", got)
	}
	if !strings.Contains(err.Error(), variable) {
		t.Fatalf("error = %v, want missing variable name %q", err, variable)
	}
}

func TestExpandIdentityAgentEnvironmentDoesNotReprocessSubstitutedTokens(t *testing.T) {
	const childEnv = "KANDEV_TEST_EXPAND_IDENTITY_AGENT_ENV"
	if os.Getenv(childEnv) == "1" {
		t.Setenv("AGENT", "${AGENT}")
		got, err := expandIdentityAgentEnvironment("${AGENT}")
		if err != nil {
			t.Fatal(err)
		}
		if got != "${AGENT}" {
			t.Fatalf("expanded value = %q, want the substituted token to remain literal", got)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run", "^TestExpandIdentityAgentEnvironmentDoesNotReprocessSubstitutedTokens$")
	cmd.Env = append(os.Environ(), childEnv+"=1")
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("environment expansion did not terminate")
	}
	if err != nil {
		t.Fatalf("child test failed: %v\n%s", err, output)
	}
}
