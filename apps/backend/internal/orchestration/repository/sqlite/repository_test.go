package sqlite

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestration/models"
	_ "github.com/mattn/go-sqlite3"
	"testing"
)

func TestRegistryWithoutOfficePreservesExistingRows(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	for _, q := range []string{
		"PRAGMA foreign_keys=ON",
		"CREATE TABLE workspaces(id TEXT PRIMARY KEY)",
		"CREATE TABLE tasks(id TEXT PRIMARY KEY)",
		"CREATE TABLE task_comments(id TEXT PRIMARY KEY)",
		"CREATE TABLE agent_profiles(id TEXT PRIMARY KEY,workspace_id TEXT,name TEXT,role TEXT,deleted_at TEXT)",
		"INSERT INTO workspaces VALUES('workspace')",
		"INSERT INTO agent_profiles VALUES('chief','workspace','Chief of staff','assistant',NULL)",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	repo := New(db, db)
	if err := repo.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.SaveOrchestratorRole(ctx, &models.OrchestratorRole{ID: "custom", Name: "Custom role", Instructions: "Preserve my instructions"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.RegisterOrchestrator(ctx, "chief", "workspace", "custom"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Migrate(); err != nil {
		t.Fatal(err)
	}
	role, err := repo.GetOrchestratorRole(ctx, "custom")
	if err != nil || role.Instructions != "Preserve my instructions" {
		t.Fatalf("role lost on migration: %v %v", role, err)
	}
	ids, err := repo.ListOrchestratorIDs(ctx, "workspace")
	if err != nil || len(ids) != 1 || ids[0] != "chief" {
		t.Fatalf("identity changed: %v %v", ids, err)
	}
	if err := repo.RegisterOrchestrator(ctx, "chief", "different-workspace", "custom"); err == nil {
		t.Fatal("cross-workspace registration accepted")
	}
	if _, err := db.Exec("UPDATE agent_profiles SET deleted_at=CURRENT_TIMESTAMP WHERE id='chief'"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Get(&count, "SELECT count(*) FROM workspace_orchestrators"); err != nil || count != 0 {
		t.Fatalf("soft delete left registration: %d %v", count, err)
	}
}
