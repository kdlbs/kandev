package sqlite_test

import (
	"context"
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresGetChildSummaries is the PostgreSQL twin of the SQLite child
// summary tests. GetChildSummaries is one statement combining a correlated
// scalar subquery for the live-child count, an EXISTS guard on the parent, a
// SUBSTR over comment bodies, and a LIMIT — SUBSTR in particular counts
// characters on both backends but is the kind of expression where a dialect
// difference would only surface in production. Membership, ordering, the cap,
// and the truncation decision are asserted here against a real Postgres backend
// so a divergence fails loudly rather than silently changing what an agent is
// told about its children.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresGetChildSummaries(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	// tasks and task_comments are created by the task repository's schema
	// init, mirroring production boot order (see failure_postgres_test.go).
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	insert := func(id, title, state, createdAt string, archived bool) {
		t.Helper()
		archivedAt := "NULL"
		if archived {
			archivedAt = "'2026-01-01 00:00:00'"
		}
		_, err := repo.ExecRaw(ctx, fmt.Sprintf(`
			INSERT INTO tasks (id, workspace_id, title, state, parent_id, created_at, updated_at, archived_at)
			VALUES ($1, 'ws-1', $2, $3, 'pg-parent', $4, $4, %s)`, archivedAt),
			id, title, state, createdAt)
		if err != nil {
			t.Fatalf("insert task %s: %v", id, err)
		}
	}

	_, err = repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, state, created_at, updated_at)
		VALUES ('pg-parent', 'ws-1', 'Parent', 'IN_PROGRESS', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	if err != nil {
		t.Fatalf("insert parent: %v", err)
	}

	// 21 live children forces the cap and the truncation flag; the archived
	// ones must not be listed and must not be able to raise the count.
	for i := range 21 {
		insert(fmt.Sprintf("pg-child-%02d", i), fmt.Sprintf("Child %02d", i),
			"COMPLETED", fmt.Sprintf("2026-01-01 00:00:%02d", i), false)
	}
	for i := range 3 {
		insert(fmt.Sprintf("pg-gone-%02d", i), "Archived", "COMPLETED",
			"2026-01-01 00:00:00", true)
	}
	// A comment one code point past the display limit, and an older one that
	// must lose the ordering.
	_, err = repo.ExecRaw(ctx, `
		INSERT INTO task_comments (id, task_id, author_type, author_id, body, source, created_at)
		VALUES ('pg-c-old', 'pg-child-00', 'agent', 'a1', 'older', 'agent', '2026-01-01 00:00:00'),
		       ('pg-c-new', 'pg-child-00', 'user', 'u1', $1, 'user', '2026-01-02 00:00:00')`,
		repeatRune('界', 600))
	if err != nil {
		t.Fatalf("insert comments: %v", err)
	}

	summaries, truncated, err := repo.GetChildSummaries(ctx, "pg-parent")
	if err != nil {
		t.Fatalf("GetChildSummaries: %v", err)
	}
	if len(summaries) != 20 {
		t.Fatalf("len(summaries) = %d, want 20", len(summaries))
	}
	if !truncated {
		t.Error("truncated = false, want true for 21 live children")
	}
	if summaries[0].TaskID != "pg-child-00" || summaries[19].TaskID != "pg-child-19" {
		t.Errorf("ordering kept %s..%s, want pg-child-00..pg-child-19",
			summaries[0].TaskID, summaries[19].TaskID)
	}
	for _, s := range summaries {
		if s.Title == "Archived" {
			t.Fatalf("archived child %s was listed", s.TaskID)
		}
	}
	if n := utf8.RuneCountInString(summaries[0].LastComment); n != 501 {
		t.Errorf("last comment = %d code points, want 501 (limit plus one)", n)
	}
	if !utf8.ValidString(summaries[0].LastComment) {
		t.Error("last comment is not valid UTF-8")
	}

	// A parent that no longer exists yields nothing, even though its child
	// rows survive it: parent_id carries no foreign key and no cascade.
	if _, err := repo.ExecRaw(ctx, `DELETE FROM tasks WHERE id = 'pg-parent'`); err != nil {
		t.Fatalf("delete parent: %v", err)
	}
	summaries, truncated, err = repo.GetChildSummaries(ctx, "pg-parent")
	if err != nil {
		t.Fatalf("GetChildSummaries after parent delete: %v", err)
	}
	if len(summaries) != 0 || truncated {
		t.Errorf("deleted parent yielded %d rows (truncated=%v), want 0 and false",
			len(summaries), truncated)
	}
}

func repeatRune(r rune, n int) string {
	out := make([]rune, n)
	for i := range out {
		out[i] = r
	}
	return string(out)
}
