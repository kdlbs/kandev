package dialect

import "fmt"

// DurationMs returns the SQL expression for the difference between two timestamps in milliseconds.
//
//	SQLite:   (julianday(end) - julianday(start)) * 86400000
//	Postgres: EXTRACT(EPOCH FROM (end - start)) * 1000
func DurationMs(driver, end, start string) string {
	if IsPostgres(driver) {
		return fmt.Sprintf("EXTRACT(EPOCH FROM (%s - %s)) * 1000", end, start)
	}
	return fmt.Sprintf("(julianday(%s) - julianday(%s)) * 86400000", end, start)
}

// DateOf returns the SQL expression to extract the date portion from a timestamp.
//
//	SQLite:   date(expr)
//	Postgres: (expr)::date
func DateOf(driver, expr string) string {
	if IsPostgres(driver) {
		return fmt.Sprintf("(%s)::date", expr)
	}
	return fmt.Sprintf("date(%s)", expr)
}

// DateTimeOf normalizes a timestamp expression that already carries explicit
// zone information — a `timestamptz` column, or text with an explicit
// ISO8601 offset/"Z" such as a value formatted via time.RFC3339Nano — into a
// canonical, directly comparable form. On Postgres this must NOT be used for
// a naive `timestamp` (without time zone) column: casting a naive value
// straight to `timestamptz` interprets it in the session's `timezone` GUC,
// not UTC, silently shifting it by the server's offset even though this
// codebase always writes such columns via time.Now().UTC(). Use
// NaiveUTCTimestampOf for that case instead.
//
// On SQLite this distinction doesn't matter (datetime() normalizes either
// input the same way, and SQLite has no session-timezone concept), but the
// two functions are still named separately so a Postgres-unaware caller
// can't pick the wrong one by accident.
//
//	SQLite:   datetime(expr)
//	Postgres: (expr)::timestamptz
func DateTimeOf(driver, expr string) string {
	if IsPostgres(driver) {
		return fmt.Sprintf("(%s)::timestamptz", expr)
	}
	return fmt.Sprintf("datetime(%s)", expr)
}

// NaiveUTCTimestampOf normalizes a naive timestamp expression — one with NO
// embedded zone information, but KNOWN to always hold UTC wall-clock values
// (every `TIMESTAMP`-typed column in this codebase, written via
// time.Now().UTC()) — into the same comparable form DateTimeOf produces for
// explicitly-zoned values. Do not use this for an expression that already
// carries zone info (a `timestamptz` column, or RFC3339 text with an
// offset/"Z") — use DateTimeOf for those; applying `AT TIME ZONE 'UTC'` to an
// already-zoned value converts it a second time and silently corrupts it.
//
// The bug this exists to prevent: on Postgres, a bare `(expr)::timestamptz`
// cast on a naive column interprets the stored wall-clock text as being in
// the session's `timezone` GUC (e.g. "Asia/Singapore", +08:00) rather than
// UTC, shifting the value by the server's offset before comparison — a
// session that started milliseconds after a UTC activation marker can come
// out looking hours "earlier" than it. `AT TIME ZONE 'UTC'` tells Postgres
// explicitly what zone the naive value is already in, which a bare cast
// cannot express.
//
//	SQLite:   datetime(expr)
//	Postgres: (expr AT TIME ZONE 'UTC')
func NaiveUTCTimestampOf(driver, expr string) string {
	if IsPostgres(driver) {
		return fmt.Sprintf("(%s AT TIME ZONE 'UTC')", expr)
	}
	return fmt.Sprintf("datetime(%s)", expr)
}

// Now returns the SQL expression for the current timestamp.
//
//	SQLite:   datetime('now')
//	Postgres: NOW()
func Now(driver string) string {
	if IsPostgres(driver) {
		return "NOW()"
	}
	return "datetime('now')"
}

// NowMinusHours returns the SQL expression for "current time minus N hours",
// where hoursExpr is a column or expression producing the number of hours.
//
//	SQLite:   datetime('now', '-' || hoursExpr || ' hours')
//	Postgres: NOW() - (hoursExpr || ' hours')::interval
func NowMinusHours(driver, hoursExpr string) string {
	if IsPostgres(driver) {
		return fmt.Sprintf("NOW() - (%s || ' hours')::interval", hoursExpr)
	}
	return fmt.Sprintf("datetime('now', '-' || %s || ' hours')", hoursExpr)
}

// GreatestTimestamp returns the greater of two timestamp expressions.
//
//	SQLite:   max(left, right)
//	Postgres: GREATEST(left, right)
func GreatestTimestamp(driver, left, right string) string {
	if IsPostgres(driver) {
		return fmt.Sprintf("GREATEST(%s, %s)", left, right)
	}
	return fmt.Sprintf("max(%s, %s)", left, right)
}

// CurrentDate returns the SQL expression for the current date (no time component).
//
//	SQLite:   date('now')
//	Postgres: CURRENT_DATE
func CurrentDate(driver string) string {
	if IsPostgres(driver) {
		return "CURRENT_DATE"
	}
	return "date('now')"
}

// DateNowMinusDays returns the SQL expression for "current date minus N days",
// where daysExpr is a parameter placeholder (e.g., "?") for the number of days.
//
//	SQLite:   date('now', '-' || ? || ' days')
//	Postgres: CURRENT_DATE - (? || ' days')::interval
func DateNowMinusDays(driver, daysExpr string) string {
	if IsPostgres(driver) {
		return fmt.Sprintf("CURRENT_DATE - (%s || ' days')::interval", daysExpr)
	}
	return fmt.Sprintf("date('now', '-' || %s || ' days')", daysExpr)
}

// DatePlusOneDay returns the SQL expression to add one day to a date expression.
//
//	SQLite:   date(expr, '+1 day')
//	Postgres: (expr)::date + INTERVAL '1 day'
func DatePlusOneDay(driver, expr string) string {
	if IsPostgres(driver) {
		return fmt.Sprintf("(%s)::date + INTERVAL '1 day'", expr)
	}
	return fmt.Sprintf("date(%s, '+1 day')", expr)
}
