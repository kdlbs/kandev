package changes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

// HistoryRepository is the task-owned persistence surface used by the shared
// history-compaction policy. It never spans Git I/O in a SQL transaction.
type HistoryRepository interface {
	ListTurnHistoryRetentionCandidates(ctx context.Context, cutoff, now time.Time, limit int) ([]*models.TurnHistoryRetentionCandidate, error)
	ListTurnHistoryRetentionCandidatesPage(ctx context.Context, cutoff, now time.Time, afterTaskID string, limit int) ([]*models.TurnHistoryRetentionCandidate, string, error)
	ExpireTurnHistory(ctx context.Context, changeSetIDs []string, cutoff time.Time, reason models.TurnChangeReason, now time.Time) (models.TurnHistoryExpiryResult, error)
	ListExpiredTurnHistoryRepositories(ctx context.Context, limit int) ([]*models.TurnRepositoryChangeSet, error)
	ListTurnRepositoryPreservationReady(ctx context.Context, limit int) ([]*models.TurnRepositoryChangeSet, error)
	ListTurnRepositoryPreservationReadyPage(ctx context.Context, afterID string, limit int) ([]*models.TurnRepositoryChangeSet, string, error)
	ClearTurnRepositoryPreservationReceipt(ctx context.Context, repositoryChangeID string) error
	ListTurnHistoryCleanupIntents(ctx context.Context, limit int) ([]*models.TurnHistoryCleanupIntent, error)
	ListExpiredCommonHistory(ctx context.Context, limit int) ([]*models.TurnRepositoryChangeSet, error)
	ClearTurnRepositoryCommonCleanup(ctx context.Context, repositoryChangeID string) error
	DeleteTurnHistoryCleanupIntent(ctx context.Context, id string) error
}

// HistoryService exposes bounded analysis and expiry work to the single
// administrative compaction policy.
type HistoryService struct {
	repository HistoryRepository
	store      *GitHistoryStore
	now        func() time.Time
}

func NewHistoryService(repository HistoryRepository, store *GitHistoryStore, now func() time.Time) *HistoryService {
	if now == nil {
		now = time.Now
	}
	return &HistoryService{repository: repository, store: store, now: now}
}

// HistoryCleanupResult reports what one bounded pass changed.
type HistoryCleanupResult struct {
	ExpiredChangeSets   int64
	DeletedContents     int64
	FreedPayloadBytes   int64
	DeletedArchiveRefs  int64
	ReclaimedBytes      int64
	ArchiveFailures     int64
	RemainingCandidates int64
}

// Analyze returns the bounded inventory for one cutoff. It pages the task
// cohort to completion up to a fixed safety bound.
func (s *HistoryService) Analyze(ctx context.Context, cutoff time.Time) (models.TurnHistoryInventory, error) {
	inventory := models.TurnHistoryInventory{}
	if s == nil || s.repository == nil {
		return inventory, errors.New("history compaction is not configured")
	}
	now := s.now().UTC()
	tasks := make(map[string]struct{})
	after := ""
	for page := 0; page < 50; page++ {
		rows, next, err := s.repository.ListTurnHistoryRetentionCandidatesPage(ctx, cutoff, now, after, 200)
		if err != nil {
			return inventory, err
		}
		for _, row := range rows {
			if row == nil {
				continue
			}
			tasks[row.TaskID] = struct{}{}
			inventory.LegacySQLBytes += row.LegacySQLBytes
			inventory.ArchiveBytes += row.PreservationBytes
			inventory.Snapshots++
		}
		if next == "" || next == after {
			inventory.Complete = true
			break
		}
		after = next
		if inventory.Snapshots >= 10000 {
			break
		}
	}
	inventory.Tasks = int64(len(tasks))
	return inventory, nil
}

// CompactOnce expires one bounded candidate batch and then drains owned-ref
// cleanup receipts. SQL evidence is committed before external mutation.
func (s *HistoryService) CompactOnce(
	ctx context.Context,
	cutoff time.Time,
	reason models.TurnChangeReason,
	limit int,
) (HistoryCleanupResult, error) {
	result := HistoryCleanupResult{}
	if s == nil || s.repository == nil {
		return result, errors.New("history compaction is not configured")
	}
	now := s.now().UTC()
	candidates, err := s.collectCandidateBatch(ctx, cutoff, now, limit)
	if err != nil {
		return result, err
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil && candidate.ChangeSetID != "" {
			ids = append(ids, candidate.ChangeSetID)
		}
	}
	if len(ids) > 0 {
		if reason == "" {
			reason = models.TurnChangeReasonExpiredAge
		}
		expired, expireErr := s.repository.ExpireTurnHistory(ctx, ids, cutoff, reason, now)
		if expireErr != nil {
			return result, expireErr
		}
		result.ExpiredChangeSets = expired.ExpiredChangeSets
		result.DeletedContents = expired.DeletedContents
		result.FreedPayloadBytes = expired.FreedPayloadBytes
	}
	if err := s.cleanupOwnedRefs(ctx, &result); err != nil {
		return result, err
	}
	remaining, remainingErr := s.remainingWork(ctx, cutoff, now)
	if remainingErr != nil {
		return result, remainingErr
	}
	result.RemainingCandidates = remaining
	return result, nil
}

func (s *HistoryService) collectCandidateBatch(
	ctx context.Context, cutoff, now time.Time, limit int,
) ([]*models.TurnHistoryRetentionCandidate, error) {
	after := ""
	for page := 0; page < 50; page++ {
		rows, next, err := s.repository.ListTurnHistoryRetentionCandidatesPage(ctx, cutoff, now, after, limit)
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			return rows, nil
		}
		if next == "" || next == after {
			return nil, nil
		}
		after = next
	}
	return nil, nil
}

func (s *HistoryService) hasHistoryCandidates(ctx context.Context, cutoff, now time.Time) (bool, error) {
	rows, err := s.collectCandidateBatch(ctx, cutoff, now, 1)
	return len(rows) > 0, err
}

func (s *HistoryService) remainingWork(ctx context.Context, cutoff, now time.Time) (int64, error) {
	remaining := int64(0)
	present, err := s.hasHistoryCandidates(ctx, cutoff, now)
	if err != nil {
		return 0, err
	}
	if present {
		remaining = 1
	}
	cleanups, err := s.repository.ListExpiredTurnHistoryRepositories(ctx, 1)
	if err != nil {
		return 0, err
	}
	if len(cleanups) > 0 {
		remaining = 1
	}
	intents, err := s.repository.ListTurnHistoryCleanupIntents(ctx, 1)
	if err != nil {
		return 0, err
	}
	if len(intents) > 0 {
		remaining = 1
	}
	common, err := s.repository.ListExpiredCommonHistory(ctx, 1)
	if err != nil {
		return 0, err
	}
	if len(common) > 0 {
		remaining = 1
	}
	return remaining, nil
}

// HasWork reports whether either history category still has candidates. It is
// read-only so backup verification can be skipped for genuinely empty batches.
func (s *HistoryService) HasWork(ctx context.Context, cutoff time.Time) (bool, error) {
	if s == nil || s.repository == nil {
		return false, errors.New("history compaction is not configured")
	}
	now := s.now().UTC()
	present, err := s.hasHistoryCandidates(ctx, cutoff, now)
	if err != nil {
		return false, err
	}
	if present {
		return true, nil
	}
	cleanups, err := s.repository.ListExpiredTurnHistoryRepositories(ctx, 1)
	if err != nil {
		return false, err
	}
	if len(cleanups) > 0 {
		return true, nil
	}
	intents, err := s.repository.ListTurnHistoryCleanupIntents(ctx, 1)
	if err != nil {
		return false, err
	}
	if len(intents) > 0 {
		return true, nil
	}
	common, err := s.repository.ListExpiredCommonHistory(ctx, 1)
	if err != nil {
		return false, err
	}
	return len(common) > 0, nil
}

func (s *HistoryService) cleanupOwnedRefs(ctx context.Context, result *HistoryCleanupResult) error {
	if !s.store.Enabled() {
		return nil
	}
	if err := s.cleanupIndependentIntents(ctx, result); err != nil {
		return err
	}
	if err := s.cleanupCommonRefs(ctx, result); err != nil {
		return err
	}
	var failures []error
	seen := make(map[string]struct{})
	for page := 0; page < 50; page++ {
		rows, err := s.repository.ListExpiredTurnHistoryRepositories(ctx, 100)
		if err != nil {
			return err
		}
		pending := make([]*models.TurnRepositoryChangeSet, 0, len(rows))
		for _, row := range rows {
			if row == nil {
				continue
			}
			if _, done := seen[row.ID]; done {
				continue
			}
			pending = append(pending, row)
		}
		if len(pending) == 0 {
			break
		}
		sourceIDs := make(map[string]struct{})
		progress, pageFailures, clearIDs := s.cleanupExpiredRefPage(ctx, pending, result, sourceIDs)
		failures = append(failures, pageFailures...)
		if progress == 0 {
			break
		}
		if reclaimErr := s.reclaimExpiredSources(ctx, result, sourceIDs); reclaimErr != nil {
			return errors.Join(append(failures, reclaimErr)...)
		}
		for _, id := range clearIDs {
			seen[id] = struct{}{}
			if err := s.repository.ClearTurnRepositoryPreservationReceipt(ctx, id); err != nil {
				failures = append(failures, err)
				result.ArchiveFailures++
			}
		}
	}
	return errors.Join(failures...)
}

func (s *HistoryService) cleanupExpiredRefPage(
	ctx context.Context,
	rows []*models.TurnRepositoryChangeSet,
	result *HistoryCleanupResult,
	sourceIDs map[string]struct{},
) (int, []error, []string) {
	progress := 0
	var failures []error
	clearIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		receipt, decodeErr := decodeArchiveReceipt(row.PreservationReceipt)
		if decodeErr != nil {
			failures = append(failures, fmt.Errorf("decode archive cleanup receipt: %w", decodeErr))
			result.ArchiveFailures++
			continue
		}
		sourceIDs[receipt.SourceID] = struct{}{}
		if deleteErr := s.store.DeleteRefs(ctx, receipt); deleteErr != nil {
			if !errors.Is(deleteErr, ErrHistoryContentUnavailable) {
				failures = append(failures, deleteErr)
				result.ArchiveFailures++
				continue
			}
		}
		result.DeletedArchiveRefs++
		progress++
		clearIDs = append(clearIDs, row.ID)
	}
	return progress, failures, clearIDs
}

func (s *HistoryService) reclaimExpiredSources(
	ctx context.Context,
	result *HistoryCleanupResult,
	sourceIDs map[string]struct{},
) error {
	if len(sourceIDs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(sourceIDs))
	for id := range sourceIDs {
		ids = append(ids, id)
	}
	freed, err := s.store.Reclaim(ctx, ids)
	result.ReclaimedBytes += freed
	if err != nil {
		result.ArchiveFailures++
	}
	return err
}

func (s *HistoryService) cleanupCommonRefs(ctx context.Context, result *HistoryCleanupResult) error {
	for page := 0; page < 50; page++ {
		rows, err := s.repository.ListExpiredCommonHistory(ctx, 100)
		if err != nil || len(rows) == 0 {
			return err
		}
		progress := 0
		for _, row := range rows {
			if row == nil {
				continue
			}
			deleteErr := s.store.DeleteCommonRefs(ctx, row.SourceGitDir,
				historyRefExpectation{name: row.StartReachabilityRef, commit: row.StartCommitOID},
				historyRefExpectation{name: row.EndReachabilityRef, commit: row.EndCommitOID})
			if deleteErr != nil && !errors.Is(deleteErr, ErrHistoryContentUnavailable) {
				result.ArchiveFailures++
				continue
			}
			if clearErr := s.repository.ClearTurnRepositoryCommonCleanup(ctx, row.ID); clearErr != nil {
				result.ArchiveFailures++
				continue
			}
			result.DeletedArchiveRefs++
			progress++
		}
		if progress == 0 {
			return nil
		}
	}
	return nil
}

func (s *HistoryService) cleanupIndependentIntents(ctx context.Context, result *HistoryCleanupResult) error {
	sourceIDs := make(map[string]struct{})
	clearIDs := make([]string, 0)
	for page := 0; page < 50; page++ {
		intents, err := s.repository.ListTurnHistoryCleanupIntents(ctx, 100)
		if err != nil {
			return err
		}
		if len(intents) == 0 {
			break
		}
		progress := 0
		for _, intent := range intents {
			if intent == nil {
				continue
			}
			var deleteErr error
			if intent.SourceGitDir != "" {
				deleteErr = s.store.DeleteCommonRefs(ctx, intent.SourceGitDir,
					historyRefExpectation{name: intent.StartRef, commit: intent.StartCommitOID},
					historyRefExpectation{name: intent.EndRef, commit: intent.EndCommitOID})
			} else {
				sourceIDs[intent.SourceID] = struct{}{}
				deleteErr = s.store.DeleteRefs(ctx, ArchiveReceipt{
					SourceID: intent.SourceID, StartRef: intent.StartRef, EndRef: intent.EndRef,
					StartCommitOID: intent.StartCommitOID, EndCommitOID: intent.EndCommitOID,
				})
			}
			if deleteErr != nil && !errors.Is(deleteErr, ErrHistoryContentUnavailable) {
				result.ArchiveFailures++
				continue
			}
			result.DeletedArchiveRefs++
			progress++
			clearIDs = append(clearIDs, intent.ID)
		}
		if progress == 0 {
			break
		}
	}
	if reclaimErr := s.reclaimExpiredSources(ctx, result, sourceIDs); reclaimErr != nil {
		return reclaimErr
	}
	var failures []error
	for _, id := range clearIDs {
		if err := s.repository.DeleteTurnHistoryCleanupIntent(ctx, id); err != nil {
			failures = append(failures, err)
			result.ArchiveFailures++
		}
	}
	return errors.Join(failures...)
}

// VerifyPreservedRefs checks archive evidence for backup preparation.
func (s *HistoryService) VerifyPreservedRefs(ctx context.Context, repositoryChangeID, rawReceipt string) error {
	if !s.store.Enabled() {
		return nil
	}
	receipt, err := decodeArchiveReceipt(rawReceipt)
	if err != nil {
		return err
	}
	return s.store.VerifyRefs(ctx, receipt)
}

// EncodedReceipt exposes the durable archive receipt for backup manifests.
func EncodedReceipt(row models.TurnRepositoryChangeSet) (ArchiveReceipt, error) {
	if row.ContentSource != models.TurnChangeSourceGitArchive || row.PreservationReceipt == "" {
		return ArchiveReceipt{}, errors.New("repository interval has no archive receipt")
	}
	var receipt ArchiveReceipt
	if err := json.Unmarshal([]byte(row.PreservationReceipt), &receipt); err != nil {
		return ArchiveReceipt{}, err
	}
	return receipt, nil
}

const historyBackupVersion = 2

// HistoryBackupSource is one self-contained pack in a retention backup.
type HistoryBackupSource struct {
	PackFile       string   `json:"pack_file"`
	PackSHA256     string   `json:"pack_sha256"`
	ReceiptDigest  string   `json:"receipt_digest"`
	ObjectCount    int64    `json:"object_count"`
	Bytes          int64    `json:"bytes"`
	ChangeSetID    string   `json:"change_set_id"`
	CheckoutID     string   `json:"checkout_id"`
	RepositoryID   string   `json:"repository_id"`
	ContentSource  string   `json:"content_source"`
	HashAlgorithm  string   `json:"hash_algorithm"`
	StartCommitOID string   `json:"start_commit_oid"`
	StartTreeOID   string   `json:"start_tree_oid"`
	EndCommitOID   string   `json:"end_commit_oid"`
	EndTreeOID     string   `json:"end_tree_oid"`
	Objects        []string `json:"objects"`
}

type HistoryBackupManifest struct {
	Version         int                   `json:"version"`
	CreatedAt       time.Time             `json:"created_at"`
	DatabaseSHA256  string                `json:"database_sha256"`
	InventoryDigest string                `json:"inventory_digest"`
	Complete        bool                  `json:"complete"`
	Sources         []HistoryBackupSource `json:"sources"`
}

func historyBackupDir(snapshotPath string) string {
	return snapshotPath + ".history"
}

// PublishRetentionBackup writes a complete, self-contained checkpoint sidecar
// bound to one database image. Sources are paged to completion.
func (s *HistoryService) PublishRetentionBackup(ctx context.Context, snapshotPath, databaseSHA256 string) error {
	if s == nil || s.repository == nil || !s.store.Enabled() {
		return nil
	}
	dir := historyBackupDir(snapshotPath)
	stage, err := os.MkdirTemp(filepath.Dir(snapshotPath), ".history-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	manifest := HistoryBackupManifest{
		Version: historyBackupVersion, CreatedAt: s.now().UTC(),
		DatabaseSHA256: databaseSHA256, Complete: true,
	}
	rows, err := loadSnapshotHistoryRows(ctx, snapshotPath)
	if err != nil {
		return err
	}
	for _, row := range rows {
		source, sourceErr := s.publishBackupSource(ctx, stage, row)
		if sourceErr != nil {
			return sourceErr
		}
		manifest.Sources = append(manifest.Sources, source)
	}
	sortHistoryBackupSources(manifest.Sources)
	manifest.InventoryDigest = historyInventoryDigest(manifest)
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, "manifest.json"), encoded, 0o600); err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.Rename(stage, dir)
}

func sortHistoryBackupSources(sources []HistoryBackupSource) {
	sort.Slice(sources, func(i, j int) bool {
		return historySourceKey(sources[i]) < historySourceKey(sources[j])
	})
}

func historySourceKey(source HistoryBackupSource) string {
	return source.ChangeSetID + "\x00" + source.CheckoutID + "\x00" + source.RepositoryID
}

func historyInventoryDigest(manifest HistoryBackupManifest) string {
	canonical := struct {
		Version        int                   `json:"version"`
		DatabaseSHA256 string                `json:"database_sha256"`
		Complete       bool                  `json:"complete"`
		Sources        []HistoryBackupSource `json:"sources"`
	}{manifest.Version, manifest.DatabaseSHA256, manifest.Complete, manifest.Sources}
	encoded, _ := json.Marshal(canonical)
	return sha256Hex(encoded)
}

// VerifyRetentionBackup checks the checkpoint sidecar against the actual
// database image and the destructive operation. A missing sidecar is allowed
// only for a genuinely legacy database-only image.
func (s *HistoryService) VerifyRetentionBackup(ctx context.Context, snapshotPath, databaseSHA256 string) error {
	if s == nil || !s.store.Enabled() {
		return nil
	}
	inventory, inventoryErr := snapshotHistoryInventory(ctx, snapshotPath)
	if inventoryErr != nil {
		return inventoryErr
	}
	dir := historyBackupDir(snapshotPath)
	manifest, err := readHistoryBackupManifest(dir)
	if err != nil {
		if len(inventory) == 0 {
			return nil
		}
		return fmt.Errorf("retention backup does not cover retained checkpoint sources")
	}
	if databaseSHA256 == "" {
		databaseSHA256, err = sha256File(snapshotPath)
		if err != nil {
			return err
		}
	}
	if err := verifyHistoryManifestBinding(manifest, databaseSHA256, len(inventory)); err != nil {
		return err
	}
	return s.verifyHistoryBackupPacks(ctx, dir, manifest, inventory)
}

type snapshotHistorySource struct {
	key           string
	receiptDigest string
}

func snapshotHistoryInventory(ctx context.Context, snapshotPath string) ([]snapshotHistorySource, error) {
	rows, err := loadSnapshotHistoryRows(ctx, snapshotPath)
	if err != nil {
		return nil, err
	}
	sources := make([]snapshotHistorySource, 0, len(rows))
	for _, row := range rows {
		source := HistoryBackupSource{
			ChangeSetID: row.TurnChangeSetID, CheckoutID: row.CheckoutID, RepositoryID: row.RepositoryID,
		}
		sources = append(sources, snapshotHistorySource{key: historySourceKey(source), receiptDigest: historyReceiptDigest(row)})
	}
	return sources, nil
}

// loadSnapshotHistoryRows reads the complete checkpoint inventory from the
// database image. It never consults the live repository: finalization can add
// a ready row after the copy, and a manifest must describe that image.
func loadSnapshotHistoryRows(ctx context.Context, snapshotPath string) ([]*models.TurnRepositoryChangeSet, error) {
	reader, err := sqlx.Open("sqlite3", "file:"+db.EscapeSQLiteURIPath(snapshotPath)+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	rows := make([]*models.TurnRepositoryChangeSet, 0)
	after := ""
	for page := 0; page < 100; page++ {
		var pageRows []*models.TurnRepositoryChangeSet
		err := reader.SelectContext(ctx, &pageRows, `
			SELECT rc.id, rc.change_set_id, rc.checkout_id, rc.repository_id, rc.content_source,
				rc.source_git_dir, rc.preservation_state, rc.preservation_receipt, rc.hash_algorithm,
				rc.start_commit_oid, rc.start_tree_oid, rc.end_commit_oid, rc.end_tree_oid
			FROM turn_repository_changes rc
			JOIN turn_change_sets cs ON cs.id = rc.change_set_id
			WHERE cs.availability <> 'expired' AND rc.content_source IN ('git_archive','git_common')
				AND rc.preservation_state = 'ready' AND rc.id > ?
			ORDER BY rc.id LIMIT 500
		`, after)
		if err != nil {
			if isLegacySnapshotSchemaError(err) {
				return nil, nil
			}
			return nil, err
		}
		rows = append(rows, pageRows...)
		if len(pageRows) < 500 {
			return rows, nil
		}
		after = pageRows[len(pageRows)-1].ID
	}
	return nil, errors.New("retention backup checkpoint inventory exceeds its page bound")
}

func historyReceiptDigest(row *models.TurnRepositoryChangeSet) string {
	if row.ContentSource == models.TurnChangeSourceGitCommon {
		return sha256Hex([]byte("common:" + row.SourceGitDir))
	}
	return sha256Hex([]byte(row.PreservationReceipt))
}

func isLegacySnapshotSchemaError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "no such column") || strings.Contains(message, "no such table")
}

func (s *HistoryService) publishBackupSource(ctx context.Context, stage string, row *models.TurnRepositoryChangeSet) (HistoryBackupSource, error) {
	var pack []byte
	var objects []string
	var startTree, endTree, receiptDigest string
	if row.ContentSource == models.TurnChangeSourceGitCommon {
		var exportErr error
		pack, objects, startTree, endTree, exportErr = s.store.ExportPackAt(ctx, row.SourceGitDir,
			row.StartCommitOID, row.EndCommitOID, row.StartTreeOID, row.EndTreeOID)
		if exportErr != nil {
			return HistoryBackupSource{}, exportErr
		}
		receiptDigest = sha256Hex([]byte("common:" + row.SourceGitDir))
	} else {
		receipt, err := decodeArchiveReceipt(row.PreservationReceipt)
		if err != nil {
			return HistoryBackupSource{}, err
		}
		var exportErr error
		pack, objects, startTree, endTree, exportErr = s.store.ExportPack(ctx, receipt)
		if exportErr != nil {
			return HistoryBackupSource{}, exportErr
		}
		receiptDigest = sha256Hex([]byte(row.PreservationReceipt))
	}
	digest := sha256Hex(pack)
	packFile := digest + ".pack"
	if err := os.WriteFile(filepath.Join(stage, packFile), pack, 0o600); err != nil {
		return HistoryBackupSource{}, err
	}
	return HistoryBackupSource{
		PackFile: packFile, PackSHA256: digest, ReceiptDigest: receiptDigest,
		ObjectCount: int64(len(objects)), Bytes: int64(len(pack)),
		ChangeSetID: row.TurnChangeSetID, CheckoutID: row.CheckoutID, RepositoryID: row.RepositoryID,
		ContentSource: string(row.ContentSource), HashAlgorithm: row.HashAlgorithm, StartCommitOID: row.StartCommitOID, StartTreeOID: startTree,
		EndCommitOID: row.EndCommitOID, EndTreeOID: endTree, Objects: objects,
	}, nil
}

func verifyHistoryManifestBinding(manifest HistoryBackupManifest, databaseSHA256 string, inventoryCount int) error {
	if len(manifest.Sources) != inventoryCount {
		return fmt.Errorf("retention backup checkpoint inventory is incomplete")
	}
	if manifest.DatabaseSHA256 != databaseSHA256 || !manifest.Complete {
		return fmt.Errorf("retention backup is not bound to this database image")
	}
	if historyInventoryDigest(manifest) != manifest.InventoryDigest {
		return fmt.Errorf("retention checkpoint inventory changed")
	}
	return nil
}

func (s *HistoryService) verifyHistoryBackupPacks(
	ctx context.Context, dir string, manifest HistoryBackupManifest, inventory []snapshotHistorySource,
) error {
	byKey := make(map[string]HistoryBackupSource, len(manifest.Sources))
	for _, source := range manifest.Sources {
		byKey[historySourceKey(source)] = source
	}
	for _, expected := range inventory {
		source, ok := byKey[expected.key]
		if !ok || source.ReceiptDigest != expected.receiptDigest {
			return fmt.Errorf("retention backup checkpoint inventory does not match the database image")
		}
		if filepath.Base(source.PackFile) != source.PackFile || !strings.HasSuffix(source.PackFile, ".pack") {
			return fmt.Errorf("retention checkpoint pack path is invalid")
		}
		if len(source.Objects) == 0 || len(source.Objects) > maxHistoryArchiveObjects {
			return fmt.Errorf("retention checkpoint object inventory is outside bounds")
		}
		pack, err := os.ReadFile(filepath.Join(dir, source.PackFile))
		if err != nil {
			return err
		}
		if sha256Hex(pack) != source.PackSHA256 {
			return fmt.Errorf("retention checkpoint pack changed")
		}
		if err := s.store.VerifyPack(ctx, pack, source); err != nil {
			return err
		}
	}
	return nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// RestoreRetentionBackup stages verified checkpoint packs into backend-owned
// storage. It does not require an open database, so it runs before pool replacement.
func (s *HistoryService) RestoreRetentionBackup(ctx context.Context, snapshotPath string) error {
	if s == nil || !s.store.Enabled() {
		return nil
	}
	if err := s.VerifyRetentionBackup(ctx, snapshotPath, ""); err != nil {
		return err
	}
	dir := historyBackupDir(snapshotPath)
	manifest, err := readHistoryBackupManifest(dir)
	if err != nil {
		return nil
	}
	for _, source := range manifest.Sources {
		pack, readErr := os.ReadFile(filepath.Join(dir, source.PackFile))
		if readErr != nil {
			return readErr
		}
		archive := &turnchanges.CheckpointArchive{
			ChangeSetID: source.ChangeSetID, CheckoutID: source.CheckoutID, HashAlgorithm: source.HashAlgorithm,
			StartCommitOID: source.StartCommitOID, StartTreeOID: source.StartTreeOID,
			EndCommitOID: source.EndCommitOID, EndTreeOID: source.EndTreeOID,
			Pack: pack, Objects: source.Objects, ObjectCount: source.ObjectCount, Complete: true,
		}
		if _, importErr := s.store.Import(ctx, source.RepositoryID, source.CheckoutID, archive); importErr != nil {
			return importErr
		}
	}
	return nil
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

// HasRetentionBackup reports whether one snapshot has checkpoint coverage.
func (s *HistoryService) HasRetentionBackup(snapshotPath string) bool {
	if s == nil {
		return false
	}
	_, err := os.Stat(filepath.Join(historyBackupDir(snapshotPath), "manifest.json"))
	return err == nil
}

func readHistoryBackupManifest(dir string) (HistoryBackupManifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return HistoryBackupManifest{}, err
	}
	var manifest HistoryBackupManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return HistoryBackupManifest{}, err
	}
	if manifest.Version != historyBackupVersion {
		return HistoryBackupManifest{}, fmt.Errorf("unsupported history backup manifest version")
	}
	return manifest, nil
}

// CleanupDeletionIntents drains task/session deletion receipts independently
// from the opt-in compaction policy.
func (s *HistoryService) CleanupDeletionIntents(ctx context.Context) (int64, error) {
	if s == nil || s.repository == nil || !s.store.Enabled() {
		return 0, nil
	}
	result := HistoryCleanupResult{}
	if err := s.cleanupIndependentIntents(ctx, &result); err != nil {
		return result.DeletedArchiveRefs, err
	}
	return result.DeletedArchiveRefs, nil
}
