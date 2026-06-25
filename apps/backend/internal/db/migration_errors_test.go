package db

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsDuplicateColumnError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "sqlite duplicate column",
			err:  errors.New("duplicate column name: branch_slug"),
			want: true,
		},
		{
			name: "postgres duplicate column",
			err: &pgconn.PgError{
				Code:    postgresDuplicateColumn,
				Message: `column "branch_slug" of relation "task_session_worktrees" already exists`,
			},
			want: true,
		},
		{
			name: "wrapped postgres duplicate column",
			err: fmt.Errorf("add column: %w", &pgconn.PgError{
				Code:    postgresDuplicateColumn,
				Message: `column "branch_slug" of relation "task_session_worktrees" already exists`,
			}),
			want: true,
		},
		{
			name: "postgres duplicate table is not a duplicate column",
			err: &pgconn.PgError{
				Code:    postgresDuplicateTable,
				Message: `relation "task_session_worktrees" already exists`,
			},
			want: false,
		},
		{
			name: "unrelated",
			err:  errors.New("no such table: task_session_worktrees"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDuplicateColumnError(tt.err); got != tt.want {
				t.Fatalf("IsDuplicateColumnError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAlreadyExistsError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "sqlite duplicate column",
			err:  errors.New("duplicate column name: branch_slug"),
			want: true,
		},
		{
			name: "sqlite already exists",
			err:  errors.New("index idx_task_session_worktrees_status already exists"),
			want: true,
		},
		{
			name: "sqlite table already exists",
			err:  errors.New("table task_session_worktrees already exists"),
			want: true,
		},
		{
			name: "sqlite unrelated already exists text",
			err:  errors.New("migration failed because a dependency already exists in an invalid state"),
			want: false,
		},
		{
			name: "postgres duplicate column",
			err:  &pgconn.PgError{Code: postgresDuplicateColumn},
			want: true,
		},
		{
			name: "wrapped postgres duplicate column",
			err:  fmt.Errorf("add column: %w", &pgconn.PgError{Code: postgresDuplicateColumn}),
			want: true,
		},
		{
			name: "postgres duplicate table",
			err:  &pgconn.PgError{Code: postgresDuplicateTable},
			want: true,
		},
		{
			name: "postgres duplicate object",
			err:  &pgconn.PgError{Code: postgresDuplicateObject},
			want: true,
		},
		{
			name: "postgres undefined column",
			err:  &pgconn.PgError{Code: "42703"},
			want: false,
		},
		{
			name: "postgres non-duplicate code ignores broad message text",
			err: &pgconn.PgError{
				Code:    "42703",
				Message: `relation "task_session_worktrees" already exists`,
			},
			want: false,
		},
		{
			name: "unrelated",
			err:  errors.New("no such table: task_session_worktrees"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAlreadyExistsError(tt.err); got != tt.want {
				t.Fatalf("IsAlreadyExistsError() = %v, want %v", got, tt.want)
			}
		})
	}
}
