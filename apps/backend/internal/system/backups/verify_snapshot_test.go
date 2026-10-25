package backups

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/jmoiron/sqlx"
)

// Each directory name holds a character that a SQLite URI filename decodes or
// treats as a delimiter. The temporary root is a native absolute path: a
// drive-letter path with backslash separators on Windows.
var snapshotPathCases = []struct {
	name      string
	dir       string
	posixOnly bool
}{
	{name: "space", dir: "backup dir"},
	{name: "hash", dir: "backup#dir"},
	{name: "percent", dir: "backup%41dir"},
	{name: "question mark", dir: "backup?dir", posixOnly: true},
}

func TestVerifySnapshotOpensNativeAbsolutePath(t *testing.T) {
	for _, tc := range snapshotPathCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.posixOnly && runtime.GOOS == "windows" {
				t.Skip("Windows file names cannot contain this character")
			}
			root := t.TempDir()
			path := seedSnapshotAt(t, root, tc.dir)

			digest, err := verifySnapshot(context.Background(), path)
			if err != nil {
				t.Fatalf("verify snapshot at %q: %v", path, err)
			}
			if want := fileSHA256(t, path); digest != want {
				t.Fatalf("digest = %q, want %q", digest, want)
			}
			assertSnapshotReaderIsReadOnly(t, path)

			if err := os.WriteFile(path, []byte("not a sqlite database"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := verifySnapshot(context.Background(), path); err == nil {
				t.Fatalf("verified a file that is not a database at %q", path)
			}
			assertDirEntries(t, root, tc.dir)
			assertDirEntries(t, filepath.Dir(path), "snapshot.db")
		})
	}
}

// seedSnapshotAt creates the database under a plain name and renames it, so
// the fixture does not depend on how SQLite interprets the final path.
func seedSnapshotAt(t *testing.T, root, dir string) string {
	t.Helper()
	seed := filepath.Join(root, "seed.db")
	writer, err := sqlx.Open("sqlite3", seed)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	if _, err := writer.Exec(`CREATE TABLE things (id TEXT PRIMARY KEY); INSERT INTO things (id) VALUES ('one')`); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, dir), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, dir, "snapshot.db")
	if err := os.Rename(seed, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertSnapshotReaderIsReadOnly(t *testing.T, path string) {
	t.Helper()
	reader, err := openSnapshotReader(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	var id string
	if err := reader.Get(&id, `SELECT id FROM things`); err != nil || id != "one" {
		t.Fatalf("read snapshot at %q: id = %q, err = %v", path, id, err)
	}
	if _, err := reader.Exec(`INSERT INTO things (id) VALUES ('two')`); err == nil {
		t.Fatalf("snapshot reader accepted a write at %q", path)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
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
