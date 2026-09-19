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

func TestIsMissingTableError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "sqlite no such table",
			err:  errors.New("no such table: tasks"),
			want: true,
		},
		{
			name: "wrapped sqlite no such table",
			err:  fmt.Errorf("count active runs: %w", errors.New("no such table: tasks")),
			want: true,
		},
		{
			name: "postgres undefined table",
			err:  &pgconn.PgError{Code: postgresUndefinedTable},
			want: true,
		},
		{
			name: "wrapped postgres undefined table",
			err:  fmt.Errorf("count active runs: %w", &pgconn.PgError{Code: postgresUndefinedTable}),
			want: true,
		},
		{
			name: "postgres undefined column is not a missing table",
			err:  &pgconn.PgError{Code: "42703"},
			want: false,
		},
		{
			name: "unrelated",
			err:  errors.New("duplicate column name: branch_slug"),
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
			if got := IsMissingTableError(tt.err); got != tt.want {
				t.Fatalf("IsMissingTableError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsForeignKeyViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "postgres foreign key violation",
			err:  &pgconn.PgError{Code: postgresForeignKeyViolation},
			want: true,
		},
		{
			name: "wrapped postgres foreign key violation",
			err:  fmt.Errorf("insert usage event: %w", &pgconn.PgError{Code: postgresForeignKeyViolation}),
			want: true,
		},
		{
			name: "postgres unique violation is not a foreign key violation",
			err:  &pgconn.PgError{Code: "23505"},
			want: false,
		},
		{
			name: "sqlite foreign key constraint failed",
			err:  errors.New("FOREIGN KEY constraint failed"),
			want: true,
		},
		{
			name: "wrapped sqlite foreign key constraint failed",
			err:  fmt.Errorf("insert usage event: %w", errors.New("FOREIGN KEY constraint failed")),
			want: true,
		},
		{
			name: "sqlite unique constraint is not a foreign key violation",
			err:  errors.New("UNIQUE constraint failed: task_usage_events.usage_event_id"),
			want: false,
		},
		{
			name: "unrelated",
			err:  errors.New("no such table: task_usage_events"),
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
			if got := IsForeignKeyViolation(tt.err); got != tt.want {
				t.Fatalf("IsForeignKeyViolation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "postgres serialization failure",
			err:  &pgconn.PgError{Code: postgresSerializationFailure},
			want: true,
		},
		{
			name: "postgres deadlock detected",
			err:  &pgconn.PgError{Code: postgresDeadlockDetected},
			want: true,
		},
		{
			name: "wrapped postgres serialization failure",
			err:  fmt.Errorf("insert usage event: %w", &pgconn.PgError{Code: postgresSerializationFailure}),
			want: true,
		},
		{
			name: "postgres foreign key violation is not transient",
			err:  &pgconn.PgError{Code: postgresForeignKeyViolation},
			want: false,
		},
		{
			name: "sqlite busy",
			err:  errors.New("database is locked"),
			want: true,
		},
		{
			name: "sqlite locked",
			err:  errors.New("database table is locked"),
			want: true,
		},
		{
			name: "wrapped sqlite busy",
			err:  fmt.Errorf("insert usage event: %w", errors.New("database is locked")),
			want: true,
		},
		{
			name: "sqlite constraint error is not transient",
			err:  errors.New("UNIQUE constraint failed: task_usage_events.usage_event_id"),
			want: false,
		},
		{
			name: "unrelated",
			err:  errors.New("no such table: task_usage_events"),
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
			if got := IsTransientError(tt.err); got != tt.want {
				t.Fatalf("IsTransientError() = %v, want %v", got, tt.want)
			}
		})
	}
}
