package sqlite

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/office/models"
)

// inFlightRunStatuses are the run statuses that mean work is still owed on a
// task. The runs table has no "running" status: a run is `queued` until the
// scheduler claims it and `claimed` while it executes, reaching `finished`,
// `failed` or `cancelled` only at the end. Anything outside this pair is
// terminal.
var inFlightRunStatuses = []string{
	string(models.RunStatusQueued),
	string(models.RunStatusClaimed),
}

// HasInFlightRunForTask reports whether the task has a queued or claimed run.
//
// The runs table carries no task_id column — a run's task lives in its payload
// JSON — so the predicate reads it out with dialect.JSONExtract rather than a
// literal json_extract. Postgres is a supported driver
// (internal/persistence/provider.go), and a SQLite-only expression here would
// pass every SQLite test and fail at runtime on Postgres. Precedent:
// HasPriorTasklessFailedRun in failure.go, covered by failure_postgres_test.go.
//
// Used by the Office decision-waiting detector as its false-positive guard: a
// task with work in flight is being worked on, not stalled, however long it has
// carried an undecided review seat.
func (r *Repository) HasInFlightRunForTask(ctx context.Context, taskID string) (bool, error) {
	if taskID == "" {
		return false, nil
	}
	taskIDExpr := dialect.JSONExtract(r.ro.DriverName(), "payload", "task_id")
	query := fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1 FROM runs
			WHERE %s = ?
			  AND status IN (?, ?)
		)
	`, taskIDExpr)
	var exists bool
	if err := r.ro.QueryRowxContext(
		ctx, r.ro.Rebind(query), taskID, inFlightRunStatuses[0], inFlightRunStatuses[1],
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("check in-flight run for task: %w", err)
	}
	return exists, nil
}
