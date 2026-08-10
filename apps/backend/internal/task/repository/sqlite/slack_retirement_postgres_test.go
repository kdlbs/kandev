package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

// secretsTableExists is dialect-sensitive — it reads sqlite_master on SQLite and
// information_schema.tables on Postgres — so the SQLite tests give no signal
// about the Postgres branch. That branch decides whether the retirement
// migration deletes the retired Slack credentials at all: a wrong query there
// fails closed and silently leaves them encrypted at rest forever, which is the
// one outcome this migration exists to prevent.
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.

func TestPostgresDropRetiredSlackIntegration(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}

	// Recreate what an upgrading Postgres install arrives with: the retired
	// config table, and the vault holding this workspace's credentials next to
	// an unrelated secret that must survive.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS slack_configs (workspace_id TEXT PRIMARY KEY, auth_method TEXT)`); err != nil {
		t.Fatalf("create legacy slack_configs: %v", err)
	}
	if _, err := db.Exec(db.Rebind(
		`INSERT INTO slack_configs (workspace_id, auth_method) VALUES (?, ?)`), "ws-1", "cookie"); err != nil {
		t.Fatalf("seed legacy slack config: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS secrets (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, user_id TEXT NOT NULL DEFAULT '',
		encrypted_value BYTEA NOT NULL, nonce BYTEA NOT NULL)`); err != nil {
		t.Fatalf("create secrets: %v", err)
	}
	// id is the vault key and name is the human label, exactly as
	// secretadapter.Set stores them — so the seeding proves the migration
	// filters on the key rather than on the label.
	seed := []struct{ id, name string }{
		{"slack:ws-1:token", "Slack token"},
		{"slack:ws-1:cookie", "Slack d cookie"},
		{"slack:singleton:token", "Slack token"},
		{"jira:ws-1:token", "Jira token"},
		// A non-Slack key whose *label* starts with "slack:" must survive:
		// matching on the label would delete an unrelated credential.
		{"github:ws-1:token", "slack:not-a-key"},
	}
	for _, s := range seed {
		if _, err := db.Exec(db.Rebind(
			`INSERT INTO secrets (id, name, encrypted_value, nonce) VALUES (?, ?, ?, ?)`),
			s.id, s.name, []byte{0}, []byte{0}); err != nil {
			t.Fatalf("seed secret %s: %v", s.id, err)
		}
	}

	if err := repo.dropRetiredSlackIntegration(); err != nil {
		t.Fatalf("dropRetiredSlackIntegration: %v", err)
	}

	var tables int
	if err := db.Get(&tables,
		`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'slack_configs'`); err != nil {
		t.Fatalf("query information_schema: %v", err)
	}
	if tables != 0 {
		t.Fatal("slack_configs still exists after retirement")
	}

	var remaining []string
	if err := db.Select(&remaining, `SELECT id FROM secrets ORDER BY id`); err != nil {
		t.Fatalf("query secrets: %v", err)
	}
	want := []string{"github:ws-1:token", "jira:ws-1:token"}
	if len(remaining) != len(want) {
		t.Fatalf("secrets = %v, want %v", remaining, want)
	}
	for i := range want {
		if remaining[i] != want[i] {
			t.Fatalf("secrets = %v, want %v", remaining, want)
		}
	}
}

// On a fresh Postgres database the task repository's migrations run before the
// secret store creates its table, so "absent" is the normal case. The Postgres
// branch of secretsTableExists has to report that rather than erroring, or the
// boot fails.
func TestPostgresDropRetiredSlackIntegrationOnFreshDatabase(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE IF EXISTS secrets`); err != nil {
		t.Fatalf("drop secrets: %v", err)
	}

	exists, err := repo.secretsTableExists()
	if err != nil {
		t.Fatalf("secretsTableExists: %v", err)
	}
	if exists {
		t.Fatal("secretsTableExists reported a dropped table as present")
	}
	if err := repo.dropRetiredSlackIntegration(); err != nil {
		t.Fatalf("dropRetiredSlackIntegration on a fresh database: %v", err)
	}
}

// Migrations replay on every boot; the second pass must be a no-op.
func TestPostgresDropRetiredSlackIntegrationReplays(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	for i := range 2 {
		if err := repo.dropRetiredSlackIntegration(); err != nil {
			t.Fatalf("dropRetiredSlackIntegration pass %d: %v", i+1, err)
		}
	}
	// The positive case above only proves the query finds a table that exists;
	// this proves it finds the one the schema actually created.
	exists, err := repo.secretsTableExists()
	if err != nil {
		t.Fatalf("secretsTableExists: %v", err)
	}
	if _, createErr := db.Exec(`CREATE TABLE IF NOT EXISTS secrets (
		id TEXT PRIMARY KEY, name TEXT NOT NULL,
		encrypted_value BYTEA NOT NULL, nonce BYTEA NOT NULL)`); createErr != nil {
		t.Fatalf("create secrets: %v", createErr)
	}
	after, err := repo.secretsTableExists()
	if err != nil {
		t.Fatalf("secretsTableExists after create: %v", err)
	}
	if exists == after {
		t.Fatalf("secretsTableExists did not observe the table being created (before=%v after=%v)", exists, after)
	}
}
