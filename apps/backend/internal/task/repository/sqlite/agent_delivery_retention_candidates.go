package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

func scanAgentDeliveryRetentionCandidates(rows *sqlx.Rows) ([]agentDeliveryRetentionCandidate, error) {
	defer func() { _ = rows.Close() }()
	var candidates []agentDeliveryRetentionCandidate
	for rows.Next() {
		candidate, err := scanAgentDeliveryRetentionCandidate(rows)
		if err != nil {
			return nil, fmt.Errorf("scan agent delivery retention candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent delivery retention candidates: %w", err)
	}
	return candidates, nil
}

func agentDeliveryRetentionCandidateQuery(
	lastStreamID string,
	lastSequence int64,
	streamIDs []string,
	limit int,
	byteLimit int64,
	graceCutoff time.Time,
	includePayload bool,
) (string, []any) {
	var sourceFilters []string
	args := make([]any, 0, 16+len(streamIDs))
	if lastStreamID != "" {
		if len(streamIDs) == 1 && streamIDs[0] == lastStreamID {
			sourceFilters = append(sourceFilters, "i.stream_id = ? AND i.sequence > ?")
		} else {
			sourceFilters = append(sourceFilters, "(i.stream_id, i.sequence) > (?, ?)")
		}
		args = append(args, lastStreamID, lastSequence)
	}
	if len(streamIDs) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(streamIDs)), ",")
		sourceFilters = append(sourceFilters, "i.stream_id IN ("+placeholders+")")
		for _, streamID := range streamIDs {
			args = append(args, streamID)
		}
	}
	return agentDeliveryRetentionFilteredQuery(sourceFilters, args, limit, byteLimit, graceCutoff, includePayload)
}

func agentDeliveryRetentionFilteredQuery(sourceFilters []string, args []any, limit int, byteLimit int64, graceCutoff time.Time, includePayload bool) (string, []any) {
	args = append(args, limit, graceCutoff,
		byteLimit, includePayload, byteLimit, byteLimit, includePayload, byteLimit, byteLimit)
	filter := ""
	if len(sourceFilters) > 0 {
		filter = " AND " + strings.Join(sourceFilters, " AND ")
	}
	return retentionCandidateProofQuery(strings.Replace(agentDeliveryRetentionCandidateSQL, "/* source filters */", filter, 1)), args
}

const agentDeliveryRetentionOutputTypesSQL = `'message_chunk', 'reasoning', 'tool_call', 'tool_update', 'plan', 'agent_plan'`

const agentDeliveryRetentionControlTypesSQL = `'agent_capabilities', 'session_status', 'available_commands',
 'session_mode', 'session_models', 'session_info', 'context_window', 'mcp_attachment', 'turn_started'`

const agentDeliveryRetentionNonterminalTypesSQL = agentDeliveryRetentionOutputTypesSQL + ", " + agentDeliveryRetentionControlTypesSQL

const agentDeliveryRetentionCandidateSQL = `
		WITH source AS MATERIALIZED (
			SELECT i.stream_id, i.sequence, i.session_id, i.incarnation_id,
				i.harness_generation, i.submission_id, i.event_type,
				i.terminal, i.received_at, i.projected_at, i.content_hash, i.hash_version,
				i.payload_pruned_at, i.payload_bytes, LENGTH(i.payload) AS payload_length,
				c.session_id AS cursor_session_id, c.incarnation_id AS cursor_incarnation_id,
				c.harness_generation AS cursor_harness_generation,
				c.projected_sequence, c.retired_sequence,
				(SELECT MIN(terminal.sequence) FROM agent_delivery_inbox terminal
				 WHERE terminal.stream_id = i.stream_id AND terminal.terminal <> 0
				   AND terminal.sequence > COALESCE(c.retired_sequence, 0)
				   AND terminal.sequence <= i.sequence
				   AND /* unsettled terminal */) AS first_terminal_sequence,
				CASE WHEN i.terminal <> 0 THEN (SELECT settled.fully_settled_at FROM agent_delivery_submissions settled WHERE settled.id = i.submission_id)
				     ELSE i.projected_at END AS eligible_at,
				CASE WHEN (
					SELECT 1 FROM agent_delivery_effects effect
					WHERE effect.stream_id = i.stream_id AND effect.sequence = i.sequence
					  AND effect.effect_key = 'agent_delivery.event:' || i.stream_id || ':' || CAST(i.sequence AS TEXT)
					  AND effect.effect_type = 'agent_delivery.event'
					  AND effect.state = 'completed' AND effect.completed_at IS NOT NULL
					LIMIT 1
				) IS NOT NULL THEN 1 ELSE 0 END AS generic_effect_complete,
				CASE WHEN (
					SELECT 1 FROM agent_delivery_effects effect
					WHERE effect.stream_id = i.stream_id AND effect.sequence = i.sequence
					  AND (effect.state <> 'completed' OR effect.completed_at IS NULL)
					LIMIT 1
				) IS NOT NULL THEN 1 ELSE 0 END AS has_pending_effect,
				CASE WHEN (
					SELECT 1 FROM agent_delivery_effects effect
					WHERE effect.stream_id = i.stream_id AND effect.sequence = i.sequence
					  AND effect.effect_type NOT IN (
						'agent_delivery.event', 'agent_delivery.terminal_settlement',
						'workflow.on_turn_complete'
					  )
					LIMIT 1
				) IS NOT NULL THEN 1 ELSE 0 END AS has_unknown_effect
			FROM agent_delivery_inbox i
			LEFT JOIN agent_delivery_cursors c ON c.stream_id = i.stream_id
			WHERE 1 = 1 /* source filters */
			ORDER BY i.stream_id, i.sequence
			LIMIT ?
		), eligible AS MATERIALIZED (
			SELECT source.*,
				CASE WHEN cursor_session_id = session_id
					AND cursor_incarnation_id = incarnation_id
					AND cursor_harness_generation = harness_generation
					AND sequence <= projected_sequence
					AND projected_at IS NOT NULL AND eligible_at <= ?
					AND event_type IN (` + agentDeliveryRetentionNonterminalTypesSQL + `, 'complete', 'error', 'cancelled', 'canceled')
					AND ((terminal = 0 AND event_type IN (` + agentDeliveryRetentionNonterminalTypesSQL + `))
						OR (/* settled event */ AND (first_terminal_sequence IS NULL OR sequence < first_terminal_sequence)))
					AND generic_effect_complete = 1 AND has_pending_effect = 0
					AND has_unknown_effect = 0
					AND payload_pruned_at IS NULL THEN 1 ELSE 0 END AS can_prune,
				CASE WHEN cursor_session_id = session_id
					AND cursor_incarnation_id = incarnation_id
					AND cursor_harness_generation = harness_generation
					AND sequence <= projected_sequence
					AND projected_at IS NOT NULL
					AND event_type IN (` + agentDeliveryRetentionNonterminalTypesSQL + `, 'complete', 'error', 'cancelled', 'canceled')
					AND /* settled event */
					AND (first_terminal_sequence IS NULL OR sequence < first_terminal_sequence)
					AND generic_effect_complete = 1 AND has_pending_effect = 0
					AND has_unknown_effect = 0 THEN 1 ELSE 0 END AS can_retire
			FROM source
		), budgeted AS (
			SELECT eligible.*,
				COALESCE(SUM(CASE WHEN can_prune = 1 AND payload_length <= ?
					THEN payload_length ELSE 0 END) OVER (
					ORDER BY stream_id, sequence ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
				), 0) AS cumulative_payload_bytes
			FROM eligible
		)
		SELECT budgeted.stream_id, budgeted.sequence, budgeted.session_id, budgeted.incarnation_id,
			budgeted.harness_generation, budgeted.submission_id, budgeted.event_type,
			budgeted.terminal, budgeted.received_at, budgeted.projected_at, budgeted.content_hash,
			budgeted.hash_version, budgeted.payload_pruned_at, budgeted.payload_bytes,
			budgeted.payload_length, budgeted.generic_effect_complete, budgeted.has_pending_effect,
			budgeted.has_unknown_effect,
			budgeted.can_prune, budgeted.can_retire, budgeted.cursor_session_id,
			budgeted.cursor_incarnation_id, budgeted.cursor_harness_generation,
			budgeted.projected_sequence, budgeted.retired_sequence, budgeted.first_terminal_sequence,
			CASE WHEN ? AND budgeted.can_prune = 1 AND budgeted.payload_length <= ?
				AND budgeted.cumulative_payload_bytes <= ? THEN i.payload ELSE NULL END AS candidate_payload,
			CASE WHEN ? AND budgeted.can_prune = 1 AND budgeted.payload_length <= ?
				AND budgeted.cumulative_payload_bytes <= ? THEN 1 ELSE 0 END AS payload_loaded
		FROM budgeted
		JOIN agent_delivery_inbox i ON i.stream_id = budgeted.stream_id AND i.sequence = budgeted.sequence
		ORDER BY budgeted.stream_id, budgeted.sequence`

type retentionCandidateScanner interface {
	Scan(dest ...any) error
}

func scanAgentDeliveryRetentionCandidate(row retentionCandidateScanner) (agentDeliveryRetentionCandidate, error) {
	var candidate agentDeliveryRetentionCandidate
	var terminal int
	var genericComplete, pending, unknownEffect, canPrune, canRetire, payloadLoaded int
	var cursorSessionID, cursorIncarnationID sql.NullString
	var cursorHarness, projectedSequence, retiredSequence sql.NullInt64
	var payload []byte
	err := row.Scan(
		&candidate.event.StreamID, &candidate.event.Sequence, &candidate.event.SessionID,
		&candidate.event.IncarnationID, &candidate.event.HarnessGeneration,
		&candidate.event.SubmissionID, &candidate.event.EventType, &terminal,
		&candidate.event.ReceivedAt, &candidate.projectedAt, &candidate.event.ContentHash,
		&candidate.event.HashVersion, &candidate.payloadPrunedAt, &candidate.event.PayloadBytes,
		&candidate.payloadLength, &genericComplete, &pending, &unknownEffect, &canPrune, &canRetire,
		&cursorSessionID, &cursorIncarnationID, &cursorHarness, &projectedSequence,
		&retiredSequence, &candidate.firstTerminalSequence, &payload, &payloadLoaded,
	)
	if err != nil {
		return candidate, err
	}
	candidate.event.Terminal = terminal != 0
	candidate.event.ProjectedAt = nullTimePointer(candidate.projectedAt)
	candidate.event.PayloadPrunedAt = nullTimePointer(candidate.payloadPrunedAt)
	candidate.genericEffectComplete = genericComplete != 0
	candidate.hasPendingEffect = pending != 0
	candidate.hasUnknownEffect = unknownEffect != 0
	candidate.canPrune = canPrune != 0
	candidate.canRetire = canRetire != 0
	candidate.cursorExists = cursorSessionID.Valid && cursorIncarnationID.Valid && cursorHarness.Valid
	candidate.cursorIdentityValid = candidate.cursorExists &&
		cursorSessionID.String == candidate.event.SessionID &&
		cursorIncarnationID.String == candidate.event.IncarnationID &&
		cursorHarness.Int64 == candidate.event.HarnessGeneration
	if projectedSequence.Valid {
		candidate.projectedSequence = projectedSequence.Int64
	}
	if retiredSequence.Valid {
		candidate.retiredSequence = retiredSequence.Int64
	}
	candidate.payloadLoaded = payloadLoaded != 0
	if candidate.payloadLoaded {
		candidate.event.Payload = payload
	}
	return candidate, nil
}
