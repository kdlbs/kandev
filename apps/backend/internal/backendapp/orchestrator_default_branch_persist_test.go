package backendapp

import (
	"context"
	"expvar"
	"os"
	"path/filepath"
	"testing"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

// readDeliveryExpvarInt reads a package-level expvar.Int published by
// internal/delivery/metrics.go, mirroring internal/delivery's own
// readExpvarInt test helper (sweep_test.go). The value is process-global
// and never resets between tests, so callers must compare a before/after
// delta rather than an absolute value.
func readDeliveryExpvarInt(t *testing.T, name string) int64 {
	t.Helper()
	v := expvar.Get(name)
	if v == nil {
		t.Fatalf("expvar %q not published", name)
	}
	iv, ok := v.(*expvar.Int)
	if !ok {
		t.Fatalf("expvar %q is not an *expvar.Int", name)
	}
	return iv.Value()
}

// TestResolveReviewBaseBranchRecordsPersistFailureForDetectedBranch covers
// Review round 3, finding #4: persistDetectedDefaultBranch used to log a
// bare Warn and otherwise swallow a failure to persist a newly detected
// default branch, leaving repositories.default_branch permanently empty
// (and the repository's delivery-ledger classification permanently
// default_branch_unknown, per spec "Degraded evaluation") with no way for
// an operator to notice. "_weird" is a branch name real git accepts (it
// only forbids a leading "-", not "_") but IsValidBranchName's allowlist
// regex rejects (it requires a leading alnum), so UpdateRepository rejects
// the persist attempt with ErrInvalidRepositorySettings — exactly the "git
// allows it, our validator doesn't" gap the finding describes, as opposed
// to a transient DB error.
func TestResolveReviewBaseBranchRecordsPersistFailureForDetectedBranch(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspace, err := harness.taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
		Name: "Workspace",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	repoPath := t.TempDir()
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "remotes", "origin"), 0o755); err != nil {
		t.Fatalf("mkdir origin refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/x\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(gitDir, "refs", "remotes", "origin", "HEAD"),
		[]byte("ref: refs/remotes/origin/_weird\n"),
		0o644,
	); err != nil {
		t.Fatalf("write origin HEAD: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(gitDir, "refs", "remotes", "origin", "_weird"),
		[]byte("0000000\n"),
		0o644,
	); err != nil {
		t.Fatalf("write _weird ref: %v", err)
	}
	repo, err := harness.taskSvc.CreateRepository(ctx, &taskservice.CreateRepositoryRequest{
		WorkspaceID:   workspace.ID,
		Name:          "owner/repo",
		SourceType:    "provider",
		LocalPath:     repoPath,
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if repo.DefaultBranch != "" {
		t.Fatalf("precondition failed: DefaultBranch = %q, want empty", repo.DefaultBranch)
	}

	before := readDeliveryExpvarInt(t, "delivery_ledger_default_branch_persist_errors_total")

	adapter := &repositoryResolverAdapter{
		taskSvc: harness.taskSvc,
		logger:  newTestLogger(),
	}
	got := adapter.resolveReviewBaseBranch(ctx, repo, repoPath, "")
	if got != "_weird" {
		t.Fatalf("resolveReviewBaseBranch = %q, want %q (the detected value must still be used for this operation "+
			"even though persistence is rejected)", got, "_weird")
	}

	after := readDeliveryExpvarInt(t, "delivery_ledger_default_branch_persist_errors_total")
	if after != before+1 {
		t.Fatalf("delivery_ledger_default_branch_persist_errors_total = %d, want %d "+
			"(a rejected persist must be recorded, not silently swallowed)", after, before+1)
	}

	stored, err := harness.taskSvc.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.DefaultBranch != "" {
		t.Fatalf("stored default_branch = %q, want empty (persistence must actually have been rejected)",
			stored.DefaultBranch)
	}
}
