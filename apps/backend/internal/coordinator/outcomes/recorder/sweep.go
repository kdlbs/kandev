package recorder

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// SQLProposals reads graded and gradable proposals from the database.
type SQLProposals struct{ DB *sqlx.DB }

// OpenProposalsOfTask lists decided create-task proposals of taskID whose
// outcome row is missing or not final.
func (p SQLProposals) OpenProposalsOfTask(ctx context.Context, taskID string) ([]string, error) {
	var ids []string
	err := p.DB.SelectContext(ctx, &ids, p.DB.Rebind(`
		SELECT p.id FROM coordinator_proposals p
		LEFT JOIN coordinator_outcomes o ON o.proposal_id = p.id
		WHERE p.task_id = ? AND p.status IN ('approved', 'rejected', 'returned') AND (o.proposal_id IS NULL OR o.final = ?)
		LIMIT ?`), taskID, false, sweepBatchSize)
	return ids, err
}

// Sweep grades every decided proposal whose row is missing or not final,
// oldest grade first, in batches until none is left that this sweep has not
// already tried. It returns the number graded.
func (p SQLProposals) Sweep(ctx context.Context, q *Queue) int {
	started := time.Now().UTC()
	tried := map[string]bool{}
	total := 0
	for ctx.Err() == nil {
		var ids []string
		err := p.DB.SelectContext(ctx, &ids, p.DB.Rebind(`
			SELECT p.id FROM coordinator_proposals p
			LEFT JOIN coordinator_outcomes o ON o.proposal_id = p.id
			WHERE p.status IN ('approved', 'rejected', 'returned') AND (o.proposal_id IS NULL OR o.final = ?)
			  AND (o.graded_at IS NULL OR o.graded_at < ?)
			ORDER BY (o.graded_at IS NOT NULL), o.graded_at, p.id
			LIMIT ?`), false, started, sweepBatchSize+len(tried))
		if err != nil {
			bump(gradeFailedTotal, GradeProposalRead)
			return total
		}
		progressed := false
		for _, id := range ids {
			if tried[id] {
				continue
			}
			tried[id] = true
			progressed = true
			q.gradeOne(ctx, id, time.Time{})
			total++
		}
		if !progressed {
			return total
		}
	}
	return total
}

// RunSweeps sweeps once now and then every day until ctx ends.
func (q *Queue) RunSweeps(ctx context.Context, p SQLProposals) {
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		p.Sweep(ctx, q)
		t := time.NewTicker(sweepInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-q.stopCh:
				return
			case <-t.C:
				p.Sweep(ctx, q)
			}
		}
	}()
}
