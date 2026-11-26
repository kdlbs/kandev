package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DELIVERY-RETENTION-005.1
func TestUnboundWorkflowCountUsesSparseIndex(t *testing.T) {
	repo := newDeliveryRetentionDeletionRepo(t, "SQLite")
	ctx := context.Background()
	seedUnboundWorkflowCountRows(t, repo)
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	count, err := repo.countUnboundWorkflowRowsTx(ctx, tx, 2)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	expectedStatement, expectedArgs := unboundWorkflowCountQuery("pgx", 2)
	var expected int
	require.NoError(t, tx.QueryRowxContext(ctx, repo.db.Rebind(expectedStatement), expectedArgs...).Scan(&expected))
	require.Equal(t, expected, count)
	statement, args := unboundWorkflowCountQuery(repo.db.DriverName(), 2)
	plan := explainUnboundWorkflowCount(t, ctx, tx, repo.db.Rebind(statement), args...)
	planText := strings.Join(plan, "\n")
	require.Contains(t, planText, "idx_agent_delivery_effects_unbound_workflow")
	require.NotContains(t, planText, "USE TEMP B-TREE")
}

// @covers AC-PLATFORM-DELIVERY-RETENTION-005.1
func TestUnboundWorkflowCountPostgresKeepsBoundPredicate(t *testing.T) {
	repo := newDeliveryRetentionDeletionRepo(t, "Postgres")
	ctx := context.Background()
	seedUnboundWorkflowCountRows(t, repo)
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	count, err := repo.countUnboundWorkflowRowsTx(ctx, tx, 2)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

func seedUnboundWorkflowCountRows(t *testing.T, repo *Repository) {
	t.Helper()
	rows := []struct {
		key, effectType, sessionID, incarnationID string
	}{
		{key: "a-unbound", effectType: "workflow.on_turn_complete"},
		{key: "b-partially-bound", effectType: "workflow.on_turn_complete", sessionID: "session-b"},
		{key: "c-bound", effectType: "workflow.on_turn_complete", sessionID: "session-c", incarnationID: "inc-c"},
		{key: "d-other-type", effectType: "agent_delivery.event"},
	}
	for _, row := range rows {
		_, err := repo.db.Exec(repo.db.Rebind(`
			INSERT INTO agent_delivery_effects
			(effect_key, stream_id, sequence, effect_type, state, created_at, session_id, incarnation_id)
			VALUES (?, ?, 1, ?, 'completed', CURRENT_TIMESTAMP, ?, ?)`),
			row.key, "stream:"+row.key, row.effectType, row.sessionID, row.incarnationID)
		require.NoError(t, err)
	}
}

func explainUnboundWorkflowCount(t *testing.T, ctx context.Context, tx *sqlx.Tx, statement string, args ...any) []string {
	t.Helper()
	rows, err := tx.QueryxContext(ctx, "EXPLAIN QUERY PLAN "+statement, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		details = append(details, detail)
	}
	require.NoError(t, rows.Err())
	return details
}
