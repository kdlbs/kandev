package sqlite

import (
	"context"
	"testing"
)

// An install from before repositories gained remote_url and provider_host
// upgrades without error: the retired Azure DevOps cleanup must not reference
// those columns before the migrations that add them.
func TestRetiredAzureDevOpsCleanupRunsOnARepositoriesTableWithoutHostColumns(t *testing.T) {
	repo := newRepoForSessionTests(t)
	for _, statement := range []string{
		`ALTER TABLE repositories DROP COLUMN provider_host`,
		`ALTER TABLE repositories DROP COLUMN remote_url`,
	} {
		if _, err := repo.db.Exec(statement); err != nil {
			t.Skipf("cannot build the legacy repositories shape: %v", err)
		}
	}
	if err := repo.runMigrations(context.Background()); err != nil {
		t.Fatalf("run migrations on a legacy repositories table: %v", err)
	}
	var count int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM repositories WHERE provider_host = '' AND remote_url = ''`).Scan(&count); err != nil {
		t.Fatalf("columns were not restored: %v", err)
	}
}
