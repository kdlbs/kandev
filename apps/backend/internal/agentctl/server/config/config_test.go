package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func TestCollectAgentEnvGitHubCLIShimSurvivesLoginShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash login-shell behavior is Unix-specific")
	}
	shimDir := filepath.Join(t.TempDir(), "managed github shim")
	if err := os.MkdirAll(shimDir, 0o700); err != nil {
		t.Fatalf("create shim directory: %v", err)
	}
	for _, name := range []string{"agentctl", "gh"} {
		path := filepath.Join(shimDir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatalf("write %s shim: %v", name, err)
		}
	}
	marker := filepath.Join(t.TempDir(), "bash-hook-ran")
	parentBashEnv := filepath.Join(t.TempDir(), "parent-bash-env.sh")
	if err := os.WriteFile(parentBashEnv, []byte("#!/bin/sh\nprintf hook-ran > \"$KANDEV_BASH_HOOK_MARKER\"\n"), 0o700); err != nil {
		t.Fatalf("write parent Bash environment: %v", err)
	}
	bashEnv := filepath.Join(shimDir, "bash-env.sh")
	bootstrap := "#!/bin/sh\n" +
		"if [ -n \"${KANDEV_GITHUB_PARENT_BASH_ENV:-}\" ]; then . \"$KANDEV_GITHUB_PARENT_BASH_ENV\"; fi\n" +
		"case \":${PATH:-}:\" in *:\"${KANDEV_GITHUB_CLI_SHIM_DIR}\":*) ;; *) PATH=\"${KANDEV_GITHUB_CLI_SHIM_DIR}:${PATH:-}\"; export PATH ;; esac\n"
	if err := os.WriteFile(bashEnv, []byte(bootstrap), 0o700); err != nil {
		t.Fatalf("write Bash environment fixture: %v", err)
	}

	env, err := CollectAgentEnvWithError(map[string]string{
		"KANDEV_GITHUB_CREDENTIAL_BROKER_URL": "https://kandev.example/resolve",
		"KANDEV_GITHUB_CLI_SHIM_DIR":          shimDir,
		"KANDEV_GITHUB_CLI_BASH_ENV":          bashEnv,
		"BASH_ENV":                            parentBashEnv,
		"KANDEV_BASH_HOOK_MARKER":             marker,
		"PATH":                                "/usr/bin:/bin",
	})
	if err != nil {
		t.Fatalf("CollectAgentEnvWithError() error = %v", err)
	}
	if got := envSliceValue(env, "BASH_ENV"); got != bashEnv {
		t.Fatalf("BASH_ENV = %q, want generated environment %q", got, bashEnv)
	}
	if got := envSliceValue(env, "KANDEV_GITHUB_PARENT_BASH_ENV"); got != parentBashEnv {
		t.Fatalf("parent Bash environment = %q, want %q", got, parentBashEnv)
	}

	command := exec.Command("bash", "-lc", "printf 'agentctl=%s\\ngh=%s\\n' \"$(command -v agentctl || true)\" \"$(command -v gh || true)\"")
	command.Env = env
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("bash login shell failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "agentctl="+filepath.Join(shimDir, "agentctl")) ||
		!strings.Contains(string(output), "gh="+filepath.Join(shimDir, "gh")) {
		t.Fatalf("login-shell tool resolution = %q", output)
	}
	if hook, err := os.ReadFile(marker); err != nil || string(hook) != "hook-ran" {
		t.Fatalf("parent Bash hook marker = %q, error = %v", hook, err)
	}
}

func TestCollectAgentEnvResolvesParameterizedBashEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash startup behavior is Unix-specific")
	}
	shimDir := t.TempDir()
	hookDir := t.TempDir()
	marker := filepath.Join(hookDir, "parent-hook-ran")
	parentBashEnv := filepath.Join(hookDir, "parent-bash-env.sh")
	if err := os.WriteFile(parentBashEnv, []byte("#!/bin/sh\nprintf hook-ran > \"$KANDEV_TEST_HOOK_MARKER\"\n"), 0o700); err != nil {
		t.Fatalf("write parent Bash environment: %v", err)
	}
	bashEnv := filepath.Join(shimDir, "bash-env.sh")
	if err := os.WriteFile(bashEnv, []byte("#!/bin/sh\n. \"$KANDEV_GITHUB_PARENT_BASH_ENV\"\n"), 0o700); err != nil {
		t.Fatalf("write managed Bash environment: %v", err)
	}

	env, err := CollectAgentEnvWithError(map[string]string{
		"KANDEV_GITHUB_CREDENTIAL_BROKER_URL": "https://kandev.example/resolve",
		"KANDEV_GITHUB_CLI_SHIM_DIR":          shimDir,
		"KANDEV_GITHUB_CLI_BASH_ENV":          bashEnv,
		"KANDEV_TEST_HOOK_ROOT":               hookDir,
		"KANDEV_TEST_HOOK_MARKER":             marker,
		"BASH_ENV":                            "${KANDEV_TEST_HOOK_ROOT}/parent-bash-env.sh",
	})
	if err != nil {
		t.Fatalf("CollectAgentEnvWithError() error = %v", err)
	}
	if got := envSliceValue(env, "KANDEV_GITHUB_PARENT_BASH_ENV"); got != parentBashEnv {
		t.Fatalf("parent Bash environment = %q, want resolved path %q", got, parentBashEnv)
	}
	command := exec.Command("bash", "-c", "true")
	command.Env = env
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("parameterized Bash environment failed: %v\n%s", err, output)
	}
	if hook, err := os.ReadFile(marker); err != nil || string(hook) != "hook-ran" {
		t.Fatalf("parent Bash hook marker = %q, error = %v", hook, err)
	}
}

func TestCollectAgentEnvAvoidsManagedBashEnvSelfSourcing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash startup behavior is Unix-specific")
	}
	startupEnv := filepath.Join(t.TempDir(), "managed-bash-env.sh")
	env, err := CollectAgentEnvWithError(map[string]string{
		"KANDEV_GITHUB_CREDENTIAL_BROKER_URL": "https://kandev.example/resolve",
		"KANDEV_GITHUB_CLI_BASH_ENV":          startupEnv,
		"KANDEV_GITHUB_PARENT_BASH_ENV":       "/stale/parent-bash-env.sh",
		"BASH_ENV":                            startupEnv,
	})
	if err != nil {
		t.Fatalf("CollectAgentEnvWithError() error = %v", err)
	}
	if got := envSliceValue(env, "BASH_ENV"); got != startupEnv {
		t.Fatalf("BASH_ENV = %q, want generated environment %q", got, startupEnv)
	}
	if got := envSliceValue(env, "KANDEV_GITHUB_PARENT_BASH_ENV"); got != "" {
		t.Fatalf("parent Bash environment = %q, want unset for self-sourcing hook", got)
	}
}

func TestExpandBashEnvParameters(t *testing.T) {
	env := map[string]string{"HOME": "/home/agent", "HOOK_ROOT": "/opt/hooks"}
	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "unbraced", value: "$HOME/bash-env.sh", want: "/home/agent/bash-env.sh"},
		{name: "braced", value: "${HOOK_ROOT}/bash-env.sh", want: "/opt/hooks/bash-env.sh"},
		{name: "unsupported expansion remains literal", value: "$(printf /tmp/hook)", want: "$(printf /tmp/hook)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := expandBashEnvParameters(tc.value, env); got != tc.want {
				t.Fatalf("expandBashEnvParameters(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestCollectAgentEnvLeavesGitHubStartupHookUntouchedWithoutBroker(t *testing.T) {
	t.Setenv("KANDEV_GITHUB_CREDENTIAL_BROKER_URL", "")
	parentBashEnv := filepath.Join(t.TempDir(), "parent-bash-env.sh")
	startupEnv := filepath.Join(t.TempDir(), "managed-bash-env.sh")
	env, err := CollectAgentEnvWithError(map[string]string{
		"KANDEV_GITHUB_CLI_SHIM_DIR": " /managed/shims ",
		"KANDEV_GITHUB_CLI_BASH_ENV": startupEnv,
		"BASH_ENV":                   parentBashEnv,
		"PATH":                       "/usr/bin:/bin",
	})
	if err != nil {
		t.Fatalf("CollectAgentEnvWithError() error = %v", err)
	}
	if got := envSliceValue(env, "BASH_ENV"); got != parentBashEnv {
		t.Fatalf("BASH_ENV = %q, want existing hook %q", got, parentBashEnv)
	}
	if got := envSliceValue(env, "KANDEV_GITHUB_PARENT_BASH_ENV"); got != "" {
		t.Fatalf("parent Bash environment = %q, want unset without broker", got)
	}
	if got := envSliceValue(env, "PATH"); got != "/usr/bin:/bin" {
		t.Fatalf("PATH = %q, want unchanged without broker", got)
	}
}

func TestCollectAgentEnvPreservesParentIndexedGitConfig(t *testing.T) {
	clearParentIndexedGitConfig(t)
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", "/opt/locstat/hooks")
	t.Setenv("GIT_CONFIG_KEY_1", "notes.augment.mergeStrategy")
	t.Setenv("GIT_CONFIG_VALUE_1", "cat_sort_uniq")

	env := CollectAgentEnv(map[string]string{
		"GIT_CONFIG_COUNT":   "2",
		"GIT_CONFIG_KEY_0":   "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_0": "!agentctl git-credential",
		"GIT_CONFIG_KEY_1":   "credential.useHttpPath",
		"GIT_CONFIG_VALUE_1": "true",
	})

	if got := envSliceValue(env, "GIT_CONFIG_COUNT"); got != "4" {
		t.Fatalf("GIT_CONFIG_COUNT = %q, want 4", got)
	}
	if got := envSliceValue(env, "GIT_CONFIG_KEY_0"); got != "core.hooksPath" {
		t.Fatalf("GIT_CONFIG_KEY_0 = %q, want core.hooksPath", got)
	}
	if got := envSliceValue(env, "GIT_CONFIG_KEY_2"); got != "credential.https://github.com.helper" {
		t.Fatalf("GIT_CONFIG_KEY_2 = %q, want managed helper", got)
	}
}

func TestCollectAgentEnvPreservesParentGitConfigHook(t *testing.T) {
	clearParentIndexedGitConfig(t)
	repository := t.TempDir()
	hooks := t.TempDir()
	marker := filepath.Join(t.TempDir(), "hook-ran")
	hookPath := filepath.Join(hooks, "pre-commit")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o755); err != nil {
		t.Fatalf("write hook: %v", err)
	}
	runGit(t, nil, "init", repository)
	runGit(t, nil, "-C", repository, "config", "user.name", "Kandev Test")
	runGit(t, nil, "-C", repository, "config", "user.email", "kandev@example.test")

	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", hooks)
	t.Setenv("GIT_CONFIG_KEY_1", "notes.augment.mergeStrategy")
	t.Setenv("GIT_CONFIG_VALUE_1", "cat_sort_uniq")
	env := CollectAgentEnv(map[string]string{
		"GIT_CONFIG_COUNT":   "2",
		"GIT_CONFIG_KEY_0":   "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_0": "!agentctl git-credential",
		"GIT_CONFIG_KEY_1":   "credential.useHttpPath",
		"GIT_CONFIG_VALUE_1": "true",
	})

	runGit(t, env, "-C", repository, "commit", "--allow-empty", "-m", "verify inherited hook")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("inherited pre-commit hook did not run: %v", err)
	}
}

func clearParentIndexedGitConfig(t *testing.T) {
	t.Helper()
	original := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || (key != "GIT_CONFIG_COUNT" &&
			!strings.HasPrefix(key, "GIT_CONFIG_KEY_") &&
			!strings.HasPrefix(key, "GIT_CONFIG_VALUE_")) {
			continue
		}
		original[key] = value
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset inherited %s: %v", key, err)
		}
	}
	t.Cleanup(func() {
		for key, value := range original {
			if err := os.Setenv(key, value); err != nil {
				t.Errorf("restore inherited %s: %v", key, err)
			}
		}
	})
}

func runGit(t *testing.T, env []string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	if env != nil {
		command.Env = env
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
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

func TestValidateCommandArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{name: "valid preserves empty argument", args: []string{"runner", "", "two words"}},
		{name: "empty argv", args: nil, want: true},
		{name: "empty executable", args: []string{"", "arg"}, want: true},
		{name: "whitespace executable", args: []string{" \t", "arg"}, want: true},
		{name: "flag executable", args: []string{"--runner", "arg"}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCommandArgs(tc.args)
			if (err != nil) != tc.want {
				t.Fatalf("ValidateCommandArgs(%#v) error = %v, want error=%v", tc.args, err, tc.want)
			}
		})
	}
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
	t.Run("explicit override wins when token is configured", func(t *testing.T) {
		cfg := &Config{AuthToken: "secret", ListenHostOverride: "127.0.0.1"}
		if got := cfg.ListenHost(); got != "127.0.0.1" {
			t.Fatalf("ListenHost() = %q, want explicit loopback override", got)
		}
	})
	t.Run("loads explicit override from launch environment", func(t *testing.T) {
		t.Setenv("AGENTCTL_BOOTSTRAP_NONCE", "nonce")
		t.Setenv("AGENTCTL_LISTEN_HOST", "127.0.0.1")
		if got := Load().ListenHost(); got != "127.0.0.1" {
			t.Fatalf("Load().ListenHost() = %q, want loopback override", got)
		}
	})
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
