package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

const pendingActionProjectionEpochMetaKey = "pending_action_projection_epoch"

// NextPendingActionProjectionEpoch atomically allocates a durable backend
// generation. Keeping this value in kandev_meta lets clients compare snapshots
// emitted on opposite sides of a backend restart without trusting wall clocks.
func (r *Repository) NextPendingActionProjectionEpoch(ctx context.Context) (uint64, error) {
	var value string
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`
		INSERT INTO kandev_meta (key, value) VALUES (?, '1')
		ON CONFLICT (key) DO UPDATE SET
			value = CAST(CAST(kandev_meta.value AS BIGINT) + 1 AS TEXT)
		WHERE kandev_meta.value = CAST(CAST(kandev_meta.value AS BIGINT) AS TEXT)
			AND CAST(kandev_meta.value AS BIGINT) > 0
		RETURNING value
	`), pendingActionProjectionEpochMetaKey).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf(
				"allocate pending-action projection epoch: stored metadata is not a canonical positive integer: %w",
				err,
			)
		}
		return 0, fmt.Errorf("allocate pending-action projection epoch: %w", err)
	}
	epoch, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid pending-action projection epoch %q: %w", value, err)
	}
	if epoch == 0 {
		return 0, fmt.Errorf("invalid pending-action projection epoch %q: must be positive", value)
	}
	return epoch, nil
}
