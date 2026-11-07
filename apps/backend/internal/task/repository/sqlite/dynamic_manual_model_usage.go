package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

type ManualModelWindowUsage struct {
	ProfileID string
	Model     string
	Usage     ManualWindowUsage
}

// GetManualModelWindowUsageForProfiles keeps missing model attribution empty
// and retains the concrete profile that owns each recorded observation.
func (r *Repository) GetManualModelWindowUsageForProfiles(ctx context.Context, profileIDs []string, start, end time.Time) ([]ManualModelWindowUsage, error) {
	ids := make([]string, 0, len(profileIDs))
	for _, id := range profileIDs {
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("manual model usage requires a concrete execution profile")
	}
	result := make([]ManualModelWindowUsage, 0)
	if !end.After(start) {
		return result, nil
	}
	profileExpr := "COALESCE(NULLIF(t.execution_profile_id, ''), e.agent_profile_id)"
	grouped := strings.Replace(manualWindowUsageQuery, "SELECT", "SELECT "+profileExpr+", e.model,", 1) + " GROUP BY " + profileExpr + ", e.model"
	query, args, err := sqlx.In(grouped, costSourceUnpriced, usageCompletenessComplete, start, end, ids, ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var profileID string
		var model sql.NullString
		var usage ManualWindowUsage
		var attributed, summaries int64
		if err := rows.Scan(&profileID, &model, &usage.EventCount, &usage.TokensTotal, &usage.CostSubcents,
			&usage.UnpricedCount, &usage.IncompleteCount, &attributed, &usage.TurnCount, &summaries); err != nil {
			return nil, err
		}
		usage.TurnsAttributed = attributed
		usage.RequestCount = usage.EventCount
		usage.RequestEstimated = summaries > 0
		usage.ProfileAttributed = max(0, usage.EventCount-attributed)
		result = append(result, ManualModelWindowUsage{ProfileID: profileID, Model: model.String, Usage: usage})
	}
	return result, rows.Err()
}
