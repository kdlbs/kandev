package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func earlierTurnOpts(workspaceID string) models.ListClarificationBundlesOptions {
	return models.ListClarificationBundlesOptions{Unscoped: true, WorkspaceID: workspaceID, Limit: 50, EarlierTurns: true}
}

func pendingIDs(bundles []models.ClarificationBundleSummary) []string {
	ids := make([]string, 0, len(bundles))
	for _, b := range bundles {
		ids = append(ids, b.PendingID)
	}
	return ids
}

// seedEarlierTurnFixture builds, in workspace ws-E, one session whose older
// turn carries several questions and whose newer turn carries one, plus the
// exclusions the earlier-turn list must apply.
func seedEarlierTurnFixture(t *testing.T, repo *Repository, base time.Time) {
	t.Helper()
	seedBundleTask(t, repo, "task-E1", "ws-E")
	seedBundleSession(t, repo, "sess-E1", "task-E1")
	seedBundleTurnAt(t, repo, "turn-E1-old", "sess-E1", "task-E1", base)
	insertClarificationMessage(t, repo, "msg-E1-a", "sess-E1", "task-E1", "turn-E1-old", "pending-E1-old", "q1", "pending", 0, base)
	// Absent status on an earlier turn is legacy, never resurrected.
	insertClarificationMessage(t, repo, "msg-E1-legacy", "sess-E1", "task-E1", "turn-E1-old", "pending-E1-legacy", "q1", "", 0, base.Add(time.Second))
	insertClarificationMessage(t, repo, "msg-E1-answered", "sess-E1", "task-E1", "turn-E1-old", "pending-E1-answered", "q1", "answered", 0, base.Add(2*time.Second))
	insertParentQuestionMessage(t, repo, "msg-E1-parent", "sess-E1", "task-E1", "turn-E1-old", "pending-E1-parent", "q1", "pending", base.Add(3*time.Second))
	insertClarificationMessageRawMetadata(t, repo, "msg-E1-noqid", "sess-E1", "task-E1", "turn-E1-old",
		map[string]interface{}{"pending_id": "pending-E1-noqid", "status": "pending"}, base.Add(4*time.Second))
	seedBundleTurnAt(t, repo, "turn-E1-new", "sess-E1", "task-E1", base.Add(time.Minute))
	insertClarificationMessage(t, repo, "msg-E1-current", "sess-E1", "task-E1", "turn-E1-new", "pending-E1-current", "q1", "pending", 0, base.Add(time.Minute))

	// A terminal session's earlier-turn question is not listed.
	seedBundleTask(t, repo, "task-E2", "ws-E")
	seedBundleSessionWithState(t, repo, "sess-E2", "task-E2", models.TaskSessionStateCancelled)
	seedBundleTurnAt(t, repo, "turn-E2-old", "sess-E2", "task-E2", base)
	insertClarificationMessage(t, repo, "msg-E2", "sess-E2", "task-E2", "turn-E2-old", "pending-E2", "q1", "pending", 0, base)
	seedBundleTurnAt(t, repo, "turn-E2-new", "sess-E2", "task-E2", base.Add(time.Minute))

	// Another workspace's earlier-turn question is not listed.
	seedBundleTask(t, repo, "task-E3", "ws-other")
	seedBundleSession(t, repo, "sess-E3", "task-E3")
	seedBundleTurnAt(t, repo, "turn-E3-old", "sess-E3", "task-E3", base)
	insertClarificationMessage(t, repo, "msg-E3", "sess-E3", "task-E3", "turn-E3-old", "pending-E3", "q1", "pending", 0, base)
	seedBundleTurnAt(t, repo, "turn-E3-new", "sess-E3", "task-E3", base.Add(time.Minute))
}

func TestListUnresolvedClarificationBundles_EarlierTurnsListsOnlyStrandedPending(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedEarlierTurnFixture(t, repo, time.Now().UTC().Add(-time.Hour))

	page, err := repo.ListUnresolvedClarificationBundles(ctx, earlierTurnOpts("ws-E"))
	if err != nil {
		t.Fatalf("earlier-turn list: %v", err)
	}
	if got := pendingIDs(page.Bundles); len(got) != 1 || got[0] != "pending-E1-old" {
		t.Fatalf("earlier-turn bundles = %v, want only pending-E1-old", got)
	}

	current := earlierTurnOpts("ws-E")
	current.EarlierTurns = false
	page, err = repo.ListUnresolvedClarificationBundles(ctx, current)
	if err != nil {
		t.Fatalf("current-turn list: %v", err)
	}
	if got := pendingIDs(page.Bundles); len(got) != 1 || got[0] != "pending-E1-current" {
		t.Fatalf("current-turn bundles = %v, want only pending-E1-current (the two lists are disjoint)", got)
	}
}

func TestListUnresolvedClarificationBundles_EarlierTurnsEmptyWhenOnlyCurrentTurn(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedBundleTask(t, repo, "task-E4", "ws-E4")
	seedBundleSession(t, repo, "sess-E4", "task-E4")
	seedBundleTurn(t, repo, "turn-E4", "sess-E4", "task-E4")
	insertClarificationMessage(t, repo, "msg-E4", "sess-E4", "task-E4", "turn-E4", "pending-E4", "q1", "pending", 0, time.Now().UTC())

	page, err := repo.ListUnresolvedClarificationBundles(context.Background(), earlierTurnOpts("ws-E4"))
	if err != nil {
		t.Fatalf("earlier-turn list: %v", err)
	}
	if len(page.Bundles) != 0 {
		t.Fatalf("earlier-turn bundles = %v, want none", pendingIDs(page.Bundles))
	}
}

func TestListUnresolvedClarificationBundles_EarlierTurnsHonorsSidecarAndLimit(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)
	seedBundleTask(t, repo, "task-E5", "ws-E5")
	seedBundleSession(t, repo, "sess-E5", "task-E5")
	seedBundleTurnAt(t, repo, "turn-E5-old", "sess-E5", "task-E5", base)
	for i, id := range []string{"a", "b", "c"} {
		insertClarificationMessage(t, repo, "msg-E5-"+id, "sess-E5", "task-E5", "turn-E5-old", "pending-E5-"+id, "q1", "pending", 0, base.Add(time.Duration(i)*time.Second))
	}
	seedBundleTurnAt(t, repo, "turn-E5-new", "sess-E5", "task-E5", base.Add(time.Minute))
	now := time.Now().UTC()
	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", "pending-E5-a", models.ClarificationSidecarDismissed, nil, now); err != nil {
		t.Fatalf("dismiss: %v", err)
	}

	opts := earlierTurnOpts("ws-E5")
	opts.Limit = 1
	opts.Sidecar = &models.ClarificationSidecarFilter{UserID: "user-1", Now: now}
	page, err := repo.ListUnresolvedClarificationBundles(ctx, opts)
	if err != nil {
		t.Fatalf("earlier-turn list: %v", err)
	}
	if got := pendingIDs(page.Bundles); len(got) != 1 || got[0] != "pending-E5-b" || !page.HasMore {
		t.Fatalf("earlier-turn bundles = %v (hasMore=%v), want pending-E5-b with more", got, page.HasMore)
	}
}
