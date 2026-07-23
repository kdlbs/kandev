package lifecycle

import (
	"context"
	"strings"
	"testing"
)

func TestWrapLoginShell(t *testing.T) {
	t.Run("empty shell defaults to bash", func(t *testing.T) {
		got := WrapLoginShell("", "echo hi")
		if !strings.HasPrefix(got, "bash -lc ") {
			t.Errorf("WrapLoginShell with empty shell = %q, want bash -lc prefix", got)
		}
	})

	t.Run("custom shell is used verbatim", func(t *testing.T) {
		got := WrapLoginShell("zsh", "echo hi")
		if !strings.HasPrefix(got, "zsh -lc ") {
			t.Errorf("WrapLoginShell with zsh = %q, want zsh -lc prefix", got)
		}
	})

	t.Run("inner command is single-quoted", func(t *testing.T) {
		got := WrapLoginShell("bash", "echo hi")
		if !strings.Contains(got, "'echo hi'") {
			t.Errorf("WrapLoginShell did not single-quote inner cmd: %q", got)
		}
	})

	t.Run("embedded single quote escaped POSIX-safe", func(t *testing.T) {
		// shellQuote's contract is to replace ' with '\'' so a payload
		// like `echo "it's"` becomes 'echo "it'\''s"' — preserving the
		// single quote literally inside the bash -lc argument.
		got := WrapLoginShell("bash", `echo "it's"`)
		if !strings.Contains(got, `'echo "it'\''s"'`) {
			t.Errorf("WrapLoginShell did not escape single quote correctly: %q", got)
		}
	})

	t.Run("multiline scripts survive intact", func(t *testing.T) {
		script := "set -e\nmkdir -p /tmp/x\ncat <<EOF > /tmp/x/f\nhello\nEOF"
		got := WrapLoginShell("bash", script)
		// Newlines inside single-quoted args are valid POSIX shell input.
		if !strings.Contains(got, "set -e\nmkdir -p /tmp/x") {
			t.Errorf("WrapLoginShell mangled multiline script: %q", got)
		}
	})
}

func TestSSHShellForRemote(t *testing.T) {
	t.Run("explicit metadata wins", func(t *testing.T) {
		md := map[string]interface{}{MetadataKeySSHShell: "fish"}
		got := sshShellForRemote(md, SSHRemotePlatform{GOOS: sshRemoteGOOSDarwin, GOARCH: sshRemoteGOARCHARM64})
		if got != "fish" {
			t.Errorf("sshShellForRemote() = %q, want fish", got)
		}
	})

	t.Run("darwin defaults to zsh", func(t *testing.T) {
		got := sshShellForRemote(nil, SSHRemotePlatform{GOOS: sshRemoteGOOSDarwin, GOARCH: sshRemoteGOARCHARM64})
		if got != "zsh" {
			t.Errorf("sshShellForRemote(darwin) = %q, want zsh", got)
		}
	})

	t.Run("linux delegates to WrapLoginShell default", func(t *testing.T) {
		got := sshShellForRemote(nil, SSHRemotePlatform{GOOS: sshRemoteGOOSLinux, GOARCH: sshRemoteGOARCHAMD64})
		if got != "bash" {
			t.Errorf("sshShellForRemote(linux) = %q, want bash", got)
		}
	})
}

func TestParentDir(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/home/zeval/.claude/credentials.json", "/home/zeval/.claude"},
		{"/etc/hosts", "/etc"},
		{"creds.json", ""},
		{"/foo", ""},
		{"", ""},
		{"/", ""},
		{"/a/b/c/d", "/a/b/c"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := parentDir(c.in); got != c.want {
				t.Errorf("parentDir(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestBuildSSHEnvInitScript(t *testing.T) {
	t.Run("empty map returns empty string", func(t *testing.T) {
		got, err := buildSSHEnvInitScript(nil)
		if err != nil {
			t.Fatalf("buildSSHEnvInitScript(nil): %v", err)
		}
		if got != "" {
			t.Errorf("buildSSHEnvInitScript(nil) = %q, want \"\"", got)
		}
		got, err = buildSSHEnvInitScript(map[string]string{})
		if err != nil {
			t.Fatalf("buildSSHEnvInitScript(empty): %v", err)
		}
		if got != "" {
			t.Errorf("buildSSHEnvInitScript(empty) = %q, want \"\"", got)
		}
	})

	t.Run("single env var is shell-quoted on its own line", func(t *testing.T) {
		got, err := buildSSHEnvInitScript(map[string]string{"FOO": "bar baz"})
		if err != nil {
			t.Fatalf("buildSSHEnvInitScript: %v", err)
		}
		// Each line is a POSIX shell assignment; the line break separates
		// entries so `. /dev/stdin` under `set -a` exports each one.
		if got != "FOO='bar baz'\n" {
			t.Errorf("buildSSHEnvInitScript = %q, want \"FOO='bar baz'\\n\"", got)
		}
	})

	t.Run("values with embedded single quotes are escaped", func(t *testing.T) {
		got, err := buildSSHEnvInitScript(map[string]string{"TOKEN": "it's-a-secret"})
		if err != nil {
			t.Fatalf("buildSSHEnvInitScript: %v", err)
		}
		// shellQuote replaces ' with '\'' for POSIX-safe escaping.
		if !strings.Contains(got, `TOKEN='it'\''s-a-secret'`) {
			t.Errorf("buildSSHEnvInitScript did not escape single quote: %q", got)
		}
		if !strings.HasSuffix(got, "\n") {
			t.Errorf("buildSSHEnvInitScript missing trailing newline: %q", got)
		}
	})

	for _, key := range []string{"BAD KEY", "BAD; touch /tmp/pwned", "BAD\nKEY", "$(touch /tmp/pwned)", "1BAD"} {
		t.Run("rejects invalid key "+key, func(t *testing.T) {
			script, err := buildSSHEnvInitScript(map[string]string{key: "secret"})
			if err == nil {
				t.Fatal("expected invalid key error")
			}
			if script != "" {
				t.Errorf("invalid key appeared in script: %q", script)
			}
		})
	}
}

func TestStartRemoteAgentctlRejectsInvalidEnvBeforeSSHLaunch(t *testing.T) {
	_, _, err := startRemoteAgentctl(
		context.Background(),
		nil,
		"bash",
		"/usr/local/bin/agentctl",
		"/workspace",
		"/tmp/session",
		map[string]string{"BAD; touch /tmp/pwned": "secret"},
		nil,
	)
	if err == nil {
		t.Fatal("expected invalid SSH environment key to abort agentctl launch")
	}
	if !strings.Contains(err.Error(), "invalid SSH environment variable key") {
		t.Errorf("startRemoteAgentctl error = %q", err)
	}
}

func TestSSHAgentctlLaunchEnvForcesLoopbackAndOmitsBearerToken(t *testing.T) {
	env := sshAgentctlLaunchEnv(map[string]string{
		"AGENTCTL_AUTH_TOKEN":  "profile-token",
		"AGENTCTL_LISTEN_HOST": "0.0.0.0",
		"OPENAI_API_KEY":       "key",
	}, "bootstrap-nonce")

	if got := env["AGENTCTL_BOOTSTRAP_NONCE"]; got != "bootstrap-nonce" {
		t.Fatalf("AGENTCTL_BOOTSTRAP_NONCE = %q, want bootstrap nonce", got)
	}
	if got := env["AGENTCTL_LISTEN_HOST"]; got != "127.0.0.1" {
		t.Fatalf("AGENTCTL_LISTEN_HOST = %q, want loopback", got)
	}
	if _, found := env["AGENTCTL_AUTH_TOKEN"]; found {
		t.Fatal("SSH agentctl launch environment must not contain bearer token")
	}
	if got := env["OPENAI_API_KEY"]; got != "key" {
		t.Fatalf("OPENAI_API_KEY = %q, want copied profile value", got)
	}
}
