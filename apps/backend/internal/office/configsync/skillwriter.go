package configsync

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/models"
)

// ErrSkillLocatorChanged is returned by UpdateSkillProjection when the CAS
// guard on office_skills.source_locator fails: another writer moved the
// locator since this run read it (AC-OFFICE-CONFIG-SYNC-003.5e). The caller
// must leave the skill untouched, warn, and continue — not retry inside the
// same transaction, where a re-read would see the same pre-commit snapshot.
var ErrSkillLocatorChanged = errors.New("configsync: skill source locator changed since read")

// SkillProjection is the owned-field view of a skill sync writes
// (AC-OFFICE-CONFIG-SYNC-003.5c). It excludes content_hash, which is derived
// rather than fetched and is recomputed on every write rather than compared
// (AC-OFFICE-CONFIG-SYNC-003.5e).
type SkillProjection struct {
	Name          string
	Description   string
	SourceType    models.SkillSourceType
	Content       string
	FileInventory string
}

// CreateSkill inserts a new sync-owned office_skills row and returns its ID.
// sourceLocator seeds the row's source_locator — materializeInline ignores
// it at runtime (R5-F2), but it is a content_hash input and the CAS guard
// future UpdateSkillProjection calls check.
//
// This is a standalone writer, not a widened UpdateSkillConfigFields: that
// shipped writer owns a narrower, CAS-on-inventory-retry contract for
// filesystem import (internal/office/config/import.go) that sync must not
// disturb (AC-OFFICE-CONFIG-SYNC-003.5d).
func CreateSkill(
	ctx context.Context, ext sqlx.ExtContext, db *sqlx.DB,
	workspaceID, slug, sourceLocator string, proj SkillProjection,
) (string, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	hash := models.SkillPackageContentHash(proj.Content, proj.FileInventory, sourceLocator)
	_, err := ext.ExecContext(ctx, db.Rebind(`
		INSERT INTO office_skills (
			id, workspace_id, name, slug, description, source_type,
			source_locator, content, file_inventory, content_hash,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), id, workspaceID, proj.Name, slug, proj.Description, proj.SourceType,
		sourceLocator, proj.Content, proj.FileInventory, hash, now, now)
	if err != nil {
		return "", err
	}
	return id, nil
}

// UpdateSkillProjection applies reconcile's guarded six-column skill update
// (name, description, source_type, content, file_inventory, content_hash) in
// one UPDATE, conditional on source_locator still equal to
// currentSourceLocator — the value read when the run started
// (AC-OFFICE-CONFIG-SYNC-003.5e). content_hash is recomputed from the fetched
// content, inventory, and the unchanged locator; sync never writes
// source_locator itself (AC-OFFICE-CONFIG-SYNC-003.5d).
//
// A zero-row result means another writer moved the locator between the read
// and this write. It returns ErrSkillLocatorChanged without retrying: the
// caller records a warning naming the skill and leaves it untouched for the
// next run to reconcile.
func UpdateSkillProjection(
	ctx context.Context, ext sqlx.ExtContext, db *sqlx.DB,
	skillID, currentSourceLocator string, proj SkillProjection,
) error {
	now := time.Now().UTC()
	hash := models.SkillPackageContentHash(proj.Content, proj.FileInventory, currentSourceLocator)
	res, err := ext.ExecContext(ctx, db.Rebind(`
		UPDATE office_skills SET
			name = ?, description = ?, source_type = ?,
			content = ?, file_inventory = ?, content_hash = ?,
			updated_at = ?
		WHERE id = ? AND source_locator = ?
	`), proj.Name, proj.Description, proj.SourceType,
		proj.Content, proj.FileInventory, hash, now,
		skillID, currentSourceLocator)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrSkillLocatorChanged
	}
	return nil
}
