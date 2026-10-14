package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// Rank each page task against its full destination queue within the same snapshot.
func loadSidebarPageQueuePositions(ctx context.Context, tx *sqlx.Tx, repo *Repository, pageRows []sidebarPageRow) error {
	if len(pageRows) == 0 {
		return nil
	}
	ids := make([]string, len(pageRows))
	for index, row := range pageRows {
		ids[index] = row.taskID
	}
	placeholders, args := buildInPlaceholders(ids)
	query := `WITH page_window AS (SELECT id FROM tasks WHERE id IN (` + placeholders + `))` +
		sidebarPageQueueCTEs(repo.ro.DriverName()) +
		` SELECT id, queue_position, queue_total FROM wip_queue_ranked WHERE id IN (` + placeholders + `)`
	args = append(args, args...)
	rows, err := tx.QueryContext(ctx, repo.ro.Rebind(query), args...)
	if err != nil {
		return fmt.Errorf("hydrate sidebar queue positions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	positions := make(map[string][2]int, len(pageRows))
	for rows.Next() {
		var id string
		var position, total int
		if err := rows.Scan(&id, &position, &total); err != nil {
			return fmt.Errorf("scan sidebar queue positions: %w", err)
		}
		positions[id] = [2]int{position, total}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read sidebar queue positions: %w", err)
	}
	for index := range pageRows {
		position := positions[pageRows[index].taskID]
		pageRows[index].wipPosition, pageRows[index].wipTotal = position[0], position[1]
	}
	return nil
}
