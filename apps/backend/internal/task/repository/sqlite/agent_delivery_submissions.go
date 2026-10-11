package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const agentDeliverySubmissionPageLimit = 128

func (r *Repository) GetAgentDeliverySubmissionMetadata(
	ctx context.Context,
	id string,
) (*models.AgentDeliverySubmission, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT id, session_id, incarnation_id, harness_generation, owner_generation,
		       dispatch_attempt_id, payload_hash, state, outcome, payload_pruned_at,
		       payload_bytes, fully_settled_at, terminal_stream_id, terminal_sequence,
		       terminal_turn_id, created_at, updated_at
		FROM agent_delivery_submissions WHERE id = ?`), id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, repoerrors.ErrAgentDeliverySubmissionNotFound
	}
	submission, err := scanAgentDeliverySubmissionMetadata(rows)
	if err != nil {
		return nil, err
	}
	return submission, nil
}

// ListAgentDeliverySubmissionsPage returns recovery metadata without prompt
// bodies. The creation timestamp and ID form a stable keyset cursor.
func (r *Repository) ListAgentDeliverySubmissionsPage(
	ctx context.Context,
	sessionID string,
	afterCreatedAt *time.Time,
	afterID string,
	limit int,
) ([]*models.AgentDeliverySubmission, error) {
	if limit <= 0 || limit > agentDeliverySubmissionPageLimit {
		limit = agentDeliverySubmissionPageLimit
	}
	query := `
		SELECT id, session_id, incarnation_id, harness_generation, owner_generation,
		       dispatch_attempt_id, payload_hash, state, outcome, payload_pruned_at,
		       payload_bytes, fully_settled_at, terminal_stream_id, terminal_sequence,
		       terminal_turn_id, created_at, updated_at
		FROM agent_delivery_submissions
		WHERE session_id = ?
		ORDER BY created_at ASC, id ASC LIMIT ?`
	args := []any{sessionID, limit}
	if afterCreatedAt != nil {
		query = `
			SELECT id, session_id, incarnation_id, harness_generation, owner_generation,
			       dispatch_attempt_id, payload_hash, state, outcome, payload_pruned_at,
			       payload_bytes, fully_settled_at, terminal_stream_id, terminal_sequence,
			       terminal_turn_id, created_at, updated_at
			FROM agent_delivery_submissions
			WHERE session_id = ? AND (created_at > ? OR (created_at = ? AND id > ?))
			ORDER BY created_at ASC, id ASC LIMIT ?`
		args = []any{sessionID, *afterCreatedAt, *afterCreatedAt, afterID, limit}
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var submissions []*models.AgentDeliverySubmission
	for rows.Next() {
		submission, err := scanAgentDeliverySubmissionMetadata(rows)
		if err != nil {
			return nil, err
		}
		submissions = append(submissions, submission)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return submissions, nil
}

// ListAgentDeliverySubmissions keeps the legacy complete-list API while
// avoiding payload reads. Recovery should prefer the paginated interface.
func (r *Repository) ListAgentDeliverySubmissions(
	ctx context.Context,
	sessionID string,
) ([]*models.AgentDeliverySubmission, error) {
	var submissions []*models.AgentDeliverySubmission
	var afterCreatedAt *time.Time
	afterID := ""
	for {
		page, err := r.ListAgentDeliverySubmissionsPage(
			ctx, sessionID, afterCreatedAt, afterID, agentDeliverySubmissionPageLimit,
		)
		if err != nil {
			return nil, err
		}
		submissions = append(submissions, page...)
		if len(page) < agentDeliverySubmissionPageLimit {
			return submissions, nil
		}
		last := page[len(page)-1]
		createdAt := last.CreatedAt
		afterCreatedAt, afterID = &createdAt, last.ID
	}
}

type agentDeliverySubmissionMetadataScanner interface {
	Scan(dest ...any) error
}

func scanAgentDeliverySubmissionMetadata(rows agentDeliverySubmissionMetadataScanner) (*models.AgentDeliverySubmission, error) {
	var submission models.AgentDeliverySubmission
	var payloadPrunedAt, fullySettledAt sql.NullTime
	if err := rows.Scan(
		&submission.ID, &submission.SessionID, &submission.IncarnationID,
		&submission.HarnessGeneration, &submission.OwnerGeneration, &submission.DispatchAttemptID,
		&submission.PayloadHash, &submission.State, &submission.Outcome, &payloadPrunedAt,
		&submission.PayloadBytes, &fullySettledAt, &submission.TerminalStreamID,
		&submission.TerminalSequence, &submission.TerminalTurnID,
		&submission.CreatedAt, &submission.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if payloadPrunedAt.Valid {
		at := payloadPrunedAt.Time
		submission.PayloadPrunedAt = &at
	}
	if fullySettledAt.Valid {
		at := fullySettledAt.Time
		submission.FullySettledAt = &at
	}
	return &submission, nil
}
