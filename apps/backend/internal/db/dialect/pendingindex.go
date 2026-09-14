package dialect

import "fmt"

// PendingIDLookupIndexDDL returns the partial expression index used by
// pending-ID-only clarification reads and claims. The pending ID expression
// is the leading key so the database can isolate one bundle before applying
// its deterministic creation ordering.
func PendingIDLookupIndexDDL(driver, indexName, table string) string {
	pendingID := JSONExtract(driver, "metadata", "pending_id")
	return fmt.Sprintf(
		"CREATE INDEX IF NOT EXISTS %s ON %s((%s), created_at, id) WHERE %s IS NOT NULL",
		indexName, table, pendingID, pendingID,
	)
}
