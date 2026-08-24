package lifecycle

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/task/models"
	"golang.org/x/crypto/ssh"
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
		"CLAUDE_CODE_OAUTH_TOKEN":       tokenFromReq,
		"NPM_TOKEN":                     "repository-token",
		"PROFILE_ONLY":                  "profile-value",
		"HOME":                          nonCredentialHome,
		"PATH":                          nonCredentialPath,
		"OPENAI_API_KEY":                openAIKey,
		envKeyGitHubCredentialBrokerURL: "https://kandev.example/api/v1/github/credentials/resolve",
		envKeyGitHubCredentialLease:     "opaque-lease",
		"GIT_CONFIG_COUNT":              "1",
		"GIT_CONFIG_KEY_0":              "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_0":            "!agentctl git-credential",
	}, ApprovedSecretEnvKeys: []string{"NPM_TOKEN"}}
	got := sshRemoteAgentEnv(req)
	if got["CLAUDE_CODE_OAUTH_TOKEN"] != tokenFromReq {
		t.Fatalf("CLAUDE_CODE_OAUTH_TOKEN = %q, want %q", got["CLAUDE_CODE_OAUTH_TOKEN"], tokenFromReq)
	}
	if got["OPENAI_API_KEY"] != openAIKey {
		t.Fatalf("OPENAI_API_KEY = %q, want %q", got["OPENAI_API_KEY"], openAIKey)
	}
	if got["NPM_TOKEN"] != "repository-token" {
		t.Fatalf("NPM_TOKEN = %q, want repository-token", got["NPM_TOKEN"])
	}
	if _, ok := got["PROFILE_ONLY"]; ok {
		t.Fatal("unapproved profile key must not be forwarded to the remote agent")
	}
	if got[envKeyGitHubCredentialLease] != "opaque-lease" || got["GIT_CONFIG_KEY_0"] == "" {
		t.Fatalf("managed GitHub broker env was not forwarded: %#v", got)
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

func TestSSHRemoteContributionEnvUsesScopedGitCredentialHelper(t *testing.T) {
	req := &ExecutorCreateRequest{Env: map[string]string{
		envKeyGitHubCredentialBrokerURL: "https://kandev.example/api/v1/github/credentials/resolve",
		envKeyGitHubCredentialLease:     "lease",
		"GIT_CONFIG_COUNT":              "1",
		"GIT_CONFIG_KEY_0":              "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_0":            "!agentctl git-credential",
	}}
	got := sshRemoteContributionEnv(req, "/home/agent/.kandev/bin/agentctl")
	if got["GIT_CONFIG_VALUE_0"] != "!/home/agent/.kandev/bin/agentctl git-credential" {
		t.Fatalf("GitHub helper = %q, want absolute agentctl helper", got["GIT_CONFIG_VALUE_0"])
	}
	if got["GIT_CONFIG_COUNT"] != "1" {
		t.Fatalf("GIT_CONFIG_COUNT = %q, want 1", got["GIT_CONFIG_COUNT"])
	}
}

func TestSSHRemoteContributionEnvRewritesPluginCredentialHelper(t *testing.T) {
	req := &ExecutorCreateRequest{Env: map[string]string{
		envKeyGitHubCredentialBrokerURL: "https://kandev.example/api/v1/github/credentials/resolve",
		envKeyGitHubCredentialLease:     "lease",
		"GIT_CONFIG_COUNT":              "2",
		"GIT_CONFIG_KEY_0":              "credential.https://bitbucket.example.test.helper",
		"GIT_CONFIG_VALUE_0":            "",
		"GIT_CONFIG_KEY_1":              "credential.https://bitbucket.example.test.helper",
		"GIT_CONFIG_VALUE_1":            githubauth.ManagedGitCredentialHelper,
	}}

	got := sshRemoteContributionEnv(req, "/home/agent/.kandev/bin/agentctl")
	if got["GIT_CONFIG_VALUE_1"] != "!/home/agent/.kandev/bin/agentctl git-credential" {
		t.Fatalf("plugin helper = %q, want absolute agentctl helper", got["GIT_CONFIG_VALUE_1"])
	}
	if got["GIT_CONFIG_COUNT"] != "2" {
		t.Fatalf("GIT_CONFIG_COUNT = %q, want 2", got["GIT_CONFIG_COUNT"])
	}
}

func TestBuildSSHCreateInstanceRequestRewritesPluginCredentialHelper(t *testing.T) {
	req := &ExecutorCreateRequest{Env: map[string]string{
		envKeyGitHubCredentialBrokerURL: "https://kandev.example/api/v1/github/credentials/resolve",
		envKeyGitHubCredentialLease:     "lease",
		"GIT_CONFIG_COUNT":              "2",
		"GIT_CONFIG_KEY_0":              "credential.https://bitbucket.example.test.helper",
		"GIT_CONFIG_VALUE_0":            "",
		"GIT_CONFIG_KEY_1":              "credential.https://bitbucket.example.test.helper",
		"GIT_CONFIG_VALUE_1":            githubauth.ManagedGitCredentialHelper,
	}}

	got := buildSSHCreateInstanceRequest(req, "/workspace", "/home/agent/.kandev/bin/agentctl")
	if got.Env["GIT_CONFIG_VALUE_1"] != "!/home/agent/.kandev/bin/agentctl git-credential" {
		t.Fatalf("plugin helper = %q, want absolute agentctl helper", got.Env["GIT_CONFIG_VALUE_1"])
	}
}

func TestSSHRemoteContributionScriptPinsTargetAndSourceIdentity(t *testing.T) {
	binding := &models.RemoteContribution{
		Version:      models.RemoteContributionVersion,
		Provider:     models.RemoteContributionProviderGitHub,
		Kind:         models.RemoteContributionKindPullRequest,
		CanonicalURL: "https://github.com/acme/widget/pull/7",
		Number:       7,
		State:        models.RemoteContributionStateOpen,
		BaseBranch:   "main",
		HeadBranch:   "feature/remote",
		HeadSHA:      strings.Repeat("a", 40),
		SourceRepository: models.RemoteContributionRepository{
			Host:      "github.com",
			Path:      "contributor/widget",
			RemoteURL: "https://github.com/contributor/widget.git",
		},
		CollaborationAllowed: true,
	}
	script := sshRemoteContributionScript("/remote/task", "https://github.com/acme/widget.git", binding)
	for _, want := range []string{
		"remote add origin",
		"fetch --no-tags origin",
		binding.SourceRepository.RemoteURL,
		"contribution_remote='" + binding.ContributionRemoteName() + "'",
		"refs/heads/feature/remote",
		strings.Repeat("a", 40),
		"branch --set-upstream-to",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("SSH contribution script missing %q", want)
		}
	}
	if strings.Contains(script, "untrusted") {
		t.Fatal("SSH contribution script contains provider-authored content")
	}
}

func TestSSHRemoteContributionScriptReusesLocalDescendant(t *testing.T) {
	root := t.TempDir()
	targetBare := filepath.Join(root, "target.git")
	sourceBare := filepath.Join(root, "source.git")
	targetSeed := filepath.Join(root, "target-seed")
	sourceSeed := filepath.Join(root, "source-seed")
	workspace := filepath.Join(root, "workspace")
	for _, args := range [][]string{
		{"init", "--bare", "--initial-branch=main", targetBare},
		{"init", "--bare", "--initial-branch=main", sourceBare},
		{"init", "--initial-branch=main", targetSeed},
		{"init", "--initial-branch=main", sourceSeed},
	} {
		runSSHContributionGit(t, root, args...)
	}
	for _, repo := range []string{targetSeed, sourceSeed} {
		runSSHContributionGit(t, repo, "config", "user.email", "test@example.com")
		runSSHContributionGit(t, repo, "config", "user.name", "Test User")
	}
	if err := os.WriteFile(filepath.Join(targetSeed, "README.md"), []byte("target\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runSSHContributionGit(t, targetSeed, "add", "README.md")
	runSSHContributionGit(t, targetSeed, "commit", "-m", "target base")
	runSSHContributionGit(t, targetSeed, "remote", "add", "origin", targetBare)
	runSSHContributionGit(t, targetSeed, "push", "origin", "main")

	if err := os.WriteFile(filepath.Join(sourceSeed, "README.md"), []byte("source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runSSHContributionGit(t, sourceSeed, "add", "README.md")
	runSSHContributionGit(t, sourceSeed, "commit", "-m", "source base")
	runSSHContributionGit(t, sourceSeed, "checkout", "-b", "feature/remote")
	if err := os.WriteFile(filepath.Join(sourceSeed, "change.txt"), []byte("contribution\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runSSHContributionGit(t, sourceSeed, "add", "change.txt")
	runSSHContributionGit(t, sourceSeed, "commit", "-m", "source contribution")
	sourceSHA := strings.TrimSpace(runSSHContributionGit(t, sourceSeed, "rev-parse", "HEAD"))
	runSSHContributionGit(t, sourceSeed, "remote", "add", "origin", sourceBare)
	runSSHContributionGit(t, sourceSeed, "push", "origin", "main", "feature/remote")

	targetURL := "https://github.com/acme/widget.git"
	sourceURL := "https://github.com/contributor/widget.git"
	configPath := filepath.Join(root, "gitconfig")
	config := "[url \"file://" + targetBare + "\"]\n\tinsteadOf = " + targetURL + "\n" +
		"[url \"file://" + sourceBare + "\"]\n\tinsteadOf = " + sourceURL + "\n"
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", configPath)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	binding := &models.RemoteContribution{
		Version:      models.RemoteContributionVersion,
		Provider:     models.RemoteContributionProviderGitHub,
		Kind:         models.RemoteContributionKindPullRequest,
		CanonicalURL: "https://github.com/acme/widget/pull/7",
		Number:       7,
		State:        models.RemoteContributionStateOpen,
		BaseBranch:   "main",
		HeadBranch:   "feature/remote",
		HeadSHA:      sourceSHA,
		SourceRepository: models.RemoteContributionRepository{
			Host: "github.com", Path: "contributor/widget", RemoteURL: sourceURL,
		},
		CollaborationAllowed: true,
	}
	runSSHContributionScript(t, sshRemoteContributionScript(workspace, targetURL, binding))
	if got := strings.TrimSpace(runSSHContributionGit(t, workspace, "rev-parse", "HEAD")); got != sourceSHA {
		t.Fatalf("initial SSH checkout HEAD = %q, want %q", got, sourceSHA)
	}
	if got := strings.TrimSpace(runSSHContributionGit(t, workspace, "branch", "--show-current")); got != binding.HeadBranch {
		t.Fatalf("initial SSH branch = %q, want %q", got, binding.HeadBranch)
	}
	wantUpstream := binding.ContributionRemoteName() + "/" + binding.HeadBranch
	if got := strings.TrimSpace(runSSHContributionGit(t, workspace, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")); got != wantUpstream {
		t.Fatalf("initial SSH upstream = %q, want %q", got, wantUpstream)
	}

	runSSHContributionGit(t, workspace, "config", "user.email", "test@example.com")
	runSSHContributionGit(t, workspace, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(workspace, "agent-change.txt"), []byte("agent change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runSSHContributionGit(t, workspace, "add", "agent-change.txt")
	runSSHContributionGit(t, workspace, "commit", "-m", "agent contribution")
	localHEAD := strings.TrimSpace(runSSHContributionGit(t, workspace, "rev-parse", "HEAD"))

	runSSHContributionScript(t, sshRemoteContributionScript(workspace, targetURL, binding))
	if got := strings.TrimSpace(runSSHContributionGit(t, workspace, "rev-parse", "HEAD")); got != localHEAD {
		t.Fatalf("resumed SSH checkout HEAD = %q, want local commit %q", got, localHEAD)
	}
	if got := strings.TrimSpace(runSSHContributionGit(t, workspace, "branch", "--show-current")); got != binding.HeadBranch {
		t.Fatalf("resumed SSH branch = %q, want %q", got, binding.HeadBranch)
	}
	if got := strings.TrimSpace(runSSHContributionGit(t, workspace, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")); got != wantUpstream {
		t.Fatalf("resumed SSH upstream = %q, want %q", got, wantUpstream)
	}
}

func runSSHContributionScript(t *testing.T, script string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "sh", "-c", script)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("SSH contribution script failed: %v\n%s", err, output)
	}
}

func runSSHContributionGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return string(output)
}

func TestSSHRemoteAgentEnvApprovedRepositoryKeyCannotReplaceManagedCredential(t *testing.T) {
	got := sshRemoteAgentEnv(&ExecutorCreateRequest{
		Env: map[string]string{
			"ANTHROPIC_API_KEY": "managed-credential",
			"NPM_TOKEN":         "repository-token",
		},
		ApprovedSecretEnvKeys: []string{"ANTHROPIC_API_KEY", "NPM_TOKEN"},
	})
	if got["ANTHROPIC_API_KEY"] != "managed-credential" {
		t.Fatalf("ANTHROPIC_API_KEY = %q, want managed credential", got["ANTHROPIC_API_KEY"])
	}
	if got["NPM_TOKEN"] != "repository-token" {
		t.Fatalf("NPM_TOKEN = %q, want repository token", got["NPM_TOKEN"])
	}
}

func TestSSHManagedBrokerResumeForcesFreshAgentctlWithNewLease(t *testing.T) {
	sshExec := NewSSHExecutor(nil, nil, nil, logger.Default())
	sshExec.sessions["instance-1"] = &sshSessionState{pid: 1234, remoteDir: "/remote/session"}
	req := &ExecutorCreateRequest{
		InstanceID: "instance-1",
		Env: map[string]string{
			envKeyGitHubCredentialBrokerURL: "https://kandev.example/api/v1/github/credentials/resolve",
			envKeyGitHubCredentialLease:     "fresh-lease-after-backend-restart",
		},
		Metadata: map[string]interface{}{
			MetadataKeySSHHost:               "remote.example",
			MetadataKeySSHRemoteSessionDir:   "/remote/session",
			MetadataKeySSHRemoteAgentctlPort: "41001",
			MetadataKeySSHRemoteAgentctlPID:  "1234",
			MetadataKeySSHLocalForwardPort:   "51001",
			MetadataKeySSHRemoteAgentctlURL:  "http://127.0.0.1:41001",
		},
	}

	var probedLease string
	sshExec.brokerPreflight = func(_ context.Context, _ *ssh.Client, probeReq *ExecutorCreateRequest, _ SSHRemotePlatform) error {
		probedLease = managedGitHubBrokerEnv(probeReq.Env)[envKeyGitHubCredentialLease]
		return nil
	}
	var stoppedPID int
	sshExec.stopRemote = func(_ context.Context, _ *ssh.Client, _ string, pid int) error {
		stoppedPID = pid
		return nil
	}
	if err := sshExec.resetManagedBrokerResume(context.Background(), req, "1234", "/remote/session"); err != nil {
		t.Fatalf("resetManagedBrokerResume() error = %v", err)
	}
	if probedLease != "fresh-lease-after-backend-restart" {
		t.Fatalf("preflight lease = %q, want fresh lease", probedLease)
	}
	if stoppedPID != 1234 {
		t.Fatalf("stopped pid = %d, want 1234", stoppedPID)
	}
	if _, tracked := sshExec.sessions["instance-1"]; tracked {
		t.Fatal("stale broker-backed SSH session remains tracked")
	}
	if _, reused := sshExec.resumedStateForCreate(req); reused {
		t.Fatal("managed broker recovery reused agentctl carrying an invalidated lease")
	}
	for _, key := range []string{
		MetadataKeySSHRemoteSessionDir,
		MetadataKeySSHRemoteAgentctlPort,
		MetadataKeySSHRemoteAgentctlPID,
		MetadataKeySSHLocalForwardPort,
		MetadataKeySSHRemoteAgentctlURL,
	} {
		if _, found := req.Metadata[key]; found {
			t.Fatalf("stale resume metadata %q was retained", key)
		}
	}
	if req.Metadata[MetadataKeySSHHost] != "remote.example" {
		t.Fatal("connection metadata required for fresh SSH launch was removed")
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
