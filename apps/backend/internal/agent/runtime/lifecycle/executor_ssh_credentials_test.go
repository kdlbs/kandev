package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/remoteauth"
)

func TestRemoteExecutorsIgnoreLegacyHostGHTokenSelection(t *testing.T) {
	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	if err := os.WriteFile(ghPath, []byte("#!/bin/sh\nprintf host-global-token\n"), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", binDir)

	tests := []struct {
		name    string
		resolve func([]string, *ExecutorCreateRequest) []string
	}{
		{name: "ssh", resolve: (&SSHExecutor{logger: newTestLogger()}).resolveGHToken},
		{name: "sprites", resolve: (&SpritesExecutor{logger: newTestLogger()}).resolveGHToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ExecutorCreateRequest{Env: map[string]string{}}
			remaining := tt.resolve([]string{"gh_cli_token", "agent:codex:files:0"}, req)
			if got := req.Env["GITHUB_TOKEN"]; got != "" {
				t.Fatalf("GITHUB_TOKEN = %q, want no host-global token injection", got)
			}
			if len(remaining) != 1 || remaining[0] != "agent:codex:files:0" {
				t.Fatalf("remaining methods = %v, want stale gh_cli_token filtered", remaining)
			}
		})
	}
}

func TestRemoteExecutorsResolveExplicitGitHubTokenSecret(t *testing.T) {
	store := &mockSecretStore{store: map[string]string{"secret-1": "workspace-token"}}
	metadata := map[string]interface{}{
		"remote_auth_secrets": `{"gh_cli_env":"secret-1"}`,
	}
	tests := []struct {
		name    string
		resolve func(context.Context, *ExecutorCreateRequest, remoteauth.Catalog)
	}{
		{
			name: "ssh",
			resolve: (&SSHExecutor{
				secretStore: store,
				logger:      newTestLogger(),
			}).resolveAuthSecrets,
		},
		{
			name: "sprites",
			resolve: (&SpritesExecutor{
				secretStore: store,
				logger:      newTestLogger(),
			}).resolveAuthSecrets,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ExecutorCreateRequest{Metadata: metadata, Env: map[string]string{}}
			tt.resolve(context.Background(), req, remoteauth.BuildCatalogForHost(nil, "linux", ""))
			if got := req.Env["GITHUB_TOKEN"]; got != "workspace-token" {
				t.Fatalf("GITHUB_TOKEN = %q, want selected stored secret", got)
			}
		})
	}
}

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
		if got := buildSSHEnvInitScript(nil); got != "" {
			t.Errorf("buildSSHEnvInitScript(nil) = %q, want \"\"", got)
		}
		if got := buildSSHEnvInitScript(map[string]string{}); got != "" {
			t.Errorf("buildSSHEnvInitScript(empty) = %q, want \"\"", got)
		}
	})

	t.Run("single env var is shell-quoted on its own line", func(t *testing.T) {
		got := buildSSHEnvInitScript(map[string]string{"FOO": "bar baz"})
		// Each line is a POSIX shell assignment; the line break separates
		// entries so `. /dev/stdin` under `set -a` exports each one.
		if got != "FOO='bar baz'\n" {
			t.Errorf("buildSSHEnvInitScript = %q, want \"FOO='bar baz'\\n\"", got)
		}
	})

	t.Run("values with embedded single quotes are escaped", func(t *testing.T) {
		got := buildSSHEnvInitScript(map[string]string{"TOKEN": "it's-a-secret"})
		// shellQuote replaces ' with '\'' for POSIX-safe escaping.
		if !strings.Contains(got, `TOKEN='it'\''s-a-secret'`) {
			t.Errorf("buildSSHEnvInitScript did not escape single quote: %q", got)
		}
		if !strings.HasSuffix(got, "\n") {
			t.Errorf("buildSSHEnvInitScript missing trailing newline: %q", got)
		}
	})
}
