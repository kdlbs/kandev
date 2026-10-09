package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func TestNewWithInitializedDBReusesSchemaWithoutMigrationReplay(t *testing.T) {
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "initialized.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	initial, err := NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	now := time.Now().UTC()
	if err := initial.CreateWorkspace(context.Background(), &models.Workspace{
		ID: "seed", Name: "Seed", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if _, err := database.Exec(`PRAGMA query_only=ON`); err != nil {
		t.Fatalf("protect initialized schema: %v", err)
	}
	initialized := NewWithInitializedDB(database, database, nil)
	workspace, err := initialized.GetWorkspace(context.Background(), "seed")
	if err != nil || workspace == nil || workspace.Name != "Seed" {
		t.Fatalf("read existing workspace: workspace=%+v err=%v", workspace, err)
	}
	if _, err := database.Exec(`PRAGMA query_only=OFF`); err != nil {
		t.Fatalf("restore writes: %v", err)
	}
	if err := initialized.CreateWorkspace(context.Background(), &models.Workspace{
		ID: "new", Name: "New", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("write through initialized repository: %v", err)
	}
}
