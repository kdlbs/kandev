package config

import (
	"os"
	"strings"
	"testing"
)

func TestCollectAgentEnvKeepsGitHubCLIShimAheadOfProfilePath(t *testing.T) {
	t.Setenv("KANDEV_GITHUB_CREDENTIAL_BROKER_URL", "https://kandev.example/api/github/credentials/resolve")
	t.Setenv("KANDEV_GITHUB_CLI_SHIM_DIR", "/kandev/shims")
	env := CollectAgentEnv(map[string]string{"PATH": "/profile/bin:/usr/bin"})
	want := strings.Join([]string{"/kandev/shims", "/profile/bin", "/usr/bin"}, string(os.PathListSeparator))
	if got := envSliceValue(env, "PATH"); got != want {
		t.Fatalf("PATH = %q, want %q", got, want)
	}
}

func envSliceValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func TestConsumeNonce(t *testing.T) {
	t.Run("valid nonce returns token and burns nonce", func(t *testing.T) {
		cfg := &Config{
			AuthToken:      "test-token-123",
			BootstrapNonce: "nonce-abc",
		}

		token := cfg.ConsumeNonce("nonce-abc")
		if token != "test-token-123" {
			t.Fatalf("expected test-token-123, got %q", token)
		}

		// Second call should fail (nonce burned)
		token2 := cfg.ConsumeNonce("nonce-abc")
		if token2 != "" {
			t.Fatalf("expected empty after nonce burn, got %q", token2)
		}
	})

	t.Run("wrong nonce returns empty", func(t *testing.T) {
		cfg := &Config{
			AuthToken:      "test-token",
			BootstrapNonce: "correct-nonce",
		}

		token := cfg.ConsumeNonce("wrong-nonce")
		if token != "" {
			t.Fatalf("expected empty for wrong nonce, got %q", token)
		}

		// Original nonce should still work
		token2 := cfg.ConsumeNonce("correct-nonce")
		if token2 != "test-token" {
			t.Fatalf("expected test-token, got %q", token2)
		}
	})

	t.Run("empty nonce returns empty", func(t *testing.T) {
		cfg := &Config{
			AuthToken:      "test-token",
			BootstrapNonce: "some-nonce",
		}

		token := cfg.ConsumeNonce("")
		if token != "" {
			t.Fatalf("expected empty for empty nonce, got %q", token)
		}
	})

	t.Run("no bootstrap nonce configured returns empty", func(t *testing.T) {
		cfg := &Config{
			AuthToken: "test-token",
		}

		token := cfg.ConsumeNonce("any-nonce")
		if token != "" {
			t.Fatalf("expected empty when no nonce configured, got %q", token)
		}
	})
}

func TestGenerateSelfToken(t *testing.T) {
	token := generateSelfToken()
	if len(token) != 64 { // 32 bytes hex-encoded = 64 chars
		t.Fatalf("expected 64-char hex token, got %d chars: %q", len(token), token)
	}

	// Tokens should be unique
	token2 := generateSelfToken()
	if token == token2 {
		t.Fatal("expected unique tokens, got identical")
	}
}

// TestInjectKandevMcpServer_HttpFirst is the McpServerConfig counterpart to
// api.TestInjectKandevMcpServers_HttpFirst. HTTP must appear before SSE so the
// downstream capability filter dedup keeps the HTTP transport for agents that
// advertise both transports.
func TestInjectKandevMcpServer_HttpFirst(t *testing.T) {
	got := injectKandevMcpServer(nil, 12345)
	if len(got) != 2 {
		t.Fatalf("expected 2 kandev entries, got %d: %+v", len(got), got)
	}
	if got[0].Name != kandevMcpServerName || got[0].Type != "http" {
		t.Errorf("expected first entry name=%q type=http, got name=%q type=%q",
			kandevMcpServerName, got[0].Name, got[0].Type)
	}
	if got[0].URL != "http://localhost:12345/mcp" {
		t.Errorf("expected http URL .../mcp, got %q", got[0].URL)
	}
	if got[1].Name != kandevMcpServerName || got[1].Type != "sse" {
		t.Errorf("expected second entry name=%q type=sse, got name=%q type=%q",
			kandevMcpServerName, got[1].Name, got[1].Type)
	}
	if got[1].URL != "http://localhost:12345/sse" {
		t.Errorf("expected sse URL .../sse, got %q", got[1].URL)
	}

	upstream := []McpServerConfig{
		{Name: "other", Type: "stdio", Command: "x"},
		{Name: kandevMcpServerName, Type: "sse", URL: "http://stale/sse"},
	}
	got = injectKandevMcpServer(upstream, 12345)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries (http+sse+other), got %d: %+v", len(got), got)
	}
	if got[0].Type != "http" || got[1].Type != "sse" {
		t.Errorf("expected injected order http,sse; got %q,%q", got[0].Type, got[1].Type)
	}
	if got[2].Name != "other" || got[2].Command != "x" {
		t.Errorf("expected upstream 'other' entry last, got %+v", got[2])
	}
}

// TestApplyOverrides_BaseBranches verifies the per-instance per-repo base
// branch map flows from the HTTP-request-driven InstanceOverrides onto the
// InstanceConfig the process manager consumes. Empty/nil overrides must NOT
// clobber a previously-set value — applyOverrides is a one-shot merge.
func TestApplyOverrides_BaseBranches(t *testing.T) {
	t.Run("non-empty overrides populate cfg", func(t *testing.T) {
		cfg := &InstanceConfig{}
		applyOverrides(cfg, &InstanceOverrides{
			BaseBranches: map[string]string{"repo-a": "main", "repo-b": "develop"},
		})
		if got := cfg.BaseBranches["repo-a"]; got != "main" {
			t.Errorf("BaseBranches[repo-a] = %q, want main", got)
		}
		if got := cfg.BaseBranches["repo-b"]; got != "develop" {
			t.Errorf("BaseBranches[repo-b] = %q, want develop", got)
		}
	})

	t.Run("empty overrides preserve existing", func(t *testing.T) {
		cfg := &InstanceConfig{
			BaseBranches: map[string]string{"repo-a": "main"},
		}
		applyOverrides(cfg, &InstanceOverrides{BaseBranches: nil})
		if got := cfg.BaseBranches["repo-a"]; got != "main" {
			t.Errorf("empty overrides clobbered existing value; got %q", got)
		}
	})
}

// TestListenHost pins the loopback-only bind guard: when auth is disabled
// (empty AuthToken) the server must bind loopback only; when a token is
// configured it binds all interfaces (empty host → ":port").
func TestListenHost(t *testing.T) {
	t.Run("no token binds loopback only", func(t *testing.T) {
		cfg := &Config{AuthToken: ""}
		if got := cfg.ListenHost(); got != "127.0.0.1" {
			t.Fatalf("ListenHost() = %q, want 127.0.0.1 when auth disabled", got)
		}
	})
	t.Run("token binds all interfaces", func(t *testing.T) {
		cfg := &Config{AuthToken: "secret"}
		if got := cfg.ListenHost(); got != "" {
			t.Fatalf("ListenHost() = %q, want \"\" (all interfaces) when auth enabled", got)
		}
	})
}
