package dialect

// Like returns the SQL LIKE operator appropriate for the driver.
//
//	SQLite:  LIKE (case-insensitive for ASCII by default)
//	Postgres: ILIKE (case-insensitive)
func Like(driver string) string {
	if IsPostgres(driver) {
		return "ILIKE"
	}
	return "LIKE"
}
