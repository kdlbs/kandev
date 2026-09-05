package plancommenttx_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestResolveAndConsumePlanComments(t *testing.T) {
	repo, db := newPlanCommentTxRepo(t)
	ctx := context.Background()
	seedPlanCommentTx(t, ctx, repo)

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := plancommenttx.LockTask(ctx, tx, db, "task-comment-tx"); err != nil {
		t.Fatal(err)
	}
	resolved, err := plancommenttx.Resolve(
		ctx, tx, db, "task-comment-tx", "session-comment-primary",
		"prefix\n"+"\x00kandev-plan-comments\x00"+"typed body",
		[]models.TaskPlanCommentRef{{ID: "comment-tx", Version: 1}}, true,
	)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantContent := "prefix\n### Plan Comments\n\n```\nselected\n```\n> stored body\n\n---\n\ntyped body"
	if resolved.Content != wantContent {
		t.Fatalf("resolved content = %q, want %q", resolved.Content, wantContent)
	}
	snapshot, err := plancommenttx.Consume(ctx, tx, db, resolved)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if snapshot.Revision != 2 || len(snapshot.Comments) != 0 {
		t.Fatalf("consumed snapshot = %#v", snapshot)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestResolveRejectsStaleDuplicateAndNonPrimaryReferences(t *testing.T) {
	repo, db := newPlanCommentTxRepo(t)
	ctx := context.Background()
	seedPlanCommentTx(t, ctx, repo)

	t.Run("stale", func(t *testing.T) {
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		_, err = plancommenttx.Resolve(ctx, tx, db, "task-comment-tx", "session-comment-primary",
			"\x00kandev-plan-comments\x00", []models.TaskPlanCommentRef{{ID: "comment-tx", Version: 9}}, false)
		var changed *plancommenttx.CommentsChangedError
		if !errors.As(err, &changed) || changed.Snapshot == nil || changed.Snapshot.Revision != 1 {
			t.Fatalf("stale error = %#v", err)
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		ref := models.TaskPlanCommentRef{ID: "comment-tx", Version: 1}
		_, err = plancommenttx.Resolve(ctx, tx, db, "task-comment-tx", "session-comment-primary",
			"\x00kandev-plan-comments\x00", []models.TaskPlanCommentRef{ref, ref}, false)
		if !errors.Is(err, repoerrors.ErrTaskPlanCommentsChanged) {
			t.Fatalf("duplicate error = %v", err)
		}
	})

	t.Run("non-primary", func(t *testing.T) {
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		_, err = plancommenttx.Resolve(ctx, tx, db, "task-comment-tx", "session-comment-secondary",
			"\x00kandev-plan-comments\x00", []models.TaskPlanCommentRef{{ID: "comment-tx", Version: 1}}, true)
		var changed *plancommenttx.PrimarySessionChangedError
		if !errors.As(err, &changed) || changed.SessionID != "session-comment-primary" {
			t.Fatalf("primary error = %#v", err)
		}
	})
}

func newPlanCommentTxRepo(t *testing.T) (*sqliterepo.Repository, *sqlx.DB) {
	t.Helper()
	dbConn, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "plan-comment-tx.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := sqliterepo.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return repo, db
}

func seedPlanCommentTx(t *testing.T, ctx context.Context, repo *sqliterepo.Repository) {
	t.Helper()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-comment-tx", Title: "Task"}); err != nil {
		t.Fatal(err)
	}
	for _, session := range []*models.TaskSession{
		{ID: "session-comment-primary", TaskID: "task-comment-tx", State: models.TaskSessionStateWaitingForInput},
		{ID: "session-comment-secondary", TaskID: "task-comment-tx", State: models.TaskSessionStateWaitingForInput},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SetSessionPrimary(ctx, "session-comment-primary"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-comment-tx", TaskID: "task-comment-tx", Content: "Plan",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-tx", TaskID: "task-comment-tx", PlanID: "plan-comment-tx",
		Body: "stored body", SelectedText: "selected", AnchorFrom: 1, AnchorTo: 4,
	}); err != nil {
		t.Fatal(err)
	}
}
