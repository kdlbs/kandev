package sqlite

import "github.com/kandev/kandev/internal/analytics/models"

type usageProjectedFactPayload struct {
	Payload    string `db:"payload"`
	KeepTokens bool   `db:"keep_tokens"`
	KeepCost   bool   `db:"keep_cost"`
}

func usageAllTimeAlternatives(filter models.SessionUsageFilter) bool {
	return filter.Start == nil && filter.End == nil && !isDatedUsageGroup(filter.GroupBy)
}

func usageProjectedMetricMask(filter models.SessionUsageFilter, metric string) string {
	if !usageAllTimeAlternatives(filter) {
		return "1=1"
	}
	condition := usageLifetimeTokenAuthority("l")
	if metric == projectionMetricCost {
		condition = usageLifetimeCostMaskAuthority("l", usageCostContributionExists("f", true))
	}
	return `(f.coverage_kind='lifetime' OR NOT EXISTS (SELECT 1 FROM usage_canonical_facts l JOIN usage_projection_applied la
 ON la.scope_id=l.scope_id AND la.generation=l.generation AND la.work_key=l.work_key AND la.build_id=l.build_id
 WHERE l.scope_id=f.scope_id AND l.generation=f.generation AND l.work_key=f.work_key
 AND (l.bundle_key<>f.bundle_key OR l.segment_key=f.segment_key) AND l.coverage_kind='lifetime' AND ` + condition + `))`
}

// An applicable lifetime counter already represents unknown cost for this
// work. Suppress only its duplicate dated missing counter, never a known dated
// amount. The published contribution proves applicability without a JSON or
// nil-based guess; a measured lifetime zero retains ordinary price authority.
func usageLifetimeCostMaskAuthority(alias, missingCost string) string {
	return `(` + usageLifetimeCostAuthority(alias) + ` OR (` + missingCost + ` AND ` + usageCostContributionExists(alias, false) + `))`
}

func usageCostContributionExists(alias string, missingOnly bool) string {
	query := `EXISTS(SELECT 1 FROM usage_projection_contributions cm
 WHERE cm.scope_id=` + alias + `.scope_id AND cm.generation=` + alias + `.generation AND cm.build_id=` + alias + `.build_id
 AND cm.fact_key=` + alias + `.fact_key AND cm.metric_kind='cost'`
	if missingOnly {
		query += ` AND cm.cost_subcents IS NULL`
	}
	return query + `)`
}
