package service

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestDedupeRepositoriesByIdentity exercises the pure winner-selection logic
// directly with explicit CreatedAt values, so the assertion is deterministic
// and independent of wall-clock/DB timestamp precision. Covers: local-path
// duplicates collapse to the earliest-created row, provider-identity
// duplicates collapse the same way, an unrelated repository of each kind is
// preserved, and two placeholder rows sharing neither identity are never
// merged into each other.
func TestDedupeRepositoriesByIdentity(t *testing.T) {
	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)

	localWinner := &models.Repository{ID: "repo-a-first", LocalPath: "/repos/dup", CreatedAt: older}
	localLoser := &models.Repository{ID: "repo-a-second", LocalPath: "/repos/dup", CreatedAt: newer}
	localDistinct := &models.Repository{ID: "repo-b", LocalPath: "/repos/distinct", CreatedAt: newer}
	providerWinner := &models.Repository{
		ID: "repo-gh-first", Provider: "github", ProviderHost: githubProviderHost,
		ProviderOwner: "kdlbs", ProviderName: "kandev", CreatedAt: older,
	}
	providerLoser := &models.Repository{
		ID: "repo-gh-second", Provider: "github", ProviderHost: githubProviderHost,
		ProviderOwner: "kdlbs", ProviderName: "kandev", CreatedAt: newer,
	}
	providerDistinct := &models.Repository{
		ID: "repo-gh-other", Provider: "github", ProviderHost: githubProviderHost,
		ProviderOwner: "kdlbs", ProviderName: "other", CreatedAt: newer,
	}
	placeholder1 := &models.Repository{ID: "repo-placeholder-1", CreatedAt: newer}
	placeholder2 := &models.Repository{ID: "repo-placeholder-2", CreatedAt: newer}

	deduped := dedupeRepositoriesByIdentity([]*models.Repository{
		localLoser, localWinner, localDistinct,
		providerLoser, providerWinner, providerDistinct,
		placeholder1, placeholder2,
	})

	gotIDs := make(map[string]bool, len(deduped))
	for _, r := range deduped {
		gotIDs[r.ID] = true
	}
	wantIDs := []string{
		"repo-a-first", "repo-b",
		"repo-gh-first", "repo-gh-other",
		"repo-placeholder-1", "repo-placeholder-2",
	}
	if len(deduped) != len(wantIDs) {
		t.Fatalf("dedupeRepositoriesByIdentity returned %d repositories, want %d: %#v", len(deduped), len(wantIDs), deduped)
	}
	for _, id := range wantIDs {
		if !gotIDs[id] {
			t.Errorf("expected surviving repository %q, missing from result %#v", id, deduped)
		}
	}
	for _, loserID := range []string{"repo-a-second", "repo-gh-second"} {
		if gotIDs[loserID] {
			t.Errorf("later-created duplicate %q should have been dropped in favor of the earliest-created winner", loserID)
		}
	}
}

// TestService_ListRepositoriesDedupesByLocalPath is the integration path for
// the local-path case: it exercises the real Service.ListRepositories (so
// pruning/filtering runs before dedupe), asserting the duplicate pair
// collapses to one entry and the unrelated repository survives. Winner
// selection itself is covered deterministically by
// TestDedupeRepositoriesByIdentity above; this test avoids asserting which of
// the two duplicate IDs wins so it never depends on DB timestamp resolution.
func TestService_ListRepositoriesDedupesByLocalPath(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	dupPath := "/repos/dup"
	for _, id := range []string{"repo-a-first", "repo-a-second"} {
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: id, WorkspaceID: "ws-1", Name: id, SourceType: sourceTypeLocal, LocalPath: dupPath,
		}); err != nil {
			t.Fatalf("CreateRepository(%s): %v", id, err)
		}
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-b", WorkspaceID: "ws-1", Name: "distinct", SourceType: sourceTypeLocal, LocalPath: "/repos/distinct",
	}); err != nil {
		t.Fatalf("CreateRepository(distinct): %v", err)
	}

	repositories, err := svc.ListRepositories(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 2 {
		t.Fatalf("ListRepositories returned %d repositories, want 2 (one per identity): %#v", len(repositories), repositories)
	}
	dupWinners, sawDistinct := 0, false
	for _, r := range repositories {
		switch r.ID {
		case "repo-a-first", "repo-a-second":
			dupWinners++
		case "repo-b":
			sawDistinct = true
		default:
			t.Fatalf("unexpected repository id %q", r.ID)
		}
	}
	if dupWinners != 1 || !sawDistinct {
		t.Fatalf("expected exactly one repo-a-* survivor plus distinct repo-b, got %#v", repositories)
	}
}

// TestService_ListRepositoriesDedupesByProviderIdentity mirrors the
// local-path case above for provider-backed repositories through the real
// Service.ListRepositories path.
func TestService_ListRepositoriesDedupesByProviderIdentity(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	for _, id := range []string{"repo-gh-first", "repo-gh-second"} {
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: id, WorkspaceID: "ws-1", Name: id, SourceType: sourceTypeProvider,
			Provider: "github", ProviderHost: githubProviderHost, ProviderOwner: "kdlbs", ProviderName: "kandev",
		}); err != nil {
			t.Fatalf("CreateRepository(%s): %v", id, err)
		}
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-gh-other", WorkspaceID: "ws-1", Name: "kdlbs/other", SourceType: sourceTypeProvider,
		Provider: "github", ProviderHost: githubProviderHost, ProviderOwner: "kdlbs", ProviderName: "other",
	}); err != nil {
		t.Fatalf("CreateRepository(distinct): %v", err)
	}

	repositories, err := svc.ListRepositories(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 2 {
		t.Fatalf("ListRepositories returned %d repositories, want 2 (one per provider identity): %#v", len(repositories), repositories)
	}
	dupWinners, sawDistinct := 0, false
	for _, r := range repositories {
		switch r.ID {
		case "repo-gh-first", "repo-gh-second":
			dupWinners++
		case "repo-gh-other":
			sawDistinct = true
		default:
			t.Fatalf("unexpected repository id %q", r.ID)
		}
	}
	if dupWinners != 1 || !sawDistinct {
		t.Fatalf("expected exactly one repo-gh-* survivor plus distinct repo-gh-other, got %#v", repositories)
	}
}

// TestService_ListRepositoriesKeepsDistinctPlaceholderRepositories guards
// against an over-broad dedupe key: two rows that both carry an empty
// local_path and no provider identity (e.g. mid-creation placeholders) share
// no real identity and must never collapse into one another.
func TestService_ListRepositoriesKeepsDistinctPlaceholderRepositories(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	for _, id := range []string{"repo-placeholder-1", "repo-placeholder-2"} {
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: id, WorkspaceID: "ws-1", Name: id, SourceType: sourceTypeLocal,
		}); err != nil {
			t.Fatalf("CreateRepository(%s): %v", id, err)
		}
	}

	repositories, err := svc.ListRepositories(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 2 {
		t.Fatalf("ListRepositories returned %d repositories, want both placeholders preserved: %#v", len(repositories), repositories)
	}
}

// TestService_FindOrCreateRepositoryByLocalPath_ConcurrentCallsConvergeOnOneRow
// is the regression for the race this change closes: many goroutines
// resolving the same on-disk repository path at once (e.g. several
// create_task_kandev calls naming the same not-yet-registered local_path)
// must converge on a single repository row instead of each inserting its
// own. All goroutines are released together via a closed channel barrier so
// they genuinely contend for repoResolveMu rather than running sequentially.
func TestService_FindOrCreateRepositoryByLocalPath_ConcurrentCallsConvergeOnOneRow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	repoPath := filepath.Join(t.TempDir(), "concurrent-repo")
	makeRepo(t, repoPath)
	canonicalPath, _, pathErr := resolveExplicitLocalRepositoryPath(repoPath)
	if pathErr != nil {
		t.Fatalf("resolveExplicitLocalRepositoryPath: %v", pathErr)
	}

	const n = 20
	var wg sync.WaitGroup
	start := make(chan struct{})
	ids := make([]string, n)
	createdFlags := make([]bool, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			resolved, created, err := svc.FindOrCreateRepositoryByLocalPath(ctx, "ws-1", canonicalPath, &CreateRepositoryRequest{
				WorkspaceID: "ws-1",
				Name:        "concurrent-repo",
				SourceType:  sourceTypeLocal,
				LocalPath:   repoPath,
			})
			errs[i] = err
			if resolved != nil {
				ids[i] = resolved.ID
			}
			createdFlags[i] = created
		}(i)
	}
	close(start) // release every goroutine at once
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d: FindOrCreateRepositoryByLocalPath: %v", i, err)
		}
	}
	firstID := ids[0]
	createdCount := 0
	for i := range ids {
		if ids[i] != firstID {
			t.Fatalf("call %d resolved to repository %q, want %q (all concurrent callers must converge on one row)", i, ids[i], firstID)
		}
		if createdFlags[i] {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("createdCount = %d, want exactly 1 (exactly one call should have inserted the row)", createdCount)
	}

	all, err := repo.ListRepositories(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("ListRepositories returned %d rows after concurrent resolution, want exactly 1: %#v", len(all), all)
	}
}
