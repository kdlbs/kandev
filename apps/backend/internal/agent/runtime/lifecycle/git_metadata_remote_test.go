package lifecycle

import (
	"encoding/json"
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
		if err == nil || !strings.Contains(err.Error(), "multiple repository") {
			t.Fatalf("validateRemoteGitMetadataRequest() error = %v, want multi-repository rejection", err)
		}
	})

	t.Run("legacy session configuration is rejected before remote launch", func(t *testing.T) {
		err := validateRemoteGitMetadataRequest(&ExecutorCreateRequest{
			AgentConfig: agents.NewCodexACP(),
			Env:         map[string]string{"CODEX_CONFIG": `{"sandbox_mode":"workspace-write"}`},
		})
		if err == nil || !strings.Contains(err.Error(), "legacy Codex sandbox") {
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
