package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newArmingScanTestRepo adds the `workspaces` table (owned by the task
// package's schema, not office) with a created_at column, so the ordering
// under test has something to order by.
func newArmingScanTestRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	repo := newTestRepo(t)
	if _, err := repo.ExecRaw(context.Background(), `
		CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	return repo
}

// TestListWorkspaceIDsOrdered_OrdersByCreatedAtThenID seeds workspace rows
// with created_at values out of insertion order and asserts the returned
// order follows created_at (ties on id), not insertion or primary-key order
// — AC-OFFICE-ROUTINE-ARMING-003.2's scan must walk workspaces in a stable,
// deterministic sequence.
func TestListWorkspaceIDsOrdered_OrdersByCreatedAtThenID(t *testing.T) {
	repo := newArmingScanTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// IDs deliberately sort alphabetically in the opposite order from
	// created_at ("ws-mike" < "ws-yankee" < "ws-zulu"), so a query that
	// silently fell back to ORDER BY id would produce a different result
	// than one ordering by created_at and be caught here.
	seed := []struct {
		id        string
		createdAt time.Time
	}{
		{"ws-mike", base.Add(2 * time.Hour)},
		{"ws-zulu", base},
		{"ws-yankee", base.Add(1 * time.Hour)},
	}
	for _, s := range seed {
		if _, err := repo.ExecRaw(ctx,
			`INSERT INTO workspaces (id, created_at) VALUES (?, ?)`, s.id, s.createdAt); err != nil {
			t.Fatalf("seed workspace %s: %v", s.id, err)
		}
	}

	ids, err := repo.ListWorkspaceIDsOrdered(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaceIDsOrdered: %v", err)
	}

	want := []string{"ws-zulu", "ws-yankee", "ws-mike"}
	if len(ids) != len(want) {
		t.Fatalf("got %v, want %v", ids, want)
	}
	for i, id := range want {
		if ids[i] != id {
			t.Errorf("position %d: got %q, want %q (full result: %v)", i, ids[i], id, ids)
		}
	}
}

// TestListWorkspaceIDsOrdered_TiesBreakByID seeds two workspaces with the
// identical created_at and asserts the tie is broken by id ascending.
func TestListWorkspaceIDsOrdered_TiesBreakByID(t *testing.T) {
	repo := newArmingScanTestRepo(t)
	ctx := context.Background()
	same := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, id := range []string{"ws-b", "ws-a"} {
		if _, err := repo.ExecRaw(ctx,
			`INSERT INTO workspaces (id, created_at) VALUES (?, ?)`, id, same); err != nil {
			t.Fatalf("seed workspace %s: %v", id, err)
		}
	}

	ids, err := repo.ListWorkspaceIDsOrdered(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaceIDsOrdered: %v", err)
	}
	want := []string{"ws-a", "ws-b"}
	if len(ids) != len(want) || ids[0] != want[0] || ids[1] != want[1] {
		t.Fatalf("got %v, want %v", ids, want)
	}
}

// TestListRoutinesOrdered_OrdersByCreatedAtThenID mirrors the workspace
// ordering test for a single workspace's routines: seeded out of order,
// asserted back in created_at order (ties on id).
func TestListRoutinesOrdered_OrdersByCreatedAtThenID(t *testing.T) {
	repo := newArmingScanTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if _, err := repo.ExecRaw(ctx,
		`INSERT INTO workspaces (id, created_at) VALUES (?, ?)`, "ws-1", base); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	// IDs deliberately sort alphabetically in the opposite order from
	// created_at, so a query that silently fell back to ORDER BY id would
	// produce a different result than one ordering by created_at.
	seed := []struct {
		id        string
		createdAt time.Time
	}{
		{"routine-mike", base.Add(2 * time.Hour)},
		{"routine-zulu", base},
		{"routine-yankee", base.Add(1 * time.Hour)},
	}
	for _, s := range seed {
		if _, err := repo.ExecRaw(ctx,
			`INSERT INTO office_routines (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			s.id, "ws-1", s.id, s.createdAt, s.createdAt); err != nil {
			t.Fatalf("seed routine %s: %v", s.id, err)
		}
	}

	routines, err := repo.ListRoutinesOrdered(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRoutinesOrdered: %v", err)
	}

	want := []string{"routine-zulu", "routine-yankee", "routine-mike"}
	if len(routines) != len(want) {
		t.Fatalf("got %d routines, want %d", len(routines), len(want))
	}
	for i, id := range want {
		if routines[i].ID != id {
			t.Errorf("position %d: got %q, want %q", i, routines[i].ID, id)
		}
	}
}

// TestListRoutinesOrdered_ScopedToWorkspace guards against the query
// dropping the workspace_id filter: a routine seeded under a different
// workspace must not appear in the result.
func TestListRoutinesOrdered_ScopedToWorkspace(t *testing.T) {
	repo := newArmingScanTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, id := range []string{"ws-1", "ws-2"} {
		if _, err := repo.ExecRaw(ctx,
			`INSERT INTO workspaces (id, created_at) VALUES (?, ?)`, id, now); err != nil {
			t.Fatalf("seed workspace %s: %v", id, err)
		}
	}
	if _, err := repo.ExecRaw(ctx,
		`INSERT INTO office_routines (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"routine-ws1", "ws-1", "routine-ws1", now, now); err != nil {
		t.Fatalf("seed routine: %v", err)
	}
	if _, err := repo.ExecRaw(ctx,
		`INSERT INTO office_routines (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"routine-ws2", "ws-2", "routine-ws2", now, now); err != nil {
		t.Fatalf("seed routine: %v", err)
	}

	routines, err := repo.ListRoutinesOrdered(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRoutinesOrdered: %v", err)
	}
	if len(routines) != 1 || routines[0].ID != "routine-ws1" {
		t.Fatalf("got %v, want exactly [routine-ws1]", routines)
	}
}
