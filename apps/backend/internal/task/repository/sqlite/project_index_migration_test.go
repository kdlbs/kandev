package sqlite

import "testing"

func TestTasksProjectIDIndexExistsAfterMigration(t *testing.T) {
	repo := newRepoForEntityTests(t)

	var indexName string
	if err := repo.db.Get(&indexName, `
		SELECT name
		FROM sqlite_master
		WHERE type = 'index' AND name = 'idx_tasks_project_id'
	`); err != nil {
		t.Fatalf("project_id index missing after task repository migration: %v", err)
	}
	if indexName != "idx_tasks_project_id" {
		t.Fatalf("index name = %q, want idx_tasks_project_id", indexName)
	}
}
