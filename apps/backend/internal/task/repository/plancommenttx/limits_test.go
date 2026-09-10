// @covers AC-TASKS-PLAN-COMMENTS-002.8
package plancommenttx_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

func TestResolveRejectsOversizedRenderedPrompt(t *testing.T) {
	repo, db := newPlanCommentTxRepo(t)
	testOversizedPlanCommentPrompt(t, repo, db)
}

func TestPostgresResolveRejectsOversizedRenderedPrompt(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := sqliterepo.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	testOversizedPlanCommentPrompt(t, repo, db)
}

func testOversizedPlanCommentPrompt(t *testing.T, repo *sqliterepo.Repository, db *sqlx.DB) {
	t.Helper()
	ctx := context.Background()
	seedPlanCommentTx(t, ctx, repo)
	if _, err := db.ExecContext(ctx, db.Rebind(`
		UPDATE task_plan_comments SET body = ? WHERE id = ?
	`), strings.Repeat("x", (1<<20)), "comment-tx"); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		resolve func(context.Context, *sqlx.Tx) error
	}{
		{name: "direct", resolve: func(ctx context.Context, tx *sqlx.Tx) error {
			_, err := plancommenttx.ResolveDirect(
				ctx, tx, db, "task-comment-tx", "session-comment-primary",
				plancomments.WithPlaceholder(""),
				[]models.TaskPlanCommentRef{{ID: "comment-tx", Version: 1}}, true, "",
			)
			return err
		}},
		{name: "queue", resolve: func(ctx context.Context, tx *sqlx.Tx) error {
			_, err := plancommenttx.ResolveQueue(
				ctx, tx, db, "task-comment-tx", "session-comment-primary",
				plancomments.WithPlaceholder(""),
				[]models.TaskPlanCommentRef{{ID: "comment-tx", Version: 1}}, true,
			)
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx, err := db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback() }()
			if err := test.resolve(ctx, tx); !errors.Is(err, plancomments.ErrRenderedTooLarge) {
				t.Fatalf("resolve error = %v", err)
			}
		})
	}
	snapshot, err := repo.ListTaskPlanComments(ctx, "task-comment-tx")
	if err != nil || len(snapshot.Comments) != 1 || snapshot.Comments[0].Version != 1 {
		t.Fatalf("rejected prompt changed comments: snapshot=%#v err=%v", snapshot, err)
	}
}
