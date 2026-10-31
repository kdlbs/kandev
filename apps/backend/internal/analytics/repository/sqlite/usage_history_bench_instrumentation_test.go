//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

// NVISIT must distinguish actual scan work from identical aggregate output.
func TestUsageHistoryBenchmarkScanVisits(t *testing.T) {
	database, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	database.SetMaxOpenConns(1)
	_, err = database.Exec(`CREATE TABLE visits (occurred INTEGER, tokens INTEGER)`)
	require.NoError(t, err)
	_, err = database.Exec(`WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i+1 FROM n WHERE i<999) INSERT INTO visits SELECT i,3 FROM n`)
	require.NoError(t, err)
	_, err = database.Exec(`CREATE INDEX visits_time ON visits(occurred)`)
	require.NoError(t, err)
	collector := &usageBenchObserver{}
	require.NoError(t, attachUsageBenchSQLite(context.Background(), database, collector))
	for _, access := range []struct {
		clause string
		visits int64
	}{{"INDEXED BY visits_time", 10}, {"NOT INDEXED", 1000}} {
		statement, err := database.Prepare(`SELECT SUM(tokens) FROM visits ` + access.clause + ` WHERE occurred>=? AND occurred<?`)
		require.NoError(t, err)
		for range 2 {
			collector.start()
			var result int
			require.NoError(t, statement.QueryRow(990, 1000).Scan(&result))
			require.Equal(t, 30, result)
			statements, err := collector.finish()
			require.NoError(t, err)
			require.Len(t, statements, 1)
			require.Len(t, statements[0].SQLiteLoops, 1)
			require.Equal(t, access.visits, statements[0].SQLiteLoops[0].Visits)
		}
		require.NoError(t, statement.Close())
	}
}
