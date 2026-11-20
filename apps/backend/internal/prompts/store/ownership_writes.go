package store

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/prompts/models"
)

// Ownership transitions between manual, synced, and override prompts. Every
// statement is guarded by the expected current source and reports whether a
// row changed, so a concurrent edit or transition is never overwritten.
//
// alternate_content holds the version not in use: the kept local version of a
// synced prompt, or the repository version of an override. Switching between
// sync and override swaps it with content (SET reads the pre-update row on
// SQLite and PostgreSQL), so neither version is lost.

// OverrideSyncedPrompt turns a synced prompt into an override. A kept local
// version becomes the content; otherwise the override starts from the synced
// content. The repository version is kept as the other version.
func (r *sqliteRepository) OverrideSyncedPrompt(ctx context.Context, id string) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts
		SET source = ?, content = COALESCE(NULLIF(alternate_content, ''), content),
			alternate_content = content, updated_at = ?
		WHERE id = ? AND source = ?
	`, models.PromptSourceOverride, time.Now().UTC(), id, models.PromptSourceSync)
}

// RevertOverride returns an override to sync ownership and resolves any
// conflict. A stored repository version becomes the content at once; the
// local content is kept as the other version. Without a stored repository
// version the next sync writes it.
func (r *sqliteRepository) RevertOverride(ctx context.Context, id string) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts
		SET source = ?, content = COALESCE(NULLIF(alternate_content, ''), content),
			alternate_content = content, conflict = 0, updated_at = ?
		WHERE id = ? AND source = ?
	`, models.PromptSourceSync, time.Now().UTC(), id, models.PromptSourceOverride)
}

// AdoptPromptAsOverride turns a manual or built-in prompt whose name a
// repository file uses into an override of that file, keeping its content and
// storing the file's content as the repository version. conflict records that
// the user has not chosen a side yet.
func (r *sqliteRepository) AdoptPromptAsOverride(
	ctx context.Context, id, sourcePath, repositoryContent string, conflict bool,
) (bool, error) {
	conflictInt := 0
	if conflict {
		conflictInt = 1
	}
	return r.execChanged(ctx, `
		UPDATE custom_prompts
		SET source = ?, source_path = ?, alternate_content = ?, conflict = ?, updated_at = ?
		WHERE id = ? AND source = ?
	`, models.PromptSourceOverride, sourcePath, repositoryContent, conflictInt, time.Now().UTC(), id, models.PromptSourceManual)
}

// AdoptPromptAsSynced turns a manual or built-in prompt into a synced prompt
// with the repository content, keeping its own content as the local version.
func (r *sqliteRepository) AdoptPromptAsSynced(ctx context.Context, id, content, sourcePath string) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts
		SET source = ?, alternate_content = content, content = ?, source_path = ?, conflict = 0, updated_at = ?
		WHERE id = ? AND source = ?
	`, models.PromptSourceSync, content, sourcePath, time.Now().UTC(), id, models.PromptSourceManual)
}

// SetOverrideRepositoryContent records the current repository version of an
// override. It writes only when the version changed.
func (r *sqliteRepository) SetOverrideRepositoryContent(ctx context.Context, id, content string) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts SET alternate_content = ?
		WHERE id = ? AND source = ? AND alternate_content <> ?
	`, content, id, models.PromptSourceOverride, content)
}

// ClearOverrideConflict records that the user keeps an adopted override.
func (r *sqliteRepository) ClearOverrideConflict(ctx context.Context, id string) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts SET conflict = 0, updated_at = ?
		WHERE id = ? AND source = ? AND conflict = 1
	`, time.Now().UTC(), id, models.PromptSourceOverride)
}

// DetachOverride drops the conflict and repository version of an override
// whose file left the source; the override and its content stay.
func (r *sqliteRepository) DetachOverride(ctx context.Context, id string) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts SET conflict = 0, alternate_content = '', updated_at = ?
		WHERE id = ? AND source = ? AND (conflict = 1 OR alternate_content <> '')
	`, time.Now().UTC(), id, models.PromptSourceOverride)
}

// RestoreSyncedPrompt returns a synced prompt to manual ownership, with its
// kept local version as the content when there is one.
func (r *sqliteRepository) RestoreSyncedPrompt(ctx context.Context, id string) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts
		SET source = ?, source_path = '', content = COALESCE(NULLIF(alternate_content, ''), content),
			alternate_content = '', conflict = 0, updated_at = ?
		WHERE id = ? AND source = ?
	`, models.PromptSourceManual, time.Now().UTC(), id, models.PromptSourceSync)
}

func (r *sqliteRepository) execChanged(ctx context.Context, query string, args ...any) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}
