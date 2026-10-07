package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const maxSidebarPreferenceIDs = 10000
const (
	sidebarGroupNone                = "none"
	sidebarStateKey                 = "state"
	sidebarActivitySortField        = "lastActivityAt"
	sidebarRunningSortField         = "running"
	sidebarRunningActivitySortField = "runningFirstActivity"
	sidebarRepositoryKey            = "repository"
	sidebarWorkflowKey              = "workflow"
	sidebarWorkflowStepKey          = "workflowStep"
	sidebarArchivedKey              = "archived"
	sidebarNotMatchesOp             = "not_matches"
	sidebarNotInOp                  = "not_in"
	sidebarCustomSortKey            = "custom"
	sidebarSQLNull                  = "NULL"
)

type sidebarPageRow struct {
	taskID         string
	groupKey       string
	groupLabel     string
	workflowName   string
	stepName       string
	stepColor      string
	parentID       string
	parentTitle    string
	groupCount     int
	depth          int
	continuesGroup bool
	wipPosition    int
	wipTotal       int
	subtaskCount   int
}

type sidebarPageQueryResult struct {
	rows         []sidebarPageRow
	totalTasks   int
	totalVisible int
	totalGroups  int
	page         int
	hasSummary   bool
}

type sidebarGroupRow struct {
	groupKey   string
	groupLabel string
	count      int
}

func (r *Repository) QuerySidebarTaskPage(
	ctx context.Context,
	workspaceID string,
	query models.SidebarTaskViewQuery,
	prefs models.SidebarTaskViewPreferences,
) (*models.SidebarTaskPageResult, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	if err := validateSidebarTaskPreferences(query, prefs); err != nil {
		return nil, err
	}
	snapshot, err := beginSidebarQuerySnapshot(ctx, r.ro)
	if err != nil {
		return nil, err
	}
	defer snapshot.close()
	snapshot.afterStage = r.sidebarQueryStage
	baseSQL, baseArgs, err := snapshot.prepare(ctx, r.ro.DriverName(), workspaceID, query, prefs)
	if err != nil {
		return nil, err
	}
	if err := snapshot.preparePreferences(ctx, prefs); err != nil {
		return nil, err
	}
	return r.readSidebarTaskPage(ctx, snapshot, workspaceID, query, prefs, baseSQL, baseArgs)
}

func (r *Repository) readSidebarTaskPage(
	ctx context.Context, snapshot *sidebarQuerySnapshot, workspaceID string,
	query models.SidebarTaskViewQuery, prefs models.SidebarTaskViewPreferences, baseSQL string, baseArgs []any,
) (*models.SidebarTaskPageResult, error) {
	tx := snapshot.tx

	pageCTEs, cteArgs := sidebarPageCTEs(r.ro.DriverName(), query, prefs)
	pageQueryArgs := append(append([]any(nil), baseArgs...), cteArgs...)
	pageSQL := baseSQL + pageCTEs + sidebarPageSelectSQL(query.Group == sidebarGroupNone)
	rows, err := tx.QueryContext(ctx, r.ro.Rebind(pageSQL), pageQueryArgs...)
	if err != nil {
		return nil, fmt.Errorf("query sidebar task page: %w", err)
	}
	pageResult, err := scanSidebarPageRows(rows)
	if err != nil {
		return nil, err
	}
	if err := snapshot.checkpoint("page"); err != nil {
		return nil, err
	}
	pageRows := pageResult.rows
	totalTasks, totalVisible, totalGroups, page := pageResult.totalTasks, pageResult.totalVisible, pageResult.totalGroups, pageResult.page
	if !pageResult.hasSummary {
		summarySQL := baseSQL + pageCTEs + ` SELECT total_tasks, total_visible_tasks, total_groups FROM page_summary`
		if err := tx.QueryRowxContext(ctx, r.ro.Rebind(summarySQL), pageQueryArgs...).Scan(&totalTasks, &totalVisible, &totalGroups); err != nil {
			return nil, fmt.Errorf("count empty sidebar task page: %w", err)
		}
		page = 1
		if err := snapshot.checkpoint("empty_count"); err != nil {
			return nil, err
		}
	}
	query.Page = page
	pageCTEs, cteArgs = sidebarPageCTEs(r.ro.DriverName(), query, prefs)
	queryArgs := append(append([]any(nil), baseArgs...), cteArgs...)
	var headerRows []sidebarGroupRow
	if len(query.CollapsedGroupKeys) > 0 {
		headerRows, err = querySidebarGroupHeaders(ctx, tx, r, baseSQL+pageCTEs, queryArgs, pageRows, query.CollapsedGroupKeys)
		if err != nil {
			return nil, err
		}
	} else {
		headerRows = sidebarHeadersFromPageRows(pageRows)
	}
	if err := snapshot.checkpoint("headers"); err != nil {
		return nil, err
	}
	tasks, err := loadSidebarPageTasks(ctx, tx, r, pageRows)
	if err != nil {
		return nil, err
	}
	if err := snapshot.checkpoint("hydrated"); err != nil {
		return nil, err
	}
	result := buildSidebarTaskPageResult(workspaceID, query, prefs, page, totalTasks, totalVisible, totalGroups, headerRows, pageRows, tasks)
	if err := snapshot.commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func sidebarTaskBaseSQL(
	driver, workspaceID string,
	query models.SidebarTaskViewQuery,
	preferences ...models.SidebarTaskViewPreferences,
) (string, []any, error) {
	baseSQL, baseArgs, err := sidebarTaskCandidateSQL(driver, workspaceID, query, preferences...)
	if err != nil {
		return "", nil, err
	}
	visibleSQL, visibleArgs := sidebarVisibleCTE(driver, query)
	return baseSQL + visibleSQL, append(baseArgs, visibleArgs...), nil
}

func sidebarQueryHasSort(query models.SidebarTaskViewQuery, key string) bool {
	for _, criterion := range query.Sort.Criteria() {
		if criterion.Key == key {
			return true
		}
	}
	return false
}

func sidebarTaskCandidateSQL(
	driver, workspaceID string,
	query models.SidebarTaskViewQuery,
	preferences ...models.SidebarTaskViewPreferences,
) (string, []any, error) {
	prefs := models.SidebarTaskViewPreferences{}
	if len(preferences) > 0 {
		prefs = preferences[0]
	}
	groupExpr, groupLabelExpr := sidebarGroupExpressions(query.Group)
	scopeFilters, projectionFilters := sidebarPartitionFilters(query.Filters)
	scopeSQL, scopeArgs, err := sidebarFilterSQL(driver, scopeFilters, true)
	if err != nil {
		return "", nil, err
	}
	filterSQL, filterArgs, err := sidebarFilterSQL(driver, projectionFilters, false)
	if err != nil {
		return "", nil, err
	}
	needsColor := sidebarQueryHasSort(query, "color")
	colorProjection, colorArgs := "", []any(nil)
	if needsColor {
		colorProjection, colorArgs, err = sidebarEffectiveColorProjection(driver, prefs)
		if err != nil {
			return "", nil, err
		}
	}
	colorSettingsCTE, colorSettingsArgs, err := sidebarColorManualCTE(driver, prefs, needsColor)
	if err != nil {
		return "", nil, err
	}
	baseSQL := sidebarBaseCTE(driver, groupExpr, groupLabelExpr, scopeSQL, query, colorProjection, colorSettingsCTE)
	baseArgs := append([]any(nil), colorSettingsArgs...)
	baseArgs = append(baseArgs, workspaceID)
	baseArgs = append(baseArgs, scopeArgs...)
	baseArgs = append(baseArgs, colorArgs...)
	materialization := "MATERIALIZED"
	if dialect.IsPostgres(driver) {
		materialization = "NOT MATERIALIZED"
	}
	baseSQL += ", filtered AS " + materialization + " (SELECT * FROM candidate WHERE " + filterSQL + ")"
	baseArgs = append(baseArgs, filterArgs...)
	return baseSQL, baseArgs, nil
}

func validateSidebarTaskPreferences(query models.SidebarTaskViewQuery, prefs models.SidebarTaskViewPreferences) error {
	count := len(prefs.PinnedTaskIDs) + len(prefs.OrderedTaskIDs)
	for _, ids := range prefs.SubtaskOrderByParentID {
		count += len(ids)
	}
	if count > maxSidebarPreferenceIDs {
		return errors.New("sidebar task preferences exceed the query limit")
	}
	if sidebarQueryHasSort(query, "color") {
		if err := validateSidebarColorSettings(prefs); err != nil {
			return err
		}
	}
	return nil
}

func sidebarHeadersFromPageRows(rows []sidebarPageRow) []sidebarGroupRow {
	groups := make([]sidebarGroupRow, 0)
	indexByKey := make(map[string]int, len(rows))
	for _, row := range rows {
		_, ok := indexByKey[row.groupKey]
		if !ok {
			index := len(groups)
			indexByKey[row.groupKey] = index
			groups = append(groups, sidebarGroupRow{groupKey: row.groupKey, groupLabel: row.groupLabel, count: row.groupCount})
		}
	}
	return groups
}

func querySidebarGroupHeaders(
	ctx context.Context,
	tx *sqlx.Tx,
	repo *Repository,
	queryCTEs string,
	cteArgs []any,
	pageRows []sidebarPageRow,
	collapsedGroupKeys []string,
) ([]sidebarGroupRow, error) {
	keysByValue := make(map[string]struct{}, len(pageRows)+len(collapsedGroupKeys))
	for _, row := range pageRows {
		keysByValue[row.groupKey] = struct{}{}
	}
	for _, key := range collapsedGroupKeys {
		keysByValue[key] = struct{}{}
	}
	if len(keysByValue) == 0 {
		return []sidebarGroupRow{}, nil
	}
	keys := make([]string, 0, len(keysByValue))
	for key := range keysByValue {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	keysSQL, keysArgs := sidebarStringListSQL(repo.ro.DriverName(), keys)
	args := append(append([]any(nil), cteArgs...), keysArgs...)
	query := queryCTEs + sidebarGroupHeaderSelectSQL(keysSQL)
	rows, err := tx.QueryContext(ctx, repo.ro.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("query sidebar task groups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	groups := make([]sidebarGroupRow, 0)
	for rows.Next() {
		var group sidebarGroupRow
		if err := rows.Scan(&group.groupKey, &group.groupLabel, &group.count); err != nil {
			return nil, fmt.Errorf("scan sidebar task group: %w", err)
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read sidebar task groups: %w", err)
	}
	return groups, nil
}

func stringSliceToAny(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func scanSidebarPageRows(rows *sql.Rows) (sidebarPageQueryResult, error) {
	defer func() { _ = rows.Close() }()
	result := sidebarPageQueryResult{rows: make([]sidebarPageRow, 0, models.MaxSidebarTaskPageSize)}
	for rows.Next() {
		var row sidebarPageRow
		if err := rows.Scan(&row.taskID, &row.groupKey, &row.groupLabel, &row.workflowName, &row.stepName, &row.stepColor,
			&row.parentID, &row.parentTitle, &row.groupCount, &row.depth, &row.continuesGroup,
			&row.wipPosition, &row.wipTotal, &row.subtaskCount,
			&result.totalTasks, &result.totalVisible, &result.totalGroups, &result.page); err != nil {
			return sidebarPageQueryResult{}, fmt.Errorf("scan sidebar task row: %w", err)
		}
		result.hasSummary = true
		result.rows = append(result.rows, row)
	}
	if err := rows.Err(); err != nil {
		return sidebarPageQueryResult{}, fmt.Errorf("read sidebar task rows: %w", err)
	}
	return result, nil
}

func loadSidebarPageTasks(ctx context.Context, tx *sqlx.Tx, repo *Repository, rows []sidebarPageRow) ([]*models.Task, error) {
	if len(rows) == 0 {
		return []*models.Task{}, nil
	}
	ids := make([]string, len(rows))
	for index, row := range rows {
		ids[index] = row.taskID
	}
	placeholders, args := buildInPlaceholders(ids)
	query := `SELECT ` + taskSelectColumns("t") + ` FROM tasks t WHERE t.id IN (` + placeholders + `)`
	taskRows, err := tx.QueryContext(ctx, repo.ro.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("hydrate sidebar task page: %w", err)
	}
	tasks, err := repo.scanTasks(taskRows)
	if err != nil {
		return nil, fmt.Errorf("scan sidebar task page: %w", err)
	}
	repositoriesByTaskID, err := listTaskRepositoriesByTaskIDs(ctx, tx, ids)
	if err != nil {
		return nil, fmt.Errorf("hydrate sidebar task repositories: %w", err)
	}
	byID := make(map[string]*models.Task, len(tasks))
	for _, task := range tasks {
		task.Repositories = repositoriesByTaskID[task.ID]
		byID[task.ID] = task
	}
	ordered := make([]*models.Task, 0, len(rows))
	for _, row := range rows {
		if task := byID[row.taskID]; task != nil {
			ordered = append(ordered, task)
		}
	}
	return ordered, nil
}

func buildSidebarTaskPageResult(
	workspaceID string,
	query models.SidebarTaskViewQuery,
	prefs models.SidebarTaskViewPreferences,
	page, totalTasks, totalVisible int,
	totalGroups int,
	headerRows []sidebarGroupRow,
	rows []sidebarPageRow,
	tasks []*models.Task,
) *models.SidebarTaskPageResult {
	entries := make([]models.SidebarTaskPageEntry, 0, len(rows)+len(headerRows))
	pageIDs := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		pageIDs[row.taskID] = struct{}{}
	}
	rowsByGroup := make(map[string][]sidebarPageRow, len(headerRows))
	for _, row := range rows {
		rowsByGroup[row.groupKey] = append(rowsByGroup[row.groupKey], row)
	}
	emittedContinuations := make(map[string]struct{})
	for _, header := range headerRows {
		groupRows := rowsByGroup[header.groupKey]
		continuation := len(groupRows) > 0 && groupRows[0].continuesGroup
		entries = append(entries, models.SidebarTaskPageEntry{
			Kind: "group", GroupKey: header.groupKey, GroupLabel: header.groupLabel,
			Continuation: continuation, MatchingCount: header.count,
		})
		for _, row := range groupRows {
			if row.parentID != "" {
				_, parentOnPage := pageIDs[row.parentID]
				_, continuationEmitted := emittedContinuations[row.parentID]
				if !parentOnPage && !continuationEmitted {
					entries = append(entries, models.SidebarTaskPageEntry{
						Kind: "continuation", ParentID: row.parentID, ParentTitle: row.parentTitle, Depth: max(row.depth-1, 0),
					})
					emittedContinuations[row.parentID] = struct{}{}
				}
			}
			entries = append(entries, models.SidebarTaskPageEntry{
				Kind: "task", TaskID: row.taskID, GroupKey: row.groupKey,
				GroupLabel: row.groupLabel, WorkflowName: row.workflowName, StepName: row.stepName,
				StepColor: row.stepColor, ParentID: row.parentID, Depth: row.depth,
				WIPQueuePosition: row.wipPosition, WIPQueueTotal: row.wipTotal, SubtaskCount: row.subtaskCount,
			})
		}
	}
	pageCount := 1
	if totalVisible > 0 {
		pageCount = (totalVisible + query.PageSize - 1) / query.PageSize
	}
	return &models.SidebarTaskPageResult{
		QueryKey: sidebarTaskQueryKey(workspaceID, query, prefs), Page: page, PageSize: query.PageSize,
		TotalEntries: totalVisible + totalGroups, TotalTasks: totalTasks, TotalVisibleTasks: totalVisible,
		HasPrevious: page > 1, HasNext: page < pageCount, Entries: entries, Tasks: tasks,
	}
}

func sidebarTaskQueryKey(workspaceID string, query models.SidebarTaskViewQuery, prefs models.SidebarTaskViewPreferences) string {
	query.Page = 1
	query.CollapsedGroupKeys = append([]string(nil), query.CollapsedGroupKeys...)
	query.CollapsedTaskIDs = append([]string(nil), query.CollapsedTaskIDs...)
	sort.Strings(query.CollapsedGroupKeys)
	sort.Strings(query.CollapsedTaskIDs)
	type queryPreferences struct {
		PinnedTaskIDs          []string            `json:"pinned_task_ids"`
		OrderedTaskIDs         []string            `json:"ordered_task_ids"`
		SubtaskOrderByParentID map[string][]string `json:"subtask_order_by_parent_id"`
		ColorSettingsDigest    string              `json:"color_settings_digest,omitempty"`
	}
	preferences := queryPreferences{
		PinnedTaskIDs: prefs.PinnedTaskIDs, OrderedTaskIDs: prefs.OrderedTaskIDs,
		SubtaskOrderByParentID: prefs.SubtaskOrderByParentID,
	}
	if sidebarQueryHasSort(query, "color") {
		encoded, _ := json.Marshal(sidebarColorSettings(prefs))
		digest := sha256.Sum256(encoded)
		preferences.ColorSettingsDigest = hex.EncodeToString(digest[:])
	}
	payload, _ := json.Marshal(struct {
		WorkspaceID string
		Query       models.SidebarTaskViewQuery
		Preferences queryPreferences
	}{workspaceID, query, preferences})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
