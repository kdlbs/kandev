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
// synced prompt, or the repository version of an override. Every switch
// between sync and override keeps both versions: the operator's save writes
// them together, and RevertOverride swaps them (SET reads the pre-update row
// on SQLite and PostgreSQL).

// PromptVersions is the ownership and both versions of a synced prompt or an
// override: the content in use and the other version.
type PromptVersions struct {
	Source           string
	Content          string
	AlternateContent string
}

// VersionsOf reads a prompt's ownership and versions.
func VersionsOf(prompt *models.Prompt) PromptVersions {
	return PromptVersions{Source: prompt.Source, Content: prompt.Content, AlternateContent: prompt.AlternateContent}
}

// SavePromptVersions writes the operator's choice of version for a synced
// prompt or an override, with both versions, in one statement, and resolves
// any conflict. It writes only when the row still matches from, so a
// concurrent sync or edit is never overwritten, and never writes a prompt
// that Prompt Sync does not manage.
func (r *sqliteRepository) SavePromptVersions(ctx context.Context, id string, from, to PromptVersions) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts
		SET source = ?, content = ?, alternate_content = ?, conflict = 0, updated_at = ?
		WHERE id = ? AND source = ? AND content = ? AND alternate_content = ? AND source IN (?, ?)
	`, to.Source, to.Content, to.AlternateContent, time.Now().UTC(),
		id, from.Source, from.Content, from.AlternateContent, models.PromptSourceSync, models.PromptSourceOverride)
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
// the user has not chosen a side yet. seen is the prompt as the sync read it;
// a prompt renamed or edited since then is left alone.
func (r *sqliteRepository) AdoptPromptAsOverride(
	ctx context.Context, seen *models.Prompt, sourcePath, repositoryContent string, conflict bool,
) (bool, error) {
	conflictInt := 0
	if conflict {
		conflictInt = 1
	}
	return r.execChanged(ctx, `
		UPDATE custom_prompts
		SET source = ?, source_path = ?, alternate_content = ?, conflict = ?, updated_at = ?
		WHERE id = ? AND source = ? AND name = ? AND content = ?
	`, models.PromptSourceOverride, sourcePath, repositoryContent, conflictInt, time.Now().UTC(),
		seen.ID, models.PromptSourceManual, seen.Name, seen.Content)
}

// AdoptPromptAsSynced turns a manual or built-in prompt into a synced prompt
// with the repository content, keeping its own content as the local version.
// seen is the prompt as the sync read it; a prompt renamed or edited since
// then is left alone.
func (r *sqliteRepository) AdoptPromptAsSynced(ctx context.Context, seen *models.Prompt, content, sourcePath string) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts
		SET source = ?, alternate_content = content, content = ?, source_path = ?, conflict = 0, updated_at = ?
		WHERE id = ? AND source = ? AND name = ? AND content = ?
	`, models.PromptSourceSync, content, sourcePath, time.Now().UTC(),
		seen.ID, models.PromptSourceManual, seen.Name, seen.Content)
}

// SetOverrideRepositoryFile records the current file path and repository
// version of an override. It writes only when either value changed.
func (r *sqliteRepository) SetOverrideRepositoryFile(ctx context.Context, id, sourcePath, content string) (bool, error) {
	return r.execChanged(ctx, `
		UPDATE custom_prompts SET source_path = ?, alternate_content = ?
		WHERE id = ? AND source = ? AND (source_path <> ? OR alternate_content <> ?)
	`, sourcePath, content, id, models.PromptSourceOverride, sourcePath, content)
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
		UPDATE custom_prompts SET conflict = 0, alternate_content = '', source_path = '', updated_at = ?
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
