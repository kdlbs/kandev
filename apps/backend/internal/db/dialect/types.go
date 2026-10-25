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

// ByteLength returns a dialect-specific SQL expression that measures the
// UTF-8 byte length of a text expression.
func ByteLength(driver, expression string) string {
	if IsPostgres(driver) {
		return "octet_length(" + expression + ")"
	}
	return "length(CAST(" + expression + " AS BLOB))"
}
