package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (r *Repository) retireAgentDeliveryPrefixes(
	ctx context.Context,
	tx *sqlx.Tx,
	candidates []agentDeliveryRetentionCandidate,
	now time.Time,
	replayAge time.Duration,
	replayEvents int,
	result *models.AgentDeliveryRetentionBatchResult,
) error {
	groups := retentionCandidateStreamGroups(candidates)
	ageCutoff := now.Add(-replayAge)
	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(group) == 0 || !group[0].cursorIdentityValid {
			continue
		}
		if err := r.retireAgentDeliveryStreamPrefix(ctx, tx, group, now, ageCutoff, replayEvents, result); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) retireAgentDeliveryStreamPrefix(
	ctx context.Context,
	tx *sqlx.Tx,
	group []agentDeliveryRetentionCandidate,
	now, ageCutoff time.Time,
	replayEvents int,
	result *models.AgentDeliveryRetentionBatchResult,
) error {
	retiredSequence := group[0].retiredSequence
	nextRetired := retiredSequence
	for _, candidate := range group {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !agentDeliveryRetentionCandidateReady(candidate, nextRetired+1, group[0].projectedSequence, ageCutoff, replayEvents) {
			break
		}
		nextRetired = candidate.event.Sequence
	}
	if nextRetired == retiredSequence {
		return nil
	}
	if err := r.deleteAgentDeliveryPrefixReceipts(ctx, tx, group, retiredSequence+1, nextRetired, result); err != nil {
		return err
	}
	if err := r.failAgentDeliveryRetention("before_retired_cursor"); err != nil {
		return err
	}
	updated, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_cursors SET retired_sequence = ?, updated_at = ?
		WHERE stream_id = ? AND retired_sequence = ?`),
		nextRetired, now, group[0].event.StreamID, retiredSequence)
	if err != nil {
		return fmt.Errorf("advance retired delivery cursor: %w", err)
	}
	rows, err := updated.RowsAffected()
	if err != nil || rows != 1 {
		if err == nil {
			err = errors.New("retired delivery cursor changed during retention")
		}
		return err
	}
	return r.failAgentDeliveryRetention("after_retired_cursor")
}

func agentDeliveryRetentionCandidateReady(
	candidate agentDeliveryRetentionCandidate,
	expectedSequence, projectedSequence int64,
	ageCutoff time.Time,
	replayEvents int,
) bool {
	return candidate.event.Sequence == expectedSequence && candidate.event.Sequence <= projectedSequence &&
		candidate.canRetire && candidate.payloadPrunedAt.Valid &&
		candidate.event.HashVersion == agentDeliveryPayloadHashVersion && candidate.event.ContentHash != "" &&
		candidate.event.PayloadBytes >= 0 && candidate.genericEffectComplete && !candidate.hasPendingEffect &&
		agentDeliveryRetentionSequenceExpired(
			candidate.event.Sequence, expectedSequence, projectedSequence,
			candidate.firstTerminalSequence, candidate.projectedAt, ageCutoff, replayEvents,
		)
}

func agentDeliveryRetentionSequenceExpired(
	sequence, expectedSequence, projectedSequence int64,
	firstTerminalSequence sql.NullInt64,
	projectedAt sql.NullTime,
	ageCutoff time.Time,
	replayEvents int,
) bool {
	if sequence != expectedSequence || sequence > projectedSequence ||
		(firstTerminalSequence.Valid && sequence >= firstTerminalSequence.Int64) {
		return false
	}
	ageExpired := projectedAt.Valid && !projectedAt.Time.After(ageCutoff)
	countExpired := sequence <= projectedSequence-int64(replayEvents)
	return ageExpired || countExpired
}

func retentionCandidateStreamGroups(candidates []agentDeliveryRetentionCandidate) [][]agentDeliveryRetentionCandidate {
	var groups [][]agentDeliveryRetentionCandidate
	for _, candidate := range candidates {
		if len(groups) == 0 || groups[len(groups)-1][0].event.StreamID != candidate.event.StreamID {
			groups = append(groups, []agentDeliveryRetentionCandidate{candidate})
			continue
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], candidate)
	}
	return groups
}

func (r *Repository) deleteAgentDeliveryPrefixReceipts(
	ctx context.Context,
	tx *sqlx.Tx,
	group []agentDeliveryRetentionCandidate,
	first, last int64,
	result *models.AgentDeliveryRetentionBatchResult,
) error {
	streamID := group[0].event.StreamID
	if err := r.failAgentDeliveryRetention("before_generic_effect"); err != nil {
		return err
	}
	deleted, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM agent_delivery_effects
		WHERE stream_id = ? AND sequence BETWEEN ? AND ?
		  AND effect_key = 'agent_delivery.event:' || stream_id || ':' || CAST(sequence AS TEXT)
		  AND effect_type = 'agent_delivery.event'
		  AND state = 'completed' AND completed_at IS NOT NULL`), streamID, first, last)
	if err != nil {
		return fmt.Errorf("delete completed agent delivery event effects: %w", err)
	}
	expected := last - first + 1
	if err := requireAgentDeliveryRetentionRows(deleted, expected); err != nil {
		return err
	}
	if err := r.failAgentDeliveryRetention("after_generic_effect"); err != nil {
		return err
	}
	// Only the exact raw keys validated as a contiguous expired prefix can
	// bypass persistent compaction. Other receipts retain compact-hash guards.
	query, args := agentDeliveryPrefixReceiptDelete(group, streamID, first, last)
	removed, err := tx.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return fmt.Errorf("delete retired agent delivery events: %w", err)
	}
	if err := requireAgentDeliveryRetentionRows(removed, expected); err != nil {
		return err
	}
	result.EventRowsRetired += int(expected)
	return nil
}

func agentDeliveryPrefixReceiptDelete(group []agentDeliveryRetentionCandidate, streamID string, first, last int64) (string, []any) {
	args := []any{streamID, first, last, agentDeliveryPayloadHashVersion}
	query := `
		DELETE FROM agent_delivery_inbox
		WHERE stream_id = ? AND sequence BETWEEN ? AND ?
		  AND ((payload_pruned_at IS NOT NULL AND content_hash <> '' AND hash_version = ?)`
	var direct []string
	for _, candidate := range group {
		if candidate.directRetirement && candidate.event.Sequence >= first && candidate.event.Sequence <= last {
			direct = append(direct, "?")
			args = append(args, candidate.event.Sequence)
		}
	}
	if len(direct) > 0 {
		query += ` OR (payload_pruned_at IS NULL AND sequence IN (` + strings.Join(direct, ",") + `))`
	}
	return query + ")", args
}

func requireAgentDeliveryRetentionRows(result sql.Result, expected int64) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != expected {
		return repoerrors.ErrAgentDeliveryReceiptCorrupt
	}
	return nil
}

func (r *Repository) compactAgentDeliveryCandidates(
	ctx context.Context,
	tx *sqlx.Tx,
	candidates []agentDeliveryRetentionCandidate,
	now time.Time,
	byteLimit int64,
	replayAge time.Duration,
	replayEvents int,
	result *models.AgentDeliveryRetentionBatchResult,
) error {
	compact, err := agentDeliveryCandidatesForCompaction(ctx, candidates, byteLimit, result)
	if err != nil || len(compact) == 0 {
		return err
	}
	for _, candidate := range compact {
		candidate.directRetirement = true
	}
	selectAgentDeliveryDirectPrefixes(candidates, now.Add(-replayAge), replayEvents)
	var retained []*agentDeliveryRetentionCandidate
	for _, candidate := range compact {
		if !candidate.directRetirement {
			retained = append(retained, candidate)
		}
	}
	if err := r.writeAgentDeliveryPayloadCompaction(ctx, tx, retained, now); err != nil {
		return err
	}
	for _, candidate := range compact {
		candidate.event.HashVersion = agentDeliveryPayloadHashVersion
		candidate.event.PayloadBytes = int64(len(candidate.event.Payload))
		candidate.payloadPrunedAt = sql.NullTime{Time: now, Valid: true}
		candidate.event.PayloadPrunedAt = &candidate.payloadPrunedAt.Time
		candidate.event.Payload = []byte{}
		result.EventPayloadsPruned++
		result.PayloadBytesPruned += candidate.event.PayloadBytes
	}
	return nil
}

func selectAgentDeliveryDirectPrefixes(candidates []agentDeliveryRetentionCandidate, ageCutoff time.Time, replayEvents int) {
	var streamID string
	var nextRetired, projectedSequence int64
	for index := range candidates {
		candidate := &candidates[index]
		if candidate.event.StreamID != streamID {
			streamID = candidate.event.StreamID
			nextRetired, projectedSequence = candidate.retiredSequence, candidate.projectedSequence
		}
		ready := *candidate
		if candidate.directRetirement {
			ready.payloadPrunedAt.Valid = true
			ready.event.HashVersion = agentDeliveryPayloadHashVersion
			ready.event.PayloadBytes = int64(len(ready.event.Payload))
		}
		if !agentDeliveryRetentionCandidateReady(ready, nextRetired+1, projectedSequence, ageCutoff, replayEvents) {
			candidate.directRetirement = false
			continue
		}
		nextRetired = candidate.event.Sequence
	}
}

func agentDeliveryCandidatesForCompaction(
	ctx context.Context,
	candidates []agentDeliveryRetentionCandidate,
	byteLimit int64,
	result *models.AgentDeliveryRetentionBatchResult,
) ([]*agentDeliveryRetentionCandidate, error) {
	var compact []*agentDeliveryRetentionCandidate
	for index := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		candidate := &candidates[index]
		if candidate.payloadLoaded {
			result.PayloadBytesRead += int64(len(candidate.event.Payload))
		}
		hash, eligible, err := agentDeliveryCompactionHash(candidate, byteLimit)
		if err != nil {
			return nil, err
		}
		if !eligible {
			continue
		}
		candidate.event.ContentHash = hash
		compact = append(compact, candidate)
	}
	return compact, nil
}

func (r *Repository) writeAgentDeliveryPayloadCompaction(
	ctx context.Context,
	tx *sqlx.Tx,
	compact []*agentDeliveryRetentionCandidate,
	now time.Time,
) error {
	_, args := agentDeliveryCompactionValues(compact)
	if err := r.failAgentDeliveryRetention("before_hash"); err != nil {
		return err
	}
	if err := r.executeAgentDeliveryCompaction(ctx, tx, args, len(compact), false); err != nil {
		return err
	}
	if err := r.failAgentDeliveryRetention("after_hash"); err != nil {
		return err
	}
	if err := r.failAgentDeliveryRetention("before_payload"); err != nil {
		return err
	}
	args = append(args, []byte{}, now)
	if err := r.executeAgentDeliveryCompaction(ctx, tx, args, len(compact), true); err != nil {
		return err
	}
	if err := r.failAgentDeliveryRetention("after_payload"); err != nil {
		return err
	}
	return nil
}

func (r *Repository) executeAgentDeliveryCompaction(ctx context.Context, tx *sqlx.Tx, args []any, count int, payload bool) error {
	if count == 0 {
		return nil
	}
	hashes, payloads, err := r.agentDeliveryCompactionStatements()
	if err != nil {
		return err
	}
	statement, operation := hashes, "backfill agent delivery event hashes"
	if payload {
		statement, operation = payloads, "prune agent delivery event payloads"
	}
	bound := tx.StmtxContext(ctx, statement)
	defer func() { _ = bound.Close() }()
	updated, err := bound.ExecContext(ctx, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return requireAgentDeliveryRetentionRows(updated, int64(count))
}

func agentDeliveryCompactionHash(candidate *agentDeliveryRetentionCandidate, byteLimit int64) (string, bool, error) {
	if !candidate.cursorIdentityValid || !candidate.canPrune || !candidate.payloadLoaded ||
		candidate.payloadLength < 0 || candidate.payloadLength > byteLimit {
		return "", false, nil
	}
	var wire streams.AgentEvent
	if json.Unmarshal(candidate.event.Payload, &wire) != nil || wire.Type != candidate.event.EventType ||
		wire.Role == authorKindUser || !retentionEventPayloadSupported(wire.Type, candidate.event.Terminal) {
		return "", false, nil
	}
	mode, content := normalizeDecodedAgentDeliveryPayload(&wire, candidate.event.Payload)
	hash := agentDeliveryEventHashNormalized(&candidate.event, mode, content)
	err := validateAgentDeliveryRetentionHash(&candidate.event, hash)
	return hash, err == nil, err
}

func validateAgentDeliveryRetentionHash(event *models.AgentDeliveryEvent, wantHash string) error {
	actualBytes := int64(len(event.Payload))
	switch {
	case event.HashVersion == 0 && event.ContentHash == "":
	case event.HashVersion == agentDeliveryPayloadHashVersion && event.ContentHash == wantHash:
	case event.HashVersion != 0 && event.HashVersion != agentDeliveryPayloadHashVersion:
		return fmt.Errorf("agent delivery event %s/%d: %w", event.StreamID, event.Sequence, repoerrors.ErrAgentDeliveryHashVersion)
	default:
		return fmt.Errorf("agent delivery event %s/%d: %w", event.StreamID, event.Sequence, repoerrors.ErrAgentDeliveryReceiptCorrupt)
	}
	if event.PayloadBytes != 0 && event.PayloadBytes != actualBytes {
		return fmt.Errorf("agent delivery event %s/%d payload byte count: %w", event.StreamID, event.Sequence, repoerrors.ErrAgentDeliveryReceiptCorrupt)
	}
	return nil
}

var agentDeliveryCompactionRowValues = strings.TrimSuffix(strings.Repeat(
	"(CAST(? AS TEXT), CAST(? AS BIGINT), CAST(? AS TEXT), CAST(? AS BIGINT)),",
	agentDeliveryRetentionCandidateLimit,
), ",")

func agentDeliveryCompactionValues(candidates []*agentDeliveryRetentionCandidate) (string, []any) {
	args := make([]any, 0, agentDeliveryRetentionCandidateLimit*4+2)
	for _, candidate := range candidates {
		args = append(args, candidate.event.StreamID, candidate.event.Sequence, candidate.event.ContentHash, int64(len(candidate.event.Payload)))
	}
	for slot := len(candidates); slot < agentDeliveryRetentionCandidateLimit; slot++ {
		args = append(args, nil, nil, nil, nil)
	}
	return agentDeliveryCompactionRowValues, args
}

const agentDeliveryCompactHashesSQL = `
 WITH compaction(stream_id, sequence, content_hash, payload_bytes) AS (VALUES /* values */)
 UPDATE agent_delivery_inbox AS inbox
 SET content_hash=compaction.content_hash, hash_version=1, payload_bytes=compaction.payload_bytes
 FROM compaction
 WHERE (inbox.stream_id, inbox.sequence) IN (SELECT stream_id, sequence FROM compaction)
  AND inbox.stream_id=compaction.stream_id AND inbox.sequence=compaction.sequence
  AND inbox.payload_pruned_at IS NULL`

const agentDeliveryCompactPayloadsSQL = `
 WITH compaction(stream_id, sequence, content_hash, payload_bytes) AS (VALUES /* values */)
 UPDATE agent_delivery_inbox AS inbox
 SET payload=?, payload_pruned_at=?
 FROM compaction
 WHERE (inbox.stream_id, inbox.sequence) IN (SELECT stream_id, sequence FROM compaction)
  AND inbox.stream_id=compaction.stream_id AND inbox.sequence=compaction.sequence
  AND inbox.content_hash=compaction.content_hash AND inbox.hash_version=1
  AND inbox.payload_pruned_at IS NULL`

var agentDeliveryFixedCompactHashesSQL = fixedAgentDeliveryCompactionSQL(agentDeliveryCompactHashesSQL)
var agentDeliveryFixedCompactPayloadsSQL = fixedAgentDeliveryCompactionSQL(agentDeliveryCompactPayloadsSQL)

func fixedAgentDeliveryCompactionSQL(query string) string {
	values, _ := agentDeliveryCompactionValues(nil)
	return strings.Replace(query, "/* values */", values, 1)
}
