package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

// AgentDeliveryRetentionPreview is a bounded, read-only sample of work that
// the live retention batches could perform. It contains aggregate values only.
type AgentDeliveryRetentionPreview struct {
	Available                   bool
	UnavailableReason           string
	EventPayloadCandidates      int
	EventPayloadBytes           int64
	SettledPromptCandidates     int
	SettledPromptBytes          int64
	EventReceiptCandidates      int
	EventReceiptBytes           int64
	RetiredSessionEvidenceRows  int
	RetiredSessionEvidenceBytes int64
	OrphanedIdentities          int
	SampleLimitReached          bool
}

// AnalyzeAgentDeliveryRetention previews the current bounded cleanup page
// without initializing, migrating, or mutating the retention schema.
func (r *Repository) AnalyzeAgentDeliveryRetention(
	ctx context.Context,
	options models.AgentDeliveryRetentionBatchOptions,
) (AgentDeliveryRetentionPreview, error) {
	var preview AgentDeliveryRetentionPreview
	if err := r.CheckAgentDeliveryRetentionReady(ctx); err != nil {
		if ctx.Err() != nil {
			return preview, ctx.Err()
		}
		preview.UnavailableReason = retentionPreviewUnavailableReason(err)
		return preview, nil
	}
	options = normalizeAgentDeliveryRetentionOptions(options)
	if err := r.prepareAgentDeliveryReloadStatement(ctx); err != nil {
		return preview, err
	}
	now := r.nowUTC()
	cutoff := now.Add(-options.PayloadGrace)
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return preview, err
	}
	defer func() { _ = tx.Rollback() }()
	progress, err := loadAgentDeliveryRetentionProgress(ctx, tx, r.db.Rebind)
	if err != nil {
		return preview, err
	}
	selectedEvents, selectedPrompts, _, _, _, err := r.selectAgentDeliveryRetentionBatch(
		ctx, tx, progress, options, cutoff,
	)
	if err != nil {
		return preview, err
	}
	events, err := r.reloadAgentDeliverySelectedCandidates(
		ctx, tx, selectedEvents, options.MaxPayloadBytes, cutoff,
	)
	if err != nil {
		return preview, err
	}
	prompts, err := r.reloadAgentDeliveryPromptRetentionCandidates(ctx, tx, selectedPrompts, cutoff)
	if err != nil {
		return preview, err
	}
	preview.Available = true
	preview.SampleLimitReached = len(events)+len(prompts) >= options.MaxCandidates
	eventBytesRead := previewEventPayloads(events, now, options.MaxPayloadBytes, &preview)
	previewPromptPayloads(prompts, eventBytesRead, options.MaxPayloadBytes, &preview)
	preview.EventReceiptCandidates, preview.EventReceiptBytes = previewRetirableReceipts(
		events, now, options.ReplayAge, options.ReplayEvents,
	)
	var retiredSampleLimited bool
	preview.RetiredSessionEvidenceRows, preview.RetiredSessionEvidenceBytes, retiredSampleLimited, err = previewRetiredSessionEvidence(
		ctx, tx, r.db.Rebind, options.MaxCandidates,
	)
	if err != nil {
		return preview, err
	}
	orphans, err := r.agentDeliveryOrphanCandidatesTx(ctx, tx, options.MaxCandidates)
	if err != nil {
		return preview, err
	}
	preview.SampleLimitReached = preview.SampleLimitReached || retiredSampleLimited
	preview.OrphanedIdentities = len(orphans)
	if len(orphans) >= options.MaxCandidates {
		preview.SampleLimitReached = true
	}
	if err := tx.Commit(); err != nil {
		return AgentDeliveryRetentionPreview{}, err
	}
	return preview, nil
}

func previewEventPayloads(events []agentDeliveryRetentionCandidate, now time.Time, byteLimit int64, preview *AgentDeliveryRetentionPreview) int64 {
	var eventBytesRead int64
	for index := range events {
		candidate := &events[index]
		if candidate.payloadLoaded {
			eventBytesRead += int64(len(candidate.event.Payload))
		}
		if hash, eligible := previewEventPayloadHash(candidate, byteLimit); eligible {
			preview.EventPayloadCandidates++
			preview.EventPayloadBytes += candidate.payloadLength
			candidate.payloadPrunedAt = sql.NullTime{Time: now, Valid: true}
			candidate.event.HashVersion = agentDeliveryPayloadHashVersion
			candidate.event.ContentHash = hash
			candidate.event.PayloadBytes = candidate.payloadLength
		}
	}
	return eventBytesRead
}

func previewPromptPayloads(prompts []agentDeliveryPromptRetentionCandidate, eventBytesRead, byteLimit int64, preview *AgentDeliveryRetentionPreview) {
	usedBytes := eventBytesRead
	for _, candidate := range prompts {
		if !candidate.canPrune || candidate.payloadLength <= 0 ||
			(candidate.payloadBytes != 0 && candidate.payloadBytes != candidate.payloadLength) ||
			candidate.payloadLength > byteLimit-usedBytes {
			continue
		}
		preview.SettledPromptCandidates++
		preview.SettledPromptBytes += candidate.payloadLength
		usedBytes += candidate.payloadLength
	}
}

func previewEventPayloadHash(candidate *agentDeliveryRetentionCandidate, byteLimit int64) (string, bool) {
	if candidate.payloadPrunedAt.Valid || !candidate.cursorIdentityValid || !candidate.canPrune ||
		!candidate.payloadLoaded || candidate.payloadLength <= 0 || candidate.payloadLength > byteLimit {
		return "", false
	}
	hash, eligible, err := agentDeliveryCompactionHash(candidate, byteLimit)
	return hash, eligible && err == nil
}

func previewRetirableReceipts(
	events []agentDeliveryRetentionCandidate,
	now time.Time,
	replayAge time.Duration,
	replayEvents int,
) (int, int64) {
	count := 0
	var bytes int64
	cutoff := now.Add(-replayAge)
	for _, group := range retentionCandidateStreamGroups(events) {
		if len(group) == 0 || !group[0].cursorIdentityValid {
			continue
		}
		retired := group[0].retiredSequence
		for _, candidate := range group {
			if !agentDeliveryRetentionCandidateReady(candidate, retired+1, group[0].projectedSequence, cutoff, replayEvents) {
				break
			}
			count++
			bytes += int64(160 + len(candidate.event.StreamID) + len(candidate.event.SessionID) +
				len(candidate.event.IncarnationID) + len(candidate.event.ContentHash) + len(candidate.event.EventType))
			retired = candidate.event.Sequence
		}
	}
	return count, bytes
}

func previewRetiredSessionEvidence(
	ctx context.Context,
	tx *sqlx.Tx,
	rebind func(string) string,
	limit int,
) (int, int64, bool, error) {
	rows, err := tx.QueryxContext(ctx, rebind(`
		SELECT job.session_id FROM agent_delivery_cleanup_jobs job
		WHERE job.phase IN ('pending', 'inbox', 'effects', 'submissions', 'cursors')
		  AND NOT EXISTS (SELECT 1 FROM task_sessions session WHERE session.id = job.session_id)
		ORDER BY job.session_id, job.incarnation_id LIMIT ?`), limit)
	if err != nil {
		return 0, 0, false, err
	}
	defer func() { _ = rows.Close() }()
	var sessionIDs []string
	seen := make(map[string]struct{})
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			return 0, 0, false, err
		}
		if _, ok := seen[sessionID]; ok {
			continue
		}
		seen[sessionID] = struct{}{}
		sessionIDs = append(sessionIDs, sessionID)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, false, err
	}
	var rowCount int
	var byteCount int64
	for _, sessionID := range sessionIDs {
		for index, query := range []string{
			`SELECT COUNT(*), COALESCE(SUM(bytes), 0) FROM (SELECT LENGTH(stream_id)+LENGTH(incarnation_id)+LENGTH(content_hash)+LENGTH(payload)+128 AS bytes FROM agent_delivery_inbox WHERE session_id = ? LIMIT ?) sample`,
			`SELECT COUNT(*), COALESCE(SUM(bytes), 0) FROM (SELECT bytes FROM (SELECT LENGTH(effect_key)+LENGTH(effect_type)+LENGTH(outcome)+128 AS bytes FROM agent_delivery_effects WHERE session_id = ? AND session_id <> '' LIMIT ?) owned UNION ALL SELECT bytes FROM (SELECT LENGTH(effect_key)+LENGTH(effect_type)+LENGTH(outcome)+128 AS bytes FROM agent_delivery_cursors c JOIN agent_delivery_effects e ON e.stream_id = c.stream_id WHERE c.session_id = ? AND e.session_id <> ? LIMIT ?) attributed LIMIT ?) sample`,
			`SELECT COUNT(*), COALESCE(SUM(bytes), 0) FROM (SELECT LENGTH(id)+LENGTH(payload_hash)+LENGTH(payload)+LENGTH(outcome)+128 AS bytes FROM agent_delivery_submissions WHERE session_id = ? LIMIT ?) sample`,
			`SELECT COUNT(*), COALESCE(SUM(bytes), 0) FROM (SELECT LENGTH(stream_id)+LENGTH(incarnation_id)+128 AS bytes FROM agent_delivery_cursors WHERE session_id = ? LIMIT ?) sample`,
		} {
			if rowCount >= limit {
				return rowCount, byteCount, true, nil
			}
			var rows int
			var bytes int64
			remaining := limit - rowCount
			args := []any{sessionID, remaining}
			if index == 1 {
				args = []any{sessionID, remaining, sessionID, sessionID, remaining, remaining}
			}
			if err := tx.QueryRowxContext(ctx, rebind(query), args...).Scan(&rows, &bytes); err != nil {
				return 0, 0, false, err
			}
			rowCount += rows
			byteCount += bytes
		}
	}
	return rowCount, byteCount, rowCount >= limit || len(sessionIDs) >= limit, nil
}

func retentionPreviewUnavailableReason(err error) string {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unsupported agent delivery retention format") {
		return "format_unsupported"
	}
	return "schema_unavailable"
}
