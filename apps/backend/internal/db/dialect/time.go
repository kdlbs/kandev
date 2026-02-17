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
