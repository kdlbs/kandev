package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestTurnChangeHistoryComparisonAcceptedOnce(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	changeSetID, repositoryChangeID := seedHistoryChangeSet(t, repo, "turn-comparison", "checkout-comparison")
	comparison := models.TurnRepositoryComparison{
		Version: turnchanges.ComparisonVersion2, Basis: turnchanges.ComparisonDirect,
		EffectiveStartCommitOID: "commit-start", EffectiveStartTreeOID: "tree-start",
		EffectiveEndCommitOID: "commit-end", EffectiveEndTreeOID: "tree-end",
		ComparisonRef: "refs/kandev/turn-comparisons/example",
	}
	accepted, err := repo.AcceptTurnRepositoryComparison(ctx, changeSetID, repositoryChangeID, comparison)
	if err != nil || !accepted {
		t.Fatalf("first acceptance = %t, %v", accepted, err)
	}
	replayed, err := repo.AcceptTurnRepositoryComparison(ctx, changeSetID, repositoryChangeID, comparison)
	if err != nil || !replayed {
		t.Fatalf("idempotent acceptance = %t, %v", replayed, err)
	}
	conflicting := comparison
	conflicting.EffectiveStartTreeOID = "different-tree"
	accepted, err = repo.AcceptTurnRepositoryComparison(ctx, changeSetID, repositoryChangeID, conflicting)
	if err != nil || accepted {
		t.Fatalf("conflicting acceptance = %t, %v", accepted, err)
	}
	row, err := repo.GetTurnRepositoryChange(ctx, changeSetID, repositoryChangeID)
	if err != nil {
		t.Fatal(err)
	}
	if row.ComparisonBasis != turnchanges.ComparisonDirect || row.EffectiveStartTreeOID != "tree-start" {
		t.Fatalf("accepted comparison row = %+v", row)
	}
}

func TestTurnChangeWriterIntervalsUseCanonicalTurnCompletion(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	changeSetID, repositoryChangeID := seedHistoryChangeSetAt(t, repo, "turn-interval", "checkout-interval", base, base.Add(10*time.Minute))
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE turn_repository_changes SET start_captured_at = ? WHERE id = ?
	`), base, repositoryChangeID); err != nil {
		t.Fatal(err)
	}
	intervals, err := repo.ListTurnChangeWriterIntervals(ctx, models.TurnChangeCheckoutIdentity{
		CheckoutID: "checkout-interval",
	}, "", 10)
	if err != nil || len(intervals) != 1 {
		t.Fatalf("intervals = %+v, %v", intervals, err)
	}
	interval := intervals[0]
	if !interval.StartCapturedAt.Equal(base) || interval.TurnCompletedAt == nil ||
		!interval.TurnCompletedAt.Equal(base.Add(10*time.Minute)) || interval.ChangeSetID != changeSetID {
		t.Fatalf("writer interval = %+v", interval)
	}
}

func TestTurnChangeExportClaimTakeoverRejectsLatePage(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	changeSetID, repositoryChangeID := seedHistoryChangeSetAt(t, repo, "turn-export-claim", "checkout-export-claim", now, now)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE turn_change_sets SET availability = 'ready' WHERE id = ?`), changeSetID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE turn_repository_changes
		   SET availability = 'ready', start_captured_at = ?, end_captured_at = ?, end_commit_oid = 'commit-end',
		       end_tree_oid = 'tree-end', end_ref = 'refs/kandev/turn-changes/example', comparison_basis = 'legacy_raw',
		       export_state = 'queued'
		 WHERE id = ?
	`), now, now, repositoryChangeID); err != nil {
		t.Fatal(err)
	}
	first, claimed, err := repo.BeginTurnChangeExportAttempt(ctx, repositoryChangeID, now, time.Minute, 3)
	if err != nil || !claimed {
		t.Fatalf("first claim = %t, %v", claimed, err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE turn_repository_changes SET export_claim_expires_at = ? WHERE id = ?
	`), now.Add(-time.Minute), repositoryChangeID); err != nil {
		t.Fatal(err)
	}
	second, claimed, err := repo.BeginTurnChangeExportAttempt(ctx, repositoryChangeID, now, time.Minute, 3)
	if err != nil || !claimed || second.Token <= first.Token {
		t.Fatalf("takeover claim = %+v, %t, %v", second, claimed, err)
	}
	receipt, stored, err := repo.StoreTurnChangeExportPage(ctx, *first, nil, models.TurnChangeExportPage{
		NextCursor: 1, Retry: true, Reason: models.TurnChangeReasonExportTimeout,
	})
	if err != nil || stored || receipt.Complete {
		t.Fatalf("late page = %+v, %t, %v; want a lost claim", receipt, stored, err)
	}
	_, stored, err = repo.StoreTurnChangeExportPage(ctx, *second, nil, models.TurnChangeExportPage{
		NextCursor: 1, Complete: true,
	})
	if err != nil || !stored {
		t.Fatalf("current claim page = %t, %v", stored, err)
	}
	row, err := repo.GetTurnRepositoryChange(ctx, changeSetID, repositoryChangeID)
	if err != nil || row.ExportState != models.TurnChangeExportComplete || row.ExportCursor != 1 {
		t.Fatalf("export progress = %+v, %v", row, err)
	}
	if err := repo.SettleTurnChangeContentProgress(ctx, changeSetID); err != nil {
		t.Fatalf("settle content progress: %v", err)
	}
}

func seedHistoryChangeSet(t *testing.T, repo *Repository, turnID, checkoutID string) (string, string) {
	t.Helper()
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	return seedHistoryChangeSetAt(t, repo, turnID, checkoutID, base, base)
}

func seedHistoryChangeSetAt(
	t *testing.T,
	repo *Repository,
	turnID, checkoutID string,
	startedAt, completedAt time.Time,
) (string, string) {
	t.Helper()
	ctx := context.Background()
	taskID := "task-" + turnID
	envID := "env-" + turnID
	sessionID := "session-" + turnID
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "History"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: envID, TaskID: taskID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, TaskEnvironmentID: envID}); err != nil {
		t.Fatal(err)
	}
	completed := completedAt
	if completed.IsZero() {
		completed = startedAt
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: turnID, TaskID: taskID, TaskSessionID: sessionID, StartedAt: startedAt, CompletedAt: &completed,
	}); err != nil {
		t.Fatal(err)
	}
	changeSetID := "set-" + turnID
	changeSet := &models.TurnChangeSet{
		ID: changeSetID, TaskID: taskID, TaskSessionID: sessionID, TurnID: turnID, TaskEnvironmentID: envID,
		CaptureEnabled: true, SettingsUserID: "user", ResolutionKind: turnchanges.PolicyAuthenticatedUser,
		Availability: models.TurnChangeAvailabilityPending,
	}
	if err := repo.CreateTurnChangeSet(ctx, changeSet); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE turn_change_sets SET start_accepted = ?, terminal_at = ? WHERE id = ?
	`), true, completed, changeSetID); err != nil {
		t.Fatal(err)
	}
	repositoryChangeID := "repo-" + turnID
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO turn_repository_changes (
			id, change_set_id, checkout_id, start_commit_oid, start_tree_oid, end_commit_oid, end_tree_oid,
			hash_algorithm, availability, created_at, updated_at
		) VALUES (?, ?, ?, 'commit-start', 'tree-start', 'commit-end', 'tree-end', 'sha1', 'pending', ?, ?)
	`), repositoryChangeID, changeSetID, checkoutID, startedAt, startedAt); err != nil {
		t.Fatal(err)
	}
	return changeSetID, repositoryChangeID
}

func TestTurnChangeProvenanceMigrationReplayLabelsLegacyOnce(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	changeSetID, repositoryChangeID := seedHistoryChangeSetAt(t, repo, "turn-legacy", "checkout-legacy", base, base)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE turn_repository_changes
		   SET availability = 'ready', start_captured_at = ?, comparison_basis = '',
		       effective_start_commit_oid = '', effective_end_tree_oid = ''
		 WHERE id = ?
	`), base, repositoryChangeID); err != nil {
		t.Fatal(err)
	}
	if err := repo.initTurnChangesSchema(); err != nil {
		t.Fatalf("replay schema: %v", err)
	}
	if err := repo.migrateTurnChangeHistorySchema(); err != nil {
		t.Fatalf("replay history migration: %v", err)
	}
	row, err := repo.GetTurnRepositoryChange(ctx, changeSetID, repositoryChangeID)
	if err != nil {
		t.Fatal(err)
	}
	if row.ComparisonBasis != turnchanges.ComparisonLegacyRaw ||
		row.EffectiveStartCommitOID != "commit-start" || row.EffectiveEndTreeOID != "tree-end" {
		t.Fatalf("legacy comparison backfill = %+v", row)
	}
	if row.StartCommitOID != "commit-start" || row.EndCommitOID != "commit-end" {
		t.Fatalf("legacy raw endpoints were rewritten: %+v", row)
	}
	if err := repo.initTurnChangesSchema(); err != nil {
		t.Fatalf("second replay: %v", err)
	}
	if err := repo.migrateTurnChangeHistorySchema(); err != nil {
		t.Fatalf("second replay history migration: %v", err)
	}
	replayed, err := repo.GetTurnRepositoryChange(ctx, changeSetID, repositoryChangeID)
	if err != nil || replayed.ComparisonBasis != turnchanges.ComparisonLegacyRaw {
		t.Fatalf("replayed legacy row = %+v, %v", replayed, err)
	}
}

func TestTurnChangeProvenancePersistsThroughEndpointAcceptance(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	changeSetID, repositoryChangeID := seedHistoryChangeSetAt(t, repo, "turn-provenance", "checkout-provenance", base, base)
	startProvenance := `{"version":2,"change_set_id":"set-turn-provenance","checkout_id":"checkout-provenance","boundary":"start"}`
	endProvenance := `{"version":2,"change_set_id":"set-turn-provenance","checkout_id":"checkout-provenance","boundary":"end"}`
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE turn_repository_changes SET start_provenance_json = ? WHERE id = ?
	`), startProvenance, repositoryChangeID); err != nil {
		t.Fatal(err)
	}
	accepted, err := repo.AcceptTurnRepositoryEnd(ctx, changeSetID, repositoryChangeID, "commit-start", "tree-start",
		models.TurnRepositoryChangeSet{
			EndCommitOID: "commit-end", EndTreeOID: "tree-end", HashAlgorithm: "sha1",
			EndReachabilityRef: "refs/kandev/turn-changes/end", EndCapturedAt: &base,
			EndProvenanceJSON: endProvenance,
		})
	if err != nil || !accepted {
		t.Fatalf("accept end = %t, %v", accepted, err)
	}
	row, err := repo.GetTurnRepositoryChange(ctx, changeSetID, repositoryChangeID)
	if err != nil || row.StartProvenanceJSON != startProvenance || row.EndProvenanceJSON != endProvenance {
		t.Fatalf("stored provenance = %+v, %v", row, err)
	}
}

func TestTurnChangeLegacyComparisonAcceptanceUsesRawPair(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	changeSetID, repositoryChangeID := seedHistoryChangeSet(t, repo, "turn-legacy-comparison", "checkout-legacy-comparison")
	accepted, err := repo.AcceptTurnRepositoryComparison(ctx, changeSetID, repositoryChangeID, models.TurnRepositoryComparison{
		Version: turnchanges.ComparisonVersionLegacy, Basis: turnchanges.ComparisonLegacyRaw,
		EffectiveStartCommitOID: "commit-start", EffectiveStartTreeOID: "tree-start",
		EffectiveEndCommitOID: "commit-end", EffectiveEndTreeOID: "tree-end",
	})
	if err != nil || !accepted {
		t.Fatalf("legacy acceptance = %t, %v", accepted, err)
	}
	row, err := repo.GetTurnRepositoryChange(ctx, changeSetID, repositoryChangeID)
	if err != nil || row.ComparisonBasis != turnchanges.ComparisonLegacyRaw ||
		row.EffectiveStartTreeOID != "tree-start" || row.EffectiveEndTreeOID != "tree-end" {
		t.Fatalf("legacy comparison row = %+v, %v", row, err)
	}
}

func TestTurnChangeUpgradeAddsHistoryColumnsBeforeExportIndex(t *testing.T) {
	dbPath := t.TempDir() + "/legacy-turn-change.db"
	conn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	store := sqlx.NewDb(conn, "sqlite3")
	repo, err := NewWithDB(store, store, nil)
	if err != nil {
		t.Fatalf("create current schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	changeSetID, repositoryChangeID := seedHistoryChangeSetAt(t, repo, "turn-upgrade", "checkout-upgrade", base, base)
	_ = changeSetID
	if _, err := repo.db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_turn_repository_changes_export`); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{
		"start_provenance_json", "end_provenance_json", "comparison_version", "comparison_basis",
		"comparison_reason", "effective_start_commit_oid", "effective_start_tree_oid",
		"effective_end_commit_oid", "effective_end_tree_oid", "comparison_ref", "export_state",
		"export_cursor", "export_attempts", "export_next_attempt_at", "export_bytes", "export_reason",
		"export_claim_owner", "export_claim_token", "export_claim_expires_at", "export_fingerprint",
	} {
		if _, err := repo.db.ExecContext(ctx, `ALTER TABLE turn_repository_changes DROP COLUMN `+column); err != nil {
			t.Fatalf("drop legacy column %s: %v", column, err)
		}
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("upgrade schema: %v", err)
	}
	upgraded := repo
	row, err := upgraded.GetTurnRepositoryChange(ctx, "set-turn-upgrade", repositoryChangeID)
	if err != nil {
		t.Fatalf("read upgraded row: %v", err)
	}
	if row.StartCommitOID != "commit-start" || row.EndCommitOID != "commit-end" {
		t.Fatalf("upgrade rewrote endpoints: %+v", row)
	}
	if row.ComparisonBasis != turnchanges.ComparisonLegacyRaw {
		t.Fatalf("upgrade did not label the legacy row: %+v", row)
	}
	var indexCount int
	if err := upgraded.db.GetContext(ctx, &indexCount, `SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_turn_repository_changes_export'`); err != nil || indexCount != 1 {
		t.Fatalf("export index after upgrade = %d, %v", indexCount, err)
	}
}

func TestTurnChangeExportResumesPastPendingInventory(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	changeSetID, repositoryChangeID := seedHistoryChangeSetAt(t, repo, "turn-paged", "checkout-paged", base, base)
	seedReadyExportRow(t, repo, changeSetID, repositoryChangeID, base)
	seedPendingExportFiles(t, repo, repositoryChangeID, "checkout-paged", 30)
	claim := beginExportClaim(t, repo, repositoryChangeID, base)

	first := models.TurnChangeExportPage{NextCursor: 10, KeepClaim: true, Fingerprint: "fingerprint", BatchBytes: 100}
	receipt, stored, err := repo.StoreTurnChangeExportPage(ctx, *claim, readyExportFiles(0, 10, repositoryChangeID, "checkout-paged"), first)
	if err != nil || !stored || !receipt.Pending || receipt.Complete {
		t.Fatalf("first page = %+v, stored=%t, err=%v", receipt, stored, err)
	}
	row, _ := repo.GetTurnRepositoryChange(ctx, changeSetID, repositoryChangeID)
	if row.ContentComplete || row.ExportCursor != 10 || row.ExportState != models.TurnChangeExportRunning {
		t.Fatalf("first page progress = %+v", row)
	}

	timeoutPrefix := models.TurnChangeExportPage{
		NextCursor: 20, Retry: true, Reason: models.TurnChangeReasonExportTimeout,
		Fingerprint: "fingerprint", BatchBytes: 100,
	}
	receipt, stored, err = repo.StoreTurnChangeExportPage(ctx, *claim, readyExportFiles(10, 20, repositoryChangeID, "checkout-paged"), timeoutPrefix)
	if err != nil || !stored || !receipt.Pending || receipt.Complete {
		t.Fatalf("timeout prefix = %+v, stored=%t, err=%v", receipt, stored, err)
	}
	row, _ = repo.GetTurnRepositoryChange(ctx, changeSetID, repositoryChangeID)
	if row.ExportState != models.TurnChangeExportRetrying || row.ExportCursor != 20 {
		t.Fatalf("timeout prefix progress = %+v", row)
	}

	claim = beginExportClaim(t, repo, repositoryChangeID, base)
	zeroProgress := models.TurnChangeExportPage{
		NextCursor: 20, Retry: true, Reason: models.TurnChangeReasonExportTimeout,
		Fingerprint: "fingerprint",
	}
	receipt, stored, err = repo.StoreTurnChangeExportPage(ctx, *claim, nil, zeroProgress)
	if err != nil || !stored || !receipt.Pending {
		t.Fatalf("zero-progress page = %+v, stored=%t, err=%v", receipt, stored, err)
	}
	claim = beginExportClaim(t, repo, repositoryChangeID, base)

	final := models.TurnChangeExportPage{NextCursor: 30, Complete: true, Fingerprint: "fingerprint", BatchBytes: 100}
	receipt, stored, err = repo.StoreTurnChangeExportPage(ctx, *claim, readyExportFiles(20, 30, repositoryChangeID, "checkout-paged"), final)
	if err != nil || !stored || !receipt.Complete || receipt.Pending {
		t.Fatalf("final page = %+v, stored=%t, err=%v", receipt, stored, err)
	}
	row, _ = repo.GetTurnRepositoryChange(ctx, changeSetID, repositoryChangeID)
	if !row.ContentComplete || row.ExportState != models.TurnChangeExportComplete {
		t.Fatalf("final progress = %+v", row)
	}
	payload, err := repo.ReadTurnChangeContent(ctx, changeSetID, fileIDFor(0), models.TurnChangeContentCanonicalPatch)
	if err != nil || payload == nil || string(payload.Content) != "patch-"+fileIDFor(0) {
		t.Fatalf("saved content = %+v, %v", payload, err)
	}
}

func TestTurnChangeExportFailureSettlesRemainingPendingFiles(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	changeSetID, repositoryChangeID := seedHistoryChangeSetAt(t, repo, "turn-paged-fail", "checkout-paged-fail", base, base)
	seedReadyExportRow(t, repo, changeSetID, repositoryChangeID, base)
	seedPendingExportFiles(t, repo, repositoryChangeID, "checkout-paged-fail", 3)
	claim := beginExportClaim(t, repo, repositoryChangeID, base)
	_, stored, err := repo.StoreTurnChangeExportPage(ctx, *claim, readyExportFiles(0, 1, repositoryChangeID, "checkout-paged-fail"),
		models.TurnChangeExportPage{NextCursor: 1, KeepClaim: true, BatchBytes: 10})
	if err != nil || !stored {
		t.Fatalf("prefix page = %t, %v", stored, err)
	}
	if err := repo.SettleTurnRepositoryExportFailure(ctx, repositoryChangeID, claim.Token, models.TurnChangeReasonExportTimeout); err != nil {
		t.Fatal(err)
	}
	row, _ := repo.GetTurnRepositoryChange(ctx, changeSetID, repositoryChangeID)
	if row.ExportState != models.TurnChangeExportUnavailable || row.ContentComplete {
		t.Fatalf("failed export row = %+v", row)
	}
	files, total, err := repo.ListTurnFileChanges(ctx, changeSetID, repositoryChangeID, 0, 10)
	if err != nil || total != 3 {
		t.Fatalf("files = %d, %v", total, err)
	}
	for _, file := range files {
		if file.ID == fileIDFor(0) {
			if file.ContentAvailability != models.TurnChangeAvailabilityReady {
				t.Fatalf("acknowledged file was changed: %+v", file)
			}
		} else if file.ContentAvailability != models.TurnChangeAvailabilityUnavailable ||
			file.ContentReason != models.TurnChangeReasonExportTimeout {
			t.Fatalf("pending file was not settled: %+v", file)
		}
	}
}

func seedReadyExportRow(t *testing.T, repo *Repository, changeSetID, repositoryChangeID string, at time.Time) {
	t.Helper()
	if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(`
		UPDATE turn_change_sets SET summary_complete = ?, terminal_at = ?, start_accepted = ?, availability = 'ready' WHERE id = ?
	`), true, at, true, changeSetID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(`
		UPDATE turn_repository_changes
		   SET availability = 'ready', end_commit_oid = 'commit-end', end_tree_oid = 'tree-end',
		       end_ref = 'refs/kandev/turn-changes/end', hash_algorithm = 'sha1',
		       comparison_basis = 'legacy_raw', comparison_version = 0, export_state = 'queued'
		 WHERE id = ?
	`), repositoryChangeID); err != nil {
		t.Fatal(err)
	}
}

func seedPendingExportFiles(t *testing.T, repo *Repository, repositoryChangeID, checkoutID string, count int) {
	t.Helper()
	files := make([]models.TurnChangeFileContent, 0, count)
	one := int64(1)
	for index := 0; index < count; index++ {
		path := fileIDFor(index)
		files = append(files, models.TurnChangeFileContent{File: models.TurnFileChange{
			ID: path, RepositoryChangeID: repositoryChangeID, CheckoutID: checkoutID,
			Path: path + ".txt", PathBytes: []byte(path + ".txt"), Kind: "modified",
			AddedLines: &one, DeletedLines: &one, ContentAvailability: models.TurnChangeAvailabilityPending,
		}})
	}
	if err := repo.StoreTurnChangeFiles(context.Background(), repositoryChangeID, files); err != nil {
		t.Fatal(err)
	}
}

func beginExportClaim(t *testing.T, repo *Repository, repositoryChangeID string, at time.Time) *models.TurnChangeExportClaim {
	t.Helper()
	claim, ok, err := repo.BeginTurnChangeExportAttempt(context.Background(), repositoryChangeID, at, time.Minute, 3)
	if err != nil || !ok {
		t.Fatalf("begin claim = %t, %v", ok, err)
	}
	return claim
}

func readyExportFiles(start, end int, repositoryChangeID, checkoutID string) []models.TurnChangeFileContent {
	files := make([]models.TurnChangeFileContent, 0, end-start)
	one := int64(1)
	for index := start; index < end; index++ {
		id := fileIDFor(index)
		files = append(files, models.TurnChangeFileContent{
			File: models.TurnFileChange{
				ID: id, RepositoryChangeID: repositoryChangeID, CheckoutID: checkoutID,
				Path: id + ".txt", PathBytes: []byte(id + ".txt"), Kind: "modified",
				AddedLines: &one, DeletedLines: &one,
				ContentAvailability: models.TurnChangeAvailabilityReady,
			},
			CanonicalPatch: []byte("patch-" + id), FilteredPatch: []byte("patch-" + id),
		})
	}
	return files
}

func fileIDFor(index int) string {
	return "file-" + string(rune('a'+index/26)) + string(rune('a'+index%26))
}

func TestTurnChangeExportLateSuccessAndErrorLoseToTakeover(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	changeSetID, repositoryChangeID := seedHistoryChangeSetAt(t, repo, "turn-takeover", "checkout-takeover", base, base)
	seedReadyExportRow(t, repo, changeSetID, repositoryChangeID, base)
	seedPendingExportFiles(t, repo, repositoryChangeID, "checkout-takeover", 2)
	first := beginExportClaim(t, repo, repositoryChangeID, base)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE turn_repository_changes SET export_claim_expires_at = ? WHERE id = ?
	`), base.Add(-time.Minute), repositoryChangeID); err != nil {
		t.Fatal(err)
	}
	second := beginExportClaim(t, repo, repositoryChangeID, base)
	if second.Token <= first.Token {
		t.Fatalf("takeover token = %d after %d", second.Token, first.Token)
	}
	_, stored, err := repo.StoreTurnChangeExportPage(ctx, *first, readyExportFiles(0, 1, repositoryChangeID, "checkout-takeover"),
		models.TurnChangeExportPage{NextCursor: 1, KeepClaim: true})
	if err != nil || stored {
		t.Fatalf("late success stored=%t err=%v", stored, err)
	}
	if err := repo.SettleTurnRepositoryExportFailure(ctx, repositoryChangeID, first.Token, models.TurnChangeReasonExportTimeout); err != nil {
		t.Fatal(err)
	}
	row, _ := repo.GetTurnRepositoryChange(ctx, changeSetID, repositoryChangeID)
	if row.ExportState != models.TurnChangeExportRunning || row.ExportClaimToken != second.Token {
		t.Fatalf("late error changed successor = %+v", row)
	}
	receipt, stored, err := repo.StoreTurnChangeExportPage(ctx, *second, readyExportFiles(0, 2, repositoryChangeID, "checkout-takeover"),
		models.TurnChangeExportPage{NextCursor: 2, Complete: true})
	if err != nil || !stored || !receipt.Complete {
		t.Fatalf("successor page = %+v stored=%t err=%v", receipt, stored, err)
	}
}

func TestTurnChangeRetentionInvalidatesActiveExportClaim(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	changeSetID, repositoryChangeID := seedHistoryChangeSetAt(t, repo, "turn-retention-export", "checkout-retention-export", base, base)
	seedReadyExportRow(t, repo, changeSetID, repositoryChangeID, base)
	seedPendingExportFiles(t, repo, repositoryChangeID, "checkout-retention-export", 2)
	claim := beginExportClaim(t, repo, repositoryChangeID, base)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE turn_change_sets SET retain_until = ? WHERE id = ?
	`), base.Add(-time.Hour), changeSetID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApplyTurnChangeRetention(ctx, models.TurnChangeRetentionPolicy{
		RetainFor: 24 * time.Hour, TaskBytes: 1 << 20, InstallationBytes: 1 << 20,
	}, base.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	row, _ := repo.GetTurnRepositoryChange(ctx, changeSetID, repositoryChangeID)
	if row.ExportState != models.TurnChangeExportUnavailable || row.ContentComplete {
		t.Fatalf("expired export row = %+v", row)
	}
	_, stored, err := repo.StoreTurnChangeExportPage(ctx, *claim, readyExportFiles(0, 1, repositoryChangeID, "checkout-retention-export"),
		models.TurnChangeExportPage{NextCursor: 1, KeepClaim: true})
	if err != nil || stored {
		t.Fatalf("page after retention stored=%t err=%v", stored, err)
	}
}

func TestTurnChangeExportAttemptsPersistAcrossResumes(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	changeSetID, repositoryChangeID := seedHistoryChangeSetAt(t, repo, "turn-attempts", "checkout-attempts", base, base)
	seedReadyExportRow(t, repo, changeSetID, repositoryChangeID, base)
	seedPendingExportFiles(t, repo, repositoryChangeID, "checkout-attempts", 1)
	now := time.Now().UTC()
	for attempt := 0; attempt < 3; attempt++ {
		claim := beginExportClaim(t, repo, repositoryChangeID, now)
		if err := repo.ReleaseTurnChangeExportClaim(ctx, *claim); err != nil {
			t.Fatal(err)
		}
		now = time.Now().UTC().Add(time.Second)
	}
	if _, ok, err := repo.BeginTurnChangeExportAttempt(ctx, repositoryChangeID, base, time.Minute, 3); err != nil || ok {
		t.Fatalf("fourth attempt = %t, %v; want exhausted", ok, err)
	}
}

func TestPostgresTurnChangeUpgradeAddsHistoryColumnsBeforeExportIndex(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("create postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	changeSetID, repositoryChangeID := seedHistoryChangeSetAt(t, repo, "turn-pg-upgrade", "checkout-pg-upgrade", base, base)
	if _, err := repo.db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_turn_repository_changes_export`); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{
		"start_provenance_json", "end_provenance_json", "comparison_version", "comparison_basis",
		"comparison_reason", "effective_start_commit_oid", "effective_start_tree_oid",
		"effective_end_commit_oid", "effective_end_tree_oid", "comparison_ref", "export_state",
		"export_cursor", "export_attempts", "export_next_attempt_at", "export_bytes", "export_reason",
		"export_claim_owner", "export_claim_token", "export_claim_expires_at", "export_fingerprint",
	} {
		if _, err := repo.db.ExecContext(ctx, `ALTER TABLE turn_repository_changes DROP COLUMN `+column); err != nil {
			t.Fatalf("drop postgres column %s: %v", column, err)
		}
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("upgrade postgres schema: %v", err)
	}
	row, err := repo.GetTurnRepositoryChange(ctx, changeSetID, repositoryChangeID)
	if err != nil || row.ComparisonBasis != turnchanges.ComparisonLegacyRaw {
		t.Fatalf("postgres upgraded row = %+v, %v", row, err)
	}
}

func TestPostgresTurnChangeExportLatePageLosesToTakeover(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("create postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	changeSetID, repositoryChangeID := seedHistoryChangeSetAt(t, repo, "turn-pg-takeover", "checkout-pg-takeover", base, base)
	seedReadyExportRow(t, repo, changeSetID, repositoryChangeID, base)
	seedPendingExportFiles(t, repo, repositoryChangeID, "checkout-pg-takeover", 2)
	first := beginExportClaim(t, repo, repositoryChangeID, base)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE turn_repository_changes SET export_claim_expires_at = ? WHERE id = ?
	`), base.Add(-time.Minute), repositoryChangeID); err != nil {
		t.Fatal(err)
	}
	second := beginExportClaim(t, repo, repositoryChangeID, base)
	_, stored, err := repo.StoreTurnChangeExportPage(ctx, *first,
		readyExportFiles(0, 1, repositoryChangeID, "checkout-pg-takeover"),
		models.TurnChangeExportPage{NextCursor: 1, KeepClaim: true})
	if err != nil || stored {
		t.Fatalf("postgres late page stored=%t err=%v", stored, err)
	}
	if err := repo.SettleTurnRepositoryExportFailure(ctx, repositoryChangeID, first.Token, models.TurnChangeReasonExportTimeout); err != nil {
		t.Fatal(err)
	}
	row, _ := repo.GetTurnRepositoryChange(ctx, changeSetID, repositoryChangeID)
	if row.ExportClaimToken != second.Token || row.ExportState != models.TurnChangeExportRunning {
		t.Fatalf("postgres successor claim changed: %+v", row)
	}
}
