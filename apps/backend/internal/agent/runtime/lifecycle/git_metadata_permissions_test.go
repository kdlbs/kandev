package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/worktree"
)

func TestCreateLaunchInstanceRejectsUnsupportedGitMetadataProjection(t *testing.T) {
	runtime := &gitMetadataRecordingExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
	req := &ExecutorCreateRequest{
		GitMetadataProjections: []*worktree.GitMetadataProjection{newLinkedGitMetadataProjection(t)},
	}

	_, err := createLaunchInstance(context.Background(), runtime, req)
	if err == nil || !strings.Contains(err.Error(), gitMetadataProjectionUnsupported) {
		t.Fatalf("createLaunchInstance error = %v, want %q", err, gitMetadataProjectionUnsupported)
	}
	if runtime.created {
		t.Fatal("unsupported runtime created an instance")
	}
}

func TestCreateLaunchInstanceSanitizesInvalidGitMetadataProjection(t *testing.T) {
	runtime := &gitMetadataRecordingExecutor{MockExecutor: MockExecutor{name: executor.NameDocker}}
	req := &ExecutorCreateRequest{
		GitMetadataProjections: []*worktree.GitMetadataProjection{{CheckoutPath: "/host/private/source"}},
	}

	_, err := createLaunchInstance(context.Background(), runtime, req)
	if err == nil || !strings.Contains(err.Error(), gitMetadataProjectionInvalid) {
		t.Fatalf("createLaunchInstance error = %v, want %q", err, gitMetadataProjectionInvalid)
	}
	if strings.Contains(err.Error(), "/host/private/source") {
		t.Fatalf("invalid projection leaked a host path: %v", err)
	}
	if runtime.created {
		t.Fatal("invalid projection created an instance")
	}
}

func TestCreateLaunchInstanceRequiresGitMetadataAttestation(t *testing.T) {
	runtime := &gitMetadataAttestingExecutor{
		MockExecutor:     MockExecutor{name: executor.NameDocker},
		attestationError: errors.New("mount policy unavailable"),
	}
	req := &ExecutorCreateRequest{GitMetadataProjections: []*worktree.GitMetadataProjection{newLinkedGitMetadataProjection(t)}}

	_, err := createLaunchInstance(context.Background(), runtime, req)
	if err == nil || !strings.Contains(err.Error(), gitMetadataProjectionUnsupported) {
		t.Fatalf("createLaunchInstance error = %v, want failed attestation", err)
	}
	if !runtime.attested || runtime.created {
		t.Fatalf("failed attestation attested=%t created=%t, want attested but not created", runtime.attested, runtime.created)
	}
}

func TestStandaloneGitMetadataPreflightRendersCodexPolicy(t *testing.T) {
	projection := newLinkedGitMetadataProjection(t)
	codexHome := t.TempDir()
	runtime := &StandaloneExecutor{}
	req := &ExecutorCreateRequest{
		WorkspacePath:          projection.CheckoutPath,
		GitMetadataProjections: []*worktree.GitMetadataProjection{projection},
		AgentConfig:            agents.NewCodexACP(),
		Env: map[string]string{
			"CODEX_HOME":   codexHome,
			"CODEX_CONFIG": `{"approval_policy":"never","permissions":{"existing":{"network":{"enabled":true}}}}`,
		},
	}

	if err := preflightGitMetadataProjection(context.Background(), runtime, req); err != nil {
		t.Fatalf("preflightGitMetadataProjection() error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(req.Env["CODEX_CONFIG"]), &config); err != nil {
		t.Fatalf("decode CODEX_CONFIG: %v", err)
	}
	if got := config["approval_policy"]; got != "never" {
		t.Fatalf("approval policy = %#v, want preserved value", got)
	}
	permissions, ok := config["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions = %#v, want object", config["permissions"])
	}
	if _, ok := permissions["existing"]; !ok {
		t.Fatalf("existing policy lost: %#v", permissions)
	}
	profile, ok := permissions[codexGitMetadataPolicyName].(map[string]any)
	if !ok {
		t.Fatalf("task metadata policy missing: %#v", permissions)
	}
	rules := profile["filesystem"].(map[string]any)
	if got := rules[projection.CommonDir]; got != "read" {
		t.Fatalf("common Git root = %#v, want read", got)
	}
	if got := rules[projection.WorktreesDir]; got != "deny" {
		t.Fatalf("common worktrees directory = %#v, want deny", got)
	}
	if got := rules[projection.GitDir]; got != "write" {
		t.Fatalf("owned gitdir = %#v, want write", got)
	}
	if got := rules[projection.ObjectDir]; got != "write" {
		t.Fatalf("object directory = %#v, want write", got)
	}
}

func TestStandaloneGitMetadataPreflightRejectsLegacyCodexSandbox(t *testing.T) {
	projection := newLinkedGitMetadataProjection(t)
	codexHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(`sandbox_mode = "workspace-write"`), 0o600); err != nil {
		t.Fatal(err)
	}
	req := &ExecutorCreateRequest{
		WorkspacePath:          projection.CheckoutPath,
		GitMetadataProjections: []*worktree.GitMetadataProjection{projection},
		AgentConfig:            agents.NewCodexACP(),
		Env:                    map[string]string{"CODEX_HOME": codexHome},
	}

	err := preflightGitMetadataProjection(context.Background(), &StandaloneExecutor{}, req)
	if err == nil || !strings.Contains(err.Error(), gitMetadataProjectionUnsupported) {
		t.Fatalf("preflightGitMetadataProjection() error = %v, want %q", err, gitMetadataProjectionUnsupported)
	}
	if strings.Contains(err.Error(), codexHome) {
		t.Fatalf("legacy sandbox error leaked config path: %v", err)
	}
}

func TestStandaloneGitMetadataPreflightRejectsAgentWithoutFilesystemPolicy(t *testing.T) {
	projection := newLinkedGitMetadataProjection(t)
	req := &ExecutorCreateRequest{
		WorkspacePath:          projection.CheckoutPath,
		GitMetadataProjections: []*worktree.GitMetadataProjection{projection},
		AgentConfig:            agents.NewClaudeACP(),
	}

	err := preflightGitMetadataProjection(context.Background(), &StandaloneExecutor{}, req)
	if err == nil || !strings.Contains(err.Error(), gitMetadataProjectionUnsupported) {
		t.Fatalf("preflightGitMetadataProjection() error = %v, want %q", err, gitMetadataProjectionUnsupported)
	}
}

func newLinkedGitMetadataProjection(t *testing.T) *worktree.GitMetadataProjection {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "source")
	runContainerGit(t, "", "init", "-b", "main", repo)
	runContainerGit(t, repo, "config", "user.email", "test@example.com")
	runContainerGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "file"), []byte("initial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runContainerGit(t, repo, "add", "file")
	runContainerGit(t, repo, "commit", "-m", "initial")
	checkout := filepath.Join(t.TempDir(), "checkout")
	runContainerGit(t, repo, "worktree", "add", "-b", "task", checkout)
	projection, err := worktree.ResolveGitMetadata(checkout)
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

type gitMetadataRecordingExecutor struct {
	MockExecutor
	created bool
}

func (r *gitMetadataRecordingExecutor) CreateInstance(_ context.Context, _ *ExecutorCreateRequest) (*ExecutorInstance, error) {
	r.created = true
	return &ExecutorInstance{}, nil
}

type gitMetadataAttestingExecutor struct {
	MockExecutor
	attested         bool
	attestationError error
	created          bool
}

func (r *gitMetadataAttestingExecutor) PrepareGitMetadataProjection(_ context.Context, _ *ExecutorCreateRequest) error {
	r.attested = true
	return r.attestationError
}

func (r *gitMetadataAttestingExecutor) CreateInstance(_ context.Context, _ *ExecutorCreateRequest) (*ExecutorInstance, error) {
	r.created = true
	return &ExecutorInstance{}, nil
}
