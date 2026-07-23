package lifecycle

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

func TestSSHControlRequestUsesBearerToken(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/instances", nil)
	if err != nil {
		t.Fatal(err)
	}
	setSSHControlAuthorization(req, "launch-token")
	if got := req.Header.Get("Authorization"); got != "Bearer launch-token" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestRequireSSHAgentctlAuthTokenRejectsEmptyToken(t *testing.T) {
	if err := requireSSHAgentctlAuthToken(""); err == nil {
		t.Fatal("expected empty SSH agentctl token to be rejected")
	}
}

func TestResolveSSHTarget_ExplicitFields(t *testing.T) {
	target, err := ResolveSSHTarget(SSHConnConfig{
		Host:              "example.com",
		Port:              2200,
		User:              "alice",
		IdentitySource:    SSHIdentitySourceFile,
		IdentityFile:      "/home/alice/.ssh/id_ed25519",
		PinnedFingerprint: "SHA256:abcdef",
	})
	if err != nil {
		t.Fatalf("ResolveSSHTarget: %v", err)
	}
	if target.Host != "example.com" {
		t.Errorf("Host = %q, want example.com", target.Host)
	}
	if target.Port != 2200 {
		t.Errorf("Port = %d, want 2200", target.Port)
	}
	if target.User != "alice" {
		t.Errorf("User = %q, want alice", target.User)
	}
	if target.IdentitySource != SSHIdentitySourceFile {
		t.Errorf("IdentitySource = %q, want file", target.IdentitySource)
	}
	if target.IdentityFile != "/home/alice/.ssh/id_ed25519" {
		t.Errorf("IdentityFile = %q", target.IdentityFile)
	}
}

func TestResolveSSHTarget_DefaultsPort22(t *testing.T) {
	target, err := ResolveSSHTarget(SSHConnConfig{
		Host:              "example.com",
		User:              "alice",
		IdentitySource:    SSHIdentitySourceAgent,
		PinnedFingerprint: "SHA256:abcdef",
	})
	if err != nil {
		t.Fatalf("ResolveSSHTarget: %v", err)
	}
	if target.Port != 22 {
		t.Errorf("default Port = %d, want 22", target.Port)
	}
}

func TestResolveSSHTarget_DefaultUserFromEnv(t *testing.T) {
	t.Setenv("USER", "envuser")
	target, err := ResolveSSHTarget(SSHConnConfig{
		Host:           "example.com",
		IdentitySource: SSHIdentitySourceAgent,
	})
	if err != nil {
		t.Fatalf("ResolveSSHTarget: %v", err)
	}
	if target.User != "envuser" {
		t.Errorf("default User = %q, want envuser", target.User)
	}
}

func TestResolveSSHTarget_HostRequired(t *testing.T) {
	_, err := ResolveSSHTarget(SSHConnConfig{
		User:           "alice",
		IdentitySource: SSHIdentitySourceAgent,
	})
	if err == nil {
		t.Fatal("expected error when host is empty")
	}
	if !strings.Contains(err.Error(), "host is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveSSHTarget_AliasInfersHostName(t *testing.T) {
	// With no ~/.ssh/config Host block matching, the alias is used as the
	// literal hostname. This is the "user typed something but has no
	// matching block" fallback.
	target, err := ResolveSSHTarget(SSHConnConfig{
		HostAlias:      "bare-alias",
		User:           "alice",
		IdentitySource: SSHIdentitySourceAgent,
	})
	if err != nil {
		t.Fatalf("ResolveSSHTarget: %v", err)
	}
	if target.Host != "bare-alias" {
		t.Errorf("Host = %q, want bare-alias", target.Host)
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

func TestSSHExecutorTargetFromMetadataMissingFingerprintNamesConnectionSettings(t *testing.T) {
	exec := &SSHExecutor{}
	_, err := exec.targetFromMetadata(map[string]interface{}{
		MetadataKeySSHHost:           "example.com",
		MetadataKeySSHUser:           "alice",
		MetadataKeySSHIdentitySource: string(SSHIdentitySourceAgent),
	})
	if err == nil {
		t.Fatal("expected missing host_fingerprint error")
	}
	msg := err.Error()
	for _, want := range []string{"host_fingerprint is required", "SSH executor connection settings", "Test connection", "trust the host"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q missing %q", msg, want)
		}
	}
}

func TestNormalizeSSHRemotePlatform(t *testing.T) {
	cases := []struct {
		name     string
		osName   string
		arch     string
		wantOS   string
		wantArch string
		wantOK   bool
	}{
		{"linux amd64", "Linux", "x86_64", "linux", "amd64", true},
		{"darwin arm64", "Darwin", "arm64", "darwin", "arm64", true},
		{"darwin amd64", "Darwin", "x86_64", "darwin", "amd64", true},
		{"linux arm64", "Linux", "aarch64", "linux", "arm64", true},
		{"freebsd amd64 unsupported", "FreeBSD", "x86_64", "", "amd64", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := normalizeSSHRemotePlatform(tc.osName, tc.arch)
			if ok != tc.wantOK {
				t.Fatalf("normalizeSSHRemotePlatform(%q, %q) ok = %v, want %v", tc.osName, tc.arch, ok, tc.wantOK)
			}
			if got.GOOS != tc.wantOS || got.GOARCH != tc.wantArch {
				t.Errorf("normalizeSSHRemotePlatform(%q, %q) = %s/%s, want %s/%s",
					tc.osName, tc.arch, got.GOOS, got.GOARCH, tc.wantOS, tc.wantArch)
			}
		})
	}
}

func TestRequireSupportedRemotePlatform(t *testing.T) {
	for _, platform := range []SSHRemotePlatform{
		{GOOS: "linux", GOARCH: "amd64", UnameOS: "Linux", UnameArch: "x86_64"},
		{GOOS: "linux", GOARCH: "arm64", UnameOS: "Linux", UnameArch: "aarch64"},
		{GOOS: "darwin", GOARCH: "arm64", UnameOS: "Darwin", UnameArch: "arm64"},
		{GOOS: "darwin", GOARCH: "amd64", UnameOS: "Darwin", UnameArch: "x86_64"},
	} {
		if err := requireSupportedRemotePlatform(platform); err != nil {
			t.Errorf("%s should be supported, got %v", platform.String(), err)
		}
	}
	unsupported := SSHRemotePlatform{GOOS: "", GOARCH: "amd64", UnameOS: "FreeBSD", UnameArch: "x86_64"}
	err := requireSupportedRemotePlatform(unsupported)
	if err == nil {
		t.Fatal("freebsd/amd64 should not be supported")
	}
	for _, want := range []string{"unsupported remote platform", "linux/{amd64,arm64}", "darwin/{amd64,arm64}", "FreeBSD"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestErrHostKeyMismatchMessage(t *testing.T) {
	e := &errHostKeyMismatch{Expected: "SHA256:aaa", Got: "SHA256:bbb"}
	msg := e.Error()
	for _, want := range []string{"host key changed", "expected SHA256:aaa", "got SHA256:bbb"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q missing %q", msg, want)
		}
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"simple":      "'simple'",
		"with space":  "'with space'",
		"don't":       `'don'\''t'`,
		"path/to/dir": "'path/to/dir'",
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
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
type noInstallScriptAgent struct {
	id   string
	name string
}

func (a *noInstallScriptAgent) ID() string            { return a.id }
func (a *noInstallScriptAgent) Name() string          { return a.name }
func (a *noInstallScriptAgent) InstallScript() string { return "" }

func TestFormatMissingAgentBinaryError_WithInstallScript(t *testing.T) {
	// MockAgent advertises a deterministic InstallScript so e2e + this test
	// can both pin the "install hint" branch without depending on real CLIs.
	ag := agents.NewMockAgent()
	got := formatMissingAgentBinaryError(ag, "npx")
	for _, want := range []string{
		"Mock Agent", // ag.Name() — must surface so users see which agent is missing
		`"npx"`,      // the binary we probed
		"$PATH",      // tells them where we looked
		"Install hint",
		ag.InstallScript(), // the actual command they should run
	} {
		if !strings.Contains(got, want) {
			t.Errorf("formatMissingAgentBinaryError(...): missing %q in %q", want, got)
		}
	}
}

func TestFormatMissingAgentBinaryError_NoInstallScriptOmitsHintBlock(t *testing.T) {
	ag := &noInstallScriptAgent{name: "PhantomAgent"}
	got := formatMissingAgentBinaryError(ag, "phantom")
	if strings.Contains(got, "Install hint") {
		t.Errorf("expected no Install hint block when InstallScript() is empty, got %q", got)
	}
	if !strings.Contains(got, "PhantomAgent") || !strings.Contains(got, `"phantom"`) {
		t.Errorf("expected agent name + binary in message, got %q", got)
	}
}

func TestFormatMissingAgentBinaryError_FallsBackToIDWhenNameEmpty(t *testing.T) {
	ag := &noInstallScriptAgent{id: "fallback-id"}
	got := formatMissingAgentBinaryError(ag, "fallback-bin")
	if !strings.Contains(got, "fallback-id") {
		t.Errorf("expected ID fallback in message when Name() is empty, got %q", got)
	}
}

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

func TestSSHRemoteAgentEnv(t *testing.T) {
	// Fixture values — named so it's clear these are arbitrary test inputs,
	// not real credentials or host config.
	const (
		tokenFromReq      = "claude-token-from-req"
		tokenFromEnv      = "claude-token-from-controlplane"
		openAIKey         = "openai-key-from-req"
		anthropicFromEnv  = "anthropic-key-from-controlplane"
		nonCredentialHome = "/home/agent"
		nonCredentialPath = "/usr/bin"
	)

	// req.Env credential keys are forwarded; non-credential keys (HOME/PATH) are not.
	req := &ExecutorCreateRequest{Env: map[string]string{
		"CLAUDE_CODE_OAUTH_TOKEN": tokenFromReq,
		"HOME":                    nonCredentialHome,
		"PATH":                    nonCredentialPath,
		"OPENAI_API_KEY":          openAIKey,
	}}
	got := sshRemoteAgentEnv(req)
	if got["CLAUDE_CODE_OAUTH_TOKEN"] != tokenFromReq {
		t.Fatalf("CLAUDE_CODE_OAUTH_TOKEN = %q, want %q", got["CLAUDE_CODE_OAUTH_TOKEN"], tokenFromReq)
	}
	if got["OPENAI_API_KEY"] != openAIKey {
		t.Fatalf("OPENAI_API_KEY = %q, want %q", got["OPENAI_API_KEY"], openAIKey)
	}
	if _, ok := got["HOME"]; ok {
		t.Error("HOME must NOT be forwarded to the remote agent")
	}
	if _, ok := got["PATH"]; ok {
		t.Error("PATH must NOT be forwarded to the remote agent")
	}

	// Credentials present ONLY in the control-plane process env must NOT be
	// forwarded (that would leak the kandev host's own credentials to any SSH
	// target). Only keys explicitly resolved into req.Env are sent.
	t.Setenv("ANTHROPIC_API_KEY", anthropicFromEnv)
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", tokenFromEnv)
	got = sshRemoteAgentEnv(&ExecutorCreateRequest{Env: map[string]string{}})
	if _, ok := got["ANTHROPIC_API_KEY"]; ok {
		t.Error("ANTHROPIC_API_KEY from control-plane env must NOT be forwarded when absent from req.Env")
	}
	if got != nil {
		t.Fatalf("expected nil when req.Env has no credential keys, got %v", got)
	}

	// req.Env is the sole source; the control-plane env is ignored even when set.
	got = sshRemoteAgentEnv(&ExecutorCreateRequest{Env: map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": tokenFromReq}})
	if got["CLAUDE_CODE_OAUTH_TOKEN"] != tokenFromReq {
		t.Fatalf("req.Env should be the source, got %q", got["CLAUDE_CODE_OAUTH_TOKEN"])
	}
}

func TestSSHRemoteAgentEnvEmpty(t *testing.T) {
	// nil req and empty req.Env both yield nil (no control-plane fallback).
	if got := sshRemoteAgentEnv(nil); got != nil {
		t.Fatalf("expected nil for nil req, got %v", got)
	}
	if got := sshRemoteAgentEnv(&ExecutorCreateRequest{}); got != nil {
		t.Fatalf("expected nil for no credentials, got %v", got)
	}
}
