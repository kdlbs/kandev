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

type manualModelWindowQueryer interface {
	QueryxContext(context.Context, string, ...interface{}) (*sqlx.Rows, error)
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
	return r.readManualModelWindow(ctx, r.ro, ids, start, end)
}

func (r *Repository) readManualModelWindow(ctx context.Context, queryer manualModelWindowQueryer, ids []string, start, end time.Time) ([]ManualModelWindowUsage, error) {
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
	rows, err := queryer.QueryxContext(ctx, r.ro.Rebind(query), args...)
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

// GetManualSelectedModelWindowUsage selects and aggregates model/profile pairs
// in one read snapshot, preserving distinct turns across selected models.
func (r *Repository) GetManualSelectedModelWindowUsage(ctx context.Context, profileIDs []string, start, end time.Time, selectModel func(string, string) (bool, error)) (ManualWindowUsage, error) {
	ids := make([]string, 0, len(profileIDs))
	for _, id := range profileIDs {
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 || selectModel == nil {
		return ManualWindowUsage{}, fmt.Errorf("manual model selection requires profiles and a selector")
	}
	if !end.After(start) {
		return ManualWindowUsage{}, nil
	}
	tx, err := r.ro.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return ManualWindowUsage{}, err
	}
	defer func() { _ = tx.Rollback() }()
	groups, err := r.readManualModelWindow(ctx, tx, ids, start, end)
	if err != nil {
		return ManualWindowUsage{}, err
	}
	pairs := make([]string, 0, len(groups))
	args := make([]interface{}, 0, len(groups)*2+6)
	for _, group := range groups {
		selected, selectErr := selectModel(group.ProfileID, group.Model)
		if selectErr != nil {
			return ManualWindowUsage{}, selectErr
		}
		if selected {
			pairs = append(pairs, "(?, ?)")
			args = append(args, group.ProfileID, group.Model)
		}
	}
	if len(pairs) == 0 {
		return ManualWindowUsage{}, tx.Commit()
	}
	args = append(args, costSourceUnpriced, usageCompletenessComplete, start, end, ids, ids)
	prefix := "WITH selected_models(profile_id, model) AS (VALUES " + strings.Join(pairs, ", ") + ") "
	filter := " AND EXISTS (SELECT 1 FROM selected_models sm WHERE sm.profile_id = COALESCE(NULLIF(t.execution_profile_id, ''), e.agent_profile_id) AND sm.model = COALESCE(e.model, ''))"
	query, args, err := sqlx.In(prefix+manualWindowUsageQuery+filter, args...)
	if err != nil {
		return ManualWindowUsage{}, err
	}
	var usage ManualWindowUsage
	var attributed, summaries int64
	err = tx.QueryRowxContext(ctx, r.ro.Rebind(query), args...).Scan(&usage.EventCount, &usage.TokensTotal, &usage.CostSubcents,
		&usage.UnpricedCount, &usage.IncompleteCount, &attributed, &usage.TurnCount, &summaries)
	if err != nil {
		return ManualWindowUsage{}, err
	}
	usage.TurnsAttributed = attributed
	usage.RequestCount = usage.EventCount
	usage.RequestEstimated = summaries > 0
	usage.ProfileAttributed = max(0, usage.EventCount-attributed)
	return usage, tx.Commit()
}
