package repository

import "testing"

func TestSQLiteRepository_CreatesPerformanceIndexes(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	rows, err := repo.DB().Query(`SELECT name FROM sqlite_master WHERE type = 'index'`)
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			t.Errorf("close rows: %v", closeErr)
		}
	}()

	indexNames := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index name: %v", err)
		}
		indexNames[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate indexes: %v", err)
	}

	required := []string{
		"idx_tasks_workspace_archived",
		"idx_messages_metadata_tool_call_id",
		"idx_messages_metadata_pending_id",
	}
	for _, idx := range required {
		if _, ok := indexNames[idx]; !ok {
			t.Fatalf("missing index %q", idx)
		}
	}
}
