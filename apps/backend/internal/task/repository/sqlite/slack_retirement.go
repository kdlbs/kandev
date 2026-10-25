package sqlite

import (
	"fmt"

	internaldb "github.com/kandev/kandev/internal/db"
)

// dropRetiredSlackIntegration removes the storage the in-tree Slack
// integration left behind. Slack now ships as kandev-plugin-slack, which
// keeps its own configuration under the plugin config/secret namespace, so
// the old `slack_configs` row and the `slack:<workspace>:token` /
// `slack:<workspace>:cookie` vault entries are unreachable by any running
// code. Dropping them rather than orphaning them matters because the vault
// entries are live Slack credentials: leaving them encrypted-at-rest forever
// is a credential-retention problem, not merely dead data.
//
// Both statements are idempotent, and the secrets delete is skipped when the
// vault table does not exist yet — on a fresh database the task repository's
// migrations run before secrets.Provide creates it, and a fresh database has
// nothing to clean up anyway.
func (r *Repository) dropRetiredSlackIntegration() error {
	if err := r.migrate.Apply("drop_slack_configs", `DROP TABLE IF EXISTS slack_configs`); err != nil {
		return fmt.Errorf("drop retired slack configs: %w", err)
	}
	exists, err := r.secretsTableExists()
	if err != nil {
		return fmt.Errorf("drop retired slack integration: %w", err)
	}
	if !exists {
		return nil
	}
	// LIKE with a literal pattern rather than a bound parameter: this file is
	// shared with PostgreSQL, whose placeholder syntax differs from SQLite's.
	if err := r.migrate.Apply("delete_slack_secrets", `DELETE FROM secrets WHERE id LIKE 'slack:%'`); err != nil {
		return fmt.Errorf("delete retired slack secrets: %w", err)
	}
	return nil
}

// secretsTableExists reports whether the shared vault table has been created
// yet. The task repository and the secret store share one database but
// initialize independently, and on a fresh boot this repository's migrations
// run first — so "absent" is the normal fresh-install case, not an error.
func (r *Repository) secretsTableExists() (bool, error) {
	return internaldb.TableExists(r.db, "secrets")
}
