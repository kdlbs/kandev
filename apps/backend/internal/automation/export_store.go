package automation

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// BeginReadTx opens a transaction on the reader pool. AC-29's export
// orchestration opens exactly one of these per export request and passes it
// through every store read it needs — this package's
// ListAutomationsForExportTx plus the sibling GetWorkflowTx/GetRepositoryTx/
// GetExecutorProfileTx (task/repository/sqlite), GetAgentProfileTx
// (agent/settings/store), and GetStepTx (workflow/repository) — so the whole
// export observes one consistent snapshot. Every store in scope shares the
// same underlying reader pool (see internal/backendapp/services.go and
// storage.go), so a *sqlx.Tx opened here is a transaction against the exact
// pool those other stores' methods also read through.
//
// RepeatableRead + ReadOnly is requested explicitly rather than relying on
// the driver default: SQLite's mattn driver ignores TxOptions entirely (its
// WAL-mode read transactions already snapshot at BEGIN regardless), but a
// Postgres reader pool defaults to READ COMMITTED, which takes a fresh
// snapshot per statement — silently breaking AC-29's one-snapshot promise
// for this export's multiple sequential reads. Requesting RepeatableRead is
// a no-op on SQLite and the correct fix on Postgres.
func (s *Store) BeginReadTx(ctx context.Context) (*sqlx.Tx, error) {
	return s.ro.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

// ListAutomationsForExportTx is ListAutomations' transaction-accepting
// counterpart: it reads automations, triggers, and repository links within
// tx instead of opening its own reads, so the export's snapshot covers this
// table too.
func (s *Store) ListAutomationsForExportTx(ctx context.Context, tx *sqlx.Tx, workspaceID string) ([]*Automation, error) {
	var automations []*Automation
	err := sqlx.SelectContext(ctx, tx, &automations, tx.Rebind(
		`SELECT `+automationColumns+` FROM automations WHERE workspace_id = ? ORDER BY created_at DESC`), workspaceID)
	if err != nil {
		return nil, err
	}
	if len(automations) == 0 {
		return automations, nil
	}

	ids := make([]string, len(automations))
	for i, a := range automations {
		ids[i] = a.ID
	}
	triggersByAutomation, err := listTriggersForAutomationsTx(ctx, tx, ids)
	if err != nil {
		return nil, fmt.Errorf("hydrate triggers: %w", err)
	}
	repoIDsByAutomation, err := listRepositoryIDsForAutomationsTx(ctx, tx, ids)
	if err != nil {
		return nil, fmt.Errorf("hydrate repository_ids: %w", err)
	}
	for _, a := range automations {
		a.Triggers = triggersByAutomation[a.ID]
		a.RepositoryIDs = repoIDsByAutomation[a.ID]
	}
	return automations, nil
}

// listTriggersForAutomationsTx mirrors listTriggersForAutomations, reading
// within tx.
func listTriggersForAutomationsTx(ctx context.Context, tx *sqlx.Tx, automationIDs []string) (map[string][]AutomationTrigger, error) {
	if len(automationIDs) == 0 {
		return make(map[string][]AutomationTrigger), nil
	}
	query, args, err := sqlx.In(
		`SELECT * FROM automation_triggers WHERE automation_id IN (?) ORDER BY created_at`, automationIDs)
	if err != nil {
		return nil, err
	}
	query = tx.Rebind(query)
	var triggers []AutomationTrigger
	if err := sqlx.SelectContext(ctx, tx, &triggers, query, args...); err != nil {
		return nil, err
	}
	hydrateTriggers(triggers)
	result := make(map[string][]AutomationTrigger, len(automationIDs))
	for i := range triggers {
		result[triggers[i].AutomationID] = append(result[triggers[i].AutomationID], triggers[i])
	}
	return result, nil
}

// listRepositoryIDsForAutomationsTx mirrors listRepositoryIDsForAutomations,
// reading within tx.
func listRepositoryIDsForAutomationsTx(ctx context.Context, tx *sqlx.Tx, automationIDs []string) (map[string][]string, error) {
	if len(automationIDs) == 0 {
		return make(map[string][]string), nil
	}
	query, args, err := sqlx.In(
		`SELECT automation_id, repository_id FROM automation_repositories
		WHERE automation_id IN (?) ORDER BY automation_id, position, repository_id`, automationIDs)
	if err != nil {
		return nil, err
	}
	query = tx.Rebind(query)
	type row struct {
		AutomationID string `db:"automation_id"`
		RepositoryID string `db:"repository_id"`
	}
	var rows []row
	if err := sqlx.SelectContext(ctx, tx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make(map[string][]string, len(automationIDs))
	for _, id := range automationIDs {
		result[id] = []string{}
	}
	for _, r := range rows {
		result[r.AutomationID] = append(result[r.AutomationID], r.RepositoryID)
	}
	return result, nil
}
