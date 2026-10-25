package inventoryrepair

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// Each directory name holds a character that a SQLite URI filename decodes or
// treats as a delimiter. The temporary root is a native absolute path: a
// drive-letter path with backslash separators on Windows.
var databasePathCases = []struct {
	name      string
	dir       string
	posixOnly bool
}{
	{name: "space", dir: "kandev data"},
	{name: "hash", dir: "kandev#data"},
	{name: "percent", dir: "kandev%41data"},
	{name: "question mark", dir: "kandev?data", posixOnly: true},
}

// seedDatabaseAt creates the database under a plain name and renames it, so
// the fixture does not depend on how SQLite interprets the final path.
func seedDatabaseAt(t *testing.T, root, dir string) Plan {
	t.Helper()
	seed := filepath.Join(root, "seed.db")
	db, err := sql.Open("sqlite3", seed)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY); INSERT INTO items (id) VALUES (7)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, dir), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, dir, "kandev.db")
	if err := os.Rename(seed, path); err != nil {
		t.Fatal(err)
	}
	return Plan{Database: path}
}

func queryInt(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var value int
	if err := db.QueryRowContext(context.Background(), query).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return value
}

func assertDirEntries(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	if !slices.Equal(got, want) {
		t.Fatalf("entries in %q = %q, want %q", dir, got, want)
	}
}

func TestOpenDatabaseReadsNativeAbsolutePath(t *testing.T) {
	for _, tc := range databasePathCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.posixOnly && runtime.GOOS == "windows" {
				t.Skip("Windows file names cannot contain this character")
			}
			root := t.TempDir()
			p := seedDatabaseAt(t, root, tc.dir)
			db, err := openDatabase(p, false)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if got := queryInt(t, db, `SELECT id FROM items`); got != 7 {
				t.Fatalf("item id = %d, want 7", got)
			}
			if got := queryInt(t, db, `PRAGMA foreign_keys`); got != 1 {
				t.Fatalf("foreign_keys = %d", got)
			}
			if _, err := db.Exec(`INSERT INTO items (id) VALUES (8)`); err == nil {
				t.Fatalf("read-only connection accepted a write at %q", p.Database)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if err := verifyBackup(context.Background(), p, p.Database); err != nil {
				t.Fatalf("verify backup at %q: %v", p.Database, err)
			}
			assertDirEntries(t, root, tc.dir)
			assertDirEntries(t, filepath.Dir(p.Database), "kandev.db")
		})
	}
}

func TestOpenDatabaseWritesNativeAbsolutePath(t *testing.T) {
	for _, tc := range databasePathCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.posixOnly && runtime.GOOS == "windows" {
				t.Skip("Windows file names cannot contain this character")
			}
			root := t.TempDir()
			p := seedDatabaseAt(t, root, tc.dir)
			db, err := openDatabase(p, true)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if _, err := db.Exec(`INSERT INTO items (id) VALUES (8)`); err != nil {
				t.Fatalf("write at %q: %v", p.Database, err)
			}
			if got := queryInt(t, db, `SELECT COUNT(*) FROM items`); got != 2 {
				t.Fatalf("items = %d, want the seeded row and the new row", got)
			}
			if got := queryInt(t, db, `PRAGMA synchronous`); got != 2 {
				t.Fatalf("synchronous = %d, want FULL (2)", got)
			}
			if got := queryInt(t, db, `PRAGMA foreign_keys`); got != 1 {
				t.Fatalf("foreign_keys = %d", got)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			assertDirEntries(t, root, tc.dir)
			assertDirEntries(t, filepath.Dir(p.Database), "kandev.db")
		})
	}
}
