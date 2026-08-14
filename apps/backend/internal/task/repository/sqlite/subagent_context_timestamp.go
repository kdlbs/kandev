// Package sqlite provides SQLite-based repository implementations.
package sqlite

// subagentPreciseTimestamp builds a SQL expression that evaluates to a value
// safe to compare with plain >= / < against another such expression, at no
// coarser than microsecond precision and monotonically across the whole
// range (AC-24e in docs/specs/subagent-context-persistence/spec.md). expr may
// be a TIMESTAMP column reference or a bind-parameter placeholder carrying an
// RFC3339(Nano) string (an activation key from kandev_meta).
//
// Deliberately not internal/db's existing timestampColumn/julianday(): its
// ~40-microsecond ULP at 2026-scale epoch distances is coarser than this
// comparison's required floor.
//
// On Postgres the column is already a native timestamptz, so a straight cast
// is exact and needs no string surgery.
//
// On SQLite, this repository's own stored TIMESTAMP text (verified against
// the mattn/go-sqlite3 driver's on-disk bytes, not the reformatted string a
// *string Scan target sees) is space-separated with a "+00:00" offset, e.g.
// "2026-08-10 09:00:00.123456+00:00", while the activation keys this package
// writes are RFC3339Nano, e.g. "2026-08-10T09:00:00.123456789Z". A raw >=
// across those two shapes silently snaps to midnight of the key's date: the
// space/offset column sorts before any 'T'-shaped key sharing its date
// prefix, because ' ' (0x20) < 'T' (0x54). Every timestamp this repository
// writes is already a UTC wall-clock instant (see rfc3339Timestamp's doc
// comment), so a blind ' '->'T' / '+00:00'->'Z' substitution is safe and
// turns both shapes into the same lexical form.
//
// That alone is not sufficient: Go's ".999999999" format (and this
// repository's own storage of it) trims trailing zero fractional digits,
// omitting the '.' entirely at a whole second. Two same-shape strings that
// differ only in fraction width do not compare correctly against each other
// as raw text — e.g. "...00Z" (an exact whole second) sorts AFTER
// "...00.123456Z" (a later fraction of that same second) under plain byte
// comparison, because 'Z' (0x5A) > '.' (0x2E). Padding every fraction to a
// fixed 6-digit (microsecond) width before comparing removes that failure
// mode; TestSubagentPreciseTimestampOrdersWholeSecondBeforeFraction pins it.
func subagentPreciseTimestamp(postgres bool, expr string) string {
	if postgres {
		return "((" + expr + ")::timestamptz)"
	}

	normalized := "REPLACE(REPLACE(" + expr + ", ' ', 'T'), '+00:00', 'Z')"
	datePart := "substr(" + normalized + ", 1, 19)"
	rest := "substr(" + normalized + ", 20)"
	fracDigits := "(CASE WHEN substr(" + rest + ", 1, 1) = '.' " +
		"THEN substr(" + rest + ", 2, length(" + rest + ") - 2) ELSE '' END)"
	fracPadded := "substr(" + fracDigits + " || '000000', 1, 6)"
	return "(" + datePart + " || '.' || " + fracPadded + " || 'Z')"
}
