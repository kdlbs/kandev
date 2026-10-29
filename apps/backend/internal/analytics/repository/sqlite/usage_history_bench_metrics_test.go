//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"encoding/json"
	"fmt"
	"strings"
)

type usageBenchPostgresNode struct {
	Type       string                   `json:"Node Type"`
	Relation   string                   `json:"Relation Name"`
	Index      string                   `json:"Index Name"`
	Rows       float64                  `json:"Actual Rows"`
	Loops      float64                  `json:"Actual Loops"`
	Filtered   float64                  `json:"Rows Removed by Filter"`
	Rechecked  float64                  `json:"Rows Removed by Index Recheck"`
	PeakMemory float64                  `json:"Peak Memory Usage"`
	SortSpace  float64                  `json:"Sort Space Used"`
	SortType   string                   `json:"Sort Space Type"`
	Plans      []usageBenchPostgresNode `json:"Plans"`
}

type usageBenchScanTotals struct {
	SQLiteVisits      int64   `json:"sqlite_nvisit"`
	PostgresRows      float64 `json:"postgres_scan_rows_returned"`
	PostgresRejected  float64 `json:"postgres_scan_rows_filtered_or_rechecked"`
	PlanNodePeakBytes int64   `json:"postgres_largest_plan_node_memory_bytes"`
	Statements        int     `json:"statement_count"`
	ReadStatements    int     `json:"read_statement_count"`
}

func usageBenchPostgresTotals(node usageBenchPostgresNode, totals *usageBenchScanTotals) {
	if strings.Contains(node.Type, "Scan") {
		totals.PostgresRows += node.Rows * node.Loops
		totals.PostgresRejected += (node.Filtered + node.Rechecked) * node.Loops
	}
	peak := int64(node.PeakMemory * 1024)
	if node.SortType == "Memory" {
		peak = max(peak, int64(node.SortSpace*1024))
	}
	totals.PlanNodePeakBytes = max(totals.PlanNodePeakBytes, peak)
	for _, child := range node.Plans {
		usageBenchPostgresTotals(child, totals)
	}
}

func usageBenchStatementTotals(statements []usageBenchStatement) (usageBenchScanTotals, error) {
	totals := usageBenchScanTotals{Statements: len(statements)}
	for _, statement := range statements {
		if usageBenchReadSQL(statement.SQL) {
			totals.ReadStatements++
		}
		for _, loop := range statement.SQLiteLoops {
			totals.SQLiteVisits += loop.Visits
		}
		if len(statement.PostgresPlan) > 0 {
			var roots []struct {
				Plan usageBenchPostgresNode `json:"Plan"`
			}
			if err := json.Unmarshal(statement.PostgresPlan, &roots); err != nil || len(roots) != 1 {
				return totals, fmt.Errorf("benchmark execution plan has no unique root")
			}
			usageBenchPostgresTotals(roots[0].Plan, &totals)
		}
	}
	return totals, nil
}

func validateUsageBenchStatements(engine string, statements []usageBenchStatement) error {
	var metadata, amounts, providers bool
	var loops int
	for _, statement := range statements {
		if strings.Contains(statement.SQL, "session_usage_buckets") || strings.Contains(statement.SQL, "session_usage_measurements") || strings.Contains(statement.SQL, "task_usage_events") {
			if !usageBenchAccountingStatusProbe(statement.SQL) {
				return fmt.Errorf("report benchmark performed an unexpected raw source read: %s", statement.SQL)
			}
		}
		metadata = metadata || strings.Contains(statement.SQL, "usage_projection_scopes")
		amounts = amounts || strings.Contains(statement.SQL, "usage_projection_contributions") || strings.Contains(statement.SQL, "usage_daily_summaries")
		providers = providers || strings.Contains(statement.SQL, "report_providers")
		loops += len(statement.SQLiteLoops)
		if engine == "postgres" && usageBenchReadSQL(statement.SQL) && len(statement.PostgresPlan) == 0 {
			return fmt.Errorf("PostgreSQL report SQL is missing EXPLAIN ANALYZE evidence")
		}
	}
	if !metadata || !amounts || !providers || engine == "sqlite" && loops == 0 {
		return fmt.Errorf("benchmark instrumentation missed metadata, amounts, provider options, or scan loops")
	}
	_, err := usageBenchStatementTotals(statements)
	return err
}

// R1 E/H added this exact indexed repair-status query to every real report.
// Keep its visits/plans in the evidence. This exception permits no history
// aggregation or different source SQL, and never drops a captured statement.
func usageBenchAccountingStatusProbe(query string) bool {
	expected := `SELECT EXISTS(SELECT 1 FROM session_usage_measurements WHERE scope_id=? AND ` + nativeUsageAccountingPendingSQL +
		`) OR EXISTS(SELECT 1 FROM session_usage_buckets WHERE scope_id=? AND ` + nativeUsageAccountingPendingSQL + `)`
	query = strings.NewReplacer("$1", "?", "$2", "?").Replace(query)
	return strings.Join(strings.Fields(query), " ") == strings.Join(strings.Fields(expected), " ")
}
