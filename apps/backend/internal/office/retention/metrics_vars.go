package retention

import (
	"expvar"
	"strings"
)

// expvar maps published at package init, exposed via stdlib's /debug/vars
// handler, mirroring internal/office/scheduler/metrics_vars.go's label
// model. Development convenience only (see the design's expvar
// section) — nothing in REQ-OFFICE-RUN-HISTORY-RETENTION-003 depends on
// these; the durable operator surfaces are structured logs and
// health.Issue.
var (
	retentionSweepTotal   = expvar.NewMap("office_retention_sweep_total")
	retentionDeletedTotal = expvar.NewMap("office_retention_deleted_total")
	retentionCensusTotal  = expvar.NewMap("office_retention_census_total")
)

// metricLabel builds a "k1=v1;k2=v2;..." label string for an expvar map
// key, matching office/scheduler's format so a downstream parser handles
// both packages identically.
func metricLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

func incSweepCompleted() {
	retentionSweepTotal.Add(metricLabel("outcome", "completed"), 1)
}

func incSweepSkipped() {
	retentionSweepTotal.Add(metricLabel("outcome", "skipped"), 1)
}

func incDeleted(table TableName, n int64) {
	if n <= 0 {
		return
	}
	retentionDeletedTotal.Add(metricLabel("table", string(table)), n)
}

func incCensus(table TableName, err error) {
	outcome := "fresh"
	if err != nil {
		outcome = "stale"
	}
	retentionCensusTotal.Add(metricLabel("table", string(table), "outcome", outcome), 1)
}
