package sqlite

import (
	"context"
	"testing"
)

func TestRetiredAzureDevOpsSchemaIsDropped(t *testing.T) {
	repo := newRepoForSessionTests(t)
	for _, table := range retiredAzureDevOpsTables {
		if _, err := repo.db.Exec(`CREATE TABLE ` + table + ` (id TEXT PRIMARY KEY)`); err != nil {
			t.Fatalf("create legacy table %s: %v", table, err)
		}
	}
	if _, err := repo.db.Exec(`CREATE TABLE secrets (id TEXT PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatalf("create secrets table: %v", err)
	}
	for _, id := range []string{"azure_devops:ws-1:pat", "azure_devops:ws-2:pat", "jira:ws-1:token"} {
		if _, err := repo.db.Exec(`INSERT INTO secrets (id, name) VALUES (?, ?)`, id, id); err != nil {
			t.Fatalf("insert secret %s: %v", id, err)
		}
	}

	for range 2 {
		if err := repo.runMigrations(context.Background()); err != nil {
			t.Fatalf("run migrations: %v", err)
		}
	}

	for _, table := range retiredAzureDevOpsTables {
		if got := countRows(t, repo,
			`SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?`, table); got != 0 {
			t.Fatalf("table %s still exists after migration", table)
		}
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM secrets WHERE id LIKE 'azure_devops:%'`); got != 0 {
		t.Fatalf("azure devops secrets after migration = %d, want 0", got)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM secrets WHERE id = 'jira:ws-1:token'`); got != 1 {
		t.Fatalf("unrelated secret rows after migration = %d, want 1", got)
	}
}

func TestRetiredAzureDevOpsCleanupToleratesMissingSecretsTable(t *testing.T) {
	repo := newRepoForSessionTests(t)
	if _, err := repo.db.Exec(`DROP TABLE IF EXISTS secrets`); err != nil {
		t.Fatalf("drop secrets table: %v", err)
	}
	if err := repo.runMigrations(context.Background()); err != nil {
		t.Fatalf("run migrations without a secrets table: %v", err)
	}
}
