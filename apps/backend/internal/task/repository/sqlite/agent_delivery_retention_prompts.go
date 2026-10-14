package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (r *Repository) selectAgentDeliveryPromptRetentionCandidates(
	ctx context.Context,
	tx *sqlx.Tx,
	lastSubmissionID string,
	limit int,
	byteLimit int64,
	cutoff time.Time,
) ([]agentDeliveryPromptRetentionCandidate, bool, error) {
	if limit <= 0 {
		return nil, false, nil
	}
	candidates, err := r.loadAgentDeliveryPromptRetentionCandidates(
		ctx, tx, lastSubmissionID, nil, limit, byteLimit, cutoff,
	)
	if err != nil || len(candidates) != 0 || lastSubmissionID == "" {
		return candidates, false, err
	}
	candidates, err = r.loadAgentDeliveryPromptRetentionCandidates(
		ctx, tx, "", nil, limit, byteLimit, cutoff,
	)
	return candidates, true, err
}

func (r *Repository) reloadAgentDeliveryPromptRetentionCandidates(
	ctx context.Context,
	tx *sqlx.Tx,
	selected []agentDeliveryPromptRetentionCandidate,
	cutoff time.Time,
) ([]agentDeliveryPromptRetentionCandidate, error) {
	if len(selected) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(selected))
	for _, candidate := range selected {
		ids = append(ids, candidate.submissionID)
	}
	return r.loadAgentDeliveryPromptRetentionCandidates(
		ctx, tx, "", ids, len(ids), agentDeliveryRetentionPayloadByteLimit, cutoff,
	)
}

func (r *Repository) loadAgentDeliveryPromptRetentionCandidates(
	ctx context.Context,
	tx *sqlx.Tx,
	lastSubmissionID string,
	ids []string,
	limit int,
	byteLimit int64,
	cutoff time.Time,
) ([]agentDeliveryPromptRetentionCandidate, error) {
	query, args := agentDeliveryPromptRetentionCandidateQuery(lastSubmissionID, ids, limit, byteLimit, cutoff)
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("scan settled agent delivery prompt candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var candidates []agentDeliveryPromptRetentionCandidate
	for rows.Next() {
		var candidate agentDeliveryPromptRetentionCandidate
		var canPrune int
		if err := rows.Scan(
			&candidate.submissionID, &candidate.sessionID, &candidate.incarnationID,
			&candidate.harnessGeneration, &candidate.dispatchAttemptID, &candidate.payloadHash,
			&candidate.payloadBytes, &candidate.payloadLength, &candidate.terminalStreamID,
			&candidate.terminalSequence, &candidate.terminalTurnID, &candidate.outcome,
			&candidate.fullySettledAt, &canPrune,
		); err != nil {
			return nil, fmt.Errorf("scan settled agent delivery prompt candidate: %w", err)
		}
		candidate.canPrune = canPrune != 0
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate settled agent delivery prompt candidates: %w", err)
	}
	return candidates, nil
}

func agentDeliveryPromptRetentionCandidateQuery(
	lastSubmissionID string,
	ids []string,
	limit int,
	byteLimit int64,
	cutoff time.Time,
) (string, []any) {
	var filters []string
	args := make([]any, 0, len(ids)+5)
	if lastSubmissionID != "" {
		filters = append(filters, "s.id > ?")
		args = append(args, lastSubmissionID)
	}
	if len(ids) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
		filters = append(filters, "s.id IN ("+placeholders+")")
		for _, id := range ids {
			args = append(args, id)
		}
	}
	filter := ""
	if len(filters) > 0 {
		filter = " AND " + strings.Join(filters, " AND ")
	}
	args = append([]any{cutoff}, args...)
	args = append(args, limit, byteLimit, byteLimit, byteLimit)
	return strings.Replace(agentDeliveryPromptRetentionCandidateSQL, "/* submission filters */", filter, 1), args
}

const agentDeliveryPromptRetentionCandidateSQL = `
	WITH source AS (
		SELECT s.id, s.session_id, s.incarnation_id, s.harness_generation,
			s.dispatch_attempt_id, s.payload_hash, s.payload_bytes, LENGTH(s.payload) AS payload_length,
			s.terminal_stream_id, s.terminal_sequence, s.terminal_turn_id, s.state, s.outcome,
			s.fully_settled_at
		FROM agent_delivery_submissions s
		WHERE s.fully_settled_at IS NOT NULL AND s.fully_settled_at <= ?
		  AND s.payload_pruned_at IS NULL /* submission filters */
		ORDER BY s.id LIMIT ?
	), proved AS (
		SELECT source.*,
			CASE WHEN state IN ('completed', 'failed', 'cancelled') AND outcome = state
			  AND payload_hash <> '' AND payload_length > 0
			  AND (payload_bytes = 0 OR payload_bytes = payload_length)
			  AND terminal_stream_id <> '' AND terminal_sequence > 0 AND terminal_turn_id <> ''
			  AND EXISTS (
				SELECT 1 FROM task_sessions sess
				WHERE sess.id = source.session_id AND sess.queue_incarnation_id = source.incarnation_id
			  )
			  AND EXISTS (
				SELECT 1 FROM agent_delivery_cursors cur
				WHERE cur.stream_id = source.terminal_stream_id AND cur.session_id = source.session_id
				  AND cur.incarnation_id = source.incarnation_id
				  AND cur.harness_generation = source.harness_generation
				  AND cur.projected_sequence >= source.terminal_sequence
			  )
			  AND (EXISTS (
				SELECT 1 FROM agent_delivery_cursors retired
				WHERE retired.stream_id = source.terminal_stream_id AND retired.session_id = source.session_id
				  AND retired.incarnation_id = source.incarnation_id AND retired.harness_generation = source.harness_generation
				  AND retired.retired_sequence >= source.terminal_sequence
			  ) OR EXISTS (
				SELECT 1 FROM agent_delivery_inbox inb
				WHERE inb.stream_id = source.terminal_stream_id AND inb.sequence = source.terminal_sequence
				  AND inb.session_id = source.session_id AND inb.incarnation_id = source.incarnation_id
				  AND inb.harness_generation = source.harness_generation AND inb.submission_id = source.id
				  AND inb.terminal <> 0
			  ))
			  AND EXISTS (
				SELECT 1 FROM agent_delivery_effects term
				WHERE term.effect_key = 'agent_delivery.terminal:' || source.terminal_stream_id || ':' || CAST(source.terminal_sequence AS TEXT)
				  AND term.effect_type = 'agent_delivery.terminal_settlement'
				  AND term.state = 'completed' AND term.completed_at IS NOT NULL
				  AND term.stream_id = source.terminal_stream_id AND term.sequence = source.terminal_sequence
				  AND term.session_id = source.session_id AND term.incarnation_id = source.incarnation_id
				  AND term.harness_generation = source.harness_generation AND term.submission_id = source.id
				  AND term.turn_id = source.terminal_turn_id AND term.outcome = source.outcome
			  )
			  AND EXISTS (
				SELECT 1 FROM agent_delivery_effects wf
				WHERE wf.effect_key = 'workflow.on_turn_complete:' || source.terminal_turn_id
				  AND wf.effect_type = 'workflow.on_turn_complete'
				  AND wf.state = 'completed' AND wf.completed_at IS NOT NULL
				  AND wf.stream_id = source.terminal_stream_id AND wf.sequence = source.terminal_sequence
				  AND wf.session_id = source.session_id AND wf.incarnation_id = source.incarnation_id
				  AND wf.harness_generation = source.harness_generation AND wf.submission_id = source.id
				  AND wf.turn_id = source.terminal_turn_id
				  AND wf.outcome IN ('applied', 'no_automatic_transition', 'not_required')
			  )
			  AND NOT EXISTS (
				SELECT 1 FROM agent_delivery_effects newer
				WHERE newer.effect_type = 'agent_delivery.terminal_settlement'
				  AND newer.submission_id = source.id AND newer.stream_id = source.terminal_stream_id
				  AND newer.sequence > source.terminal_sequence
			  )
			  AND NOT EXISTS (
				SELECT 1 FROM session_recovery_blocks blk
				WHERE blk.session_id = source.session_id AND blk.incarnation_id = source.incarnation_id
				  AND blk.expected_generation = source.harness_generation
				  AND blk.consumer_reference = 'agent_delivery' AND blk.delivery_submission_id = source.id
				  AND (blk.delivery_stream_id = '' OR blk.delivery_stream_id = source.terminal_stream_id)
				  AND blk.state = 'open'
			  ) THEN 1 ELSE 0 END AS proven
		FROM source
	), budgeted AS (
		SELECT proved.*,
			COALESCE(SUM(CASE WHEN proven = 1 AND payload_length <= ?
				THEN payload_length ELSE 0 END) OVER (
				ORDER BY id ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
			), 0) AS cumulative_payload_bytes
		FROM proved
	)
	SELECT id, session_id, incarnation_id, harness_generation, dispatch_attempt_id,
		payload_hash, payload_bytes, payload_length, terminal_stream_id, terminal_sequence,
		terminal_turn_id, outcome, fully_settled_at,
		CASE WHEN proven = 1 AND payload_length <= ? AND cumulative_payload_bytes <= ? THEN 1 ELSE 0 END
	FROM budgeted ORDER BY id`

func (r *Repository) pruneAgentDeliveryPromptPayloads(
	ctx context.Context,
	tx *sqlx.Tx,
	candidates []agentDeliveryPromptRetentionCandidate,
	now time.Time,
	payloadGrace time.Duration,
	byteLimit int64,
	result *models.AgentDeliveryRetentionBatchResult,
) error {
	usedBytes := result.PayloadBytesRead + result.PromptPayloadBytesPruned
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !candidate.canPrune || candidate.payloadLength <= 0 || candidate.payloadLength > byteLimit-usedBytes {
			continue
		}
		if candidate.payloadBytes != 0 && candidate.payloadBytes != candidate.payloadLength {
			continue
		}
		if err := r.failAgentDeliveryRetention("before_prompt_payload"); err != nil {
			return err
		}
		cutoff := now.Add(-payloadGrace)
		updated, err := tx.ExecContext(ctx, r.db.Rebind(agentDeliveryPromptRetentionUpdateSQL), []byte{}, now, candidate.payloadLength, now,
			candidate.submissionID, candidate.sessionID, candidate.incarnationID, candidate.harnessGeneration,
			candidate.dispatchAttemptID, candidate.payloadHash, 0, candidate.payloadLength, candidate.payloadLength, cutoff,
			candidate.terminalStreamID, candidate.terminalSequence, candidate.terminalTurnID)
		if err != nil {
			return fmt.Errorf("prune settled agent delivery prompt payload %q: %w", candidate.submissionID, err)
		}
		rows, err := updated.RowsAffected()
		if err != nil {
			return fmt.Errorf("read pruned settled prompt row count: %w", err)
		}
		if rows == 0 {
			continue
		}
		if rows != 1 {
			return repoerrors.ErrAgentDeliveryReceiptCorrupt
		}
		result.PromptPayloadsPruned++
		result.PromptPayloadBytesPruned += candidate.payloadLength
		result.PayloadBytesPruned += candidate.payloadLength
		usedBytes += candidate.payloadLength
		if err := r.failAgentDeliveryRetention("after_prompt_payload"); err != nil {
			return err
		}
	}
	return nil
}

const agentDeliveryPromptRetentionUpdateSQL = `
			UPDATE agent_delivery_submissions
			SET payload = ?, payload_pruned_at = ?,
			    payload_bytes = CASE WHEN payload_bytes = 0 THEN ? ELSE payload_bytes END,
			    updated_at = ?
			WHERE id = ? AND session_id = ? AND incarnation_id = ? AND harness_generation = ?
			  AND dispatch_attempt_id = ? AND payload_hash = ? AND payload_pruned_at IS NULL
			  AND payload_bytes IN (?, ?) AND LENGTH(payload) = ?
			  AND fully_settled_at IS NOT NULL AND fully_settled_at <= ?
			  AND state IN ('completed', 'failed', 'cancelled') AND outcome = state
			  AND terminal_stream_id = ? AND terminal_sequence = ? AND terminal_turn_id = ?
			  AND EXISTS (
				SELECT 1 FROM task_sessions session
				WHERE session.id = agent_delivery_submissions.session_id
				  AND session.queue_incarnation_id = agent_delivery_submissions.incarnation_id
			  )
			  AND EXISTS (
				SELECT 1 FROM agent_delivery_cursors cursor
				WHERE cursor.stream_id = agent_delivery_submissions.terminal_stream_id
				  AND cursor.session_id = agent_delivery_submissions.session_id
				  AND cursor.incarnation_id = agent_delivery_submissions.incarnation_id
				  AND cursor.harness_generation = agent_delivery_submissions.harness_generation
				  AND cursor.projected_sequence >= agent_delivery_submissions.terminal_sequence
			  )
			  AND (EXISTS (
				SELECT 1 FROM agent_delivery_cursors retired
				WHERE retired.stream_id = agent_delivery_submissions.terminal_stream_id
				  AND retired.session_id = agent_delivery_submissions.session_id
				  AND retired.incarnation_id = agent_delivery_submissions.incarnation_id
				  AND retired.harness_generation = agent_delivery_submissions.harness_generation
				  AND retired.retired_sequence >= agent_delivery_submissions.terminal_sequence
			  ) OR EXISTS (
				SELECT 1 FROM agent_delivery_inbox inbox
				WHERE inbox.stream_id = agent_delivery_submissions.terminal_stream_id
				  AND inbox.sequence = agent_delivery_submissions.terminal_sequence
				  AND inbox.session_id = agent_delivery_submissions.session_id
				  AND inbox.incarnation_id = agent_delivery_submissions.incarnation_id
				  AND inbox.harness_generation = agent_delivery_submissions.harness_generation
				  AND inbox.submission_id = agent_delivery_submissions.id AND inbox.terminal <> 0
			  ))
			  AND EXISTS (
				SELECT 1 FROM agent_delivery_effects terminal
				WHERE terminal.effect_key = 'agent_delivery.terminal:' || agent_delivery_submissions.terminal_stream_id
				  || ':' || CAST(agent_delivery_submissions.terminal_sequence AS TEXT)
				  AND terminal.effect_type = 'agent_delivery.terminal_settlement'
				  AND terminal.state = 'completed' AND terminal.completed_at IS NOT NULL
				  AND terminal.stream_id = agent_delivery_submissions.terminal_stream_id
				  AND terminal.sequence = agent_delivery_submissions.terminal_sequence
				  AND terminal.session_id = agent_delivery_submissions.session_id
				  AND terminal.incarnation_id = agent_delivery_submissions.incarnation_id
				  AND terminal.harness_generation = agent_delivery_submissions.harness_generation
				  AND terminal.submission_id = agent_delivery_submissions.id
				  AND terminal.turn_id = agent_delivery_submissions.terminal_turn_id
				  AND terminal.outcome = agent_delivery_submissions.outcome
			  )
			  AND EXISTS (
				SELECT 1 FROM agent_delivery_effects workflow
				WHERE workflow.effect_key = 'workflow.on_turn_complete:' || agent_delivery_submissions.terminal_turn_id
				  AND workflow.effect_type = 'workflow.on_turn_complete'
				  AND workflow.state = 'completed' AND workflow.completed_at IS NOT NULL
				  AND workflow.stream_id = agent_delivery_submissions.terminal_stream_id
				  AND workflow.sequence = agent_delivery_submissions.terminal_sequence
				  AND workflow.session_id = agent_delivery_submissions.session_id
				  AND workflow.incarnation_id = agent_delivery_submissions.incarnation_id
				  AND workflow.harness_generation = agent_delivery_submissions.harness_generation
				  AND workflow.submission_id = agent_delivery_submissions.id
				  AND workflow.turn_id = agent_delivery_submissions.terminal_turn_id
				  AND workflow.outcome IN ('applied', 'no_automatic_transition', 'not_required')
			  )
			  AND NOT EXISTS (
				SELECT 1 FROM agent_delivery_effects newer
				WHERE newer.effect_type = 'agent_delivery.terminal_settlement'
				  AND newer.submission_id = agent_delivery_submissions.id
				  AND newer.stream_id = agent_delivery_submissions.terminal_stream_id
				  AND newer.sequence > agent_delivery_submissions.terminal_sequence
			  )
			  AND NOT EXISTS (
				SELECT 1 FROM session_recovery_blocks block
				WHERE block.session_id = agent_delivery_submissions.session_id
				  AND block.incarnation_id = agent_delivery_submissions.incarnation_id
				  AND block.expected_generation = agent_delivery_submissions.harness_generation
				  AND block.consumer_reference = 'agent_delivery'
				  AND block.delivery_submission_id = agent_delivery_submissions.id
				  AND (block.delivery_stream_id = '' OR block.delivery_stream_id = agent_delivery_submissions.terminal_stream_id)
				  AND block.state = 'open'
			  )`
