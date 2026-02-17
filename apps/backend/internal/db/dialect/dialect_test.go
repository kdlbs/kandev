package dialect

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
)

func TestIsPostgres(t *testing.T) {
	if !IsPostgres(PGX) {
		t.Error("expected pgx to be postgres")
	}
	if IsPostgres(SQLite3) {
		t.Error("expected sqlite3 to not be postgres")
	}
}

func TestBoolToInt(t *testing.T) {
	if BoolToInt(true) != 1 {
		t.Error("expected 1 for true")
	}
	if BoolToInt(false) != 0 {
		t.Error("expected 0 for false")
	}
}

func TestJSONExtract(t *testing.T) {
	got := JSONExtract(SQLite3, "metadata", "status")
	if got != "json_extract(metadata, '$.status')" {
		t.Errorf("sqlite: got %q", got)
	}
	got = JSONExtract(PGX, "metadata", "status")
	if got != "metadata::jsonb->>'status'" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestJSONExtractIsNotNull(t *testing.T) {
	got := JSONExtractIsNotNull(SQLite3, "m", "id")
	if got != "json_extract(m, '$.id') IS NOT NULL" {
		t.Errorf("sqlite: got %q", got)
	}
	got = JSONExtractIsNotNull(PGX, "m", "id")
	if got != "m::jsonb->>'id' IS NOT NULL" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestJSONSet(t *testing.T) {
	got := JSONSet(SQLite3, "metadata", "status", "complete")
	if got != "json_set(metadata, '$.status', 'complete')" {
		t.Errorf("sqlite: got %q", got)
	}
	got = JSONSet(PGX, "metadata", "status", "complete")
	if got != `jsonb_set(metadata::jsonb, '{status}', '"complete"')::text` {
		t.Errorf("pgx: got %q", got)
	}
}

func TestDurationMs(t *testing.T) {
	got := DurationMs(SQLite3, "completed_at", "started_at")
	if got != "(julianday(completed_at) - julianday(started_at)) * 86400000" {
		t.Errorf("sqlite: got %q", got)
	}
	got = DurationMs(PGX, "completed_at", "started_at")
	if got != "EXTRACT(EPOCH FROM (completed_at - started_at)) * 1000" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestDateOf(t *testing.T) {
	got := DateOf(SQLite3, "created_at")
	if got != "date(created_at)" {
		t.Errorf("sqlite: got %q", got)
	}
	got = DateOf(PGX, "created_at")
	if got != "(created_at)::date" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestNow(t *testing.T) {
	if Now(SQLite3) != "datetime('now')" {
		t.Errorf("sqlite: got %q", Now(SQLite3))
	}
	if Now(PGX) != "NOW()" {
		t.Errorf("pgx: got %q", Now(PGX))
	}
}

func TestNowMinusHours(t *testing.T) {
	got := NowMinusHours(SQLite3, "ws.hours")
	if got != "datetime('now', '-' || ws.hours || ' hours')" {
		t.Errorf("sqlite: got %q", got)
	}
	got = NowMinusHours(PGX, "ws.hours")
	if got != "NOW() - (ws.hours || ' hours')::interval" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestCurrentDate(t *testing.T) {
	if CurrentDate(SQLite3) != "date('now')" {
		t.Errorf("sqlite: got %q", CurrentDate(SQLite3))
	}
	if CurrentDate(PGX) != "CURRENT_DATE" {
		t.Errorf("pgx: got %q", CurrentDate(PGX))
	}
}

func TestDateNowMinusDays(t *testing.T) {
	got := DateNowMinusDays(SQLite3, "?")
	if got != "date('now', '-' || ? || ' days')" {
		t.Errorf("sqlite: got %q", got)
	}
	got = DateNowMinusDays(PGX, "?")
	if got != "CURRENT_DATE - (? || ' days')::interval" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestDatePlusOneDay(t *testing.T) {
	got := DatePlusOneDay(SQLite3, "date")
	if got != "date(date, '+1 day')" {
		t.Errorf("sqlite: got %q", got)
	}
	got = DatePlusOneDay(PGX, "date")
	if got != "(date)::date + INTERVAL '1 day'" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestLike(t *testing.T) {
	if Like(SQLite3) != "LIKE" {
		t.Errorf("sqlite: got %q", Like(SQLite3))
	}
	if Like(PGX) != "ILIKE" {
		t.Errorf("pgx: got %q", Like(PGX))
	}
}

func TestInsertReturningID_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	rawDB, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlxDB := sqlx.NewDb(rawDB, SQLite3)
	t.Cleanup(func() { _ = sqlxDB.Close() })

	_, err = sqlxDB.Exec(`CREATE TABLE test_insert (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	id, err := InsertReturningID(context.Background(), sqlxDB, `INSERT INTO test_insert (name) VALUES (?)`, "hello")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if id != 1 {
		t.Errorf("expected id 1, got %d", id)
	}

	id, err = InsertReturningID(context.Background(), sqlxDB, `INSERT INTO test_insert (name) VALUES (?)`, "world")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if id != 2 {
		t.Errorf("expected id 2, got %d", id)
	}
}
