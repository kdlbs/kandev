package persistence

import (
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func memSQLiteDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestReadKey_EmptyOnFreshDB verifies that readKey returns "" when the key
// is absent.
func TestReadKey_EmptyOnFreshDB(t *testing.T) {
	db := memSQLiteDB(t)
	if err := ensureMetaTable(db); err != nil {
		t.Fatalf("ensureMetaTable: %v", err)
	}

	val, err := readKey(db, "kandev_version")
	if err != nil {
		t.Fatalf("readKey: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty value for missing key, got %q", val)
	}
}

// TestWriteAndReadKey verifies the write-then-read roundtrip.
func TestWriteAndReadKey_Roundtrip(t *testing.T) {
	db := memSQLiteDB(t)
	if err := ensureMetaTable(db); err != nil {
		t.Fatalf("ensureMetaTable: %v", err)
	}

	if err := writeKey(db, "kandev_version", "v1.2.3"); err != nil {
		t.Fatalf("writeKey: %v", err)
	}
	val, err := readKey(db, "kandev_version")
	if err != nil {
		t.Fatalf("readKey: %v", err)
	}
	if val != "v1.2.3" {
		t.Errorf("got %q, want %q", val, "v1.2.3")
	}

	// Overwrite should work (upsert).
	if err := writeKey(db, "kandev_version", "v2.0.0"); err != nil {
		t.Fatalf("writeKey overwrite: %v", err)
	}
	val2, err := readKey(db, "kandev_version")
	if err != nil {
		t.Fatalf("readKey after overwrite: %v", err)
	}
	if val2 != "v2.0.0" {
		t.Errorf("got %q after overwrite, want %q", val2, "v2.0.0")
	}
}

// TestHasUserTables_BeforeAndAfter verifies that hasUserTables returns
// false on an empty meta-only DB and true once a user table is created.
func TestHasUserTables_BeforeAndAfter(t *testing.T) {
	db := memSQLiteDB(t)
	if err := ensureMetaTable(db); err != nil {
		t.Fatalf("ensureMetaTable: %v", err)
	}

	has, err := hasUserTables(db)
	if err != nil {
		t.Fatalf("hasUserTables: %v", err)
	}
	if has {
		t.Error("expected no user tables on fresh DB, got true")
	}

	if _, err := db.Exec(`CREATE TABLE my_table (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	has2, err := hasUserTables(db)
	if err != nil {
		t.Fatalf("hasUserTables after create: %v", err)
	}
	if !has2 {
		t.Error("expected hasUserTables=true after creating a user table")
	}
}

// TestWriteAndReadLatestVersion_RoundtripAndUpsert verifies that we can
// store the GitHub-polled latest release and read it back, including overwrites.
func TestWriteAndReadLatestVersion_RoundtripAndUpsert(t *testing.T) {
	db := memSQLiteDB(t)
	if err := ensureMetaTable(db); err != nil {
		t.Fatalf("ensureMetaTable: %v", err)
	}

	// On a fresh DB, all three reads should return zero values.
	v, url, when, err := ReadLatestVersion(db)
	if err != nil {
		t.Fatalf("ReadLatestVersion fresh: %v", err)
	}
	if v != "" || url != "" || !when.IsZero() {
		t.Errorf("expected zero values on fresh DB, got v=%q url=%q when=%v", v, url, when)
	}

	t1 := time.Unix(1_700_000_000, 0).UTC()
	if err := WriteLatestVersion(db, "v1.2.3", "https://example/r/v1.2.3", t1); err != nil {
		t.Fatalf("WriteLatestVersion: %v", err)
	}
	v, url, when, err = ReadLatestVersion(db)
	if err != nil {
		t.Fatalf("ReadLatestVersion after write: %v", err)
	}
	if v != "v1.2.3" {
		t.Errorf("version = %q, want %q", v, "v1.2.3")
	}
	if url != "https://example/r/v1.2.3" {
		t.Errorf("url = %q", url)
	}
	if !when.Equal(t1) {
		t.Errorf("checkedAt = %v, want %v", when, t1)
	}

	// Upsert.
	t2 := time.Unix(1_700_000_100, 0).UTC()
	if err := WriteLatestVersion(db, "v1.2.4", "https://example/r/v1.2.4", t2); err != nil {
		t.Fatalf("WriteLatestVersion overwrite: %v", err)
	}
	v, url, when, err = ReadLatestVersion(db)
	if err != nil {
		t.Fatalf("ReadLatestVersion after overwrite: %v", err)
	}
	if v != "v1.2.4" || url != "https://example/r/v1.2.4" || !when.Equal(t2) {
		t.Errorf("after overwrite got v=%q url=%q when=%v", v, url, when)
	}
}

// TestReadLatestVersion_PartialPersistedState verifies the read tolerates
// only a subset of the three keys being present (which can happen if a
// previous write was interrupted).
func TestReadLatestVersion_PartialPersistedState(t *testing.T) {
	db := memSQLiteDB(t)
	if err := ensureMetaTable(db); err != nil {
		t.Fatalf("ensureMetaTable: %v", err)
	}
	if err := writeKey(db, "latest_version", "v9.9.9"); err != nil {
		t.Fatalf("writeKey: %v", err)
	}
	v, url, when, err := ReadLatestVersion(db)
	if err != nil {
		t.Fatalf("ReadLatestVersion: %v", err)
	}
	if v != "v9.9.9" {
		t.Errorf("version = %q", v)
	}
	if url != "" {
		t.Errorf("url should be empty, got %q", url)
	}
	if !when.IsZero() {
		t.Errorf("checkedAt should be zero, got %v", when)
	}
}

// TestShouldBackup verifies the backup decision logic.
func TestShouldBackup(t *testing.T) {
	tests := []struct {
		name       string
		stored     string
		current    string
		userTables bool
		want       bool
	}{
		{"fresh install no tables", "", "v1.0.0", false, false},
		{"pre-meta upgrade", "", "v1.0.0", true, true},
		{"version change", "v0.9.0", "v1.0.0", true, true},
		{"same release", "v1.0.0", "v1.0.0", true, false},
		{"same release no tables", "v1.0.0", "v1.0.0", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldBackup(tc.stored, tc.current, tc.userTables)
			if got != tc.want {
				t.Errorf("shouldBackup(%q, %q, %v) = %v, want %v",
					tc.stored, tc.current, tc.userTables, got, tc.want)
			}
		})
	}
}
