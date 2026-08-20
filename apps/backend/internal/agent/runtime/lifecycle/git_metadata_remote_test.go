package lifecycle

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/worktree"
)

func TestRemoteRegularGitMetadataPolicyOnlyWritesTaskGitDir(t *testing.T) {
	req := &ExecutorCreateRequest{AgentConfig: agents.NewCodexACP()}
	metadata := remoteRegularGitMetadata{
		CheckoutPath: "/remote/tasks/task-1",
		GitDir:       "/remote/tasks/task-1/.git",
		CurrentRef:   "refs/heads/task-1",
	}
	if err := prepareRemoteRegularGitMetadataPolicy(req, metadata); err != nil {
		t.Fatalf("prepareRemoteRegularGitMetadataPolicy() error = %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal([]byte(req.Env["CODEX_CONFIG"]), &config); err != nil {
		t.Fatalf("decode CODEX_CONFIG: %v", err)
	}
	profile := config["permissions"].(map[string]any)[codexGitMetadataPolicyName].(map[string]any)
	rules := profile["filesystem"].(map[string]any)
	if got := rules[metadata.GitDir]; got != "write" {
		t.Fatalf("remote GitDir permission = %#v, want write", got)
	}
	for path, access := range rules {
		if path == ":minimal" {
			continue
		}
		if path != metadata.GitDir || access != "write" {
			t.Fatalf("remote policy unexpectedly grants %q=%#v", path, access)
		}
	}
}

func TestRemoteRegularGitMetadataPolicyWritesOnlyAttestedCloneDirectories(t *testing.T) {
	req := &ExecutorCreateRequest{AgentConfig: agents.NewCodexACP()}
	metadata := []remoteRegularGitMetadata{
		{CheckoutPath: "/workspace", GitDir: "/workspace/.git"},
		{CheckoutPath: "/workspace/frontend-main", GitDir: "/workspace/frontend-main/.git"},
	}
	if err := prepareRemoteRegularGitMetadataPolicy(req, metadata...); err != nil {
		t.Fatalf("prepareRemoteRegularGitMetadataPolicy() error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(req.Env["CODEX_CONFIG"]), &config); err != nil {
		t.Fatalf("decode CODEX_CONFIG: %v", err)
	}
	rules := config["permissions"].(map[string]any)[codexGitMetadataPolicyName].(map[string]any)["filesystem"].(map[string]any)
	for _, item := range metadata {
		if rules[item.GitDir] != "write" {
			t.Fatalf("GitDir %q permission = %#v, want write", item.GitDir, rules[item.GitDir])
		}
	}
	for path := range rules {
		if path != ":minimal" && path != metadata[0].GitDir && path != metadata[1].GitDir {
			t.Fatalf("unexpected filesystem policy path %q", path)
		}
	}
}

func TestRemoteGitMetadataRequestFailsClosed(t *testing.T) {
	t.Run("agent has no renderer", func(t *testing.T) {
		err := validateRemoteGitMetadataRequest(&ExecutorCreateRequest{AgentConfig: agents.NewClaudeACP()})
		if err == nil || !strings.Contains(err.Error(), "filesystem policy") {
			t.Fatalf("validateRemoteGitMetadataRequest() error = %v, want renderer rejection", err)
		}
	})

	t.Run("multiple host projections cannot authorize remote paths", func(t *testing.T) {
		err := validateRemoteGitMetadataRequest(&ExecutorCreateRequest{
			AgentConfig: agents.NewCodexACP(),
			GitMetadataProjections: []*worktree.GitMetadataProjection{
				{}, {},
			},
		})
		if err == nil || !strings.Contains(err.Error(), "multi-repository") {
			t.Fatalf("validateRemoteGitMetadataRequest() error = %v, want multi-repository rejection", err)
		}
	})

	t.Run("legacy session configuration is rejected before remote launch", func(t *testing.T) {
		err := validateRemoteGitMetadataRequest(&ExecutorCreateRequest{
			AgentConfig: agents.NewCodexACP(),
			Env:         map[string]string{"CODEX_CONFIG": `{"sandbox_mode":"workspace-write"}`},
		})
		if err == nil || !strings.Contains(err.Error(), "compatible Codex ACP") {
			t.Fatalf("validateRemoteGitMetadataRequest() error = %v, want legacy sandbox rejection", err)
		}
	})
}

func TestRemoteRegularGitMetadataParserRejectsEscapes(t *testing.T) {
	valid := "/remote/task\n/remote/task/.git\nrefs/heads/feature/test\n"
	metadata, err := parseRemoteRegularGitMetadata(valid)
	if err != nil {
		t.Fatalf("parseRemoteRegularGitMetadata(valid): %v", err)
	}
	if metadata.GitDir != "/remote/task/.git" {
		t.Fatalf("GitDir = %q", metadata.GitDir)
	}

	for _, output := range []string{
		"/remote/task\n/other/.git\nrefs/heads/main\n",
		"/remote/task\n/remote/task/.git\nrefs/heads/../other\n",
		"/remote/task\n/remote/task/.git\nrefs/tags/v1\n",
		"/remote/task\n/remote/task/.git\nrefs/heads/main\nextra\n",
	} {
		if _, err := parseRemoteRegularGitMetadata(output); err == nil {
			t.Fatalf("parseRemoteRegularGitMetadata(%q) succeeded, want rejection", output)
		}
	}
}

func TestRemoteRegularGitMetadataProbeRejectsLegacyAndSymlinkedState(t *testing.T) {
	script := remoteRegularGitMetadataProbeScript("/remote/task")
	for _, want := range []string{
		"[ \"$gitdir\" = \"$workspace/.git\" ]",
		"[ ! -L \"$gitdir\" ]",
		"[ ! -L \"$gitdir/objects\" ]",
		"sandbox_mode|sandbox_workspace_write",
		"[ ! -L \"$gitdir/refs\" ]",
		"[ ! -L \"$gitdir/logs\" ]",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("probe script missing %q:\n%s", want, script)
		}
	}
}

func TestRemoteRegularGitMetadataProbeRunsAgainstRealCheckout(t *testing.T) {
	workspace := t.TempDir()
	runContainerGit(t, "", "init", "-b", "main", workspace)
	runContainerGit(t, workspace, "config", "user.email", "test@example.com")
	runContainerGit(t, workspace, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(workspace, "tracked"), []byte("initial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runContainerGit(t, workspace, "add", "tracked")
	runContainerGit(t, workspace, "commit", "-m", "initial")

	output, err := runRemoteRegularGitMetadataProbe(t, workspace)
	if err != nil {
		t.Fatalf("remote probe on regular checkout: %v", err)
	}
	metadata, err := parseRemoteRegularGitMetadata(output)
	if err != nil {
		t.Fatalf("parse probe output: %v", err)
	}
	if metadata.GitDir != filepath.Join(workspace, ".git") || metadata.CurrentRef != "refs/heads/main" {
		t.Fatalf("metadata = %+v, want task regular checkout", metadata)
	}

	if err := os.Mkdir(filepath.Join(workspace, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".codex", "config.toml"), []byte(`sandbox_mode = "workspace-write"`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runRemoteRegularGitMetadataProbe(t, workspace); err == nil {
		t.Fatal("remote probe accepted a legacy Codex sandbox setting")
	}
	if err := os.Remove(filepath.Join(workspace, ".codex", "config.toml")); err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Rename(filepath.Join(workspace, ".git"), filepath.Join(workspace, ".git-real")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(workspace, ".git-real"), filepath.Join(workspace, ".git")); err != nil {
		t.Fatal(err)
	}
	if _, err := runRemoteRegularGitMetadataProbe(t, workspace); err == nil {
		t.Fatal("remote probe accepted a symlinked .git directory")
	}
}

func runRemoteRegularGitMetadataProbe(t *testing.T, workspace string) (string, error) {
	t.Helper()
	home := t.TempDir()
	cmd := exec.Command("sh", "-c", remoteRegularGitMetadataProbeScript(workspace))
	cmd.Env = append(os.Environ(), "HOME="+home, "CODEX_HOME="+filepath.Join(home, ".codex"))
	output, err := cmd.Output()
	return string(output), err
}

func TestSSHRemoteAgentEnvForwardsGeneratedFilesystemPolicy(t *testing.T) {
	env := sshRemoteAgentEnv(&ExecutorCreateRequest{
		AgentConfig: agents.NewCodexACP(),
		Env: map[string]string{
			"CODEX_CONFIG": `{"default_permissions":"kandev_task_git_metadata"}`,
			"UNRELATED":    "not-forwarded",
		},
	})
	if got := env["CODEX_CONFIG"]; got == "" {
		t.Fatalf("CODEX_CONFIG = %q, want generated policy forwarded", got)
	}
	if _, leaked := env["UNRELATED"]; leaked {
		t.Fatalf("remote environment leaked unrelated key: %+v", env)
	}
}

func TestSpriteReconnectReplacesChildWhenGitPolicyChanges(t *testing.T) {
	if shouldReplaceSpriteAgentInstance(&ExecutorCreateRequest{}) {
		t.Fatal("empty request should reuse healthy Sprite child")
	}
	if !shouldReplaceSpriteAgentInstance(&ExecutorCreateRequest{GitMetadataProjections: []*worktree.GitMetadataProjection{{}}}) {
		t.Fatal("Git metadata projection must replace stale Sprite child")
	}
	if !shouldReplaceSpriteAgentInstance(&ExecutorCreateRequest{RequiresCloneGitMetadataPolicy: true}) {
		t.Fatal("clone Git metadata policy must replace stale Sprite child")
	}
}
