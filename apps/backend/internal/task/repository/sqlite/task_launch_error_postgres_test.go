package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresRemoveTaskMetadataKeyIfStampUsesJSONB(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, title, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-pg-launch-error-cas", "Launch error CAS", `{"last_launch_error":{"stamp":"new-stamp","message":"boom"}}`, now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	removed, err := repo.RemoveTaskMetadataKeyIfStamp(context.Background(), "task-pg-launch-error-cas", "last_launch_error", "old-stamp")
	if err != nil {
		t.Fatalf("stale RemoveTaskMetadataKeyIfStamp: %v", err)
	}
	if removed {
		t.Fatal("stale stamp removed the newer launch error")
	}
	removed, err = repo.RemoveTaskMetadataKeyIfStamp(context.Background(), "task-pg-launch-error-cas", "last_launch_error", "new-stamp")
	if err != nil {
		t.Fatalf("current RemoveTaskMetadataKeyIfStamp: %v", err)
	}
	if !removed {
		t.Fatal("current stamp did not remove the launch error")
	}
}
