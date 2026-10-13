package launcher

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
)

const backupProbeSchema = "CREATE TABLE snapshot_probe (id INTEGER PRIMARY KEY, marker TEXT NOT NULL)"

// @covers AC-PLATFORM-GO-DEV-LAUNCHER-001.10, AC-PLATFORM-GO-DEV-LAUNCHER-001.11
func TestBackupProductionDbCommittedWAL(t *testing.T) {
	for _, offline := range []bool{false, true} {
		name := "open_WAL"
		if offline {
			name = "offline_WAL_image"
		}
		t.Run(name, func(t *testing.T) {
			origin := filepath.Join(t.TempDir(), "source.db")
			createBackupFixture(t, origin, true)
			assertBackupRecords(t, origin)
			source := origin
			if offline {
				source = filepath.Join(t.TempDir(), "offline.db")
				for _, suffix := range []string{"", "-wal"} {
					copyBackupFixtureFile(t, origin+suffix, source+suffix)
				}
			}
			mainBefore := readBackupFixtureFile(t, source)
			walBefore := readBackupFixtureFile(t, source+"-wal")
			if len(walBefore) <= 32 {
				t.Fatal("fixture must retain committed WAL frames")
			}
			created, err := backupProductionDb(context.Background(), source, t.TempDir(), time.Now())
			if err != nil {
				t.Fatal(err)
			}
			assertBackupRecords(t, created)
			assertBackupSourceUnchanged(t, source, mainBefore, walBefore)
			assertBackupRecords(t, source)
		})
	}
}

func TestBackupProductionDbRollbackJournal(t *testing.T) {
	source := filepath.Join(t.TempDir(), "rollback.db")
	createBackupFixture(t, source, false)
	before := readBackupFixtureFile(t, source)
	created, err := backupProductionDb(context.Background(), source, t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	assertBackupRecords(t, created)
	assertBackupSourceUnchanged(t, source, before, nil)
	assertBackupRecords(t, source)
}

func TestBackupProductionDbURIPath(t *testing.T) {
	name := "source%3f#?.db"
	if runtime.GOOS == "windows" {
		name = "source%3f#.db"
	}
	source := filepath.Join(t.TempDir(), name)
	createBackupFixture(t, source, true)
	home := filepath.Join(t.TempDir(), name+"-home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	created, err := backupProductionDb(context.Background(), source, home, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	assertBackupRecords(t, created)
	assertBackupRecords(t, source)
}

// @covers AC-PLATFORM-GO-DEV-LAUNCHER-001.12, AC-PLATFORM-GO-DEV-LAUNCHER-001.13
func TestBackupProductionDbFailureCleanup(t *testing.T) {
	for _, name := range []string{"copy_failure", "verification_failure"} {
		t.Run(name, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "source.db")
			createBackupFixture(t, source, true)
			mainBefore := readBackupFixtureFile(t, source)
			walBefore := readBackupFixtureFile(t, source+"-wal")
			home := t.TempDir()
			previous := seedBackupHistory(t, home)
			oldCopy := copyProductionSnapshotFn
			t.Cleanup(func() { copyProductionSnapshotFn = oldCopy })
			copyProductionSnapshotFn = func(ctx context.Context, source, staged string) error {
				if name == "verification_failure" {
					if err := copyProductionSnapshot(ctx, source, staged); err != nil {
						return err
					}
				}
				if err := os.WriteFile(staged, []byte("owned incomplete snapshot"), 0o600); err != nil {
					return err
				}
				if name == "copy_failure" {
					return errors.New("injected copy failure")
				}
				return nil
			}
			created, err := backupProductionDb(context.Background(), source, home, time.Now())
			if err == nil || created != "" {
				t.Fatalf("failed snapshot = %q, %v", created, err)
			}
			assertBackupHistoryUnchanged(t, home, previous)
			assertBackupSourceUnchanged(t, source, mainBefore, walBefore)
			assertBackupRecords(t, source)
		})
	}
}

func TestBackupProductionDbCanceled(t *testing.T) {
	for _, name := range []string{"before_admission", "after_copy", "expired_deadline"} {
		t.Run(name, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "source.db")
			createBackupFixture(t, source, false)
			home := t.TempDir()
			previous := seedBackupHistory(t, home)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			wantErr := context.Canceled
			switch name {
			case "before_admission":
				cancel()
			case "expired_deadline":
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithDeadline(ctx, time.Unix(1, 0))
				defer deadlineCancel()
				wantErr = context.DeadlineExceeded
			default:
				oldCopy := copyProductionSnapshotFn
				t.Cleanup(func() { copyProductionSnapshotFn = oldCopy })
				copyProductionSnapshotFn = func(ctx context.Context, source, staged string) error {
					err := copyProductionSnapshot(ctx, source, staged)
					cancel()
					return err
				}
			}
			created, err := backupProductionDb(ctx, source, home, time.Now())
			if created != "" || !errors.Is(err, wantErr) {
				t.Fatalf("canceled backup = %q, %v, want %v", created, err, wantErr)
			}
			assertBackupHistoryUnchanged(t, home, previous)
			assertBackupRecords(t, source)
		})
	}
}

func TestBackupProductionDbInvalidSource(t *testing.T) {
	source := filepath.Join(t.TempDir(), "invalid.db")
	if err := os.WriteFile(source, []byte("not a SQLite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	previous := seedBackupHistory(t, home)
	created, err := backupProductionDb(context.Background(), source, home, time.Now())
	if err == nil || created != "" {
		t.Fatalf("invalid source backup = %q, %v", created, err)
	}
	assertBackupHistoryUnchanged(t, home, previous)
}

func TestBackupProductionDbPreservesExistingDestination(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.db")
	createBackupFixture(t, source, false)
	home := t.TempDir()
	stamp := time.Now()
	previous := seedBackupHistory(t, home)
	name := backupPrefix + backupTimestamp(stamp) + backupSuffix
	previous[name] = []byte("existing destination sentinel")
	destination := filepath.Join(home, ".kandev", "data", "backups", name)
	if err := os.WriteFile(destination, previous[name], 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := backupProductionDb(context.Background(), source, home, stamp)
	if err == nil || created != "" {
		t.Fatalf("collision backup = %q, %v", created, err)
	}
	assertBackupHistoryUnchanged(t, home, previous)
}

func seedBackupHistory(t *testing.T, home string) map[string][]byte {
	t.Helper()
	dir := filepath.Join(home, ".kandev", "data", "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	contents := make(map[string][]byte)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 6; i++ {
		name := backupPrefix + backupTimestamp(base.Add(time.Duration(i)*time.Second)) + backupSuffix
		contents[name] = []byte(name)
		if err := os.WriteFile(filepath.Join(dir, name), contents[name], 0o600); err != nil {
			t.Fatal(err)
		}
		stamp := base.Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(filepath.Join(dir, name), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	return contents
}

func assertBackupHistoryUnchanged(t *testing.T, home string, previous map[string][]byte) {
	t.Helper()
	dir := filepath.Join(home, ".kandev", "data", "backups")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(previous) {
		t.Fatalf("backup directory has %d entries, want original %d: %v", len(entries), len(previous), entries)
	}
	for _, entry := range entries {
		want, exists := previous[entry.Name()]
		if !exists || !bytes.Equal(want, readBackupFixtureFile(t, filepath.Join(dir, entry.Name()))) {
			t.Fatalf("existing backup %q changed", entry.Name())
		}
	}
}

func createBackupFixture(t *testing.T, path string, wal bool) {
	t.Helper()
	journal := "DELETE"
	if wal {
		journal = "WAL"
	}
	uri := "file:" + db.EscapeSQLiteURIPath(filepath.ToSlash(path))
	writer, err := sql.Open("sqlite3", uri+"?mode=rwc&_journal_mode="+journal)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	writer.SetMaxOpenConns(1)
	statements := []string{
		"PRAGMA wal_autocheckpoint=0",
		backupProbeSchema,
		"INSERT INTO snapshot_probe VALUES (1, 'checkpointed')",
	}
	if wal {
		statements = append(statements, "PRAGMA wal_checkpoint(TRUNCATE)")
	}
	statements = append(statements, "INSERT INTO snapshot_probe VALUES (2, 'committed-in-wal')")
	for _, statement := range statements {
		if _, err := writer.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

func assertBackupRecords(t *testing.T, path string) {
	t.Helper()
	uri := "file:" + db.EscapeSQLiteURIPath(filepath.ToSlash(path)) + "?mode=ro"
	reader, err := sql.Open("sqlite3", uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	defer func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	}()
	rows, err := reader.Query("SELECT marker FROM snapshot_probe ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var markers []string
	for rows.Next() {
		var marker string
		if err := rows.Scan(&marker); err != nil {
			t.Fatal(err)
		}
		markers = append(markers, marker)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(markers) != 2 {
		t.Fatalf("standalone database %s has %d rows, want both 2 committed source rows: %v", path, len(markers), markers)
	}
	if !reflect.DeepEqual(markers, []string{"checkpointed", "committed-in-wal"}) {
		t.Fatalf("snapshot records = %v", markers)
	}
	var schema, integrity string
	if err := reader.QueryRow("SELECT sql FROM sqlite_schema WHERE name='snapshot_probe'").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	if schema != backupProbeSchema {
		t.Fatalf("snapshot schema = %q, want %q", schema, backupProbeSchema)
	}
	if err := reader.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("snapshot integrity = %q, error = %v", integrity, err)
	}
}

func assertBackupSourceUnchanged(t *testing.T, path string, mainBefore, walBefore []byte) {
	t.Helper()
	if !bytes.Equal(mainBefore, readBackupFixtureFile(t, path)) {
		t.Fatal("backup changed source main database")
	}
	if walBefore != nil && !bytes.Equal(walBefore, readBackupFixtureFile(t, path+"-wal")) {
		t.Fatal("backup changed source WAL")
	}
}

func readBackupFixtureFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func copyBackupFixtureFile(t *testing.T, from, to string) {
	t.Helper()
	if err := os.WriteFile(to, readBackupFixtureFile(t, from), 0o600); err != nil {
		t.Fatal(err)
	}
}
