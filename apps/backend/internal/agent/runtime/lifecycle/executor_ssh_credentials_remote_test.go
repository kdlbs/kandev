package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/remoteauth"
)

// remoteAuthAgent declares a RemoteAuth block so BuildCatalog produces real
// method entries for it.
type remoteAuthAgent struct {
	*agents.MockAgent
	id         string
	remoteAuth *agents.RemoteAuth
}

func (a *remoteAuthAgent) ID() string                     { return a.id }
func (a *remoteAuthAgent) DisplayName() string            { return a.id }
func (a *remoteAuthAgent) RemoteAuth() *agents.RemoteAuth { return a.remoteAuth }
func (a *remoteAuthAgent) InstallScript() string          { return "" }
func (a *remoteAuthAgent) Name() string                   { return a.id }
func (a *remoteAuthAgent) newCatalog() remoteauth.Catalog {
	return remoteauth.BuildCatalog([]agents.Agent{a})
}
func (a *remoteAuthAgent) lister() RemoteAuthAgentLister {
	return &mockAgentLister{agents: []agents.Agent{a}}
}
func (a *remoteAuthAgent) fileMethodID(index string) string {
	return "agent:" + a.id + ":files:" + index
}

var _ agents.Agent = (*remoteAuthAgent)(nil)

// newCredentialAgent declares one file method (a credentials file under the
// home dir) and one env method carrying a setup script.
func newCredentialAgent(sourceFile string) *remoteAuthAgent {
	return &remoteAuthAgent{
		MockAgent: agents.NewMockAgent(),
		id:        "credential-agent",
		remoteAuth: &agents.RemoteAuth{Methods: []agents.RemoteAuthMethod{
			{
				Type:         "files",
				SourceFiles:  map[string][]string{runtime.GOOS: {sourceFile}},
				TargetRelDir: ".credential-agent",
				Label:        "Copy credentials",
			},
			{
				Type:        "env",
				EnvVar:      "CREDENTIAL_AGENT_TOKEN",
				SetupScript: "printf %s \"$CREDENTIAL_AGENT_TOKEN\" > ~/.credential-agent/token",
			},
		}},
	}
}

func TestSSHExecutorBuildRemoteAuthCatalog(t *testing.T) {
	t.Run("no agent list still yields the built-in gh spec", func(t *testing.T) {
		exec := &SSHExecutor{logger: newTestLogger()}
		catalog := exec.buildRemoteAuthCatalog()
		if _, ok := catalog.FindMethod("gh_cli_env"); !ok {
			t.Fatalf("catalog must always carry gh_cli_env, got %+v", catalog.Specs)
		}
	})

	t.Run("enabled agents contribute their methods", func(t *testing.T) {
		agent := newCredentialAgent(".credential-agent/creds.json")
		exec := &SSHExecutor{logger: newTestLogger(), agentList: agent.lister()}
		catalog := exec.buildRemoteAuthCatalog()
		if _, ok := catalog.FindMethod(agent.fileMethodID("0")); !ok {
			t.Fatalf("catalog missing the agent file method, got %+v", catalog.Specs)
		}
		if _, ok := catalog.FindMethod("agent:credential-agent:env:CREDENTIAL_AGENT_TOKEN"); !ok {
			t.Fatal("catalog missing the agent env method")
		}
	})
}

func TestSelectFileMethodsKeepsOnlySelectedFileMethods(t *testing.T) {
	agent := newCredentialAgent(".credential-agent/creds.json")
	catalog := agent.newCatalog()

	methods := selectFileMethods(catalog, []string{
		agent.fileMethodID("0"),                             // file method — kept
		"agent:credential-agent:env:CREDENTIAL_AGENT_TOKEN", // env method — dropped
		"totally-unknown",                                   // unknown — warned and dropped
	}, newTestLogger())

	if len(methods) != 1 {
		t.Fatalf("selected methods = %+v, want only the file method", methods)
	}
	if methods[0].MethodID != agent.fileMethodID("0") || methods[0].Type != authMethodTypeFiles {
		t.Fatalf("selected method = %+v", methods[0])
	}
	if got := selectFileMethods(catalog, nil, newTestLogger()); len(got) != 0 {
		t.Fatalf("empty selection = %+v, want none", got)
	}
}

func TestSSHExecutorResolveRemoteAuthHomeDir(t *testing.T) {
	t.Run("metadata override wins without probing", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		exec := &SSHExecutor{logger: newTestLogger()}
		home, err := exec.resolveRemoteAuthHomeDir(context.Background(), server.dial(t),
			&ExecutorCreateRequest{Metadata: map[string]interface{}{
				MetadataKeyRemoteAuthHome: "  /srv/home  ",
			}}, SSHRemotePlatform{})
		if err != nil {
			t.Fatalf("resolveRemoteAuthHomeDir: %v", err)
		}
		if home != "/srv/home" {
			t.Fatalf("home = %q, want the trimmed override", home)
		}
		if len(server.commands()) != 0 {
			t.Fatalf("the override must skip the probe, commands: %v", server.commands())
		}
	})

	t.Run("probes the remote through the platform shell", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOut("/Users/kandev\n") })
		exec := &SSHExecutor{logger: newTestLogger()}
		home, err := exec.resolveRemoteAuthHomeDir(context.Background(), server.dial(t),
			&ExecutorCreateRequest{}, SSHRemotePlatform{GOOS: "darwin", GOARCH: "arm64"})
		if err != nil {
			t.Fatalf("resolveRemoteAuthHomeDir: %v", err)
		}
		if home != "/Users/kandev" {
			t.Fatalf("home = %q", home)
		}
		// Darwin must probe under zsh, not bash.
		if got := server.commands(); len(got) != 1 || !strings.HasPrefix(got[0], "zsh -lc ") {
			t.Fatalf("probe command = %v, want a zsh login shell on darwin", got)
		}
	})

	t.Run("empty and failing probes are errors", func(t *testing.T) {
		blank := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOut("  \n") })
		exec := &SSHExecutor{logger: newTestLogger()}
		_, err := exec.resolveRemoteAuthHomeDir(context.Background(), blank.dial(t),
			&ExecutorCreateRequest{}, SSHRemotePlatform{})
		if err == nil || !strings.Contains(err.Error(), "empty string") {
			t.Fatalf("error = %v", err)
		}

		broken := newFakeSSHServer(t, func(string, string) sshExecResult { return sshFail("closed") })
		_, err = exec.resolveRemoteAuthHomeDir(context.Background(), broken.dial(t),
			&ExecutorCreateRequest{}, SSHRemotePlatform{})
		if err == nil || !strings.Contains(err.Error(), "resolve remote $HOME for credentials") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestSSHExecutorRunAuthSetupScripts(t *testing.T) {
	agent := newCredentialAgent(".credential-agent/creds.json")

	t.Run("runs only for env methods whose env var is present", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOK })
		exec := &SSHExecutor{logger: newTestLogger(), agentList: agent.lister()}

		req := &ExecutorCreateRequest{Env: map[string]string{
			"CREDENTIAL_AGENT_TOKEN": "trigger-value",
			envKeyGitHubToken:        "ghp-s3cr3t",
		}}
		exec.runAuthSetupScripts(context.Background(), server.dial(t), req, agent.newCatalog(),
			SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"})

		calls := server.execCalls()
		if len(calls) != 1 {
			t.Fatalf("setup script runs = %d, want 1", len(calls))
		}
		script := unwrapLoginShell(t, "bash", calls[0].Command)
		if !strings.HasPrefix(script, "set -a; . /dev/stdin; set +a\n") {
			t.Fatalf("setup script must source stdin under set -a:\n%s", script)
		}
		if !strings.Contains(script, `printf %s "$CREDENTIAL_AGENT_TOKEN"`) {
			t.Fatalf("setup script body missing:\n%s", script)
		}
		// Secrets ride on stdin, never on the remote argv, so they stay out of
		// `ps aux` / /proc/PID/cmdline while the script runs.
		if strings.Contains(calls[0].Command, "ghp-s3cr3t") {
			t.Fatalf("the secret must never reach argv:\n%s", calls[0].Command)
		}
		if calls[0].Stdin != "GITHUB_TOKEN='ghp-s3cr3t'\n" {
			t.Fatalf("stdin = %q, want the allowlisted credential as an env assignment", calls[0].Stdin)
		}
	})

	t.Run("absent env var skips the script", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOK })
		exec := &SSHExecutor{logger: newTestLogger(), agentList: agent.lister()}
		exec.runAuthSetupScripts(context.Background(), server.dial(t),
			&ExecutorCreateRequest{Env: map[string]string{}}, agent.newCatalog(), SSHRemotePlatform{})
		if len(server.commands()) != 0 {
			t.Fatalf("no setup script should run, commands: %v", server.commands())
		}
	})

	t.Run("a failing setup script is best-effort", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshFail("boom") })
		exec := &SSHExecutor{logger: newTestLogger(), agentList: agent.lister()}
		exec.runAuthSetupScripts(context.Background(), server.dial(t),
			&ExecutorCreateRequest{Env: map[string]string{"CREDENTIAL_AGENT_TOKEN": "tok"}},
			agent.newCatalog(), SSHRemotePlatform{})
		if len(server.commands()) != 1 {
			t.Fatalf("commands = %v, want the failure swallowed after one attempt", server.commands())
		}
	})

}

func TestSSHExecutorUploadCredentials(t *testing.T) {
	t.Run("no selection uploads nothing", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOK })
		exec := &SSHExecutor{logger: newTestLogger()}
		err := exec.uploadCredentials(context.Background(), server.dial(t),
			&ExecutorCreateRequest{Metadata: map[string]interface{}{}}, SSHRemotePlatform{})
		if err != nil {
			t.Fatalf("uploadCredentials: %v", err)
		}
	})

	t.Run("malformed remote_credentials is an error", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOK })
		exec := &SSHExecutor{logger: newTestLogger()}
		err := exec.uploadCredentials(context.Background(), server.dial(t),
			&ExecutorCreateRequest{Metadata: map[string]interface{}{"remote_credentials": "{"}},
			SSHRemotePlatform{})
		if err == nil || !strings.Contains(err.Error(), "failed to parse remote_credentials") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("selected file methods land under the remote home", func(t *testing.T) {
		localHome := t.TempDir()
		t.Setenv("HOME", localHome)
		if err := os.MkdirAll(filepath.Join(localHome, ".credential-agent"), 0o755); err != nil {
			t.Fatalf("seed local credential dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(localHome, ".credential-agent", "creds.json"),
			[]byte(`{"token":"local"}`), 0o600); err != nil {
			t.Fatalf("seed local credential file: %v", err)
		}

		remoteHome := t.TempDir()
		server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
			if strings.Contains(command, remoteHomeCommand) {
				return sshOut(remoteHome)
			}
			return sshOK
		})
		server.enableSFTP()

		agent := newCredentialAgent(".credential-agent/creds.json")
		exec := &SSHExecutor{logger: newTestLogger(), agentList: agent.lister()}
		req := &ExecutorCreateRequest{Metadata: map[string]interface{}{
			"remote_credentials": `["` + agent.fileMethodID("0") + `","gh_cli_token"]`,
		}}

		if err := exec.uploadCredentials(context.Background(), server.dial(t), req, SSHRemotePlatform{}); err != nil {
			t.Fatalf("uploadCredentials: %v", err)
		}

		uploaded := filepath.Join(remoteHome, ".credential-agent", "creds.json")
		data, err := os.ReadFile(uploaded)
		if err != nil {
			t.Fatalf("read uploaded credential: %v", err)
		}
		if string(data) != `{"token":"local"}` {
			t.Fatalf("uploaded credential = %q", data)
		}
	})

	t.Run("unresolvable remote home is the one hard failure", func(t *testing.T) {
		localHome := t.TempDir()
		t.Setenv("HOME", localHome)
		if err := os.MkdirAll(filepath.Join(localHome, ".credential-agent"), 0o755); err != nil {
			t.Fatalf("seed local credential dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(localHome, ".credential-agent", "creds.json"),
			[]byte("x"), 0o600); err != nil {
			t.Fatalf("seed local credential file: %v", err)
		}
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshFail("no shell") })
		agent := newCredentialAgent(".credential-agent/creds.json")
		exec := &SSHExecutor{logger: newTestLogger(), agentList: agent.lister()}
		err := exec.uploadCredentials(context.Background(), server.dial(t),
			&ExecutorCreateRequest{Metadata: map[string]interface{}{
				"remote_credentials": `["` + agent.fileMethodID("0") + `"]`,
			}}, SSHRemotePlatform{})
		if err == nil || !strings.Contains(err.Error(), "resolve remote $HOME for credentials") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestSSHFileUploaderWriteFileCreatesParents(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	server.enableSFTP()
	uploader := &sshFileUploader{client: server.dial(t)}
	target := filepath.Join(t.TempDir(), "a", "b", "c", "creds.json")

	if err := uploader.WriteFile(context.Background(), target, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "secret" {
		t.Fatalf("content = %q", data)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}

	// Writing again over the existing tree must still succeed.
	if err := uploader.WriteFile(context.Background(), target, []byte("rotated"), 0o644); err != nil {
		t.Fatalf("second WriteFile: %v", err)
	}
	data, _ = os.ReadFile(target)
	if string(data) != "rotated" {
		t.Fatalf("rotated content = %q", data)
	}
}

func TestSSHFileUploaderWriteFileFailsOnUncreatableParent(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	server.enableSFTP()
	uploader := &sshFileUploader{client: server.dial(t)}

	// A regular file cannot become a directory, so mkdir -p must fail.
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	err := uploader.WriteFile(context.Background(), filepath.Join(blocker, "nested", "creds"),
		[]byte("x"), 0o600)
	if err == nil {
		t.Fatal("expected the write to fail when the parent cannot be created")
	}
}
