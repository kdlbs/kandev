package changes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
	_ "github.com/mattn/go-sqlite3"
)

func TestHistoryCompactionIncludesHistoryOnlyTasks(t *testing.T) {
	repository := &fakeHistoryRepository{
		candidates: []*models.TurnHistoryRetentionCandidate{
			{ChangeSetID: "set-only-history", TaskID: "task-history-only", LegacySQLBytes: 1024, PreservationBytes: 2048},
			{ChangeSetID: "set-mixed", TaskID: "task-mixed", LegacySQLBytes: 512},
		},
	}
	service := NewHistoryService(repository, nil, nil)
	inventory, err := service.Analyze(context.Background(), time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatalf("analyze history: %v", err)
	}
	if inventory.Tasks != 2 || inventory.Snapshots != 2 || inventory.LegacySQLBytes != 1536 || inventory.ArchiveBytes != 2048 {
		t.Fatalf("history inventory = %+v", inventory)
	}
	if len(repository.analyzedCutoffs) != 1 {
		t.Fatal("analysis did not use one shared cutoff")
	}
	result, err := service.CompactOnce(context.Background(), time.Now().UTC().Add(-time.Hour), models.TurnChangeReasonExpiredAge, 200)
	if err != nil {
		t.Fatalf("compact history: %v", err)
	}
	if result.ExpiredChangeSets != 2 || len(repository.expiredIDs) != 2 {
		t.Fatalf("history expiry = %+v ids=%v", result, repository.expiredIDs)
	}
}

func TestHistoryCompactionResumesOwnedRefCleanup(t *testing.T) {
	archive, _, _, _ := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	receipt, err := store.Import(context.Background(), "repo-cleanup", "checkout-cleanup", archive)
	if err != nil {
		t.Fatalf("import archive: %v", err)
	}
	encoded, _ := json.Marshal(receipt)
	row := &models.TurnRepositoryChangeSet{
		ID: uuid.NewString(), TurnChangeSetID: "set-cleanup", ContentSource: models.TurnChangeSourceGitArchive,
		PreservationState: models.TurnChangePreservationReady, PreservationReceipt: string(encoded),
	}
	repository := &fakeHistoryRepository{expiredRows: []*models.TurnRepositoryChangeSet{row}}
	service := NewHistoryService(repository, store, nil)
	result, err := service.CompactOnce(context.Background(), time.Now().UTC(), models.TurnChangeReasonExpiredAge, 200)
	if err != nil {
		t.Fatalf("compact archived history: %v", err)
	}
	if result.DeletedArchiveRefs != 1 || repository.clearedReceipts != 1 {
		t.Fatalf("owned ref cleanup = %+v cleared=%d", result, repository.clearedReceipts)
	}
	if err := store.VerifyRefs(context.Background(), receipt); !errors.Is(err, ErrHistoryContentUnavailable) {
		t.Fatalf("refs survived acknowledged cleanup: %v", err)
	}
}

func TestHistoryCompactionProtectsLeasedSources(t *testing.T) {
	repository := &fakeHistoryRepository{
		candidates: []*models.TurnHistoryRetentionCandidate{{ChangeSetID: "set-leased", TaskID: "task-leased"}},
		leased:     true,
	}
	service := NewHistoryService(repository, nil, nil)
	result, err := service.CompactOnce(context.Background(), time.Now().UTC(), models.TurnChangeReasonExpiredAge, 200)
	if err != nil {
		t.Fatalf("compact leased history: %v", err)
	}
	if result.ExpiredChangeSets != 0 || len(repository.expiredIDs) != 0 {
		t.Fatalf("leased history was expired: %+v", result)
	}
}

type fakeHistoryRepository struct {
	candidates      []*models.TurnHistoryRetentionCandidate
	preserved       []*models.TurnRepositoryChangeSet
	expiredRows     []*models.TurnRepositoryChangeSet
	expiredIDs      []string
	clearedReceipts int
	leased          bool
	analyzedCutoffs []time.Time
	expireErr       error
	cleanupListErr  error
}

func (f *fakeHistoryRepository) ListTurnHistoryRetentionCandidates(
	_ context.Context, cutoff, _ time.Time, limit int,
) ([]*models.TurnHistoryRetentionCandidate, error) {
	f.analyzedCutoffs = append(f.analyzedCutoffs, cutoff)
	if f.leased {
		return nil, nil
	}
	if len(f.candidates) > limit {
		return f.candidates[:limit], nil
	}
	return f.candidates, nil
}

func (f *fakeHistoryRepository) ExpireTurnHistory(
	_ context.Context, ids []string, _ time.Time, _ models.TurnChangeReason, _ time.Time,
) (models.TurnHistoryExpiryResult, error) {
	if f.expireErr != nil {
		return models.TurnHistoryExpiryResult{}, f.expireErr
	}
	f.expiredIDs = append(f.expiredIDs, ids...)
	return models.TurnHistoryExpiryResult{ExpiredChangeSets: int64(len(ids))}, nil
}

func (f *fakeHistoryRepository) ListTurnHistoryRetentionCandidatesPage(
	ctx context.Context, cutoff, now time.Time, afterTaskID string, limit int,
) ([]*models.TurnHistoryRetentionCandidate, string, error) {
	rows, err := f.ListTurnHistoryRetentionCandidates(ctx, cutoff, now, limit)
	return rows, "", err
}

func (f *fakeHistoryRepository) ListTurnRepositoryPreservationReady(context.Context, int) ([]*models.TurnRepositoryChangeSet, error) {
	return f.preserved, nil
}

func (f *fakeHistoryRepository) ListTurnRepositoryPreservationReadyPage(context.Context, string, int) ([]*models.TurnRepositoryChangeSet, string, error) {
	return f.preserved, "", nil
}

func (f *fakeHistoryRepository) ListExpiredTurnHistoryRepositories(context.Context, int) ([]*models.TurnRepositoryChangeSet, error) {
	if f.cleanupListErr != nil {
		return nil, f.cleanupListErr
	}
	return f.expiredRows, nil
}

func (f *fakeHistoryRepository) ClearTurnRepositoryPreservationReceipt(context.Context, string) error {
	f.clearedReceipts++
	f.expiredRows = nil
	return nil
}

func TestHistoryCompactionBackupRestoresGitContent(t *testing.T) {
	archive, _, startTree, endTree := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	receipt, err := store.Import(context.Background(), "repo-backup", "checkout-backup", archive)
	if err != nil {
		t.Fatalf("import archive: %v", err)
	}
	encoded, _ := json.Marshal(receipt)
	row := &models.TurnRepositoryChangeSet{
		ID: uuid.NewString(), TurnChangeSetID: archive.ChangeSetID, CheckoutID: archive.CheckoutID,
		RepositoryID: "repo-backup", HashAlgorithm: "sha1",
		StartCommitOID: archive.StartCommitOID, StartTreeOID: startTree,
		EndCommitOID: archive.EndCommitOID, EndTreeOID: endTree,
		ContentSource: models.TurnChangeSourceGitArchive, PreservationState: models.TurnChangePreservationReady,
		PreservationReceipt: string(encoded),
	}
	repository := &fakeHistoryRepository{preserved: []*models.TurnRepositoryChangeSet{row}}
	service := NewHistoryService(repository, store, nil)
	snapshotPath := filepath.Join(t.TempDir(), "manual-test.db")
	writeHistoryBackupSnapshot(t, snapshotPath, row)
	digest, err := sha256File(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.PublishRetentionBackup(context.Background(), snapshotPath, digest); err != nil {
		t.Fatalf("publish retention backup: %v", err)
	}
	if err := service.VerifyRetentionBackup(context.Background(), snapshotPath, digest); err != nil {
		t.Fatalf("verify retention backup: %v", err)
	}
	if err := store.DeleteRefs(context.Background(), receipt); err != nil {
		t.Fatalf("simulate source loss: %v", err)
	}
	if err := service.RestoreRetentionBackup(context.Background(), snapshotPath); err != nil {
		t.Fatalf("restore retention backup: %v", err)
	}
	if err := store.VerifyRefs(context.Background(), receipt); err != nil {
		t.Fatalf("restored refs: %v", err)
	}
	patch, err := store.ReadPatch(context.Background(), receipt,
		turnchanges.CheckpointArchive{StartTreeOID: startTree, EndTreeOID: endTree},
		HistoricalFile{PathBytes: []byte("src/app.txt")}, false)
	if err != nil || !strings.Contains(string(patch), "+new") {
		t.Fatalf("historical patch after restore = %q, %v", patch, err)
	}
}

func TestHistoryCompactionBackupRequiresSourceCoverage(t *testing.T) {
	store := NewGitHistoryStore(t.TempDir(), nil)
	repository := &fakeHistoryRepository{preserved: []*models.TurnRepositoryChangeSet{{
		ID: uuid.NewString(), TurnChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(),
		ContentSource: models.TurnChangeSourceGitArchive, PreservationState: models.TurnChangePreservationReady,
		PreservationReceipt: `{"source_id":"repo","hash_algorithm":"sha1","start_commit_oid":"a","end_commit_oid":"b"}`,
	}}}
	service := NewHistoryService(repository, store, nil)
	snapshotPath := filepath.Join(t.TempDir(), "manual-legacy.db")
	writeHistoryBackupSnapshot(t, snapshotPath, repository.preserved[0])
	digest, err := sha256File(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.VerifyRetentionBackup(context.Background(), snapshotPath, digest); err == nil {
		t.Fatal("database-only backup claimed protection for retained checkpoint sources")
	}
}

func TestHistoryCompactionAccountsOnlyCommittedDeltasAcrossBatches(t *testing.T) {
	repository := newBatchHistoryRepository(300)
	service := NewHistoryService(repository, nil, nil)
	cutoff := time.Now().UTC().Add(-time.Hour)
	first, err := service.CompactOnce(context.Background(), cutoff, models.TurnChangeReasonExpiredAge, 200)
	if err != nil {
		t.Fatalf("first history batch: %v", err)
	}
	if first.ExpiredChangeSets != 200 || first.RemainingCandidates == 0 {
		t.Fatalf("first batch = %+v", first)
	}
	second, err := service.CompactOnce(context.Background(), cutoff, models.TurnChangeReasonExpiredAge, 200)
	if err != nil {
		t.Fatalf("second history batch: %v", err)
	}
	if second.ExpiredChangeSets != 100 || second.RemainingCandidates != 0 {
		t.Fatalf("second batch = %+v", second)
	}
	if repository.expiredCount() != 300 {
		t.Fatalf("expired %d intervals, want 300", repository.expiredCount())
	}
}

func TestHistoryCompactionDrainsAllOwnedRefReceipts(t *testing.T) {
	store := NewGitHistoryStore(t.TempDir(), nil)
	rows := make([]*models.TurnRepositoryChangeSet, 0, 150)
	for index := 0; index < 150; index++ {
		receipt, _ := json.Marshal(ArchiveReceipt{
			SourceID: "missing-source", HashAlgorithm: "sha1",
			StartRef: "refs/kandev/history/start", EndRef: "refs/kandev/history/end",
			StartCommitOID: "a", EndCommitOID: "b",
		})
		rows = append(rows, &models.TurnRepositoryChangeSet{ID: uuid.NewString(), PreservationReceipt: string(receipt)})
	}
	repository := &refCleanupHistoryRepository{rows: rows}
	service := NewHistoryService(repository, store, nil)
	result, err := service.CompactOnce(context.Background(), time.Now().UTC().Add(-time.Hour), models.TurnChangeReasonExpiredAge, 200)
	if err != nil {
		t.Fatalf("drain ref cleanup: %v", err)
	}
	if result.DeletedArchiveRefs != 150 || result.RemainingCandidates != 0 {
		t.Fatalf("ref cleanup = %+v", result)
	}
	if repository.cleared != 150 {
		t.Fatalf("cleared %d receipts, want 150", repository.cleared)
	}
}

type batchHistoryRepository struct {
	*fakeHistoryRepository
	remaining []*models.TurnHistoryRetentionCandidate
	expired   map[string]bool
}

func newBatchHistoryRepository(count int) *batchHistoryRepository {
	rows := make([]*models.TurnHistoryRetentionCandidate, 0, count)
	for index := 0; index < count; index++ {
		rows = append(rows, &models.TurnHistoryRetentionCandidate{
			ChangeSetID: uuid.NewString(), TaskID: uuid.NewString(),
		})
	}
	return &batchHistoryRepository{fakeHistoryRepository: &fakeHistoryRepository{}, remaining: rows, expired: map[string]bool{}}
}

func (f *batchHistoryRepository) ListTurnHistoryRetentionCandidates(context.Context, time.Time, time.Time, int) ([]*models.TurnHistoryRetentionCandidate, error) {
	panic("unused")
}

func (f *batchHistoryRepository) ListTurnHistoryRetentionCandidatesPage(_ context.Context, _ time.Time, _ time.Time, afterTaskID string, limit int) ([]*models.TurnHistoryRetentionCandidate, string, error) {
	active := make([]*models.TurnHistoryRetentionCandidate, 0, len(f.remaining))
	start := 0
	for _, row := range f.remaining {
		if f.expired[row.ChangeSetID] {
			continue
		}
		if afterTaskID != "" && row.TaskID == afterTaskID {
			start = len(active) + 1
		}
		active = append(active, row)
	}
	if start > len(active) {
		start = len(active)
	}
	active = active[start:]
	next := ""
	if len(active) > limit {
		active = active[:limit]
		next = active[len(active)-1].TaskID
	}
	return active, next, nil
}

func (f *batchHistoryRepository) ExpireTurnHistory(_ context.Context, ids []string, _ time.Time, _ models.TurnChangeReason, _ time.Time) (models.TurnHistoryExpiryResult, error) {
	for _, id := range ids {
		f.expired[id] = true
	}
	return models.TurnHistoryExpiryResult{ExpiredChangeSets: int64(len(ids))}, nil
}

func (f *batchHistoryRepository) ListExpiredTurnHistoryRepositories(context.Context, int) ([]*models.TurnRepositoryChangeSet, error) {
	return nil, nil
}

func (f *batchHistoryRepository) ListTurnRepositoryPreservationReady(context.Context, int) ([]*models.TurnRepositoryChangeSet, error) {
	panic("unused")
}

func (f *batchHistoryRepository) ClearTurnRepositoryPreservationReceipt(context.Context, string) error {
	panic("unused")
}

func (f *batchHistoryRepository) expiredCount() int { return len(f.expired) }

type refCleanupHistoryRepository struct {
	rows    []*models.TurnRepositoryChangeSet
	cleared int
}

func (f *refCleanupHistoryRepository) ListTurnHistoryRetentionCandidates(context.Context, time.Time, time.Time, int) ([]*models.TurnHistoryRetentionCandidate, error) {
	return nil, nil
}

func (f *refCleanupHistoryRepository) ListTurnHistoryRetentionCandidatesPage(context.Context, time.Time, time.Time, string, int) ([]*models.TurnHistoryRetentionCandidate, string, error) {
	return nil, "", nil
}

func (f *refCleanupHistoryRepository) ExpireTurnHistory(context.Context, []string, time.Time, models.TurnChangeReason, time.Time) (models.TurnHistoryExpiryResult, error) {
	return models.TurnHistoryExpiryResult{}, nil
}

func (f *refCleanupHistoryRepository) ListExpiredTurnHistoryRepositories(context.Context, int) ([]*models.TurnRepositoryChangeSet, error) {
	rows := f.rows
	f.rows = nil
	return rows, nil
}

func (f *refCleanupHistoryRepository) ListTurnRepositoryPreservationReady(context.Context, int) ([]*models.TurnRepositoryChangeSet, error) {
	return nil, nil
}

func (f *refCleanupHistoryRepository) ClearTurnRepositoryPreservationReceipt(context.Context, string) error {
	f.cleared++
	return nil
}

func writeHistoryBackupSnapshot(t *testing.T, path string, rows ...*models.TurnRepositoryChangeSet) {
	t.Helper()
	database, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	for _, statement := range []string{
		`CREATE TABLE turn_change_sets(id TEXT PRIMARY KEY, availability TEXT NOT NULL)`,
		`CREATE TABLE turn_repository_changes(id TEXT PRIMARY KEY, change_set_id TEXT NOT NULL, checkout_id TEXT NOT NULL,
			repository_id TEXT NOT NULL, content_source TEXT NOT NULL, source_git_dir TEXT NOT NULL DEFAULT '',
			preservation_state TEXT NOT NULL, preservation_receipt TEXT NOT NULL,
			hash_algorithm TEXT NOT NULL DEFAULT '', start_commit_oid TEXT NOT NULL DEFAULT '',
			start_tree_oid TEXT NOT NULL DEFAULT '', end_commit_oid TEXT NOT NULL DEFAULT '',
			end_tree_oid TEXT NOT NULL DEFAULT '')`,
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for index, row := range rows {
		changeSetID := row.TurnChangeSetID
		if changeSetID == "" {
			changeSetID = "set-" + uuid.NewString()
		}
		if _, err := database.Exec(`INSERT OR IGNORE INTO turn_change_sets(id, availability) VALUES(?, 'ready')`, changeSetID); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`INSERT INTO turn_repository_changes(id, change_set_id, checkout_id, repository_id,
			content_source, source_git_dir, preservation_state, preservation_receipt, hash_algorithm,
			start_commit_oid, start_tree_oid, end_commit_oid, end_tree_oid)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, row.ID+string(rune(index)), changeSetID, row.CheckoutID, row.RepositoryID,
			row.ContentSource, row.SourceGitDir, models.TurnChangePreservationReady, row.PreservationReceipt,
			row.HashAlgorithm, row.StartCommitOID, row.StartTreeOID, row.EndCommitOID, row.EndTreeOID); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *refCleanupHistoryRepository) ListTurnRepositoryPreservationReadyPage(context.Context, string, int) ([]*models.TurnRepositoryChangeSet, string, error) {
	return nil, "", nil
}

func (f *fakeHistoryRepository) ListTurnHistoryCleanupIntents(context.Context, int) ([]*models.TurnHistoryCleanupIntent, error) {
	return nil, nil
}

func (f *fakeHistoryRepository) DeleteTurnHistoryCleanupIntent(context.Context, string) error {
	return nil
}

func (f *batchHistoryRepository) ListTurnHistoryCleanupIntents(context.Context, int) ([]*models.TurnHistoryCleanupIntent, error) {
	return nil, nil
}

func (f *batchHistoryRepository) DeleteTurnHistoryCleanupIntent(context.Context, string) error {
	return nil
}

func (f *refCleanupHistoryRepository) ListTurnHistoryCleanupIntents(context.Context, int) ([]*models.TurnHistoryCleanupIntent, error) {
	return nil, nil
}

func (f *refCleanupHistoryRepository) DeleteTurnHistoryCleanupIntent(context.Context, string) error {
	return nil
}

func TestHistoryCompactionAnalysisPaginatesPastOneThousandIntervals(t *testing.T) {
	repository := newBatchHistoryRepository(1200)
	service := NewHistoryService(repository, nil, nil)
	inventory, err := service.Analyze(context.Background(), time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatalf("paginated analysis: %v", err)
	}
	if inventory.Snapshots != 1200 || inventory.Tasks != 1200 || !inventory.Complete {
		t.Fatalf("paginated inventory = %+v", inventory)
	}
}

func TestHistoryCompactionBackupRejectsTruncatedManifest(t *testing.T) {
	archive, _, startTree, endTree := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	receipt, err := store.Import(context.Background(), "repo-truncated", "checkout-truncated", archive)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(receipt)
	row := &models.TurnRepositoryChangeSet{
		ID: uuid.NewString(), TurnChangeSetID: archive.ChangeSetID, CheckoutID: archive.CheckoutID,
		RepositoryID: "repo-truncated", HashAlgorithm: "sha1", StartCommitOID: archive.StartCommitOID,
		StartTreeOID: startTree, EndCommitOID: archive.EndCommitOID, EndTreeOID: endTree,
		ContentSource: models.TurnChangeSourceGitArchive, PreservationState: models.TurnChangePreservationReady,
		PreservationReceipt: string(encoded),
	}
	service := NewHistoryService(&fakeHistoryRepository{preserved: []*models.TurnRepositoryChangeSet{row}}, store, nil)
	snapshotPath := filepath.Join(t.TempDir(), "manual-truncated.db")
	writeHistoryBackupSnapshot(t, snapshotPath, row)
	digest, _ := sha256File(snapshotPath)
	if err := service.PublishRetentionBackup(context.Background(), snapshotPath, digest); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(snapshotPath+".history", "manifest.json")
	raw, _ := os.ReadFile(manifestPath)
	var manifest HistoryBackupManifest
	_ = json.Unmarshal(raw, &manifest)
	manifest.Sources = nil
	truncated, _ := json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, truncated, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := service.VerifyRetentionBackup(context.Background(), snapshotPath, digest); err == nil {
		t.Fatal("truncated checkpoint manifest passed verification")
	}
}

func (f *fakeHistoryRepository) ListExpiredCommonHistory(context.Context, int) ([]*models.TurnRepositoryChangeSet, error) {
	return nil, nil
}

func (f *fakeHistoryRepository) ClearTurnRepositoryCommonCleanup(context.Context, string) error {
	return nil
}

func (f *batchHistoryRepository) ListExpiredCommonHistory(context.Context, int) ([]*models.TurnRepositoryChangeSet, error) {
	return nil, nil
}

func (f *batchHistoryRepository) ClearTurnRepositoryCommonCleanup(context.Context, string) error {
	return nil
}

func (f *refCleanupHistoryRepository) ListExpiredCommonHistory(context.Context, int) ([]*models.TurnRepositoryChangeSet, error) {
	return nil, nil
}

func (f *refCleanupHistoryRepository) ClearTurnRepositoryCommonCleanup(context.Context, string) error {
	return nil
}

func TestHistoryCompactionBackupRestoresCommonSourceAfterRepositoryLoss(t *testing.T) {
	archive, repoDir, startTree, endTree := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	row := &models.TurnRepositoryChangeSet{
		ID: uuid.NewString(), TurnChangeSetID: archive.ChangeSetID, CheckoutID: archive.CheckoutID,
		RepositoryID: "repo-common-backup", HashAlgorithm: "sha1", SourceGitDir: filepath.Join(repoDir, ".git"),
		StartCommitOID: archive.StartCommitOID, StartTreeOID: startTree,
		EndCommitOID: archive.EndCommitOID, EndTreeOID: endTree,
		ContentSource: models.TurnChangeSourceGitCommon, PreservationState: models.TurnChangePreservationReady,
	}
	service := NewHistoryService(&fakeHistoryRepository{preserved: []*models.TurnRepositoryChangeSet{row}}, store, nil)
	snapshotPath := filepath.Join(t.TempDir(), "manual-common.db")
	writeHistoryBackupSnapshot(t, snapshotPath, row)
	digest, _ := sha256File(snapshotPath)
	if err := service.PublishRetentionBackup(context.Background(), snapshotPath, digest); err != nil {
		t.Fatalf("publish common backup: %v", err)
	}
	if err := service.VerifyRetentionBackup(context.Background(), snapshotPath, digest); err != nil {
		t.Fatalf("verify common backup: %v", err)
	}
	if err := os.RemoveAll(repoDir); err != nil {
		t.Fatal(err)
	}
	if err := service.RestoreRetentionBackup(context.Background(), snapshotPath); err != nil {
		t.Fatalf("restore common backup: %v", err)
	}
	source := &models.TurnFileContentSource{
		ChangeSet: &models.TurnChangeSet{ID: row.TurnChangeSetID, Availability: models.TurnChangeAvailabilityReady},
		Repository: func() *models.TurnRepositoryChangeSet {
			copy := *row
			return &copy
		}(),
		File: &models.TurnFileChange{ID: uuid.NewString(), PathBytes: []byte("src/app.txt"), ContentAvailability: models.TurnChangeAvailabilityReady},
	}
	content := NewContentService(&fakeContentRepository{}, nil)
	content.SetHistoryStore(store)
	payload, err := content.readCommon(context.Background(), source, models.TurnChangeContentCanonicalPatch)
	if err != nil || payload == nil || !strings.Contains(string(payload.Content), "+new") {
		t.Fatalf("restored common read = %+v, %v", payload, err)
	}
}
