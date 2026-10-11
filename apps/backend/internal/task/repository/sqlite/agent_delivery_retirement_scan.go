package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// Each recursive step seeks the next session through the four session-leading
// indexes. Live rows and unknown provenance advance the same durable cursor.
const agentDeliveryOrphanScanQuery = `WITH RECURSIVE sessions(session_id, n) AS (
 SELECT (SELECT MIN(value) FROM (SELECT MIN(session_id) AS value FROM agent_delivery_inbox WHERE session_id > ? UNION ALL SELECT MIN(session_id) AS value FROM agent_delivery_submissions WHERE session_id > ? UNION ALL SELECT MIN(session_id) AS value FROM agent_delivery_cursors WHERE session_id > ? UNION ALL SELECT MIN(session_id) AS value FROM agent_delivery_effects WHERE session_id > ? AND session_id <> '')), 1
 UNION ALL
 SELECT (SELECT MIN(value) FROM (SELECT MIN(session_id) AS value FROM agent_delivery_inbox WHERE session_id > sessions.session_id UNION ALL SELECT MIN(session_id) AS value FROM agent_delivery_submissions WHERE session_id > sessions.session_id UNION ALL SELECT MIN(session_id) AS value FROM agent_delivery_cursors WHERE session_id > sessions.session_id UNION ALL SELECT MIN(session_id) AS value FROM agent_delivery_effects WHERE session_id > sessions.session_id AND session_id <> '')), n + 1 FROM sessions
 WHERE session_id IS NOT NULL AND n < ?
)
SELECT sessions.session_id, COALESCE((SELECT MIN(value) FROM (SELECT MIN(incarnation_id) AS value FROM agent_delivery_inbox WHERE session_id = sessions.session_id AND incarnation_id > '' UNION ALL SELECT MIN(incarnation_id) AS value FROM agent_delivery_submissions WHERE session_id = sessions.session_id AND incarnation_id > '' UNION ALL SELECT MIN(incarnation_id) AS value FROM agent_delivery_cursors WHERE session_id = sessions.session_id AND incarnation_id > '' UNION ALL SELECT MIN(incarnation_id) AS value FROM agent_delivery_effects WHERE session_id = sessions.session_id AND session_id <> '' AND incarnation_id > '')), ''),
 EXISTS (SELECT 1 FROM task_sessions live WHERE live.id = sessions.session_id),
 EXISTS (SELECT 1 FROM agent_delivery_cleanup_jobs job WHERE job.session_id = sessions.session_id)
FROM sessions WHERE sessions.session_id IS NOT NULL ORDER BY sessions.session_id`

func (r *Repository) agentDeliveryOrphanCandidatesTx(ctx context.Context, tx *sqlx.Tx, limit int) ([]agentDeliverySessionIdentity, error) {
	progress, err := loadAgentDeliveryRetentionProgress(ctx, tx, r.db.Rebind)
	if err != nil {
		return nil, err
	}
	candidates, _, _, err := r.scanAgentDeliveryOrphanCandidatesTx(ctx, tx, progress.lastSessionID, limit)
	return candidates, err
}

func (r *Repository) scanAgentDeliveryOrphanCandidatesTx(ctx context.Context, tx *sqlx.Tx, after string, limit int) ([]agentDeliverySessionIdentity, int, string, error) {
	candidates, scanned, last, err := r.loadAgentDeliveryOrphanCandidatesTx(ctx, tx, after, limit)
	if err != nil {
		return nil, 0, after, err
	}
	if scanned == 0 && after != "" {
		return r.loadAgentDeliveryOrphanCandidatesTx(ctx, tx, "", limit)
	}
	return candidates, scanned, last, nil
}

func (r *Repository) loadAgentDeliveryOrphanCandidatesTx(ctx context.Context, tx *sqlx.Tx, after string, limit int) ([]agentDeliverySessionIdentity, int, string, error) {
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(agentDeliveryOrphanScanQuery), after, after, after, after, limit)
	if err != nil {
		return nil, 0, after, fmt.Errorf("seek agent delivery orphan identities: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var candidates []agentDeliverySessionIdentity
	scanned := 0
	last := ""
	for rows.Next() {
		var candidate agentDeliverySessionIdentity
		var live, retiring bool
		if err := rows.Scan(&candidate.sessionID, &candidate.incarnationID, &live, &retiring); err != nil {
			return nil, 0, after, err
		}
		scanned++
		last = candidate.sessionID
		if !live && !retiring && candidate.incarnationID != "" {
			candidates = append(candidates, candidate)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, after, fmt.Errorf("iterate agent delivery orphan identities: %w", err)
	}
	return candidates, scanned, last, nil
}
