package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func seedAnswerableBundle(t *testing.T, repo *Repository, n int) (pendingID string) {
	t.Helper()
	taskID := "task-sidecar-" + itoa(n)
	sessionID := "session-sidecar-" + itoa(n)
	turnID := "turn-sidecar-" + itoa(n)
	pendingID = "pending-sidecar-" + itoa(n)
	seedBundleTask(t, repo, taskID, "workspace-sidecar")
	seedBundleSession(t, repo, sessionID, taskID)
	seedBundleTurn(t, repo, turnID, sessionID, taskID)
	insertClarificationMessage(t, repo, "msg-sidecar-"+itoa(n), sessionID, taskID, turnID, pendingID, "q1", "", 0, time.Now().UTC())
	return pendingID
}

func itoa(n int) string {
	digits := "0123456789"
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(digits[n%10]) + out
		n /= 10
	}
	return out
}

func TestUpsertClarificationInboxSidecar_DismissedBundleExcludedFromMainList(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	pendingID := seedAnswerableBundle(t, repo, 1)

	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", pendingID, models.ClarificationSidecarDismissed, nil, time.Now().UTC()); err != nil {
		t.Fatalf("upsert sidecar: %v", err)
	}

	opts := unscopedOpts(50)
	opts.Sidecar = &models.ClarificationSidecarFilter{UserID: "user-1", Now: time.Now().UTC()}
	page, err := repo.ListUnresolvedClarificationBundles(ctx, opts)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Bundles) != 0 {
		t.Fatalf("expected dismissed bundle excluded from main list, got %d bundles", len(page.Bundles))
	}

	// A different operator's sidecar must not hide it.
	otherOpts := unscopedOpts(50)
	otherOpts.Sidecar = &models.ClarificationSidecarFilter{UserID: "user-2", Now: time.Now().UTC()}
	otherPage, err := repo.ListUnresolvedClarificationBundles(ctx, otherOpts)
	if err != nil {
		t.Fatalf("list other user: %v", err)
	}
	if len(otherPage.Bundles) != 1 {
		t.Fatalf("expected bundle visible to a different operator, got %d bundles", len(otherPage.Bundles))
	}
}

func TestUpsertClarificationInboxSidecar_SnoozeExpiryReappearsAtBoundary(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	pendingID := seedAnswerableBundle(t, repo, 2)
	now := time.Now().UTC()
	snoozeUntil := now.Add(time.Hour)

	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", pendingID, models.ClarificationSidecarSnoozed, &snoozeUntil, now); err != nil {
		t.Fatalf("upsert sidecar: %v", err)
	}

	stillHidden := unscopedOpts(50)
	stillHidden.Sidecar = &models.ClarificationSidecarFilter{UserID: "user-1", Now: snoozeUntil.Add(-time.Minute)}
	page, err := repo.ListUnresolvedClarificationBundles(ctx, stillHidden)
	if err != nil {
		t.Fatalf("list before expiry: %v", err)
	}
	if len(page.Bundles) != 0 {
		t.Fatalf("expected bundle still hidden before expiry, got %d bundles", len(page.Bundles))
	}

	// AC .24: an expiry exactly at the current instant reappears (ties
	// resolve toward showing the question).
	atExpiry := unscopedOpts(50)
	atExpiry.Sidecar = &models.ClarificationSidecarFilter{UserID: "user-1", Now: snoozeUntil}
	page, err = repo.ListUnresolvedClarificationBundles(ctx, atExpiry)
	if err != nil {
		t.Fatalf("list at expiry: %v", err)
	}
	if len(page.Bundles) != 1 {
		t.Fatalf("expected bundle visible exactly at expiry, got %d bundles", len(page.Bundles))
	}
}

func TestDeleteClarificationInboxSidecar_RestoresBundleAndIsIdempotent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	pendingID := seedAnswerableBundle(t, repo, 3)
	now := time.Now().UTC()

	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", pendingID, models.ClarificationSidecarDismissed, nil, now); err != nil {
		t.Fatalf("upsert sidecar: %v", err)
	}
	if err := repo.DeleteClarificationInboxSidecar(ctx, "user-1", pendingID); err != nil {
		t.Fatalf("delete sidecar: %v", err)
	}
	// Deleting again (already-restored) must succeed as a no-op.
	if err := repo.DeleteClarificationInboxSidecar(ctx, "user-1", pendingID); err != nil {
		t.Fatalf("delete sidecar again: %v", err)
	}
	// Deleting an entirely unknown pending_id must also succeed as a no-op.
	if err := repo.DeleteClarificationInboxSidecar(ctx, "user-1", "unknown-pending-id"); err != nil {
		t.Fatalf("delete unknown sidecar: %v", err)
	}

	opts := unscopedOpts(50)
	opts.Sidecar = &models.ClarificationSidecarFilter{UserID: "user-1", Now: now}
	page, err := repo.ListUnresolvedClarificationBundles(ctx, opts)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Bundles) != 1 {
		t.Fatalf("expected restored bundle visible again, got %d bundles", len(page.Bundles))
	}
}

func TestCountHiddenClarificationBundles_MatchesEnumerationAndReportsEarliestExpiry(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()

	dismissedID := seedAnswerableBundle(t, repo, 4)
	snoozedSoonID := seedAnswerableBundle(t, repo, 5)
	snoozedLaterID := seedAnswerableBundle(t, repo, 6)
	visibleID := seedAnswerableBundle(t, repo, 7)
	_ = visibleID

	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", dismissedID, models.ClarificationSidecarDismissed, nil, now); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	soon := now.Add(time.Hour)
	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", snoozedSoonID, models.ClarificationSidecarSnoozed, &soon, now); err != nil {
		t.Fatalf("snooze soon: %v", err)
	}
	later := now.Add(4 * time.Hour)
	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", snoozedLaterID, models.ClarificationSidecarSnoozed, &later, now); err != nil {
		t.Fatalf("snooze later: %v", err)
	}

	countOpts := unscopedOpts(50)
	countOpts.Sidecar = &models.ClarificationSidecarFilter{UserID: "user-1", Only: true, Now: now}
	summary, err := repo.CountHiddenClarificationBundles(ctx, countOpts)
	if err != nil {
		t.Fatalf("count hidden: %v", err)
	}
	if summary.HiddenCount != 3 {
		t.Fatalf("expected 3 hidden bundles, got %d", summary.HiddenCount)
	}
	if summary.NextSnoozeExpiry == nil || !summary.NextSnoozeExpiry.Equal(soon) {
		t.Fatalf("expected next snooze expiry %v, got %v", soon, summary.NextSnoozeExpiry)
	}

	// The hidden-bundles enumeration must return exactly the same set the
	// count sees, using the same Only=true filter, with the main list a
	// strict complement (AC .37's "partition of one answerable set").
	hiddenPage, err := repo.ListUnresolvedClarificationBundles(ctx, countOpts)
	if err != nil {
		t.Fatalf("list hidden: %v", err)
	}
	if len(hiddenPage.Bundles) != summary.HiddenCount {
		t.Fatalf("expected hidden list length %d to equal hidden count %d", len(hiddenPage.Bundles), summary.HiddenCount)
	}

	mainOpts := unscopedOpts(50)
	mainOpts.Sidecar = &models.ClarificationSidecarFilter{UserID: "user-1", Now: now}
	mainPage, err := repo.ListUnresolvedClarificationBundles(ctx, mainOpts)
	if err != nil {
		t.Fatalf("list main: %v", err)
	}
	if len(mainPage.Bundles) != 1 || mainPage.Bundles[0].PendingID != visibleID {
		t.Fatalf("expected exactly the un-hidden bundle in the main list, got %+v", mainPage.Bundles)
	}
}

func TestCountHiddenClarificationBundles_ZeroHiddenReportsNoExpiry(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedAnswerableBundle(t, repo, 8)

	countOpts := unscopedOpts(50)
	countOpts.Sidecar = &models.ClarificationSidecarFilter{UserID: "user-1", Only: true, Now: now}
	summary, err := repo.CountHiddenClarificationBundles(ctx, countOpts)
	if err != nil {
		t.Fatalf("count hidden: %v", err)
	}
	if summary.HiddenCount != 0 {
		t.Fatalf("expected 0 hidden bundles, got %d", summary.HiddenCount)
	}
	if summary.NextSnoozeExpiry != nil {
		t.Fatalf("expected no next snooze expiry, got %v", summary.NextSnoozeExpiry)
	}
}

func TestGetClarificationInboxSidecarStates_ResolvesStateAndExpiry(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	dismissedID := seedAnswerableBundle(t, repo, 9)
	snoozedID := seedAnswerableBundle(t, repo, 10)
	snoozeUntil := now.Add(time.Hour)

	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", dismissedID, models.ClarificationSidecarDismissed, nil, now); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", snoozedID, models.ClarificationSidecarSnoozed, &snoozeUntil, now); err != nil {
		t.Fatalf("snooze: %v", err)
	}

	states, err := repo.GetClarificationInboxSidecarStates(ctx, "user-1", []string{dismissedID, snoozedID})
	if err != nil {
		t.Fatalf("get states: %v", err)
	}
	if states[dismissedID].State != models.ClarificationSidecarDismissed || states[dismissedID].SnoozeUntil != nil {
		t.Fatalf("unexpected dismissed entry: %+v", states[dismissedID])
	}
	if states[snoozedID].State != models.ClarificationSidecarSnoozed ||
		states[snoozedID].SnoozeUntil == nil || !states[snoozedID].SnoozeUntil.Equal(snoozeUntil) {
		t.Fatalf("unexpected snoozed entry: %+v", states[snoozedID])
	}
}
