package sqlite

import "testing"

// clarification_resolutions was a durable-claim table designed during this
// feature's spec review but retired before ever shipping in favor of
// upstream's CompleteActiveClarificationBundle (message-status based claim,
// see message_clarification_response.go). It never reached a released
// install, so there is no drop migration to run — the assertion here is
// simply that initSchema/runMigrations never create it, on a fresh database
// or a replayed one. The claim path itself has no statement of its own to
// assert against: CompleteActiveClarificationBundle is the only clarification
// claim in the repository package, enforced by the compiler once the retired
// repo/model files are gone (docs/specs/external-question-answering/spec.md,
// P1).
func TestClarificationResolutionsTableNeverCreated(t *testing.T) {
	repo := newRepoForSessionTests(t)

	var tables int
	if err := repo.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'clarification_resolutions'`,
	).Scan(&tables); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if tables != 0 {
		t.Fatal("clarification_resolutions exists after initSchema; the retired mechanism must not create it")
	}

	// Migrations replay on every boot; the second pass must not resurrect it.
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations replay: %v", err)
	}
	if err := repo.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'clarification_resolutions'`,
	).Scan(&tables); err != nil {
		t.Fatalf("query sqlite_master after replay: %v", err)
	}
	if tables != 0 {
		t.Fatal("clarification_resolutions exists after migration replay; the retired mechanism must not create it")
	}
}
