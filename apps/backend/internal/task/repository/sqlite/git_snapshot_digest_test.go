package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// snapshotForDigestTest builds a minimal, otherwise-identical git snapshot
// for a session so tests can vary just the fields that matter to them.
func snapshotForDigestTest(id, sessionID, triggeredBy string, createdAt time.Time) *models.GitSnapshot {
	return &models.GitSnapshot{
		ID:           id,
		SessionID:    sessionID,
		SnapshotType: models.SnapshotTypeStatusUpdate,
		Branch:       "feature/digest",
		RemoteBranch: "origin/feature/digest",
		HeadCommit:   "head-sha-digest",
		BaseCommit:   "base-sha-digest",
		Ahead:        1,
		Behind:       0,
		Files: map[string]interface{}{
			"apps/backend/main.go": map[string]interface{}{"status": "modified"},
		},
		TriggeredBy: triggeredBy,
		CreatedAt:   createdAt,
	}
}

// TestCreateGitSnapshotSetsContentDigestAndAlwaysRecords proves
// CreateGitSnapshot computes a stable content digest for identical
// repository state and a different one when content changes, while still
// recording every poll (content-based dedup for this wave is read-only via
// ListDuplicateGitSnapshotCandidates, not a write-time skip - see that
// function's docs for why).
func TestCreateGitSnapshotSetsContentDigestAndAlwaysRecords(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-digest-basic", "session-digest-basic")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	first := snapshotForDigestTest("snap-digest-1", "session-digest-basic", TriggeredByLiveMonitor, base)
	if err := repo.CreateGitSnapshot(ctx, first); err != nil {
		t.Fatalf("CreateGitSnapshot(first): %v", err)
	}
	if first.ContentDigest == "" {
		t.Fatal("CreateGitSnapshot did not set ContentDigest")
	}

	// Identical repository state, later poll: content_digest matches and the
	// row is still recorded (no write-time skip in this wave).
	second := snapshotForDigestTest("snap-digest-2", "session-digest-basic", TriggeredByLiveMonitor, base.Add(time.Minute))
	if err := repo.CreateGitSnapshot(ctx, second); err != nil {
		t.Fatalf("CreateGitSnapshot(second, identical content): %v", err)
	}
	if second.ContentDigest != first.ContentDigest {
		t.Fatalf("ContentDigest = %q, want %q (identical repository state)", second.ContentDigest, first.ContentDigest)
	}
	count := countRows(t, repo, `SELECT COUNT(*) FROM task_session_git_snapshots WHERE session_id = ?`, "session-digest-basic")
	if count != 2 {
		t.Fatalf("row count = %d, want 2 (every poll is still recorded)", count)
	}

	// Changed content produces a different digest.
	changed := snapshotForDigestTest("snap-digest-3", "session-digest-basic", TriggeredByLiveMonitor, base.Add(2*time.Minute))
	changed.HeadCommit = "head-sha-digest-changed"
	if err := repo.CreateGitSnapshot(ctx, changed); err != nil {
		t.Fatalf("CreateGitSnapshot(changed): %v", err)
	}
	if changed.ContentDigest == first.ContentDigest {
		t.Fatal("changed snapshot content digest matched the original despite a different head commit")
	}
}

func TestUpsertLatestLiveGitSnapshotSetsContentDigest(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-digest-upsert", "session-digest-upsert")

	snapshot := snapshotForDigestTest("snap-upsert-1", "session-digest-upsert", "", time.Now().UTC())
	if err := repo.UpsertLatestLiveGitSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("UpsertLatestLiveGitSnapshot: %v", err)
	}
	if snapshot.ContentDigest == "" {
		t.Fatal("UpsertLatestLiveGitSnapshot did not set ContentDigest")
	}

	var stored string
	if err := repo.db.QueryRowContext(ctx, repo.db.Rebind(
		`SELECT content_digest FROM task_session_git_snapshots WHERE id = ?`),
		snapshot.ID).Scan(&stored); err != nil {
		t.Fatalf("query stored content_digest: %v", err)
	}
	if stored != snapshot.ContentDigest {
		t.Fatalf("stored content_digest = %q, want %q", stored, snapshot.ContentDigest)
	}
}

// TestListDuplicateGitSnapshotCandidatesKeepsNewestPerGroup exercises the
// read-only retention-candidate selection: every row created via
// CreateGitSnapshot with content-identical repository state is recorded
// (this wave never skips a write), and the candidate query must report all
// but the newest row in each (session, content_digest) group for a later,
// explicit maintenance pass to prune.
func TestListDuplicateGitSnapshotCandidatesKeepsNewestPerGroup(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-digest-candidates", "session-digest-candidates")

	base := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	ids := []string{"snap-cand-1", "snap-cand-2", "snap-cand-3"}
	for i, offset := range []time.Duration{0, time.Minute, 2 * time.Minute} {
		snap := snapshotForDigestTest(ids[i], "session-digest-candidates", TriggeredByLiveMonitor, base.Add(offset))
		if err := repo.CreateGitSnapshot(ctx, snap); err != nil {
			t.Fatalf("CreateGitSnapshot(%d): %v", i, err)
		}
	}

	count := countRows(t, repo, `SELECT COUNT(*) FROM task_session_git_snapshots WHERE session_id = ?`, "session-digest-candidates")
	if count != 3 {
		t.Fatalf("seeded row count = %d, want 3", count)
	}

	candidates, err := repo.ListDuplicateGitSnapshotCandidates(ctx, 0)
	if err != nil {
		t.Fatalf("ListDuplicateGitSnapshotCandidates: %v", err)
	}
	var seenIDs []string
	for _, c := range candidates {
		if c.SessionID != "session-digest-candidates" {
			continue
		}
		seenIDs = append(seenIDs, c.ID)
		if c.ID == "snap-cand-3" {
			t.Fatal("newest duplicate row (snap-cand-3) must not be reported as a candidate")
		}
	}
	if len(seenIDs) != 2 {
		t.Fatalf("candidates for this session = %v, want exactly [snap-cand-1 snap-cand-2]", seenIDs)
	}
}

// TestListDuplicateGitSnapshotCandidatesIsNonDestructive confirms the
// selection is read-only: calling it does not remove or alter any snapshot
// row, matching the plan's "non-destructive until maintenance executes"
// constraint.
func TestListDuplicateGitSnapshotCandidatesIsNonDestructive(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-digest-nondestructive", "session-digest-nondestructive")

	base := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)
	for i, offset := range []time.Duration{0, time.Minute} {
		snap := snapshotForDigestTest([]string{"snap-nd-1", "snap-nd-2"}[i], "session-digest-nondestructive", TriggeredByLiveMonitor, base.Add(offset))
		if err := repo.CreateGitSnapshot(ctx, snap); err != nil {
			t.Fatalf("CreateGitSnapshot(%d): %v", i, err)
		}
	}

	before := countRows(t, repo, `SELECT COUNT(*) FROM task_session_git_snapshots WHERE session_id = ?`, "session-digest-nondestructive")
	if _, err := repo.ListDuplicateGitSnapshotCandidates(ctx, 0); err != nil {
		t.Fatalf("ListDuplicateGitSnapshotCandidates: %v", err)
	}
	after := countRows(t, repo, `SELECT COUNT(*) FROM task_session_git_snapshots WHERE session_id = ?`, "session-digest-nondestructive")
	if before != after {
		t.Fatalf("row count changed from %d to %d after a read-only candidate listing", before, after)
	}
}
