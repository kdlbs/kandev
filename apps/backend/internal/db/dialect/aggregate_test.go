package dialect

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
)

func TestOrderedIDConcat_FragmentShape(t *testing.T) {
	got := OrderedIDConcat(SQLite3, "parent_id = p.id")
	want := "(SELECT GROUP_CONCAT(w.id, ',') FROM (SELECT id FROM tasks WHERE parent_id = p.id ORDER BY id) w)"
	if got != want {
		t.Errorf("sqlite: got %q, want %q", got, want)
	}

	got = OrderedIDConcat(PGX, "parent_id = p.id")
	want = "(SELECT string_agg(w.id, ',' ORDER BY w.id) FROM (SELECT id FROM tasks WHERE parent_id = p.id) w)"
	if got != want {
		t.Errorf("pgx: got %q, want %q", got, want)
	}
}

// TestOrderedIDConcat_SQLite_OrdersAscendingByID proves the fragment
// actually orders ascending by id when executed, not just that its text
// looks right: ids are inserted out of order (and lexicographically
// non-sequential, UUID-shaped) so an unordered GROUP_CONCAT would produce a
// different string.
func TestOrderedIDConcat_SQLite_OrdersAscendingByID(t *testing.T) {
	tmpDir := t.TempDir()
	rawDB, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlxDB := sqlx.NewDb(rawDB, SQLite3)
	t.Cleanup(func() { _ = sqlxDB.Close() })

	if _, err := sqlxDB.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, parent_id TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	// Insertion order deliberately disagrees with ascending id order.
	for _, id := range []string{
		"f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"0e02b2c3-58cc-4372-a567-f47ac10bd479",
		"a567f47a-58cc-4372-0e02-c10bb2c3d479",
	} {
		if _, err := sqlxDB.Exec(`INSERT INTO tasks (id, parent_id) VALUES (?, 'parent-1')`, id); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}

	frag := OrderedIDConcat(SQLite3, "parent_id = 'parent-1'")
	var got string
	if err := sqlxDB.QueryRowxContext(context.Background(), "SELECT "+frag).Scan(&got); err != nil {
		t.Fatalf("query: %v", err)
	}

	want := "0e02b2c3-58cc-4372-a567-f47ac10bd479,a567f47a-58cc-4372-0e02-c10bb2c3d479,f47ac10b-58cc-4372-a567-0e02b2c3d479"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestOrderedIDConcat_Postgres_OrdersAscendingByID is the PostgreSQL twin:
// AC-OFFICE-WAKE-WAVE-IDENTITY-001.13 names cross-dialect ordering
// divergence as a silent failure mode, so this asserts byte-identical
// ordering against the same UUID-shaped fixture the SQLite test uses, not
// sequential ids that would happen to sort the same under any collation.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestOrderedIDConcat_Postgres_OrdersAscendingByID(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	sqlxDB := testutil.OpenIsolatedPostgres(t, dsn)

	if _, err := sqlxDB.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, parent_id TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	for _, id := range []string{
		"f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"0e02b2c3-58cc-4372-a567-f47ac10bd479",
		"a567f47a-58cc-4372-0e02-c10bb2c3d479",
	} {
		if _, err := sqlxDB.Exec(sqlxDB.Rebind(
			`INSERT INTO tasks (id, parent_id) VALUES (?, 'parent-1')`), id); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}

	frag := OrderedIDConcat(PGX, "parent_id = 'parent-1'")
	var got string
	if err := sqlxDB.QueryRowxContext(context.Background(), "SELECT "+frag).Scan(&got); err != nil {
		t.Fatalf("query: %v", err)
	}

	want := "0e02b2c3-58cc-4372-a567-f47ac10bd479,a567f47a-58cc-4372-0e02-c10bb2c3d479,f47ac10b-58cc-4372-a567-0e02b2c3d479"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestOrderedPairConcat_FragmentShape(t *testing.T) {
	got := OrderedPairConcat(SQLite3, "parent_id = p.id")
	want := "(SELECT GROUP_CONCAT(w.id || ':' || w.state, ',') FROM (SELECT id, state FROM tasks WHERE parent_id = p.id ORDER BY id) w)"
	if got != want {
		t.Errorf("sqlite: got %q, want %q", got, want)
	}

	got = OrderedPairConcat(PGX, "parent_id = p.id")
	want = "(SELECT string_agg(w.id || ':' || w.state, ',' ORDER BY w.id) FROM (SELECT id, state FROM tasks WHERE parent_id = p.id) w)"
	if got != want {
		t.Errorf("pgx: got %q, want %q", got, want)
	}
}

// orderedPairConcatWant is the byte-identical result both dialect tests
// below must produce — this is the child-set key format ListStuckParents
// compares directly against formatChildSetKey's Go-built output.
const orderedPairConcatWant = "c-aa:CANCELLED,c-mm:COMPLETED,c-zz:COMPLETED"

// seedOrderedPairConcatFixture seeds ids whose lexicographic order
// disagrees with insertion order, so an unordered aggregate would produce a
// different string than orderedPairConcatWant.
func seedOrderedPairConcatFixture(t *testing.T, sqlxDB *sqlx.DB) {
	t.Helper()
	if _, err := sqlxDB.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, parent_id TEXT, state TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	rows := []struct{ id, state string }{
		{"c-zz", "COMPLETED"},
		{"c-aa", "CANCELLED"},
		{"c-mm", "COMPLETED"},
	}
	for _, row := range rows {
		if _, err := sqlxDB.Exec(sqlxDB.Rebind(
			`INSERT INTO tasks (id, parent_id, state) VALUES (?, 'parent-1', ?)`), row.id, row.state); err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}
}

// TestOrderedPairConcat_SQLite_OrdersAscendingByID is
// TestOrderedIDConcat_SQLite_OrdersAscendingByID's twin for the id:state
// pair form.
func TestOrderedPairConcat_SQLite_OrdersAscendingByID(t *testing.T) {
	tmpDir := t.TempDir()
	rawDB, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlxDB := sqlx.NewDb(rawDB, SQLite3)
	t.Cleanup(func() { _ = sqlxDB.Close() })
	seedOrderedPairConcatFixture(t, sqlxDB)

	frag := OrderedPairConcat(SQLite3, "parent_id = 'parent-1'")
	var got string
	if err := sqlxDB.QueryRowxContext(context.Background(), "SELECT "+frag).Scan(&got); err != nil {
		t.Fatalf("query: %v", err)
	}
	if got != orderedPairConcatWant {
		t.Fatalf("got %q, want %q", got, orderedPairConcatWant)
	}
}

// TestOrderedPairConcat_Postgres_OrdersAscendingByID is the PostgreSQL
// twin, asserting the exact same byte-identical string as the SQLite test:
// the child-set key must not drift between dialects, since ListStuckParents
// compares it directly. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestOrderedPairConcat_Postgres_OrdersAscendingByID(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	sqlxDB := testutil.OpenIsolatedPostgres(t, dsn)
	seedOrderedPairConcatFixture(t, sqlxDB)

	frag := OrderedPairConcat(PGX, "parent_id = 'parent-1'")
	var got string
	if err := sqlxDB.QueryRowxContext(context.Background(), "SELECT "+frag).Scan(&got); err != nil {
		t.Fatalf("query: %v", err)
	}
	if got != orderedPairConcatWant {
		t.Fatalf("got %q, want %q", got, orderedPairConcatWant)
	}
}
