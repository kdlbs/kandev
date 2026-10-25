package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestBindRebindsAfterInExpansion(t *testing.T) {
	fake := &rebindRecorder{placeholder: "$"}
	query, args, err := Bind(fake, "SELECT * FROM tasks WHERE id IN (?) AND state = ?", []string{"a", "b"}, "open")
	if err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if query != "SELECT * FROM tasks WHERE id IN ($1, $2) AND state = $3" {
		t.Fatalf("Bind() query = %q", query)
	}
	if len(args) != 3 || args[0] != "a" || args[1] != "b" || args[2] != "open" {
		t.Fatalf("Bind() args = %#v", args)
	}
	if fake.calls != 1 {
		t.Fatalf("Rebind() calls = %d, want 1", fake.calls)
	}
}

func TestExecContextBindsAtFinalBoundary(t *testing.T) {
	fake := &execRecorder{rebindRecorder: rebindRecorder{placeholder: "$"}}
	_, err := ExecContext(context.Background(), fake, "UPDATE tasks SET state = ? WHERE id = ?", "done", "task-1")
	if err != nil {
		t.Fatalf("ExecContext() error = %v", err)
	}
	if fake.query != "UPDATE tasks SET state = $1 WHERE id = $2" {
		t.Fatalf("ExecContext() query = %q", fake.query)
	}
	if fake.calls != 1 {
		t.Fatalf("Rebind() calls = %d, want 1", fake.calls)
	}
}

type rebindRecorder struct {
	placeholder string
	calls       int
}

func (r *rebindRecorder) Rebind(query string) string {
	r.calls++
	return sqlx.Rebind(sqlx.DOLLAR, query)
}

type execRecorder struct {
	rebindRecorder
	query string
	args  []any
}

func (r *execRecorder) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	r.query = query
	r.args = args
	return fakeResult{}, nil
}

type fakeResult struct{}

func (fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (fakeResult) RowsAffected() (int64, error) { return 0, nil }
