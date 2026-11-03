package automation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/db"
)

// historyRunRow is the source row for one automation history observation. It
// keeps the stored status next to the task-derived projection so a reader can
// report both without changing the existing execution readers. Task, session,
// and turn identities are the recorded source values, not the UI projection's
// current-session fallback.
type historyRunRow struct {
	AutomationRun
	AutomationName string    `db:"automation_name"`
	SourceStatus   RunStatus `db:"source_status"`
	WorkspaceID    string    `db:"workspace_id"`
}

// historyRunColumnsSQL is a narrow history projection. It deliberately omits
// trigger bodies, retry launch/prompt snapshots, claim state, and other private
// columns so a bounded history query never materializes large excluded
// payloads before the public snapshot is measured.
const historyRunColumnsSQL = `
		ar.id, ar.automation_id, ar.trigger_id, ar.trigger_type,
		ar.task_id, ar.session_id, ar.turn_id,` + runObservedStatusSQL + `,
		ar.status AS source_status,
		ar.managed_conversation_id, ar.delivery_status,
		ar.retry_group_id, ar.retry_parent_run_id, ar.attempt_number, ar.retry_state,
		ar.retry_scheduled_at, ar.retry_claimed_at, ar.created_at, ar.error_message,
		a.name AS automation_name, a.workspace_id AS workspace_id`

// historyRunRawColumnsSQL is the fallback for isolated automation stores
// without the task projection tables. It keeps the raw stored status as both
// the source and observed status.
const historyRunRawColumnsSQL = `
		ar.id, ar.automation_id, ar.trigger_id, ar.trigger_type,
		ar.task_id, ar.session_id, ar.turn_id, ar.status AS status,
		ar.status AS source_status,
		ar.managed_conversation_id, ar.delivery_status,
		ar.retry_group_id, ar.retry_parent_run_id, ar.attempt_number, ar.retry_state,
		ar.retry_scheduled_at, ar.retry_claimed_at, ar.created_at, ar.error_message,
		a.name AS automation_name, a.workspace_id AS workspace_id`

// ListHistoryAutomations returns a bounded definition snapshot ordered by
// creation time and identity. The bool reports that at least one additional
// matching row exists beyond the requested limit.
func (s *Store) ListHistoryAutomations(ctx context.Context, filter HistoryDefinitionFilter) ([]*Automation, bool, error) {
	if filter.AutomationID != "" {
		exists, err := s.historyAutomationExists(ctx, filter.WorkspaceID, filter.AutomationID)
		if err != nil {
			return nil, false, err
		}
		if !exists {
			return nil, false, ErrHistoryAutomationNotFound
		}
	}
	query := `SELECT ` + automationColumns + ` FROM automations WHERE workspace_id = ?`
	args := []any{filter.WorkspaceID}
	if filter.AutomationID != "" {
		query += ` AND id = ?`
		args = append(args, filter.AutomationID)
	}
	if filter.CreatedFrom != nil {
		query += ` AND created_at >= ?`
		args = append(args, filter.CreatedFrom.UTC())
	}
	if filter.CreatedBefore != nil {
		query += ` AND created_at < ?`
		args = append(args, filter.CreatedBefore.UTC())
	}
	query += ` ORDER BY created_at ASC, id ASC LIMIT ?`
	args = append(args, filter.Limit+1)

	var automations []*Automation
	if err := s.ro.SelectContext(ctx, &automations, s.ro.Rebind(query), args...); err != nil {
		return nil, false, err
	}
	overflow := len(automations) > filter.Limit
	if overflow {
		automations = automations[:filter.Limit]
	}
	if err := s.hydrateAutomations(ctx, automations); err != nil {
		return nil, false, err
	}
	return automations, overflow, nil
}

// ListHistoryRuns returns a bounded run snapshot ordered newest first. Every
// row retains its stored status alongside the same task-derived projection the
// existing run lists use, but preserves the recorded task/session/turn links.
func (s *Store) ListHistoryRuns(ctx context.Context, filter HistoryRunFilter) ([]*historyRunRow, bool, error) {
	if filter.AutomationID != "" {
		exists, err := s.historyAutomationExists(ctx, filter.WorkspaceID, filter.AutomationID)
		if err != nil {
			return nil, false, err
		}
		if !exists {
			return nil, false, ErrHistoryAutomationNotFound
		}
	}
	rows, err := s.listHistoryRunsWithTaskState(ctx, filter)
	if db.IsMissingTableError(err) {
		rows, err = s.listHistoryRunsRaw(ctx, filter)
	}
	if err != nil {
		return nil, false, err
	}
	overflow := len(rows) > filter.Limit
	if overflow {
		rows = rows[:filter.Limit]
	}
	if err := s.hydrateHistoryRunSummaries(ctx, rows); err != nil {
		return nil, false, err
	}
	return rows, overflow, nil
}

// GetHistoryRun refreshes one run through its owning automation in the
// requested workspace. Foreign and missing identities are the same outcome.
func (s *Store) GetHistoryRun(ctx context.Context, workspaceID, runID string) (*historyRunRow, error) {
	var row historyRunRow
	err := s.ro.GetContext(ctx, &row, s.ro.Rebind(`
		SELECT`+historyRunColumnsSQL+`
		FROM automation_runs ar
		JOIN automations a ON a.id = ar.automation_id
		LEFT JOIN tasks t ON t.id = ar.task_id
		WHERE a.workspace_id = ? AND ar.id = ?`),
		append(runTaskStateArgs(), workspaceID, runID)...)
	if db.IsMissingTableError(err) {
		err = s.ro.GetContext(ctx, &row, s.ro.Rebind(`
			SELECT`+historyRunRawColumnsSQL+`
			FROM automation_runs ar
			JOIN automations a ON a.id = ar.automation_id
			WHERE a.workspace_id = ? AND ar.id = ?`), workspaceID, runID)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrHistoryRunNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.hydrateHistoryRunSummaries(ctx, []*historyRunRow{&row}); err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) historyAutomationExists(ctx context.Context, workspaceID, automationID string) (bool, error) {
	var count int
	err := s.ro.GetContext(ctx, &count, s.ro.Rebind(
		`SELECT COUNT(1) FROM automations WHERE workspace_id = ? AND id = ?`), workspaceID, automationID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) listHistoryRunsWithTaskState(ctx context.Context, filter HistoryRunFilter) ([]*historyRunRow, error) {
	query := `SELECT` + historyRunColumnsSQL + `
		FROM automation_runs ar
		JOIN automations a ON a.id = ar.automation_id
		LEFT JOIN tasks t ON t.id = ar.task_id
		WHERE a.workspace_id = ?`
	args := append(runTaskStateArgs(), filter.WorkspaceID)
	query, args = applyHistoryRunFilter(query, args, filter)
	query += ` ORDER BY ar.created_at DESC, ar.id DESC LIMIT ?`
	args = append(args, filter.Limit+1)
	var rows []*historyRunRow
	err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(query), args...)
	return rows, err
}

func (s *Store) listHistoryRunsRaw(ctx context.Context, filter HistoryRunFilter) ([]*historyRunRow, error) {
	query := `SELECT` + historyRunRawColumnsSQL + `
		FROM automation_runs ar
		JOIN automations a ON a.id = ar.automation_id
		WHERE a.workspace_id = ?`
	args := []any{filter.WorkspaceID}
	query, args = applyHistoryRunFilter(query, args, filter)
	query += ` ORDER BY ar.created_at DESC, ar.id DESC LIMIT ?`
	args = append(args, filter.Limit+1)
	var rows []*historyRunRow
	err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(query), args...)
	return rows, err
}

func applyHistoryRunFilter(query string, args []any, filter HistoryRunFilter) (string, []any) {
	if filter.AutomationID != "" {
		query += ` AND ar.automation_id = ?`
		args = append(args, filter.AutomationID)
	}
	if filter.CreatedFrom != nil {
		query += ` AND ar.created_at >= ?`
		args = append(args, filter.CreatedFrom.UTC())
	}
	if filter.CreatedBefore != nil {
		query += ` AND ar.created_at < ?`
		args = append(args, filter.CreatedBefore.UTC())
	}
	return query, args
}

type historyTaskSummaryRow struct {
	TaskID  string         `db:"task_id"`
	Summary sql.NullString `db:"summary"`
}

func (s *Store) hydrateHistoryRunSummaries(ctx context.Context, rows []*historyRunRow) error {
	plain := make([]*AutomationRun, len(rows))
	for i := range rows {
		plain[i] = &rows[i].AutomationRun
	}
	if err := s.hydrateRunSummariesWithPrefix(ctx, plain, historyRunSummaryPrefixBytes); err != nil {
		return err
	}

	// Runs created before turn binding have no turn identity. Preserve the
	// existing feed's task-level fallback for those rows only.
	taskIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.TurnID == "" && row.TaskID != "" {
			taskIDs = append(taskIDs, row.TaskID)
		}
	}
	if len(taskIDs) == 0 {
		return nil
	}
	summaries, err := s.latestTaskSummaries(ctx, taskIDs)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.TurnID != "" {
			continue
		}
		if summary, ok := summaries[row.TaskID]; ok {
			row.Summary = summary
		}
	}
	return nil
}

func (s *Store) latestTaskSummaries(ctx context.Context, taskIDs []string) (map[string]string, error) {
	placeholders := strings.TrimRight(strings.Repeat("?,", len(taskIDs)), ",")
	args := make([]any, len(taskIDs))
	for i, taskID := range taskIDs {
		args[i] = taskID
	}
	var summaries []historyTaskSummaryRow
	err := s.ro.SelectContext(ctx, &summaries, s.ro.Rebind(fmt.Sprintf(`
		SELECT m.task_id, substr(m.content, 1, %d) AS summary
		FROM task_session_messages m
		WHERE m.task_id IN (`+placeholders+`)
		  AND m.author_type = 'agent' AND m.type = 'message'
		  AND NOT EXISTS (
			SELECT 1 FROM task_session_messages newer
			WHERE newer.task_id = m.task_id
			  AND newer.author_type = 'agent' AND newer.type = 'message'
			  AND (newer.created_at > m.created_at OR
				(newer.created_at = m.created_at AND newer.id > m.id))
			)`, historyRunSummaryPrefixBytes)), args...)
	if db.IsMissingTableError(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(summaries))
	for _, summary := range summaries {
		if summary.Summary.Valid {
			result[summary.TaskID] = summary.Summary.String
		}
	}
	return result, nil
}
