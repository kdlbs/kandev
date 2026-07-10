package dialect

// BlobType returns the portable binary column type for the active driver.
func BlobType(driver string) string {
	if IsPostgres(driver) {
		return "BYTEA"
	}
	return "BLOB"
}
