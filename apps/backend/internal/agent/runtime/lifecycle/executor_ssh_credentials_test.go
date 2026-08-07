package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/remoteauth"
)

func TestRemoteExecutorsIgnoreLegacyHostGHTokenSelection(t *testing.T) {
	// resolveGHToken is a pure filter: it can no longer reach the request, so
	// a stale gh_cli_token selection is dropped rather than turned into a
	// host-global GITHUB_TOKEN injection.
	tests := []struct {
		name    string
		resolve func([]string) []string
	}{
		{name: "ssh", resolve: (&SSHExecutor{logger: newTestLogger()}).resolveGHToken},
		{name: "sprites", resolve: (&SpritesExecutor{logger: newTestLogger()}).resolveGHToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remaining := tt.resolve([]string{"gh_cli_token", "agent:codex:files:0"})
			if len(remaining) != 1 || remaining[0] != "agent:codex:files:0" {
				t.Fatalf("remaining methods = %v, want stale gh_cli_token filtered", remaining)
			}
			if got := tt.resolve([]string{"agent:codex:files:0"}); len(got) != 1 || got[0] != "agent:codex:files:0" {
				t.Fatalf("resolve without gh_cli_token = %v, want untouched selection", got)
			}
		})
	}
}

func TestRemoteExecutorsResolveExplicitGitHubTokenSecret(t *testing.T) {
	store := &mockSecretStore{store: map[string]string{"secret-1": "workspace-token"}}
	metadata := map[string]interface{}{"remote_auth_secrets": `{"gh_cli_env":"secret-1"}`}
	tests := []struct {
		name    string
		resolve func(context.Context, *ExecutorCreateRequest, remoteauth.Catalog)
	}{
		{name: "ssh", resolve: (&SSHExecutor{secretStore: store, logger: newTestLogger()}).resolveAuthSecrets},
		{name: "sprites", resolve: (&SpritesExecutor{secretStore: store, logger: newTestLogger()}).resolveAuthSecrets},
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

func TestRetryRemoteAgentctlPortRetriesBindCollisions(t *testing.T) {
	ports := []int{41001, 41002, 41003}
	nextPort := 0
	attempts := 0

	port, pid, err := retryRemoteAgentctlPort(
		func() int {
			port := ports[nextPort]
			nextPort++
			return port
		},
		func(port int) (int, error) {
			attempts++
			if attempts < 3 {
				return 0, fmt.Errorf("%w: %d", errSSHAgentctlPortInUse, port)
			}
			return 1234, nil
		},
	)

	if err != nil {
		t.Fatalf("retryRemoteAgentctlPort: %v", err)
	}
	if port != 41003 || pid != 1234 || attempts != 3 {
		t.Fatalf("result = port %d pid %d attempts %d, want 41003/1234/3", port, pid, attempts)
	}
}

func TestRetryRemoteAgentctlPortDoesNotRetryOtherFailures(t *testing.T) {
	attempts := 0
	wantErr := errors.New("remote launch failed")
	_, _, err := retryRemoteAgentctlPort(
		func() int { return 41001 },
		func(int) (int, error) {
			attempts++
			return 0, wantErr
		},
	)

	if !errors.Is(err, wantErr) {
		t.Fatalf("retryRemoteAgentctlPort error = %v, want %v", err, wantErr)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestRemoteAgentctlStopCommandWaitsAndEscalatesBeforeCleanup(t *testing.T) {
	command := remoteAgentctlStopCommand("/tmp/session with space", 1234)

	wantOrder := []string{
		"kill 1234 2>/dev/null || true",
		"while kill -0 1234",
		"kill -9 1234",
		"rm -rf '/tmp/session with space'",
	}
	previous := -1
	for _, fragment := range wantOrder {
		index := strings.Index(command, fragment)
		if index <= previous {
			t.Fatalf("command fragment %q is missing or out of order:\n%s", fragment, command)
		}
		previous = index
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
