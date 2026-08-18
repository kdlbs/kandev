package sqlite

import (
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/gitcredentials"
)

const repositoryCredentialOriginBackfillMigrationName = "repositories.provider_host.credential_origin_backfill"

// migrateRepositoryCredentialOriginBackfill fills provider_host on legacy or
// plugin-associated repository rows whose remote_url is an exact public
// github.com clone URL (HTTPS or SSH) but whose provider_host is still empty.
// Managed Git credential issuance requires provider_host to trust a host, and
// "repositories.provider_host.github_backfill" only covers rows whose
// provider column is literally "github" - it never sees a repository added
// before that column existed, imported through a plugin, or otherwise left
// with an empty/non-github provider value even though it clones GitHub.
//
// This reuses gitcredentials.ResolveRepositoryIdentity, the same policy the
// executor and credential broker apply at launch time, so a row only gets
// backfilled here if it would already be trusted at runtime. It never
// rewrites provider, remote_url, or any other repository identity field,
// never touches a row whose provider_host is already set, and silently
// leaves ambiguous, custom-host, or malformed rows for manual repair -
// runMigrations' best-effort contract logs failures at WARN and never aborts
// startup.
func (r *Repository) migrateRepositoryCredentialOriginBackfill() {
	type candidateRow struct {
		id, provider, remoteURL string
	}

	rows, err := r.db.Queryx(`
		SELECT id, provider, remote_url FROM repositories
		WHERE TRIM(COALESCE(provider_host, '')) = ''`)
	if err != nil {
		r.warnRepositoryCredentialOriginMigration(err)
		return
	}
	var candidates []candidateRow
	for rows.Next() {
		var c candidateRow
		if scanErr := rows.Scan(&c.id, &c.provider, &c.remoteURL); scanErr != nil {
			_ = rows.Close()
			r.warnRepositoryCredentialOriginMigration(scanErr)
			return
		}
		candidates = append(candidates, c)
	}
	closeErr := rows.Close()
	if err := rows.Err(); err != nil {
		r.warnRepositoryCredentialOriginMigration(err)
		return
	}
	if closeErr != nil {
		r.warnRepositoryCredentialOriginMigration(closeErr)
		return
	}

	for _, c := range candidates {
		// Already covered by repositories.provider_host.github_backfill.
		if strings.EqualFold(strings.TrimSpace(c.provider), "github") {
			continue
		}
		identity, resolveErr := gitcredentials.ResolveRepositoryIdentity(gitcredentials.RepositoryIdentityInput{
			RepositoryID: c.id, Provider: c.provider, RemoteURL: c.remoteURL,
		})
		if resolveErr != nil || !strings.EqualFold(identity.Host, "github.com") {
			continue
		}
		if _, execErr := r.db.Exec(r.db.Rebind(`
			UPDATE repositories SET provider_host = 'https://github.com'
			WHERE id = ? AND TRIM(COALESCE(provider_host, '')) = ''`), c.id); execErr != nil {
			r.warnRepositoryCredentialOriginMigration(execErr)
			return
		}
	}
}

func (r *Repository) warnRepositoryCredentialOriginMigration(err error) {
	if r.log == nil {
		return
	}
	r.log.Warn("migration failed", zap.String("name", repositoryCredentialOriginBackfillMigrationName), zap.Error(err))
}
