package store

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/prompts/models"
)

// Ownership transitions between manual, synced, and override prompts. Every
// statement is guarded by the expected current source and reports whether a
// row changed, so a concurrent edit or transition is never overwritten.

// OverrideSyncedPrompt turns a synced prompt into an override. A kept local
// version becomes the content again and is consumed; otherwise the override
// starts from the synced content.
func (r *sqliteRepository) OverrideSyncedPrompt(ctx context.Context, id string) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts
		SET source = ?, content = CASE WHEN local_content <> '' THEN local_content ELSE content END,
			local_content = '', updated_at = ?
		WHERE id = ? AND source = ?
	`, models.PromptSourceOverride, time.Now().UTC(), id, models.PromptSourceSync)
}

// RevertOverride returns an override to sync ownership, keeping its content
// as the local version and resolving any conflict. The next sync writes the
// repository version.
func (r *sqliteRepository) RevertOverride(ctx context.Context, id string) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts SET source = ?, local_content = content, conflict = 0, updated_at = ?
		WHERE id = ? AND source = ?
	`, models.PromptSourceSync, time.Now().UTC(), id, models.PromptSourceOverride)
}

// AdoptPromptAsOverride turns a manual or built-in prompt whose name a
// repository file uses into an override of that file, keeping its content.
// conflict records that the user has not chosen a side yet.
func (r *sqliteRepository) AdoptPromptAsOverride(ctx context.Context, id, sourcePath string, conflict bool) (bool, error) {
	conflictInt := 0
	if conflict {
		conflictInt = 1
	}
	return r.execChanged(ctx, `
		UPDATE custom_prompts SET source = ?, source_path = ?, conflict = ?, updated_at = ?
		WHERE id = ? AND source = ?
	`, models.PromptSourceOverride, sourcePath, conflictInt, time.Now().UTC(), id, models.PromptSourceManual)
}

// AdoptPromptAsSynced turns a manual or built-in prompt into a synced prompt
// with the repository content, keeping its own content as the local version.
func (r *sqliteRepository) AdoptPromptAsSynced(ctx context.Context, id, content, sourcePath string) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts
		SET source = ?, local_content = content, content = ?, source_path = ?, conflict = 0, updated_at = ?
		WHERE id = ? AND source = ?
	`, models.PromptSourceSync, content, sourcePath, time.Now().UTC(), id, models.PromptSourceManual)
}

// ClearOverrideConflict records that the user keeps an adopted override.
func (r *sqliteRepository) ClearOverrideConflict(ctx context.Context, id string) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts SET conflict = 0, updated_at = ?
		WHERE id = ? AND source = ? AND conflict = 1
	`, time.Now().UTC(), id, models.PromptSourceOverride)
}

// RestoreSyncedPrompt returns a synced prompt to manual ownership, with its
// kept local version as the content when there is one.
func (r *sqliteRepository) RestoreSyncedPrompt(ctx context.Context, id string) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts
		SET source = ?, source_path = '', content = CASE WHEN local_content <> '' THEN local_content ELSE content END,
			local_content = '', conflict = 0, updated_at = ?
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
