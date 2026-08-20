package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func (r *gitMetadataAttestingExecutor) PrepareGitMetadataProjection(_ context.Context, _ []*worktree.GitMetadataProjection) error {
	r.attested = true
	return r.attestationError
}

func (r *gitMetadataAttestingExecutor) CreateInstance(_ context.Context, _ *ExecutorCreateRequest) (*ExecutorInstance, error) {
	r.created = true
	return &ExecutorInstance{}, nil
}
