package backups

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/system/maintenance"
)

// RetentionReceipt identifies a completely published, integrity-checked backup.
// Callers persist this server-generated receipt with the preparation revision.
type RetentionReceipt struct {
	Name        string    `json:"name"`
	SizeBytes   int64     `json:"size_bytes"`
	CompletedAt time.Time `json:"completed_at"`
	SHA256      string    `json:"sha256"`
}

// CreateForRetention holds maintenance admission through snapshot verification
// and publication. It does not authorize retention or prune existing backups.
func (s *Service) CreateForRetention(ctx context.Context) (RetentionReceipt, error) {
	return s.createSnapshot(ctx, true)
}

func (s *Service) createSnapshot(ctx context.Context, verify bool) (RetentionReceipt, error) {
	release, err := maintenance.ForPool(s.pool).Acquire(ctx)
	if err != nil {
		return RetentionReceipt{}, err
	}
	defer release()
	if s.pool == nil || s.pool.Writer() == nil {
		return RetentionReceipt{}, fmt.Errorf("no database pool")
	}
	if err := s.ensureSQLiteRestore(); err != nil {
		return RetentionReceipt{}, err
	}
	return s.writeSnapshot(ctx, verify)
}

func (s *Service) writeSnapshot(ctx context.Context, verify bool) (RetentionReceipt, error) {
	if err := s.ensureBackupsDir(); err != nil {
		return RetentionReceipt{}, err
	}
	s.sweepStaleTmpFiles()
	stage, err := os.MkdirTemp(s.backupsDir(), ".snapshot-*")
	if err != nil {
		return RetentionReceipt{}, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	path := filepath.Join(stage, "snapshot.db")
	size, err := persistence.SnapshotSQLiteContext(ctx, s.pool.Writer(), path)
	if err != nil {
		return RetentionReceipt{}, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		return RetentionReceipt{}, err
	}
	var digest string
	if verify {
		digest, err = verifySnapshot(ctx, path)
		if err != nil {
			return RetentionReceipt{}, err
		}
	}
	name := fmt.Sprintf("%s%d%s", manualPrefix, time.Now().UTC().UnixNano(), dbSuffix)
	if err := ctx.Err(); err != nil {
		return RetentionReceipt{}, err
	}
	if err := os.Rename(path, filepath.Join(s.backupsDir(), name)); err != nil {
		return RetentionReceipt{}, err
	}
	return RetentionReceipt{Name: name, SizeBytes: size, CompletedAt: time.Now().UTC(), SHA256: digest}, nil
}

// VerifyRetentionBackup checks the persisted receipt against the current backup
// file. Only the backup is read; no integrity scan touches the active database.
// Call before taking a retention batch lease: this method acquires admission.
func (s *Service) VerifyRetentionBackup(ctx context.Context, receipt RetentionReceipt) error {
	release, err := maintenance.ForPool(s.pool).Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	return s.VerifyRetentionBackupUnderLease(ctx, receipt)
}

// VerifyRetentionBackupUnderLease validates a receipt while the caller owns
// maintenance.ForPool(s.pool). Keep the lease until the first batch commits so
// backup deletion cannot interleave between validation and payload removal.
func (s *Service) VerifyRetentionBackupUnderLease(ctx context.Context, receipt RetentionReceipt) error {
	if receipt.CompletedAt.IsZero() || receipt.SizeBytes <= 0 || len(receipt.SHA256) != 64 {
		return fmt.Errorf("invalid retention backup receipt")
	}
	path, err := s.resolveSnapshotPath(receipt.Name)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != receipt.SizeBytes {
		return fmt.Errorf("retention backup changed")
	}
	digest, err := verifySnapshot(ctx, path)
	if err != nil {
		return err
	}
	if digest != receipt.SHA256 {
		return fmt.Errorf("retention backup changed")
	}
	return nil
}

func verifySnapshot(ctx context.Context, path string) (string, error) {
	uri := &url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	reader, err := sqlx.Open("sqlite3", uri.String())
	if err != nil {
		return "", err
	}
	defer func() { _ = reader.Close() }()
	var results []string
	if err := reader.SelectContext(ctx, &results, "PRAGMA integrity_check"); err != nil {
		return "", fmt.Errorf("verify snapshot: %w", err)
	}
	if len(results) != 1 || results[0] != "ok" {
		return "", fmt.Errorf("snapshot integrity check failed")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, &contextReader{ctx: ctx, reader: file}); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
