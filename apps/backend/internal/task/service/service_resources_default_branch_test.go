package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestService_CreateRepositoryRejectsInvalidDefaultBranchWithoutPersistence
// covers Review round 2, finding #1 (SEC-005): default_branch reaches the
// git subprocess in internal/delivery/ancestry.go as raw, unvalidated argv
// (both as an option-injection surface and as git revision syntax), so it
// must be validated at ingestion the same way WorktreeBranchPrefix already
// is in this function.
func TestService_CreateRepositoryRejectsInvalidDefaultBranchWithoutPersistence(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	invalidBranches := []string{"-x", "--all", "main~1", "main^{commit}", "main..other"}
	for _, branch := range invalidBranches {
		t.Run(branch, func(t *testing.T) {
			_, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
				WorkspaceID:   "ws-1",
				Name:          "Invalid Repo",
				SourceType:    sourceTypeProvider,
				Provider:      "github",
				DefaultBranch: branch,
			})
			if !errors.Is(err, ErrInvalidRepositorySettings) {
				t.Fatalf("CreateRepository error = %v, want ErrInvalidRepositorySettings", err)
			}
		})
	}
	repositories, err := repo.ListRepositories(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 0 {
		t.Fatalf("invalid repository was persisted: %+v", repositories)
	}
}

// TestService_CreateRepositoryAllowsEmptyDefaultBranch confirms the
// validation added for Review round 2 finding #1 does not reject the
// legitimate "not yet known" state: repository_discovery.go's makeRepo
// leaves DefaultBranch empty when local git probing fails, and
// FindOrCreateRepository's create path already routes through here.
func TestService_CreateRepositoryAllowsEmptyDefaultBranch(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID: "ws-1",
		Name:        "Repo",
		SourceType:  sourceTypeProvider,
		Provider:    "github",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if created.DefaultBranch != "" {
		t.Fatalf("DefaultBranch = %q, want empty", created.DefaultBranch)
	}
}

// TestService_UpdateRepositoryRejectsInvalidDefaultBranchWithoutPersistence
// is UpdateRepository's twin of the CreateRepository test above.
func TestService_UpdateRepositoryRejectsInvalidDefaultBranchWithoutPersistence(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID: "ws-1",
		Name:        "Provider Repo",
		SourceType:  sourceTypeProvider,
		Provider:    "github",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	invalidBranch := "-x"
	_, err = svc.UpdateRepository(ctx, created.ID, &UpdateRepositoryRequest{DefaultBranch: &invalidBranch})
	if !errors.Is(err, ErrInvalidRepositorySettings) {
		t.Fatalf("UpdateRepository error = %v, want ErrInvalidRepositorySettings", err)
	}
	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.DefaultBranch != "" {
		t.Fatalf("invalid DefaultBranch persisted as %q", stored.DefaultBranch)
	}
}

// TestApplyRepositoryUpdates_DefaultBranchRejectsFlagLikeValue covers Review
// round 2, finding #1 (SEC-005): an unvalidated default_branch reaches
// internal/delivery/ancestry.go as raw git argv. A flag-like value must be
// rejected here, mirroring worktree.ValidateBranchPrefix's existing gate in
// this same function, and the repository struct must be left unmodified.
func TestApplyRepositoryUpdates_DefaultBranchRejectsFlagLikeValue(t *testing.T) {
	repo := &models.Repository{DefaultBranch: "main"}
	invalid := "-x"
	err := applyRepositoryUpdates(repo, &UpdateRepositoryRequest{DefaultBranch: &invalid})
	if !errors.Is(err, ErrInvalidRepositorySettings) {
		t.Fatalf("applyRepositoryUpdates error = %v, want ErrInvalidRepositorySettings", err)
	}
	if repo.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want unmodified %q", repo.DefaultBranch, "main")
	}
}

// TestApplyRepositoryUpdates_DefaultBranchRejectsRevisionSyntax covers the
// second consequence named in finding #1: a value like "main~1" is not
// flag-like, but is parsed by git as revision syntax (a parent-commit
// modifier) rather than a literal ref name, which can resolve to the wrong
// commit entirely when it reaches resolveDefaultBranchRef.
func TestApplyRepositoryUpdates_DefaultBranchRejectsRevisionSyntax(t *testing.T) {
	repo := &models.Repository{DefaultBranch: "main"}
	invalid := "main~1"
	err := applyRepositoryUpdates(repo, &UpdateRepositoryRequest{DefaultBranch: &invalid})
	if !errors.Is(err, ErrInvalidRepositorySettings) {
		t.Fatalf("applyRepositoryUpdates error = %v, want ErrInvalidRepositorySettings", err)
	}
	if repo.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want unmodified %q", repo.DefaultBranch, "main")
	}
}

// TestApplyRepositoryUpdates_DefaultBranchEmptyStringClears mirrors the
// CopyFiles empty-string-clears convention: an explicit empty pointer is a
// valid "clear the default branch" update, not a validation failure.
func TestApplyRepositoryUpdates_DefaultBranchEmptyStringClears(t *testing.T) {
	repo := &models.Repository{DefaultBranch: "main"}
	empty := ""
	if err := applyRepositoryUpdates(repo, &UpdateRepositoryRequest{DefaultBranch: &empty}); err != nil {
		t.Fatalf("applyRepositoryUpdates: %v", err)
	}
	if repo.DefaultBranch != "" {
		t.Errorf("DefaultBranch = %q, want empty string", repo.DefaultBranch)
	}
}

// TestApplyRepositoryUpdates_DefaultBranchAcceptsValidValue is the positive
// control confirming the new validation does not regress an ordinary
// update.
func TestApplyRepositoryUpdates_DefaultBranchAcceptsValidValue(t *testing.T) {
	repo := &models.Repository{DefaultBranch: "main"}
	valid := "develop"
	if err := applyRepositoryUpdates(repo, &UpdateRepositoryRequest{DefaultBranch: &valid}); err != nil {
		t.Fatalf("applyRepositoryUpdates: %v", err)
	}
	if repo.DefaultBranch != "develop" {
		t.Errorf("DefaultBranch = %q, want %q", repo.DefaultBranch, "develop")
	}
}

// TestService_FindOrCreateRepositoryRejectsInvalidDefaultBranchBackfill
// covers Review round 3, finding #1: createRepository and
// applyRepositoryUpdates validate default_branch, but FindOrCreateRepository's
// own backfill block (the third write site) did not, leaving a path to the
// same git argv (internal/delivery ancestry check, and independently the
// worktree fetch/pull FallbackBaseBranch path, which ancestry.Check does not
// cover at all) unvalidated. Unlike the LocalPath backfill above, an invalid
// default_branch must not fail the whole find-or-create call — it is a
// best-effort backfill — so this asserts the call still succeeds, the field
// is simply left unset, and nothing invalid reaches the repository row.
func TestService_FindOrCreateRepositoryRejectsInvalidDefaultBranchBackfill(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:   "ws-1",
		Name:          "owner/repo",
		SourceType:    sourceTypeProvider,
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if created.DefaultBranch != "" {
		t.Fatalf("precondition failed: DefaultBranch = %q, want empty", created.DefaultBranch)
	}

	resolved, wasCreated, err := svc.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID:   "ws-1",
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
		DefaultBranch: "-x",
	})
	if err != nil {
		t.Fatalf("FindOrCreateRepository: %v", err)
	}
	if wasCreated || resolved.ID != created.ID {
		t.Fatalf("resolved repository = %q (created=%t), want existing %q", resolved.ID, wasCreated, created.ID)
	}
	if resolved.DefaultBranch != "" {
		t.Fatalf("resolved.DefaultBranch = %q, want empty (backfill skipped)", resolved.DefaultBranch)
	}
	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.DefaultBranch != "" {
		t.Fatalf("invalid DefaultBranch backfill persisted as %q", stored.DefaultBranch)
	}
}

// TestService_CreateRepositoryRejectsGitSymbolicRefDefaultBranch covers a
// PR-review finding on this same field: "HEAD", "main/", and "a//b" all
// pass the plain securityutil.IsValidBranchName allowlist this function
// used before, but "HEAD" in particular is a git symbolic pseudo-ref, not a
// real branch name. Persisting it as default_branch reaches the worktree
// FallbackBaseBranch path (internal/worktree), which resolves it via a
// local rev-parse fallback to whatever commit the checkout's HEAD currently
// points at — silently the wrong revision, not a resolution failure. This
// asserts the ingestion-time reject, mirroring the existing flag-like/
// revision-syntax coverage above.
func TestService_CreateRepositoryRejectsGitSymbolicRefDefaultBranch(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	invalidBranches := []string{"HEAD", "ORIG_HEAD", "FETCH_HEAD", "MERGE_HEAD", "main/", "a//b"}
	for _, branch := range invalidBranches {
		t.Run(branch, func(t *testing.T) {
			_, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
				WorkspaceID:   "ws-1",
				Name:          "Invalid Repo " + branch,
				SourceType:    sourceTypeProvider,
				Provider:      "github",
				DefaultBranch: branch,
			})
			if !errors.Is(err, ErrInvalidRepositorySettings) {
				t.Fatalf("CreateRepository error = %v, want ErrInvalidRepositorySettings", err)
			}
		})
	}
	repositories, err := repo.ListRepositories(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 0 {
		t.Fatalf("invalid repository was persisted: %+v", repositories)
	}
}

// TestApplyRepositoryUpdates_DefaultBranchRejectsGitSymbolicRef is
// applyRepositoryUpdates' twin of the CreateRepository test above.
func TestApplyRepositoryUpdates_DefaultBranchRejectsGitSymbolicRef(t *testing.T) {
	repo := &models.Repository{DefaultBranch: "main"}
	invalid := "HEAD"
	err := applyRepositoryUpdates(repo, &UpdateRepositoryRequest{DefaultBranch: &invalid})
	if !errors.Is(err, ErrInvalidRepositorySettings) {
		t.Fatalf("applyRepositoryUpdates error = %v, want ErrInvalidRepositorySettings", err)
	}
	if repo.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want unmodified %q", repo.DefaultBranch, "main")
	}
}

// TestService_FindOrCreateRepositoryBackfillRejectsGitSymbolicRef is the
// FindOrCreateRepository backfill's twin of the same coverage, mirroring
// TestService_FindOrCreateRepositoryRejectsInvalidDefaultBranchBackfill's
// "best-effort backfill, call still succeeds" shape.
func TestService_FindOrCreateRepositoryBackfillRejectsGitSymbolicRef(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:   "ws-1",
		Name:          "owner/repo",
		SourceType:    sourceTypeProvider,
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	resolved, wasCreated, err := svc.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID:   "ws-1",
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
		DefaultBranch: "HEAD",
	})
	if err != nil {
		t.Fatalf("FindOrCreateRepository: %v", err)
	}
	if wasCreated || resolved.ID != created.ID {
		t.Fatalf("resolved repository = %q (created=%t), want existing %q", resolved.ID, wasCreated, created.ID)
	}
	if resolved.DefaultBranch != "" {
		t.Fatalf("resolved.DefaultBranch = %q, want empty (backfill skipped)", resolved.DefaultBranch)
	}
	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.DefaultBranch != "" {
		t.Fatalf("invalid DefaultBranch backfill persisted as %q", stored.DefaultBranch)
	}
}

// TestService_FindOrCreateRepositoryBackfillsValidDefaultBranch is the
// positive control for the test above: a legitimate default_branch value
// still backfills onto a previously-empty row.
func TestService_FindOrCreateRepositoryBackfillsValidDefaultBranch(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:   "ws-1",
		Name:          "owner/repo",
		SourceType:    sourceTypeProvider,
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	resolved, _, err := svc.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID:   "ws-1",
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("FindOrCreateRepository: %v", err)
	}
	if resolved.DefaultBranch != "main" {
		t.Fatalf("resolved.DefaultBranch = %q, want %q", resolved.DefaultBranch, "main")
	}
	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.DefaultBranch != "main" {
		t.Fatalf("stored.DefaultBranch = %q, want %q", stored.DefaultBranch, "main")
	}
}
