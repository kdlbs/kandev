//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageHistoryCurrentTargetConstantScanVisits(t *testing.T) {
	repo, _ := currentTargetDatabaseFixture(t, false)
	observer := &usageBenchObserver{}
	require.NoError(t, attachUsageBenchSQLite(t.Context(), repo.db.DB, observer))
	queries := []currentUsageTargetSelector{{TaskID: "task-1"}, {SessionID: "session-1"}, {ExternalSessionID: "transcript-1"}}
	first := currentTargetForTest(t, repo, "collector-a", "ws-1", queries[0])
	queries = append(queries, currentUsageTargetSelector{SubjectBinding: first.BindingID})
	measure := func(query currentUsageTargetSelector) (int64, []usageBenchStatement) {
		observer.start()
		target := currentTargetForTest(t, repo, "collector-a", "ws-1", query)
		statements, err := observer.finish()
		require.NoError(t, err)
		require.NotNil(t, target)
		return retainedReportVisits(statements), statements
	}
	before := make([]int64, len(queries))
	for index, query := range queries {
		before[index], _ = measure(query)
	}
	seedUnrelatedCurrentTargets(t, repo, 451)
	for index, query := range queries {
		after, statements := measure(query)
		t.Logf("selector %d: baseline NVISIT=%d; with 902 historical sessions NVISIT=%d", index, before[index], after)
		for _, statement := range statements {
			if !strings.Contains(statement.SQL, "task_sessions") && !strings.Contains(statement.SQL, "usage_collection_bindings") {
				continue
			}
			for _, loop := range statement.SQLiteLoops {
				t.Logf("selector %d: %s; NLOOP=%d NVISIT=%d", index, loop.Plan, loop.Loops, loop.Visits)
			}
		}
		require.LessOrEqual(t, after, before[index]+16, "actual SQL visits must remain bounded independently of unrelated retained and legacy targets")
	}
}
