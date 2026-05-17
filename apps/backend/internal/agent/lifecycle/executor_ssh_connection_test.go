package lifecycle

import (
	"os"
	"strings"
	"testing"
)

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

func TestRequireSupportedArch(t *testing.T) {
	if err := requireSupportedArch("x86_64"); err != nil {
		t.Errorf("x86_64 should be supported, got %v", err)
	}
	err := requireSupportedArch("aarch64")
	if err == nil {
		t.Fatal("aarch64 should not be supported")
	}
	if !strings.Contains(err.Error(), "aarch64") || !strings.Contains(err.Error(), "linux/amd64") {
		t.Errorf("error should name the arch and the supported one: %v", err)
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
