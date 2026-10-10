package sqlite

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
)

func TestTurnChangeContentDeduplicatesAndVerifiesPayload(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	changeSetID, repositoryChangeID := seedTurnChangeContentOwner(t, repo, "content-dedupe", time.Now().UTC().Add(24*time.Hour))
	patch := []byte("diff --git a/a b/a\n+new\n")
	files := []models.TurnChangeFileContent{
		turnChangeFileContent("file-content-a", repositoryChangeID, "a.txt", patch, []byte("old"), []byte("new")),
		turnChangeFileContent("file-content-b", repositoryChangeID, "b.txt", patch, []byte("before"), []byte("after")),
	}
	files[0].NewRendering = nil
	if err := repo.StoreTurnChangeFiles(ctx, repositoryChangeID, files); err != nil {
		t.Fatalf("StoreTurnChangeFiles: %v", err)
	}
	var contentCount int
	if err := repo.db.GetContext(ctx, &contentCount, `SELECT COUNT(*) FROM turn_change_contents`); err != nil {
		t.Fatalf("count content rows: %v", err)
	}
	if contentCount != 4 {
		t.Fatalf("content rows = %d, want four unique canonical/filtered/rendering payloads", contentCount)
	}
	initialSet, err := repo.GetTurnChangeSet(ctx, "task-content-dedupe", "session-content-dedupe", changeSetID)
	if err != nil {
		t.Fatalf("get change set before retry: %v", err)
	}
	if err := repo.StoreTurnChangeFiles(ctx, repositoryChangeID, files); err != nil {
		t.Fatalf("retry StoreTurnChangeFiles: %v", err)
	}
	retriedSet, err := repo.GetTurnChangeSet(ctx, "task-content-dedupe", "session-content-dedupe", changeSetID)
	if err != nil || retriedSet.ContentBytes != initialSet.ContentBytes {
		t.Fatalf("idempotent retry content bytes = %+v, %v; initial=%d", retriedSet, err, initialSet.ContentBytes)
	}
	files[0].CanonicalPatch = []byte("different accepted content")
	if err := repo.StoreTurnChangeFiles(ctx, repositoryChangeID, files); !errors.Is(err, repoerrors.ErrTurnChangeRelationship) {
		t.Fatalf("conflicting retry error = %v, want relationship conflict", err)
	}
	got, err := repo.ReadTurnChangeContent(ctx, changeSetID, "file-content-a", models.TurnChangeContentCanonicalPatch)
	if err != nil || string(got.Content) != string(patch) || got.Digest == "" {
		t.Fatalf("ReadTurnChangeContent = %+v, %v", got, err)
	}
	emptyFile := turnChangeFileContent("file-empty-rendering", repositoryChangeID, "empty.txt", []byte("p"), []byte{}, []byte("x"))
	if err := repo.StoreTurnChangeFiles(ctx, repositoryChangeID, []models.TurnChangeFileContent{emptyFile}); err != nil {
		t.Fatalf("store empty rendering: %v", err)
	}
	got, err = repo.ReadTurnChangeContent(ctx, changeSetID, "file-empty-rendering", models.TurnChangeContentOldRendering)
	if err != nil || got == nil || len(got.Content) != 0 {
		t.Fatalf("empty rendering = %+v, %v, want stored zero-byte blob", got, err)
	}
	if _, err := repo.ReadTurnChangeContent(ctx, changeSetID, "file-content-a", models.TurnChangeContentNewRendering); !errors.Is(err, repoerrors.ErrTurnChangeContentNotFound) {
		t.Fatalf("missing variant error = %v, want not found", err)
	}
	if _, err := repo.ReadTurnChangeContent(ctx, "another-change-set", "file-content-a", models.TurnChangeContentCanonicalPatch); !errors.Is(err, repoerrors.ErrTurnChangeContentNotFound) {
		t.Fatalf("cross-change-set read error = %v, want not found", err)
	}
	loaded, err := repo.GetTurnChangeSet(ctx, "task-content-dedupe", "session-content-dedupe", changeSetID)
	if err != nil || loaded.ContentBytes <= 0 {
		t.Fatalf("content byte accounting = %+v, %v", loaded, err)
	}
}

func TestPostgresTurnChangeContentRoundTripAndRetention(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres schema: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	changeSetID, repositoryChangeID := seedTurnChangeContentOwner(t, repo, "postgres-content", now.Add(-time.Minute))
	file := turnChangeFileContent("file-postgres-content", repositoryChangeID, "src/data.go", []byte("patch"), []byte("old"), []byte("new"))
	if err := repo.StoreTurnChangeFiles(ctx, repositoryChangeID, []models.TurnChangeFileContent{file}); err != nil {
		t.Fatalf("store postgres content: %v", err)
	}
	payload, err := repo.ReadTurnChangeContent(ctx, changeSetID, file.File.ID, models.TurnChangeContentCanonicalPatch)
	if err != nil || string(payload.Content) != "patch" {
		t.Fatalf("read postgres content = %+v, %v", payload, err)
	}
	lease, err := repo.AcquireTurnChangeContentLease(ctx, changeSetID, time.Minute)
	if err != nil {
		t.Fatalf("acquire postgres lease: %v", err)
	}
	policy := models.TurnChangeRetentionPolicy{RetainFor: time.Hour, TaskBytes: 1 << 20, InstallationBytes: 1 << 20}
	result, err := repo.ApplyTurnChangeRetention(ctx, policy, now)
	if err != nil || result.ExpiredChangeSets != 0 {
		t.Fatalf("postgres leased retention = %+v, %v", result, err)
	}
	if err := repo.ReleaseTurnChangeContentLease(ctx, lease.ID); err != nil {
		t.Fatalf("release postgres lease: %v", err)
	}
	result, err = repo.ApplyTurnChangeRetention(ctx, policy, now)
	if err != nil || result.ExpiredChangeSets != 1 || result.DeletedContents == 0 {
		t.Fatalf("postgres retention after lease = %+v, %v", result, err)
	}
}

func TestTurnChangeContentMarksOversizedPatchPartialWithoutLosingMetadata(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	_, repositoryChangeID := seedTurnChangeContentOwner(t, repo, "content-limit", time.Now().UTC().Add(time.Hour))
	file := turnChangeFileContent("file-content-too-large", repositoryChangeID, "large.txt", []byte(strings.Repeat("x", maxTurnChangePatchBytes+1)), []byte("old"), []byte("new"))
	if err := repo.StoreTurnChangeFiles(ctx, repositoryChangeID, []models.TurnChangeFileContent{file}); err != nil {
		t.Fatalf("StoreTurnChangeFiles: %v", err)
	}
	var availability, reason string
	var truncated bool
	var canonicalID string
	if err := repo.db.QueryRowContext(ctx, `SELECT content_availability, content_reason, content_truncated, canonical_content_id FROM turn_file_changes WHERE id = ?`, file.File.ID).
		Scan(&availability, &reason, &truncated, &canonicalID); err != nil {
		t.Fatalf("load partial file row: %v", err)
	}
	if availability != string(models.TurnChangeAvailabilityUnavailable) || reason != string(models.TurnChangeReasonSizeLimit) || !truncated || canonicalID != "" {
		t.Fatalf("oversized file status = %q/%q truncated=%t canonical=%q", availability, reason, truncated, canonicalID)
	}
	var contentCount int
	if err := repo.db.GetContext(ctx, &contentCount, `SELECT COUNT(*) FROM turn_change_contents`); err != nil || contentCount != 2 {
		t.Fatalf("bounded old/new blobs retained = %d, %v; want two while patch is omitted", contentCount, err)
	}
	var fileCount int
	if err := repo.db.GetContext(ctx, &fileCount, `SELECT COUNT(*) FROM turn_file_changes`); err != nil || fileCount != 1 {
		t.Fatalf("file metadata rows = %d, %v; want one", fileCount, err)
	}
}

func TestTurnChangeContentAggregateLimitKeepsEarlierVariants(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	_, repositoryChangeID := seedTurnChangeContentOwner(t, repo, "content-aggregate-limit", time.Now().UTC().Add(time.Hour))
	secondRepositoryChangeID := "repository-change-content-aggregate-limit-second"
	now := time.Now().UTC()
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO turn_repository_changes (id, change_set_id, checkout_id, environment_repo_id, repository_id, availability, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), secondRepositoryChangeID, "change-set-content-aggregate-limit", "checkout-second", "env-repo-second", "repo-second",
		models.TurnChangeAvailabilityPending, now, now); err != nil {
		t.Fatalf("insert second checkout row: %v", err)
	}
	patch := bytes.Repeat([]byte("p"), maxTurnChangePatchBytes)
	rendering := bytes.Repeat([]byte("r"), maxTurnChangeRenderingBytes)
	first := models.TurnChangeFileContent{
		File:           models.TurnFileChange{ID: "file-aggregate-first", RepositoryChangeID: repositoryChangeID, Path: "first", PathBytes: []byte("first"), Kind: "modified", ContentAvailability: models.TurnChangeAvailabilityReady},
		CanonicalPatch: patch, FilteredPatch: append([]byte(nil), patch...), OldRendering: rendering, NewRendering: append([]byte(nil), rendering...),
	}
	if err := repo.StoreTurnChangeFiles(ctx, repositoryChangeID, []models.TurnChangeFileContent{first}); err != nil {
		t.Fatalf("store first bounded file: %v", err)
	}
	second := first
	second.File = models.TurnFileChange{ID: "file-aggregate-second", RepositoryChangeID: secondRepositoryChangeID, CheckoutID: "checkout-second", Path: "second", PathBytes: []byte("second"), Kind: "modified", ContentAvailability: models.TurnChangeAvailabilityReady}
	receipt, err := repo.StoreTurnChangeFilesWithReceipt(ctx, secondRepositoryChangeID, []models.TurnChangeFileContent{second})
	if err != nil {
		t.Fatalf("store second bounded file: %v", err)
	}
	if receipt.Complete || receipt.Reason != models.TurnChangeReasonSizeLimit || receipt.StoredBytes != 8<<20 {
		t.Fatalf("second repository storage receipt = %+v; want retained patch bytes with unavailable renderings", receipt)
	}
	var contentBytes int64
	if err := repo.db.GetContext(ctx, &contentBytes, repo.db.Rebind(`SELECT content_bytes FROM turn_change_sets WHERE id = ?`), "change-set-content-aggregate-limit"); err != nil || contentBytes != maxTurnChangeExportBytes {
		t.Fatalf("aggregate content bytes = %d, %v; want %d", contentBytes, err, maxTurnChangeExportBytes)
	}
	var availability, reason string
	var oldContentID, newContentID string
	if err := repo.db.QueryRowContext(ctx, `SELECT content_availability, content_reason, old_content_id, new_content_id FROM turn_file_changes WHERE id = ?`, second.File.ID).
		Scan(&availability, &reason, &oldContentID, &newContentID); err != nil {
		t.Fatalf("load truncated second file: %v", err)
	}
	if availability != string(models.TurnChangeAvailabilityUnavailable) || reason != string(models.TurnChangeReasonSizeLimit) || oldContentID != "" || newContentID != "" {
		t.Fatalf("second file aggregate-bound status = %q/%q old=%q new=%q", availability, reason, oldContentID, newContentID)
	}
}

func TestTurnChangeRetentionLeaseAndSharedContent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	changeSetID, repositoryChangeID := seedTurnChangeContentOwner(t, repo, "retention-leased", now.Add(-time.Hour))
	shared := []byte("same patch")
	if err := repo.StoreTurnChangeFiles(ctx, repositoryChangeID, []models.TurnChangeFileContent{
		turnChangeFileContent("file-retained", repositoryChangeID, "a.txt", shared, []byte("old"), []byte("new")),
	}); err != nil {
		t.Fatalf("store content: %v", err)
	}
	lease, err := repo.AcquireTurnChangeContentLease(ctx, changeSetID, time.Minute)
	if err != nil {
		t.Fatalf("AcquireTurnChangeContentLease: %v", err)
	}
	policy := models.TurnChangeRetentionPolicy{RetainFor: time.Hour, TaskBytes: 1, InstallationBytes: 1}
	result, err := repo.ApplyTurnChangeRetention(ctx, policy, now)
	if err != nil || result.ExpiredChangeSets != 0 {
		t.Fatalf("retention while leased = %+v, %v; want protected content", result, err)
	}
	if _, err := repo.ReadTurnChangeContent(ctx, changeSetID, "file-retained", models.TurnChangeContentCanonicalPatch); err != nil {
		t.Fatalf("leased content read: %v", err)
	}
	if err := repo.ReleaseTurnChangeContentLease(ctx, lease.ID); err != nil {
		t.Fatalf("ReleaseTurnChangeContentLease: %v", err)
	}
	result, err = repo.ApplyTurnChangeRetention(ctx, policy, now)
	if err != nil || result.ExpiredChangeSets != 1 || result.DeletedContents == 0 {
		t.Fatalf("retention after release = %+v, %v; want one expired set and deleted blobs", result, err)
	}
	if _, err := repo.ReadTurnChangeContent(ctx, changeSetID, "file-retained", models.TurnChangeContentCanonicalPatch); !errors.Is(err, repoerrors.ErrTurnChangeContentNotFound) {
		t.Fatalf("expired content error = %v, want not found", err)
	}
	set, err := repo.GetTurnChangeSet(ctx, "task-retention-leased", "session-retention-leased", changeSetID)
	if err != nil || set.Availability != models.TurnChangeAvailabilityExpired || set.FileCount != 0 {
		// The durable summary remains; this fixture has not finalized aggregate
		// file metadata, so its file count is intentionally zero.
		t.Fatalf("expired summary = %+v, %v", set, err)
	}
	if set.Reason != models.TurnChangeReasonExpiredAge || set.ExpiryReason != string(models.TurnChangeReasonExpiredAge) {
		t.Fatalf("expired summary reason = %q/%q, want age", set.Reason, set.ExpiryReason)
	}
	result, err = repo.ApplyTurnChangeRetention(ctx, policy, now)
	if err != nil || result.ExpiredChangeSets != 0 {
		t.Fatalf("repeated retention = %+v, %v; want no second expiration", result, err)
	}
	replayed, err := repo.GetTurnChangeSet(ctx, "task-retention-leased", "session-retention-leased", changeSetID)
	if err != nil || replayed.Revision != set.Revision {
		t.Fatalf("expired summary replay = %+v, %v; revision changed from %d", replayed, err, set.Revision)
	}
}

func TestTurnChangeRetentionKeepsContentSharedByUnexpiredSet(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_, expiredRepositoryID := seedTurnChangeContentOwner(t, repo, "retention-expired-owner", now.Add(-time.Hour))
	_, liveRepositoryID := seedTurnChangeContentOwner(t, repo, "retention-live-owner", now.Add(time.Hour))
	shared := []byte("shared canonical patch")
	if err := repo.StoreTurnChangeFiles(ctx, expiredRepositoryID, []models.TurnChangeFileContent{
		turnChangeFileContent("file-expired-share", expiredRepositoryID, "a", shared, nil, nil),
	}); err != nil {
		t.Fatalf("store expired content: %v", err)
	}
	if err := repo.StoreTurnChangeFiles(ctx, liveRepositoryID, []models.TurnChangeFileContent{
		turnChangeFileContent("file-live-share", liveRepositoryID, "a", shared, nil, nil),
	}); err != nil {
		t.Fatalf("store live content: %v", err)
	}
	policy := models.TurnChangeRetentionPolicy{RetainFor: time.Hour, TaskBytes: 1 << 20, InstallationBytes: 1 << 20}
	result, err := repo.ApplyTurnChangeRetention(ctx, policy, now)
	if err != nil || result.ExpiredChangeSets != 1 {
		t.Fatalf("retention = %+v, %v; want one aged change set", result, err)
	}
	if result.DeletedContents != 0 {
		t.Fatalf("deleted shared contents = %d, want zero while live link remains", result.DeletedContents)
	}
	if _, err := repo.ReadTurnChangeContent(ctx, "change-set-retention-live-owner", "file-live-share", models.TurnChangeContentCanonicalPatch); err != nil {
		t.Fatalf("shared content for unexpired turn was removed: %v", err)
	}
}

func TestTurnChangeRetentionRecordsCapacityReason(t *testing.T) {
	for _, test := range []struct {
		name       string
		policy     models.TurnChangeRetentionPolicy
		wantReason models.TurnChangeReason
	}{
		{
			name:       "task limit",
			policy:     models.TurnChangeRetentionPolicy{RetainFor: 24 * time.Hour, TaskBytes: 1, InstallationBytes: 1 << 20},
			wantReason: models.TurnChangeReasonExpiredTaskLimit,
		},
		{
			name:       "installation limit",
			policy:     models.TurnChangeRetentionPolicy{RetainFor: 24 * time.Hour, TaskBytes: 1 << 20, InstallationBytes: 1},
			wantReason: models.TurnChangeReasonExpiredInstallLimit,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			now := time.Now().UTC()
			changeSetID, repositoryChangeID := seedTurnChangeContentOwner(t, repo, "capacity-"+strings.ReplaceAll(test.name, " ", "-"), now.Add(time.Hour))
			file := turnChangeFileContent("file-capacity", repositoryChangeID, "src/file", []byte("bounded historical patch"), []byte("old rendering"), []byte("new rendering"))
			if err := repo.StoreTurnChangeFiles(ctx, repositoryChangeID, []models.TurnChangeFileContent{file}); err != nil {
				t.Fatalf("store content: %v", err)
			}
			result, err := repo.ApplyTurnChangeRetention(ctx, test.policy, now)
			if err != nil || result.ExpiredChangeSets != 1 {
				t.Fatalf("retention = %+v, %v; want one capacity eviction", result, err)
			}
			set, err := repo.GetTurnChangeSet(ctx, "task-capacity-"+strings.ReplaceAll(test.name, " ", "-"), "session-capacity-"+strings.ReplaceAll(test.name, " ", "-"), changeSetID)
			if err != nil || set.ExpiryReason != string(test.wantReason) {
				t.Fatalf("expiry reason = %+v, %v; want %q", set, err, test.wantReason)
			}
		})
	}
}

func turnChangeFileContent(id, repositoryChangeID, path string, patch, old, new []byte) models.TurnChangeFileContent {
	return models.TurnChangeFileContent{
		File: models.TurnFileChange{
			ID: id, RepositoryChangeID: repositoryChangeID, Path: path,
			PathBytes: []byte(path), Kind: "modified", ContentAvailability: models.TurnChangeAvailabilityReady,
		},
		CanonicalPatch: patch, FilteredPatch: append([]byte(nil), patch...), OldRendering: old, NewRendering: new,
	}
}

func seedTurnChangeContentOwner(t *testing.T, repo *Repository, suffix string, retainUntil time.Time) (string, string) {
	t.Helper()
	ctx := context.Background()
	taskID, sessionID, turnID := "task-"+suffix, "session-"+suffix, "turn-"+suffix
	envID, envRepoID := "env-"+suffix, "env-repo-"+suffix
	now := time.Now().UTC()
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Turn changes content"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: envID, TaskID: taskID, Status: models.TaskEnvironmentStatusCreating,
		Repos: []*models.TaskEnvironmentRepo{{ID: envRepoID, RepositoryID: "repository-" + suffix}},
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, TaskEnvironmentID: envID}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: turnID, TaskID: taskID, TaskSessionID: sessionID, StartedAt: now}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	changeSetID := "change-set-" + suffix
	changeSet := &models.TurnChangeSet{
		ID: changeSetID, TaskID: taskID, TaskSessionID: sessionID, TurnID: turnID, TaskEnvironmentID: envID,
		CaptureEnabled: true, SettingsUserID: "settings-user", ResolutionKind: "authenticated_user",
		Availability: models.TurnChangeAvailabilityPending,
	}
	if err := repo.CreateTurnChangeSet(ctx, changeSet); err != nil {
		t.Fatalf("create change set: %v", err)
	}
	repositoryChangeID := "repository-change-" + suffix
	start := models.TurnRepositoryChangeSet{
		ID: repositoryChangeID, TurnChangeSetID: changeSetID, CheckoutID: "checkout-" + suffix,
		TaskEnvironmentRepoID: envRepoID, RepositoryID: "repository-" + suffix,
		StartCommitOID: "abc123", StartTreeOID: "tree123", HashAlgorithm: "sha1",
		StartReachabilityRef: "refs/kandev/turn-changes/start/" + suffix,
	}
	accepted, err := repo.AcceptTurnChangeSetStart(ctx, changeSetID, 1, []models.TurnRepositoryChangeSet{start})
	if err != nil || !accepted {
		t.Fatalf("accept change-set start = %t, %v", accepted, err)
	}
	terminalAt := time.Now().UTC()
	end := models.TurnRepositoryChangeSet{
		ID: repositoryChangeID, TurnChangeSetID: changeSetID, CheckoutID: start.CheckoutID,
		EndCommitOID: "def456", EndTreeOID: "tree456", HashAlgorithm: "sha1",
		EndReachabilityRef: "refs/kandev/turn-changes/end/" + suffix, EndCapturedAt: &terminalAt,
	}
	accepted, err = repo.AcceptTurnRepositoryEnd(ctx, changeSetID, repositoryChangeID, start.StartCommitOID, start.StartTreeOID, end)
	if err != nil || !accepted {
		t.Fatalf("accept change-set end = %t, %v", accepted, err)
	}
	accepted, err = repo.FinalizeTurnChangeSet(ctx, changeSetID, 2, models.TurnChangeSetFinalization{
		Availability: models.TurnChangeAvailabilityReady, Complete: true, SummaryComplete: true, ContentComplete: true,
		TerminalAt: terminalAt, RetainUntil: &retainUntil, Repositories: []models.TurnRepositoryChangeSet{{
			ID: repositoryChangeID, TurnChangeSetID: changeSetID, CheckoutID: start.CheckoutID,
			EndCommitOID: end.EndCommitOID, EndTreeOID: end.EndTreeOID,
			EndReachabilityRef: end.EndReachabilityRef, EndCapturedAt: end.EndCapturedAt,
			Availability: models.TurnChangeAvailabilityReady, EnumerationComplete: true, ComparisonComplete: true, ContentComplete: true,
		}},
	})
	if err != nil || !accepted {
		t.Fatalf("finalize change set = %t, %v", accepted, err)
	}
	return changeSetID, repositoryChangeID
}
