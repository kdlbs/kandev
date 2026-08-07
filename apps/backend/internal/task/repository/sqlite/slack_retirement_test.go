package sqlite

import "testing"

// The in-tree Slack integration was replaced by kandev-plugin-slack. Its
// `slack_configs` row is merely dead, but its vault entries are live Slack
// credentials, so the retirement migration has to actually remove them rather
// than leave them encrypted at rest forever.

func TestDropRetiredSlackIntegrationRemovesConfigAndSecrets(t *testing.T) {
	repo := newRepoForSessionTests(t)

	// Recreate the shape an upgrading install arrives with: the retired config
	// table, and the vault holding this workspace's token and cookie plus an
	// unrelated secret that must survive.
	if _, err := repo.db.Exec(`CREATE TABLE slack_configs (workspace_id TEXT PRIMARY KEY, auth_method TEXT)`); err != nil {
		t.Fatalf("create legacy slack_configs: %v", err)
	}
	if _, err := repo.db.Exec(`INSERT INTO slack_configs (workspace_id, auth_method) VALUES ('ws-1', 'cookie')`); err != nil {
		t.Fatalf("seed legacy slack config: %v", err)
	}
	if _, err := repo.db.Exec(`CREATE TABLE IF NOT EXISTS secrets (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, user_id TEXT NOT NULL DEFAULT '',
		encrypted_value BLOB NOT NULL, nonce BLOB NOT NULL)`); err != nil {
		t.Fatalf("create secrets: %v", err)
	}
	// id is the vault key, name is the human label — exactly what
	// secretadapter.Set stores. Seeding them with different values is the point:
	// with id == name the test would pass just as well against a migration that
	// filtered on `name`, which in production matches nothing (the label is
	// "Slack token", not "slack:...") and would leave the credentials on disk.
	for _, secret := range []struct{ id, name string }{
		{"slack:ws-1:token", "Slack token"},
		{"slack:ws-1:cookie", "Slack d cookie"},
		{"slack:singleton:token", "Slack token"},
		{"jira:ws-1:token", "Jira token"},
		// A non-Slack key whose *label* starts with "slack:". Matching on the
		// label would delete an unrelated provider's credential.
		{"github:ws-1:token", "slack:not-a-key"},
	} {
		if _, err := repo.db.Exec(
			`INSERT INTO secrets (id, name, encrypted_value, nonce) VALUES (?, ?, x'00', x'00')`,
			secret.id, secret.name,
		); err != nil {
			t.Fatalf("seed secret %s: %v", secret.id, err)
		}
	}

	if err := repo.dropRetiredSlackIntegration(); err != nil {
		t.Fatalf("dropRetiredSlackIntegration: %v", err)
	}

	var tables int
	if err := repo.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'slack_configs'`,
	).Scan(&tables); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if tables != 0 {
		t.Fatal("slack_configs still exists after retirement")
	}

	var remaining []string
	rows, err := repo.db.Query(`SELECT id FROM secrets ORDER BY id`)
	if err != nil {
		t.Fatalf("query secrets: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan secret: %v", err)
		}
		remaining = append(remaining, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate secrets: %v", err)
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

// A fresh database reaches this migration before secrets.Provide has created
// the vault, so an absent table is the normal case and must not fail the boot.
func TestDropRetiredSlackIntegrationOnFreshDatabase(t *testing.T) {
	repo := newRepoForSessionTests(t)
	if _, err := repo.db.Exec(`DROP TABLE IF EXISTS secrets`); err != nil {
		t.Fatalf("drop secrets: %v", err)
	}
	if err := repo.dropRetiredSlackIntegration(); err != nil {
		t.Fatalf("dropRetiredSlackIntegration on a fresh database: %v", err)
	}
}

// Migrations replay on every boot; the second pass must be a no-op.
func TestDropRetiredSlackIntegrationReplays(t *testing.T) {
	repo := newRepoForSessionTests(t)
	for i := range 2 {
		if err := repo.dropRetiredSlackIntegration(); err != nil {
			t.Fatalf("dropRetiredSlackIntegration pass %d: %v", i+1, err)
		}
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations replay: %v", err)
	}
}
