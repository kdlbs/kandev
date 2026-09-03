package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/worktree"
)

func TestInstallAttestedCloneGitMetadataPolicyAttestsBeforeRendering(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workspace/attest-git-metadata" {
			t.Fatalf("attestation path = %q", r.URL.Path)
		}
		called = true
		_, _ = w.Write([]byte(`{"attested":true,"checkouts":[{"checkout_path":"/executor/workspace","git_dir":"/executor/workspace/.git"},{"checkout_path":"/executor/workspace/second-main","git_dir":"/executor/workspace/second-main/.git"}]}`))
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	req := &ExecutorCreateRequest{
		GitMetadataRequirement: cloneGitMetadataRequirement(true),
		AgentConfig:            agents.NewCodexACP(),
		Env:                    map[string]string{"CODEX_CONFIG": `{}`},
	}
	instance := &ExecutorInstance{
		Client:               agentctl.NewClient(parsed.Hostname(), port, log),
		WorkspacePath:        "/executor/workspace",
		WorkspaceSourceRoots: []string{"/executor/workspace", "/executor/workspace/second-main"},
	}

	if err := installAttestedCloneGitMetadataPolicy(context.Background(), req, instance); err != nil {
		t.Fatalf("installAttestedCloneGitMetadataPolicy: %v", err)
	}
	if !called {
		t.Fatal("agentctl checkout attestation was not called")
	}
	if len(req.GitMetadataProjections) != 0 {
		t.Fatalf("clone launch synthesized host projections: %#v", req.GitMetadataProjections)
	}
	policyEnv := instance.Metadata["runtime_env"].(map[string]string)
	if !strings.Contains(req.Env["CODEX_CONFIG"], "/executor/workspace/.git") {
		t.Fatalf("launch request did not receive attested clone policy: %s", req.Env["CODEX_CONFIG"])
	}
	if strings.Contains(policyEnv["CODEX_CONFIG"], "/host/") {
		t.Fatalf("rendered clone policy leaked host path: %s", policyEnv["CODEX_CONFIG"])
	}
	for _, gitDir := range []string{"/executor/workspace/.git", "/executor/workspace/second-main/.git"} {
		if !strings.Contains(policyEnv["CODEX_CONFIG"], gitDir) {
			t.Fatalf("rendered clone policy lacks attested git dir %q: %s", gitDir, policyEnv["CODEX_CONFIG"])
		}
	}
}

func TestInstallAttestedCloneGitMetadataPolicyFailsClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"workspace Git metadata validation failed"}`))
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	req := &ExecutorCreateRequest{
		GitMetadataRequirement: cloneGitMetadataRequirement(true),
		AgentConfig:            agents.NewCodexACP(),
		Env:                    map[string]string{"CODEX_CONFIG": `{}`},
	}
	instance := &ExecutorInstance{Client: agentctl.NewClient(parsed.Hostname(), port, log), WorkspacePath: "/executor/workspace"}

	err = installAttestedCloneGitMetadataPolicy(context.Background(), req, instance)
	if err == nil || !strings.Contains(err.Error(), "start a new session") {
		t.Fatalf("installAttestedCloneGitMetadataPolicy error = %v, want fail-closed recovery", err)
	}
	if instance.Metadata != nil {
		t.Fatalf("failed attestation installed runtime metadata: %#v", instance.Metadata)
	}
}

func TestInstallAttestedCloneGitMetadataPolicyRejectsForeignFinalCheckout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"attested":true,"checkouts":[{"checkout_path":"/executor/workspace","git_dir":"/executor/workspace/.git"},{"checkout_path":"/foreign/repository","git_dir":"/foreign/repository/.git"}]}`))
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	req := &ExecutorCreateRequest{
		GitMetadataRequirement: cloneGitMetadataRequirement(true),
		AgentConfig:            agents.NewCodexACP(),
		Env:                    map[string]string{"CODEX_CONFIG": `{}`},
	}
	instance := &ExecutorInstance{
		Client:               agentctl.NewClient(parsed.Hostname(), port, log),
		WorkspacePath:        "/executor/workspace",
		WorkspaceSourceRoots: []string{"/executor/workspace", "/executor/workspace/second-main"},
	}

	err = installAttestedCloneGitMetadataPolicy(context.Background(), req, instance)
	if err == nil || !strings.Contains(err.Error(), "start a new session") {
		t.Fatalf("installAttestedCloneGitMetadataPolicy error = %v, want fail-closed recovery", err)
	}
	if instance.Metadata != nil || req.Env["CODEX_CONFIG"] != `{}` {
		t.Fatalf("foreign checkout produced clone policy: metadata=%#v config=%s", instance.Metadata, req.Env["CODEX_CONFIG"])
	}
}

func TestInstallAttestedCloneGitMetadataPolicyRejectsReorderedFinalCheckouts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"attested":true,"checkouts":[{"checkout_path":"/executor/workspace/second-main","git_dir":"/executor/workspace/second-main/.git"},{"checkout_path":"/executor/workspace","git_dir":"/executor/workspace/.git"}]}`))
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	req := &ExecutorCreateRequest{
		GitMetadataRequirement: cloneGitMetadataRequirement(true),
		AgentConfig:            agents.NewCodexACP(),
		Env:                    map[string]string{"CODEX_CONFIG": `{}`},
	}
	instance := &ExecutorInstance{
		Client:               agentctl.NewClient(parsed.Hostname(), port, log),
		WorkspacePath:        "/executor/workspace",
		WorkspaceSourceRoots: []string{"/executor/workspace", "/executor/workspace/second-main"},
	}

	err = installAttestedCloneGitMetadataPolicy(context.Background(), req, instance)
	if err == nil || !strings.Contains(err.Error(), "start a new session") {
		t.Fatalf("installAttestedCloneGitMetadataPolicy error = %v, want reordered attestation rejection", err)
	}
	if instance.Metadata != nil || req.Env["CODEX_CONFIG"] != `{}` {
		t.Fatalf("reordered checkout produced clone policy: metadata=%#v config=%s", instance.Metadata, req.Env["CODEX_CONFIG"])
	}
}

func TestRemoteRegularGitMetadataFromAttestationsRejectsIncompleteOrUnexpectedOrderedResponses(t *testing.T) {
	expected := []string{"/executor/workspace", "/executor/workspace/second-main"}
	for _, testCase := range []struct {
		name     string
		approved []agentctl.GitMetadataAttestation
	}{
		{name: "duplicate", approved: []agentctl.GitMetadataAttestation{{CheckoutPath: expected[0], GitDir: expected[0] + "/.git"}, {CheckoutPath: expected[0], GitDir: expected[0] + "/.git"}}},
		{name: "missing", approved: []agentctl.GitMetadataAttestation{{CheckoutPath: expected[0], GitDir: expected[0] + "/.git"}}},
		{name: "unexpected", approved: []agentctl.GitMetadataAttestation{{CheckoutPath: expected[0], GitDir: expected[0] + "/.git"}, {CheckoutPath: "/foreign/repository", GitDir: "/foreign/repository/.git"}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := remoteRegularGitMetadataFromAttestations(testCase.approved, expected); err == nil {
				t.Fatalf("remoteRegularGitMetadataFromAttestations(%s) succeeded", testCase.name)
			}
		})
	}
}

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
	profile := config["permissions"].(map[string]any)[gitMetadataPolicyName].(map[string]any)
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
	rules := config["permissions"].(map[string]any)[gitMetadataPolicyName].(map[string]any)["filesystem"].(map[string]any)
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
		if err == nil || !strings.Contains(err.Error(), gitMetadataProjectionUnsupported) {
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

func TestRemoteRegularGitMetadataProbeRejectsSymlinkedState(t *testing.T) {
	script := remoteRegularGitMetadataProbeScript("/remote/task")
	for _, want := range []string{
		"[ \"$gitdir\" = \"$workspace/.git\" ]",
		"[ ! -L \"$gitdir\" ]",
		"[ ! -L \"$gitdir/objects\" ]",
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
	if err := os.WriteFile(filepath.Join(workspace, "tracked"), []byte("updated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runContainerGit(t, workspace, "add", "tracked")
	runContainerGit(t, workspace, "commit", "-m", "attested clone mutation")
	runContainerGit(t, workspace, "update-ref", "refs/heads/attested-lock-coverage", "HEAD")
	if _, err := os.Stat(filepath.Join(workspace, ".git", "refs", "heads", "attested-lock-coverage")); err != nil {
		t.Fatalf("attested clone ref update did not reach regular .git: %v", err)
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

// TestRemoteRegularGitMetadataProbeRejectsSymlinkedRefsOnDetachedHead guards
// the refs/logs symlink checks against being skipped when HEAD is detached.
// The checks previously lived inside `if [ -n "$ref" ]`, which is false for a
// detached HEAD (symbolic-ref fails and ref becomes empty), so a symlinked
// .git/refs directory would have passed attestation and let write access
// redirect Git ref/log writes outside the task checkout.
func TestRemoteRegularGitMetadataProbeRejectsSymlinkedRefsOnDetachedHead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on windows")
	}
	workspace := t.TempDir()
	runContainerGit(t, "", "init", "-b", "main", workspace)
	runContainerGit(t, workspace, "config", "user.email", "test@example.com")
	runContainerGit(t, workspace, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(workspace, "tracked"), []byte("initial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runContainerGit(t, workspace, "add", "tracked")
	runContainerGit(t, workspace, "commit", "-m", "initial")
	runContainerGit(t, workspace, "checkout", "--detach", "HEAD")

	if _, err := runRemoteRegularGitMetadataProbe(t, workspace); err != nil {
		t.Fatalf("remote probe on a clean detached-HEAD checkout: %v", err)
	}

	realRefs := filepath.Join(workspace, ".git", "refs")
	elsewhere := t.TempDir()
	if err := os.RemoveAll(realRefs); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, realRefs); err != nil {
		t.Fatal(err)
	}

	if _, err := runRemoteRegularGitMetadataProbe(t, workspace); err == nil {
		t.Fatal("remote probe accepted a symlinked refs directory on a detached-HEAD checkout")
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
	if !shouldReplaceSpriteAgentInstance(&ExecutorCreateRequest{GitMetadataRequirement: GitMetadataRequirement{Mode: gitMetadataRequirementMutableClone}}) {
		t.Fatal("clone Git metadata policy must replace stale Sprite child")
	}
}
