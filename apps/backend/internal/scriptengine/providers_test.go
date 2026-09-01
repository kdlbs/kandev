package scriptengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// shellUnquote runs `printf %s <arg>` through /bin/sh so the test can prove that
// a single-quoted, shell-escaped value parses back to the intended literal
// without any command substitution firing.
func shellUnquote(t *testing.T, arg string) string {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	out, err := exec.Command("sh", "-c", "printf %s "+arg).CombinedOutput()
	if err != nil {
		t.Fatalf("sh failed for %q: %v\n%s", arg, err, out)
	}
	return string(out)
}

func TestAgentInstallProvider(t *testing.T) {
	t.Run("empty scripts produce empty placeholder", func(t *testing.T) {
		provider := AgentInstallProvider(nil)
		vars := provider()
		if got := vars["kandev.agents.install"]; got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("deduplicates scripts", func(t *testing.T) {
		scripts := []string{
			"npm install -g @anthropic-ai/claude-code@2.1.50",
			"npm install -g @openai/codex@0.104.0",
			"npm install -g @anthropic-ai/claude-code@2.1.50",
		}
		provider := AgentInstallProvider(scripts)
		vars := provider()
		want := "npm install -g @anthropic-ai/claude-code@2.1.50\nnpm install -g @openai/codex@0.104.0"
		if got := vars["kandev.agents.install"]; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("trims whitespace and skips empty", func(t *testing.T) {
		scripts := []string{"  npm install -g foo  ", "", "   ", "npm install -g bar"}
		provider := AgentInstallProvider(scripts)
		vars := provider()
		want := "npm install -g foo\nnpm install -g bar"
		if got := vars["kandev.agents.install"]; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestAgentctlProviderSupportsExecutorBinaryPath(t *testing.T) {
	vars := AgentctlProviderWithOptions(8765, "/workspace", AgentctlProviderOptions{
		BinaryPath: "/opt/kandev/agentctl",
		Start:      true,
	})()

	if got, want := vars["kandev.agentctl.install"], "chmod +x '/opt/kandev/agentctl'"; got != want {
		t.Fatalf("kandev.agentctl.install = %q, want %q", got, want)
	}
}

func TestAgentctlProviderShellQuotesExecutorBinaryAndWorkspacePaths(t *testing.T) {
	binaryPath := "/opt/kandev agentctl'$(touch /tmp/pwned)"
	workspacePath := "/workspace/project name'$(touch /tmp/workspace-pwned)"
	vars := AgentctlProviderWithOptions(8765, workspacePath, AgentctlProviderOptions{
		BinaryPath: binaryPath,
		Start:      true,
	})()

	if got, want := vars["kandev.agentctl.install"], "chmod +x "+shellQuote(binaryPath); got != want {
		t.Fatalf("kandev.agentctl.install = %q, want %q", got, want)
	}
	wantStart := "nohup " + shellQuote(binaryPath) +
		" --port 8765 --workdir " + shellQuote(workspacePath) +
		" > /tmp/agentctl.log 2>&1 &\nsleep 1"
	if got := vars["kandev.agentctl.start"]; got != wantStart {
		t.Fatalf("kandev.agentctl.start = %q, want %q", got, wantStart)
	}
}

func TestAgentctlProviderCanLeaveStartToManagedBootstrap(t *testing.T) {
	vars := AgentctlProviderWithOptions(8765, "/workspace", AgentctlProviderOptions{
		BinaryPath: "/opt/kandev/agentctl",
		Start:      false,
	})()

	if got := vars["kandev.agentctl.start"]; got != "" {
		t.Fatalf("kandev.agentctl.start = %q, want empty managed-bootstrap expansion", got)
	}
}

func TestAgentctlProviderPreservesLegacyExpansions(t *testing.T) {
	got := AgentctlProvider(8765, "/workspace")()
	want := map[string]string{
		"kandev.agentctl.port":    "8765",
		"kandev.agentctl.install": "chmod +x '/usr/local/bin/agentctl'",
		"kandev.agentctl.start":   "nohup 'agentctl' --port 8765 --workdir '/workspace' > /tmp/agentctl.log 2>&1 &\nsleep 1",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AgentctlProvider() = %#v, want %#v", got, want)
	}
}

func TestRepositoryProvider_UsesRepositorySetupScriptKey(t *testing.T) {
	provider := RepositoryProvider(map[string]any{
		"repository_path":         "/tmp/repo",
		"base_branch":             "main",
		"repository_setup_script": "npm ci",
		"setup_script":            "should-not-be-used",
	}, nil, nil, nil)

	vars := provider()
	if got := vars["repository.setup_script"]; got != "npm ci" {
		t.Fatalf("repository.setup_script = %q, want %q", got, "npm ci")
	}
}

func TestRemoteContributionSetupScriptRejectsInvalidBinding(t *testing.T) {
	script, err := RemoteContributionSetupScript(&models.RemoteContribution{})
	if err == nil {
		t.Fatal("RemoteContributionSetupScript() accepted an invalid binding")
	}
	if script != "" {
		t.Fatalf("script = %q, want empty script on validation error", script)
	}
}

func TestContributionDestinationSetupScriptPreservesOriginAndRoutesPushes(t *testing.T) {
	destination := &models.ContributionDestination{
		Version:  models.ContributionDestinationVersion,
		Provider: models.ContributionDestinationProviderGitHub,
		SourceRepository: models.ContributionDestinationRepository{
			Host: "github.com", Path: "kdlbs/kandev", ProviderID: "100", RemoteURL: "https://github.com/kdlbs/kandev.git",
		},
		TargetRepository: models.ContributionDestinationRepository{
			Host: "github.com", Path: "agent/kandev", ProviderID: "200", RemoteURL: "https://github.com/agent/kandev.git",
		},
	}
	script, err := ContributionDestinationSetupScriptAt(destination, "/remote/task")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"cd '/remote/task'",
		"remote.$destination_remote.pushurl",
		"branch.$current_branch.pushRemote",
		"https://github.com/agent/kandev.git",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("destination setup script missing %q", want)
		}
	}
	if strings.Contains(script, "remote.origin") || strings.Contains(script, "origin.url") {
		t.Error("destination setup script rewrites origin")
	}
}

// TestProviders_ShellEscapeDataPlaceholders is the scriptengine-level
// regression guard for the branch-name command-injection RCE. Every data
// placeholder that lands in shell text (branch/url/path) must be emitted as a
// fully self-contained single-quoted token (shellQuote), so a hostile value
// like "$(touch pwned)" or "a;b" is a literal even when a template — including
// a stored/legacy prepare_script — references the placeholder BARE.
func TestProviders_ShellEscapeDataPlaceholders(t *testing.T) {
	// A payload containing a single quote exercises the escape sequence: the
	// embedded `'` becomes `'"'"'`, and shellQuote wraps the whole in `'...'`.
	const evil = `x'$(touch pwned)`
	const wantQuoted = `'x'"'"'$(touch pwned)'` // '  x  '"'"'  $(touch pwned)  '

	// assertQuoted checks the provider emitted the fully-quoted token AND that
	// the token parses back to the original literal through a real shell (no
	// command substitution fires).
	assertQuoted := func(t *testing.T, key, got string) {
		t.Helper()
		if got != wantQuoted {
			t.Errorf("%s = %q, want %q", key, got, wantQuoted)
		}
		if out := shellUnquote(t, got); out != evil {
			t.Errorf("%s token %q evaluated to %q, want literal %q", key, got, out, evil)
		}
	}

	t.Run("WorktreeProvider quotes branch and paths", func(t *testing.T) {
		vars := WorktreeProvider(evil, evil, "wt-id", evil, evil)()
		for _, key := range []string{"worktree.base_path", "worktree.path", "worktree.branch", "worktree.base_branch"} {
			assertQuoted(t, key, vars[key])
		}
		// worktree.id is a kandev UUID, intentionally not quoted.
		if got := vars["worktree.id"]; got != "wt-id" {
			t.Errorf("worktree.id = %q, want unmodified", got)
		}
	})

	t.Run("WorkspaceProvider quotes path", func(t *testing.T) {
		assertQuoted(t, "workspace.path", WorkspaceProvider(evil)()["workspace.path"])
	})

	t.Run("RepositoryProvider quotes branch, paths and clone url", func(t *testing.T) {
		vars := RepositoryProvider(map[string]any{
			"base_branch":          evil,
			"repository_clone_url": evil,
			"repository_path":      evil,
		}, nil, nil, nil)()
		assertQuoted(t, "repository.branch", vars["repository.branch"])
		assertQuoted(t, "repository.clone_url", vars["repository.clone_url"])
		assertQuoted(t, "repository.path", vars["repository.path"])
		// repository.name is derived from the (hostile) path's last segment and
		// must also be quoted, since it can land in shell text.
		assertQuoted(t, "repository.name", vars["repository.name"])
	})

	t.Run("resolver-derived repository.ssh_url is quoted", func(t *testing.T) {
		// When no explicit clone URL is set, the provider resolves the remote
		// via repoURLResolver; that value is attacker-influenceable (a remote
		// URL) and must be quoted too.
		resolver := func(string) (string, error) { return evil, nil }
		vars := RepositoryProvider(map[string]any{
			"repository_path": "/tmp/repo",
		}, nil, resolver, nil)()
		assertQuoted(t, "repository.ssh_url", vars["repository.ssh_url"])
		assertQuoted(t, "repository.clone_url", vars["repository.clone_url"])
	})

	t.Run("repository.setup_script is NOT quoted (script fragment)", func(t *testing.T) {
		fragment := "npm ci\necho 'hi'"
		vars := RepositoryProvider(map[string]any{
			"repository_setup_script": fragment,
		}, nil, nil, nil)()
		if got := vars["repository.setup_script"]; got != fragment {
			t.Errorf("repository.setup_script = %q, want unmodified %q", got, fragment)
		}
	})
}

func TestGitHubAuthProvider(t *testing.T) {
	t.Run("nil env returns fallback commands", func(t *testing.T) {
		provider := GitHubAuthProvider(nil)
		vars := provider()
		setup := vars["github.auth_setup"]
		if !strings.Contains(setup, "fallback") {
			t.Errorf("expected fallback comment, got %q", setup)
		}
		if strings.Contains(setup, "credential") {
			t.Error("expected no credential helper when no token")
		}
	})

	t.Run("empty env returns fallback commands", func(t *testing.T) {
		provider := GitHubAuthProvider(map[string]string{})
		vars := provider()
		setup := vars["github.auth_setup"]
		if !strings.Contains(setup, "fallback") {
			t.Errorf("expected fallback comment, got %q", setup)
		}
	})

	t.Run("GH_TOKEN present configures credential helper", func(t *testing.T) {
		provider := GitHubAuthProvider(map[string]string{"GH_TOKEN": "ghp_test123"})
		vars := provider()
		setup := vars["github.auth_setup"]
		if !strings.Contains(setup, "credential.https://github.com.helper") {
			t.Errorf("expected credential helper, got %q", setup)
		}
		// Must use /bin/sh, not /bin/bash
		if strings.Contains(setup, "/bin/bash") {
			t.Error("expected /bin/sh, not /bin/bash")
		}
		if !strings.Contains(setup, "/bin/sh") {
			t.Error("expected /bin/sh in credential helper")
		}
		// Token must NOT be hardcoded in the output script
		if strings.Contains(setup, "ghp_test123") {
			t.Error("token value must not appear literally in the script")
		}
		// Must use env var fallback pattern
		if !strings.Contains(setup, "${GH_TOKEN:-${GITHUB_TOKEN}}") {
			t.Error("expected ${GH_TOKEN:-${GITHUB_TOKEN}} fallback pattern")
		}
	})

	t.Run("GITHUB_TOKEN fallback configures credential helper", func(t *testing.T) {
		provider := GitHubAuthProvider(map[string]string{"GITHUB_TOKEN": "ghp_fallback"})
		vars := provider()
		setup := vars["github.auth_setup"]
		if !strings.Contains(setup, "credential.https://github.com.helper") {
			t.Errorf("expected credential helper with GITHUB_TOKEN fallback, got %q", setup)
		}
		if strings.Contains(setup, "ghp_fallback") {
			t.Error("token value must not appear literally in the script")
		}
	})

	t.Run("includes gh CLI setup", func(t *testing.T) {
		provider := GitHubAuthProvider(map[string]string{"GH_TOKEN": "ghp_test"})
		vars := provider()
		setup := vars["github.auth_setup"]
		if !strings.Contains(setup, "gh config set git_protocol https") {
			t.Error("expected gh CLI protocol config")
		}
		if !strings.Contains(setup, "gh auth setup-git") {
			t.Error("expected gh auth setup-git backup")
		}
	})

	// `gh auth setup-git` — emitted by the no-token branch above and by the
	// with-token branch itself — leaves credential.https://github.com.helper
	// multi-valued. A plain `git config --global <key> <value>` refuses to
	// overwrite a multi-valued key and exits 5, aborting the prepare script
	// under `set -euo pipefail`.
	t.Run("credential helper write uses --replace-all", func(t *testing.T) {
		provider := GitHubAuthProvider(map[string]string{"GH_TOKEN": "ghp_test"})
		vars := provider()
		setup := vars["github.auth_setup"]
		if !strings.Contains(setup, "git config --global --replace-all credential.https://github.com.helper") {
			t.Errorf("credential helper must be written with --replace-all, got %q", setup)
		}
	})
}

// TestGitHubAuthCredentialWriteIsIdempotent is a behavioural regression test: it
// runs the complete auth setup against a real git binary and a real (temporary)
// global config twice. The first setup leaves the key multi-valued, so the
// second setup proves that the write handles the persistent state. Asserting
// the flag string alone would not catch a future rewrite that drops back to a
// single-value write by another spelling — git's exit 5 is the contract.
func TestGitHubAuthCredentialWriteIsIdempotent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	home := t.TempDir()
	bin := t.TempDir()
	ghPath := filepath.Join(bin, "gh")
	ghScript := `#!/bin/sh
case "$1 $2" in
  "config set")
    exit 0
    ;;
  "auth setup-git")
    git config --global --add credential.https://github.com.helper ""
    git config --global --add credential.https://github.com.helper "!/fake/gh auth git-credential"
    exit 0
    ;;
esac
exit 0
`
	if err := os.WriteFile(ghPath, []byte(ghScript), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}

	env := []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + home,
		"GH_TOKEN=ghp_test",
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	runGit := func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	setup := GitHubAuthProvider(map[string]string{"GH_TOKEN": "ghp_test"})()["github.auth_setup"]

	// Run the complete setup twice. The fake gh command models the extra values
	// that `gh auth setup-git` adds after the token helper write.
	for i := 1; i <= 2; i++ {
		cmd := exec.Command("sh", "-c", setup)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("run %d: full auth setup failed: %v: %s", i, err, out)
		}
	}

	out, err := runGit("config", "--global", "--get-all", "credential.https://github.com.helper")
	if err != nil {
		t.Fatalf("reading back config failed: %v: %s", err, out)
	}
	values := strings.Split(strings.TrimRight(out, "\r\n"), "\n")
	if len(values) != 3 {
		t.Fatalf("expected the full setup to leave three helper values, got %d: %q", len(values), values)
	}
	if !strings.Contains(values[0], "x-access-token") ||
		values[1] != "" || values[2] != "!/fake/gh auth git-credential" {
		t.Errorf("unexpected helper values after full setup: %q", values)
	}
}
