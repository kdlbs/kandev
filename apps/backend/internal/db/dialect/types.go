package dialect

// TimestampType returns the portable timestamp column type for the active
// driver.
func TimestampType(driver string) string {
	if IsPostgres(driver) {
		return "TIMESTAMPTZ"
	}
	return "DATETIME"
}

// BlobType returns the portable binary column type for the active driver.
func BlobType(driver string) string {
	if IsPostgres(driver) {
		return "BYTEA"
	}
	return "BLOB"
}
