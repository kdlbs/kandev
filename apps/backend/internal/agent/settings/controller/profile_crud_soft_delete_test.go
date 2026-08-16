package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
)

// newSQLiteBackedController wires the controller to the real sqlite settings
// store rather than a fake. The not-found classification below depends on the
// exact error shapes that store produces (sql.ErrNoRows from the read path, a
// message from the delete path), which is precisely what a fake cannot pin.
func newSQLiteBackedController(t *testing.T) (*Controller, store.Repository) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, cleanup, err := store.Provide(db, db, log)
	if err != nil {
		t.Fatalf("settings store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	return NewController(repo, nil, registry.NewRegistry(log), nil, log), repo
}

// TestDeleteProfile_SoftDeletedRowIsNotFound is the production shape of the
// bug: the row exists but carries deleted_at, so GetAgentProfile hides it
// behind sql.ErrNoRows. Classifying that lookup by error message alone never
// matched, and the raw sql.ErrNoRows fell through to the caller's 500 branch.
func TestDeleteProfile_SoftDeletedRowIsNotFound(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	ctx := context.Background()

	profile := &models.AgentProfile{AgentID: "agent-1", Name: "Doomed", Model: "model-a"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	if _, err := ctrl.DeleteProfile(ctx, profile.ID, false); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	if _, err := repo.GetAgentProfileIncludingDeleted(ctx, profile.ID); err != nil {
		t.Fatalf("row should still exist as a soft-deleted row: %v", err)
	}

	_, err := ctrl.DeleteProfile(ctx, profile.ID, false)
	if !errors.Is(err, ErrAgentProfileNotFound) {
		t.Fatalf("second delete err = %v, want ErrAgentProfileNotFound", err)
	}
}

// TestDeleteProfile_UnknownIDIsNotFound covers the other entry into the same
// branch, where no row was ever written.
func TestDeleteProfile_UnknownIDIsNotFound(t *testing.T) {
	ctrl, _ := newSQLiteBackedController(t)

	_, err := ctrl.DeleteProfile(context.Background(), "never-existed", false)
	if !errors.Is(err, ErrAgentProfileNotFound) {
		t.Fatalf("delete err = %v, want ErrAgentProfileNotFound", err)
	}
}
