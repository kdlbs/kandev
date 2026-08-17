package sqlite

import (
	"testing"
	"time"
)

// TestSubagentPreciseTimestampOrdersWholeSecondBeforeFraction covers AC-24e:
// a raw >= between this repository's stored TIMESTAMP text and an RFC3339Nano
// activation key silently snaps to midnight of the key's date (' ' sorts
// before 'T'), and even after fixing that shape mismatch, a whole-second
// value with no fractional digits sorts AFTER a later fractional value in the
// same second under plain byte comparison ('Z' > '.'). subagentPreciseTimestamp
// must get both directions right.
func TestSubagentPreciseTimestampOrdersWholeSecondBeforeFraction(t *testing.T) {
	repo, _ := newSubagentMigrationTestRepo(t)
	if _, err := repo.db.Exec(`CREATE TABLE ts_probe (label TEXT, ts TIMESTAMP)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	whole := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	frac := time.Date(2026, 8, 10, 9, 0, 0, 123456000, time.UTC)
	if _, err := repo.db.Exec(`INSERT INTO ts_probe (label, ts) VALUES (?, ?), (?, ?)`,
		"whole", whole, "frac", frac); err != nil {
		t.Fatalf("seed probe rows: %v", err)
	}

	expr := subagentPreciseTimestamp(false, "ts")

	var wholeLTfrac int
	if err := repo.db.QueryRow(`
		SELECT (SELECT ` + expr + ` FROM ts_probe WHERE label='whole')
		     < (SELECT ` + expr + ` FROM ts_probe WHERE label='frac')
	`).Scan(&wholeLTfrac); err != nil {
		t.Fatalf("compare whole < frac: %v", err)
	}
	if wholeLTfrac != 1 {
		t.Errorf("whole (09:00:00.000000) < frac (09:00:00.123456) = %d, want 1 (true)", wholeLTfrac)
	}

	// The activation key comparison AC-24e actually governs: an RFC3339Nano
	// key bound as a parameter must compare correctly against the raw
	// space+offset stored column, for a key on the same date but a later
	// time-of-day than a stored row. The key is bound once via a subquery
	// alias so subagentPreciseTimestamp can reference it as a column
	// expression (repeated internally) without needing repeated bind args.
	key := time.Date(2026, 8, 10, 9, 0, 0, 500000000, time.UTC).Format(time.RFC3339Nano)
	keyExpr := subagentPreciseTimestamp(false, "k.key_raw")
	keySubquery := `(SELECT ? AS key_raw) k`

	var rowGEKey int
	if err := repo.db.QueryRow(`
		SELECT (SELECT `+expr+` FROM ts_probe WHERE label='frac') >= `+keyExpr+`
		FROM `+keySubquery,
		key,
	).Scan(&rowGEKey); err != nil {
		t.Fatalf("compare frac row >= key: %v", err)
	}
	if rowGEKey != 0 {
		t.Errorf("frac row (09:00:00.123456) >= key (09:00:00.5) = %d, want 0 (false)", rowGEKey)
	}

	var wholeGEKey int
	if err := repo.db.QueryRow(`
		SELECT (SELECT `+expr+` FROM ts_probe WHERE label='whole') >= `+keyExpr+`
		FROM `+keySubquery,
		key,
	).Scan(&wholeGEKey); err != nil {
		t.Fatalf("compare whole row >= key: %v", err)
	}
	if wholeGEKey != 0 {
		t.Errorf("whole row (09:00:00.0, same date) >= key (09:00:00.5) = %d, want 0 (false) — "+
			"a naive raw comparison would incorrectly snap this to true", wholeGEKey)
	}
}
