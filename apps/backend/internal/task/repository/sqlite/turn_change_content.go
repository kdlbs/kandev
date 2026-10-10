package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const (
	maxTurnChangePatchBytes     = 4 << 20
	maxTurnChangeRenderingBytes = 8 << 20
	maxTurnChangeExportBytes    = 32 << 20
	maxTurnChangeFilesPerExport = 20_000
	maxTurnChangeLeaseDuration  = 5 * time.Minute
	turnChangeRetentionLock     = "turn-change-retention"
)

var turnChangeContentVariants = []models.TurnChangeContentVariant{
	models.TurnChangeContentCanonicalPatch,
	models.TurnChangeContentFilteredPatch,
	models.TurnChangeContentOldRendering,
	models.TurnChangeContentNewRendering,
}

func (r *Repository) StoreTurnChangeFiles(ctx context.Context, repositoryChangeID string, files []models.TurnChangeFileContent) error {
	_, err := r.StoreTurnChangeFilesWithReceipt(ctx, repositoryChangeID, files)
	return err
}

func (r *Repository) StoreTurnChangeFilesWithReceipt(
	ctx context.Context,
	repositoryChangeID string,
	files []models.TurnChangeFileContent,
) (models.TurnChangeContentStoreReceipt, error) {
	receipt := models.TurnChangeContentStoreReceipt{Complete: true}
	if repositoryChangeID == "" || len(files) > maxTurnChangeFilesPerExport {
		return receipt, fmt.Errorf("store turn change files: invalid repository change or file count")
	}
	prepared, _, err := prepareTurnChangeFileContents(files)
	if err != nil {
		return receipt, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return receipt, err
	}
	defer func() { _ = tx.Rollback() }()
	changeSetID, checkoutID, remaining, err := prepareTurnChangeContentWrite(ctx, tx, r.db, repositoryChangeID)
	if err != nil {
		return receipt, err
	}
	now := time.Now().UTC()
	storedPayloadBytes := int64(0)
	for index := range prepared {
		if err := prepareTurnChangeFileRecord(ctx, tx, r.db, repositoryChangeID, checkoutID, &prepared[index].File, now); err != nil {
			return receipt, err
		}
		partial, storedBytes, err := storeTurnChangeFilePayloads(ctx, tx, r.db, &prepared[index], &remaining, now)
		if err != nil {
			return receipt, err
		}
		storedPayloadBytes += storedBytes
		if err := finishStoredTurnChangeFile(ctx, tx, r.db, &prepared[index], partial); err != nil {
			return receipt, err
		}
	}
	receipt, err = settleTurnChangeContentReceipt(ctx, tx, r.db, repositoryChangeID, changeSetID, storedPayloadBytes, now)
	if err != nil {
		return receipt, err
	}
	if err := tx.Commit(); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func prepareTurnChangeContentWrite(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	repositoryChangeID string,
) (string, string, int64, error) {
	var changeSetID, checkoutID string
	err := tx.QueryRowxContext(ctx, db.Rebind(`SELECT change_set_id, checkout_id FROM turn_repository_changes WHERE id = ?`), repositoryChangeID).Scan(&changeSetID, &checkoutID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", 0, repoerrors.ErrTurnChangeRelationship
	}
	if err != nil {
		return "", "", 0, err
	}
	if _, err := lockTurnChangeSet(ctx, tx, db, changeSetID); err != nil {
		return "", "", 0, err
	}
	var existingBytes int64
	if err := tx.GetContext(ctx, &existingBytes, db.Rebind(`SELECT content_bytes FROM turn_change_sets WHERE id = ?`), changeSetID); err != nil {
		return "", "", 0, err
	}
	if existingBytes < 0 {
		return "", "", 0, fmt.Errorf("store turn change files: invalid existing content byte count")
	}
	remaining := int64(maxTurnChangeExportBytes) - existingBytes
	if remaining < 0 {
		remaining = 0
	}
	return changeSetID, checkoutID, remaining, nil
}

func settleTurnChangeContentReceipt(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	repositoryChangeID, changeSetID string,
	storedPayloadBytes int64,
	now time.Time,
) (models.TurnChangeContentStoreReceipt, error) {
	receipt := models.TurnChangeContentStoreReceipt{StoredBytes: storedPayloadBytes}
	if _, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE turn_repository_changes SET content_complete = NOT EXISTS (
			SELECT 1 FROM turn_file_changes
			WHERE repository_change_id = ? AND (content_availability <> 'ready' OR content_truncated = ?)
		) WHERE id = ?
	`), repositoryChangeID, true, repositoryChangeID); err != nil {
		return receipt, err
	}
	if _, err := tx.ExecContext(ctx, db.Rebind(`UPDATE turn_change_sets SET content_bytes = content_bytes + ?, updated_at = ? WHERE id = ?`), storedPayloadBytes, now, changeSetID); err != nil {
		return receipt, err
	}
	var incomplete int
	if err := tx.GetContext(ctx, &incomplete, db.Rebind(`
		SELECT COUNT(*) FROM turn_file_changes
		WHERE repository_change_id = ? AND (content_availability <> 'ready' OR content_truncated = ?)
	`), repositoryChangeID, true); err != nil {
		return receipt, err
	}
	receipt.Complete = incomplete == 0
	if incomplete == 0 {
		return receipt, nil
	}
	var reason string
	if err := tx.GetContext(ctx, &reason, db.Rebind(`
		SELECT content_reason FROM turn_file_changes
		WHERE repository_change_id = ? AND (content_availability <> 'ready' OR content_truncated = ?)
		ORDER BY id LIMIT 1
	`), repositoryChangeID, true); err != nil {
		return receipt, err
	}
	receipt.Reason = models.TurnChangeReason(reason)
	if receipt.Reason == "" {
		receipt.Reason = models.TurnChangeReasonContentUnavailable
	}
	return receipt, nil
}

func prepareTurnChangeFileRecord(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	repositoryChangeID, checkoutID string,
	file *models.TurnFileChange,
	now time.Time,
) error {
	if err := prepareStoredTurnChangeFile(file, repositoryChangeID, now); err != nil {
		return err
	}
	if file.CheckoutID != "" && file.CheckoutID != checkoutID {
		return repoerrors.ErrTurnChangeRelationship
	}
	file.CheckoutID = checkoutID
	existing, found, err := findTurnChangeFile(ctx, tx, db, repositoryChangeID, file.PathBytes)
	if err != nil {
		return err
	}
	if !found {
		file.CanonicalContentID, file.FilteredContentID, file.OldContentID, file.NewContentID = "", "", "", ""
		return insertTurnChangeFile(ctx, tx, db, file)
	}
	if !sameTurnChangeFileIdentity(*existing, *file) {
		return repoerrors.ErrTurnChangeRelationship
	}
	file.ID = existing.ID
	file.CanonicalContentID, file.FilteredContentID = existing.CanonicalContentID, existing.FilteredContentID
	file.OldContentID, file.NewContentID = existing.OldContentID, existing.NewContentID
	file.CanonicalContentBytes = existing.CanonicalContentBytes
	return nil
}

func storeTurnChangeFilePayloads(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	prepared *models.TurnChangeFileContent,
	remaining *int64,
	now time.Time,
) (bool, int64, error) {
	partial, storedBytes := prepared.File.ContentTruncated, int64(0)
	for _, payload := range turnChangeFilePayloads(prepared) {
		if payload.bytes == nil {
			continue
		}
		limit := int64(maxTurnChangePatchBytes)
		if payload.variant == models.TurnChangeContentOldRendering || payload.variant == models.TurnChangeContentNewRendering {
			limit = maxTurnChangeRenderingBytes
		}
		if int64(len(payload.bytes)) > limit {
			partial = true
			continue
		}
		contentID, linked, err := existingTurnChangeContentLink(ctx, tx, db, prepared.File.ID, payload.variant)
		if err != nil {
			return false, 0, err
		}
		if linked {
			if err := validateLinkedTurnChangeContent(ctx, tx, db, contentID, payload); err != nil {
				return false, 0, err
			}
			setTurnChangeFileContentID(&prepared.File, payload.variant, contentID)
			continue
		}
		payloadBytes := int64(len(payload.bytes))
		if payloadBytes > *remaining {
			partial = true
			continue
		}
		contentID, err = putTurnChangeContent(ctx, tx, db, payload.bytes, now)
		if err != nil {
			return false, 0, err
		}
		if err := linkTurnChangeContent(ctx, tx, db, prepared.File.ID, payload.variant, contentID, now); err != nil {
			return false, 0, err
		}
		setTurnChangeFileContentID(&prepared.File, payload.variant, contentID)
		*remaining -= payloadBytes
		storedBytes += payloadBytes
	}
	return partial, storedBytes, nil
}

func validateLinkedTurnChangeContent(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	contentID string,
	payload turnChangePayloadInput,
) error {
	var storedDigest string
	if err := tx.GetContext(ctx, &storedDigest, db.Rebind(`SELECT digest FROM turn_change_contents WHERE id = ?`), contentID); err != nil {
		return err
	}
	if storedDigest != sha256Hex(payload.bytes) {
		return repoerrors.ErrTurnChangeRelationship
	}
	return nil
}

func finishStoredTurnChangeFile(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	prepared *models.TurnChangeFileContent,
	partial bool,
) error {
	file := &prepared.File
	file.ContentTruncated = partial
	if partial {
		file.ContentAvailability = models.TurnChangeAvailabilityUnavailable
		if file.ContentReason == "" {
			file.ContentReason = models.TurnChangeReasonSizeLimit
		}
	}
	if file.ContentAvailability == models.TurnChangeAvailabilityReady && !hasRequiredTurnChangeContent(file) {
		file.ContentAvailability = models.TurnChangeAvailabilityUnavailable
		file.ContentReason = models.TurnChangeReasonContentUnavailable
		file.ContentTruncated = true
	}
	if file.CanonicalContentID == "" {
		file.CanonicalContentBytes = 0
	} else if file.CanonicalContentBytes == 0 {
		file.CanonicalContentBytes = int64(len(prepared.CanonicalPatch))
	}
	return updateTurnChangeFileContent(ctx, tx, db, file)
}

func hasRequiredTurnChangeContent(file *models.TurnFileChange) bool {
	if file.CanonicalContentID == "" || file.FilteredContentID == "" {
		return false
	}
	if file.Binary || file.Submodule {
		return true
	}
	if hasTurnChangeBlob(file.OldBlobOID) && file.OldContentID == "" {
		return false
	}
	if hasTurnChangeBlob(file.NewBlobOID) && file.NewContentID == "" {
		return false
	}
	return true
}

func hasTurnChangeBlob(oid string) bool {
	if oid == "" {
		return false
	}
	for _, digit := range oid {
		if digit != '0' {
			return true
		}
	}
	return false
}

type turnChangePayloadInput struct {
	variant models.TurnChangeContentVariant
	bytes   []byte
}

func prepareTurnChangeFileContents(files []models.TurnChangeFileContent) ([]models.TurnChangeFileContent, int, error) {
	prepared := append([]models.TurnChangeFileContent(nil), files...)
	seen := make(map[string]struct{}, len(files))
	total := 0
	for index := range prepared {
		file := &prepared[index].File
		if file.ID == "" {
			file.ID = uuid.NewString()
		}
		if len(file.PathBytes) == 0 {
			file.PathBytes = []byte(file.Path)
		}
		if len(file.PathBytes) == 0 || strings.ContainsRune(string(file.PathBytes), '\x00') {
			return nil, 0, fmt.Errorf("store turn change files: file path is empty or contains NUL")
		}
		if !utf8.Valid(file.PathBytes) {
			file.Path = "\uFFFD"
			file.ContentAvailability = models.TurnChangeAvailabilityUnavailable
			file.ContentReason = models.TurnChangeReasonInvalidPathEncoding
		} else if file.Path == "" {
			file.Path = string(file.PathBytes)
		}
		if _, ok := allowedTurnFileKind(file.Kind); !ok {
			return nil, 0, fmt.Errorf("store turn change files: invalid file kind %q", file.Kind)
		}
		identity := file.RepositoryChangeID + "\x00" + string(file.PathBytes)
		if _, exists := seen[identity]; exists {
			return nil, 0, fmt.Errorf("store turn change files: duplicate path identity")
		}
		seen[identity] = struct{}{}
		for _, payload := range turnChangeFilePayloads(&prepared[index]) {
			if payload.bytes == nil {
				continue
			}
			limit := maxTurnChangePatchBytes
			if payload.variant == models.TurnChangeContentOldRendering || payload.variant == models.TurnChangeContentNewRendering {
				limit = maxTurnChangeRenderingBytes
			}
			if len(payload.bytes) > limit {
				continue
			}
			total += len(payload.bytes)
		}
	}
	return prepared, total, nil
}

func allowedTurnFileKind(kind string) (bool, bool) {
	switch kind {
	case "added", statusDeleted, "modified", "renamed", "copied", "mode_changed", "type_changed":
		return true, true
	default:
		return false, false
	}
}

func turnChangeFilePayloads(file *models.TurnChangeFileContent) []turnChangePayloadInput {
	return []turnChangePayloadInput{
		{models.TurnChangeContentCanonicalPatch, file.CanonicalPatch},
		{models.TurnChangeContentFilteredPatch, file.FilteredPatch},
		{models.TurnChangeContentOldRendering, file.OldRendering},
		{models.TurnChangeContentNewRendering, file.NewRendering},
	}
}

func prepareStoredTurnChangeFile(file *models.TurnFileChange, repositoryChangeID string, now time.Time) error {
	if file.RepositoryChangeID != "" && file.RepositoryChangeID != repositoryChangeID {
		return repoerrors.ErrTurnChangeRelationship
	}
	file.RepositoryChangeID = repositoryChangeID
	if !file.ContentAvailability.Valid() {
		return fmt.Errorf("store turn change files: invalid content availability %q", file.ContentAvailability)
	}
	if !file.ContentReason.Valid() {
		return fmt.Errorf("store turn change files: invalid content reason %q", file.ContentReason)
	}
	if file.CreatedAt.IsZero() {
		file.CreatedAt = now
	}
	return nil
}

func insertTurnChangeFile(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, file *models.TurnFileChange) error {
	_, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO turn_file_changes (
			id, repository_change_id, checkout_id, path, path_bytes, old_path, old_path_bytes,
			kind, old_blob_oid, new_blob_oid, old_mode, new_mode, submodule, is_binary,
			added_lines, deleted_lines, content_availability, content_reason, content_truncated,
			canonical_content_bytes, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), file.ID, file.RepositoryChangeID, file.CheckoutID, file.Path, file.PathBytes, file.OldPath, nullableBytes(file.OldPathBytes),
		file.Kind, file.OldBlobOID, file.NewBlobOID, file.OldMode, file.NewMode, file.Submodule, file.Binary,
		file.AddedLines, file.DeletedLines, file.ContentAvailability, file.ContentReason, file.ContentTruncated,
		file.CanonicalContentBytes, file.CreatedAt)
	return err
}

func findTurnChangeFile(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, repositoryChangeID string, pathBytes []byte) (*models.TurnFileChange, bool, error) {
	file := &models.TurnFileChange{}
	err := tx.GetContext(ctx, file, db.Rebind(`
		SELECT id, repository_change_id, checkout_id, path, path_bytes, old_path, old_path_bytes,
			kind, old_blob_oid, new_blob_oid, old_mode, new_mode, submodule, is_binary,
			added_lines, deleted_lines, canonical_content_id, filtered_content_id, old_content_id, new_content_id,
			content_availability, content_reason, content_truncated, canonical_content_bytes, created_at
		FROM turn_file_changes WHERE repository_change_id = ? AND path_bytes = ?
	`), repositoryChangeID, pathBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return file, true, nil
}

func sameTurnChangeFileIdentity(existing, incoming models.TurnFileChange) bool {
	return existing.RepositoryChangeID == incoming.RepositoryChangeID && existing.CheckoutID == incoming.CheckoutID &&
		bytes.Equal(existing.PathBytes, incoming.PathBytes) && bytes.Equal(existing.OldPathBytes, incoming.OldPathBytes) &&
		existing.Kind == incoming.Kind && existing.OldBlobOID == incoming.OldBlobOID && existing.NewBlobOID == incoming.NewBlobOID &&
		existing.OldMode == incoming.OldMode && existing.NewMode == incoming.NewMode &&
		existing.Submodule == incoming.Submodule && existing.Binary == incoming.Binary &&
		sameTurnChangeCount(existing.AddedLines, incoming.AddedLines) && sameTurnChangeCount(existing.DeletedLines, incoming.DeletedLines)
}

func sameTurnChangeCount(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func existingTurnChangeContentLink(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, fileID string, variant models.TurnChangeContentVariant) (string, bool, error) {
	var contentID string
	err := tx.GetContext(ctx, &contentID, db.Rebind(`SELECT content_id FROM turn_change_content_links WHERE file_change_id = ? AND variant = ?`), fileID, variant)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return contentID, err == nil, err
}

func updateTurnChangeFileContent(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, file *models.TurnFileChange) error {
	_, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE turn_file_changes SET canonical_content_id = ?, filtered_content_id = ?, old_content_id = ?, new_content_id = ?,
			content_availability = ?, content_reason = ?, content_truncated = ?, canonical_content_bytes = ?
		WHERE id = ?
	`), file.CanonicalContentID, file.FilteredContentID, file.OldContentID, file.NewContentID,
		file.ContentAvailability, file.ContentReason, file.ContentTruncated, file.CanonicalContentBytes, file.ID)
	return err
}

func nullableBytes(value []byte) any {
	if value == nil {
		return nil
	}
	return value
}

func putTurnChangeContent(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, content []byte, now time.Time) (string, error) {
	digest := sha256Hex(content)
	contentID := "sha256:" + digest
	compressed, err := gzipCompress(content)
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO turn_change_contents (id, digest, codec, uncompressed_bytes, payload_bytes, created_at)
		VALUES (?, ?, 'gzip', ?, ?, ?)
		ON CONFLICT (digest) DO NOTHING
	`), contentID, digest, len(content), compressed, now)
	if err != nil {
		return "", err
	}
	var storedID, storedDigest, codec string
	var storedSize, payloadSize int64
	var storedPayload []byte
	if err := tx.QueryRowxContext(ctx, db.Rebind(`SELECT id, digest, codec, uncompressed_bytes, length(payload_bytes), payload_bytes FROM turn_change_contents WHERE digest = ?`), digest).
		Scan(&storedID, &storedDigest, &codec, &storedSize, &payloadSize, &storedPayload); err != nil {
		return "", err
	}
	if storedID != contentID || storedDigest != digest || codec != "gzip" || storedSize != int64(len(content)) || payloadSize != int64(len(storedPayload)) {
		return "", fmt.Errorf("stored turn change content metadata failed validation")
	}
	decompressed, err := gzipDecompressExpectedSize(storedPayload, maxTurnChangeRenderingBytes, storedSize)
	if err != nil || !bytes.Equal(decompressed, content) {
		return "", fmt.Errorf("content-addressed turn change payload integrity mismatch")
	}
	return storedID, nil
}

func linkTurnChangeContent(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, fileID string, variant models.TurnChangeContentVariant, contentID string, now time.Time) error {
	if !isTurnChangeContentVariant(variant) {
		return fmt.Errorf("store turn change files: invalid content variant %q", variant)
	}
	_, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO turn_change_content_links (file_change_id, variant, content_id, created_at)
		VALUES (?, ?, ?, ?)
	`), fileID, variant, contentID, now)
	return err
}

func setTurnChangeFileContentID(file *models.TurnFileChange, variant models.TurnChangeContentVariant, id string) {
	switch variant {
	case models.TurnChangeContentCanonicalPatch:
		file.CanonicalContentID = id
	case models.TurnChangeContentFilteredPatch:
		file.FilteredContentID = id
	case models.TurnChangeContentOldRendering:
		file.OldContentID = id
	case models.TurnChangeContentNewRendering:
		file.NewContentID = id
	}
}

func isTurnChangeContentVariant(variant models.TurnChangeContentVariant) bool {
	for _, candidate := range turnChangeContentVariants {
		if candidate == variant {
			return true
		}
	}
	return false
}

func (r *Repository) ReadTurnChangeContent(ctx context.Context, changeSetID, fileChangeID string, variant models.TurnChangeContentVariant) (*models.TurnChangeContentPayload, error) {
	if changeSetID == "" || fileChangeID == "" || !isTurnChangeContentVariant(variant) {
		return nil, repoerrors.ErrTurnChangeContentNotFound
	}
	payload := &models.TurnChangeContentPayload{FileChangeID: fileChangeID, Variant: variant}
	var contentID, codec string
	var expectedSize, compressedSize int64
	var compressed []byte
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT c.id, c.digest, c.codec, c.uncompressed_bytes, length(c.payload_bytes), c.payload_bytes
		FROM turn_change_content_links l
		JOIN turn_change_contents c ON c.id = l.content_id
		JOIN turn_file_changes f ON f.id = l.file_change_id
		JOIN turn_repository_changes rc ON rc.id = f.repository_change_id
		WHERE l.file_change_id = ? AND l.variant = ?
		AND rc.change_set_id = ?
	`), fileChangeID, variant, changeSetID).Scan(&contentID, &payload.Digest, &codec, &expectedSize, &compressedSize, &compressed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repoerrors.ErrTurnChangeContentNotFound
	}
	if err != nil {
		return nil, err
	}
	if codec != "gzip" || expectedSize < 0 || expectedSize > maxTurnChangeRenderingBytes || compressedSize != int64(len(compressed)) || compressedSize > maxTurnChangeRenderingBytes {
		return nil, fmt.Errorf("turn change content metadata exceeds or violates read bounds")
	}
	content, err := gzipDecompressExpectedSize(compressed, maxTurnChangeRenderingBytes, expectedSize)
	if err != nil {
		return nil, err
	}
	if sha256Hex(content) != payload.Digest {
		return nil, fmt.Errorf("turn change content digest mismatch")
	}
	payload.Content = content
	return payload, nil
}

func (r *Repository) SetTurnRepositoryContentStatus(ctx context.Context, repositoryChangeID string, complete bool, reason models.TurnChangeReason) error {
	if repositoryChangeID == "" || !reason.Valid() {
		return fmt.Errorf("set turn repository content status: invalid identity or reason")
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE turn_repository_changes SET content_complete = ?, reason = CASE WHEN ? = '' THEN reason ELSE ? END, updated_at = ? WHERE id = ?`),
		complete, reason, reason, time.Now().UTC(), repositoryChangeID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return repoerrors.ErrTurnChangeRelationship
	}
	return nil
}

func (r *Repository) AcquireTurnChangeContentLease(ctx context.Context, changeSetID string, duration time.Duration) (*models.TurnChangeContentLease, error) {
	if changeSetID == "" {
		return nil, repoerrors.ErrTurnChangeSetNotFound
	}
	if duration <= 0 || duration > maxTurnChangeLeaseDuration {
		duration = maxTurnChangeLeaseDuration
	}
	now := time.Now().UTC()
	lease := &models.TurnChangeContentLease{ID: uuid.NewString(), ChangeSetID: changeSetID, ExpiresAt: now.Add(duration)}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockTurnChangeRetention(ctx, tx, r.db); err != nil {
		return nil, err
	}
	var availability models.TurnChangeAvailability
	query := `SELECT availability FROM turn_change_sets WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE`
	}
	if err := tx.GetContext(ctx, &availability, r.db.Rebind(query), changeSetID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repoerrors.ErrTurnChangeSetNotFound
		}
		return nil, err
	}
	if availability == models.TurnChangeAvailabilityExpired {
		return nil, repoerrors.ErrTurnChangeContentNotFound
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO turn_change_content_leases (id, change_set_id, expires_at, created_at) VALUES (?, ?, ?, ?)`), lease.ID, changeSetID, lease.ExpiresAt, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return lease, nil
}

func (r *Repository) ReleaseTurnChangeContentLease(ctx context.Context, leaseID string) error {
	if leaseID == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM turn_change_content_leases WHERE id = ?`), leaseID)
	return err
}
