package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

const (
	agentDeliveryRetentionPhaseCompaction = "compaction"
	agentDeliveryRetentionPhaseReceipts   = "receipts"
)

func (r *Repository) selectAgentDeliveryEventRetentionPages(ctx context.Context, tx *sqlx.Tx, progress agentDeliveryRetentionProgress, limit int, cutoff time.Time) ([]agentDeliveryRetentionCandidate, bool, error) {
	if limit <= 0 {
		return nil, false, nil
	}
	compactionFirst := progress.eventSweepPhase != agentDeliveryRetentionPhaseReceipts
	firstLimit := (limit + 1) / 2
	if compactionFirst {
		firstLimit = limit
	}
	var compact, receipts []agentDeliveryRetentionCandidate
	var wrapped bool
	var err error
	if compactionFirst {
		compact, err = r.selectAgentDeliveryCompactionCandidates(ctx, tx, progress, firstLimit, cutoff)
		if err == nil {
			receipts, wrapped, err = r.selectAgentDeliveryReceiptCandidates(ctx, tx, progress, limit-len(compact))
		}
	} else {
		receipts, wrapped, err = r.selectAgentDeliveryReceiptCandidates(ctx, tx, progress, firstLimit)
		if err == nil {
			compact, err = r.selectAgentDeliveryCompactionCandidates(ctx, tx, progress, limit-len(receipts), cutoff)
		}
	}
	if err != nil {
		return nil, false, err
	}
	// Both discovery indexes contain disjoint rows; the combined proof page is bounded.
	compact = append(compact, receipts...)
	combined := compact
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].event.StreamID < combined[j].event.StreamID || (combined[i].event.StreamID == combined[j].event.StreamID && combined[i].event.Sequence < combined[j].event.Sequence)
	})
	return combined, wrapped, nil
}

func agentDeliveryCompactionCandidateQuery(progress agentDeliveryRetentionProgress, limit int, cutoff time.Time) (string, []any) {
	filters := []string{"i.payload_pruned_at IS NULL", "i.projected_at IS NOT NULL", "i.projected_at <= ?"}
	args := []any{cutoff}
	if progress.lastCompactionAt.Valid {
		filters = append(filters, "(i.projected_at,i.stream_id,i.sequence) > (?,?,?)")
		args = append(args, progress.lastCompactionAt.Time, progress.lastCompactionStreamID, progress.lastCompactionSequence)
	}
	return agentDeliveryDiscoveryQuery(filters, args, limit, "i.projected_at,i.stream_id,i.sequence")
}

func (r *Repository) selectAgentDeliveryCompactionCandidates(ctx context.Context, tx *sqlx.Tx, progress agentDeliveryRetentionProgress, limit int, cutoff time.Time) ([]agentDeliveryRetentionCandidate, error) {
	if limit <= 0 {
		return nil, nil
	}
	query, args := agentDeliveryCompactionCandidateQuery(progress, limit, cutoff)
	candidates, err := r.scanAgentDeliveryDiscoveryCandidates(ctx, tx, query, args)
	if err == nil && len(candidates) == 0 && progress.lastCompactionAt.Valid {
		query, args = agentDeliveryCompactionCandidateQuery(agentDeliveryRetentionProgress{}, limit, cutoff)
		candidates, err = r.scanAgentDeliveryDiscoveryCandidates(ctx, tx, query, args)
	}
	for index := range candidates {
		candidates[index].compactionSource = true
	}
	return candidates, err
}

func agentDeliveryReceiptCandidateQuery(lastStreamID string, lastSequence int64, limit int) (string, []any) {
	filters := []string{"i.payload_pruned_at IS NOT NULL"}
	var args []any
	if lastStreamID != "" {
		filters = append(filters, "(i.stream_id,i.sequence) > (?,?)")
		args = append(args, lastStreamID, lastSequence)
	}
	return agentDeliveryDiscoveryQuery(filters, args, limit, "i.stream_id,i.sequence")
}

func (r *Repository) selectAgentDeliveryReceiptCandidates(ctx context.Context, tx *sqlx.Tx, progress agentDeliveryRetentionProgress, limit int) ([]agentDeliveryRetentionCandidate, bool, error) {
	if limit <= 0 {
		return nil, false, nil
	}
	query, args := agentDeliveryReceiptCandidateQuery(progress.lastStreamID, progress.lastSequence, limit)
	candidates, err := r.scanAgentDeliveryDiscoveryCandidates(ctx, tx, query, args)
	if err != nil || len(candidates) > 0 || progress.lastStreamID == "" {
		return candidates, false, err
	}
	query, args = agentDeliveryReceiptCandidateQuery("", 0, limit)
	candidates, err = r.scanAgentDeliveryDiscoveryCandidates(ctx, tx, query, args)
	return candidates, true, err
}

func agentDeliveryDiscoveryQuery(filters []string, args []any, limit int, order string) (string, []any) {
	query := `SELECT i.stream_id,i.sequence,i.session_id,i.projected_at
	 FROM agent_delivery_inbox i WHERE ` + strings.Join(filters, " AND ") + ` ORDER BY ` + order + ` LIMIT ?`
	return query, append(args, limit)
}

func (r *Repository) scanAgentDeliveryDiscoveryCandidates(ctx context.Context, tx *sqlx.Tx, query string, args []any) ([]agentDeliveryRetentionCandidate, error) {
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("scan agent delivery discovery identities: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var candidates []agentDeliveryRetentionCandidate
	for rows.Next() {
		var candidate agentDeliveryRetentionCandidate
		if err := rows.Scan(&candidate.event.StreamID, &candidate.event.Sequence, &candidate.event.SessionID, &candidate.projectedAt); err != nil {
			return nil, fmt.Errorf("scan agent delivery discovery identity: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent delivery discovery identities: %w", err)
	}
	return candidates, nil
}

func (r *Repository) reloadAgentDeliverySelectedCandidates(ctx context.Context, tx *sqlx.Tx, selected []agentDeliveryRetentionCandidate, byteLimit int64, cutoff time.Time) ([]agentDeliveryRetentionCandidate, error) {
	if len(selected) == 0 {
		return nil, nil
	}
	if len(selected) > agentDeliveryRetentionCandidateLimit {
		return nil, fmt.Errorf("agent delivery locked reload exceeds candidate limit")
	}
	sources := make(map[string]map[int64]bool)
	for _, candidate := range selected {
		if sources[candidate.event.StreamID] == nil {
			sources[candidate.event.StreamID] = make(map[int64]bool)
		}
		sources[candidate.event.StreamID][candidate.event.Sequence] = candidate.compactionSource
	}
	_, args := agentDeliverySelectedCandidateQuery(selected, byteLimit, cutoff)
	prepared, err := r.agentDeliveryReloadStatement()
	if err != nil {
		return nil, err
	}
	statement := tx.StmtxContext(ctx, prepared)
	defer func() { _ = statement.Close() }()
	rows, err := statement.QueryxContext(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("scan agent delivery retention candidates: %w", err)
	}
	candidates, err := scanAgentDeliveryRetentionCandidates(rows)
	for index := range candidates {
		candidates[index].compactionSource = sources[candidates[index].event.StreamID][candidates[index].event.Sequence]
	}
	return candidates, err
}

var agentDeliveryLockedReloadQuery = func() string {
	values := strings.TrimSuffix(strings.Repeat("(?,?),", agentDeliveryRetentionCandidateLimit), ",")
	query, _ := agentDeliveryRetentionFilteredQuery([]string{"(i.stream_id,i.sequence) IN (" + values + ")"}, nil, 0, 0, time.Time{}, true)
	return query
}()

func agentDeliverySelectedCandidateQuery(selected []agentDeliveryRetentionCandidate, byteLimit int64, cutoff time.Time) (string, []any) {
	args := make([]any, 0, agentDeliveryRetentionCandidateLimit*2+9)
	for _, candidate := range selected {
		args = append(args, candidate.event.StreamID, candidate.event.Sequence)
	}
	for slot := len(selected); slot < agentDeliveryRetentionCandidateLimit; slot++ {
		args = append(args, nil, nil)
	}
	args = append(args, len(selected), cutoff, byteLimit, true, byteLimit, byteLimit, true, byteLimit, byteLimit)
	return agentDeliveryLockedReloadQuery, args
}

func advanceAgentDeliveryEventRetentionProgress(progress *agentDeliveryRetentionProgress, candidates []agentDeliveryRetentionCandidate, receiptWrapped bool) {
	var latestCompaction *agentDeliveryRetentionCandidate
	receiptSeen := false
	for index := range candidates {
		candidate := &candidates[index]
		if !candidate.compactionSource {
			progress.lastStreamID = candidate.event.StreamID
			progress.lastSequence = candidate.event.Sequence
			receiptSeen = true
			continue
		}
		if latestCompaction == nil || compactionCandidateAfter(candidate, latestCompaction) {
			latestCompaction = candidate
		}
	}
	if latestCompaction != nil {
		progress.lastCompactionAt = sql.NullTime{Time: latestCompaction.projectedAt.Time, Valid: true}
		progress.lastCompactionStreamID = latestCompaction.event.StreamID
		progress.lastCompactionSequence = latestCompaction.event.Sequence
	}
	if !receiptSeen && receiptWrapped {
		progress.lastStreamID = ""
		progress.lastSequence = 0
	}
	if progress.eventSweepPhase == agentDeliveryRetentionPhaseReceipts {
		progress.eventSweepPhase = agentDeliveryRetentionPhaseCompaction
	} else {
		progress.eventSweepPhase = agentDeliveryRetentionPhaseReceipts
	}
}

func compactionCandidateAfter(candidate, previous *agentDeliveryRetentionCandidate) bool {
	if !candidate.projectedAt.Time.Equal(previous.projectedAt.Time) {
		return candidate.projectedAt.Time.After(previous.projectedAt.Time)
	}
	return candidate.event.StreamID > previous.event.StreamID || (candidate.event.StreamID == previous.event.StreamID && candidate.event.Sequence > previous.event.Sequence)
}

// Only increasing persisted frontiers justify continuing a protected-only phase.
// Wrapping or changing sweep preference alone cannot keep a pass repeating.
func agentDeliveryDiscoveryAdvanced(previous agentDeliveryRetentionProgress, candidates []agentDeliveryRetentionCandidate, prompts []agentDeliveryPromptRetentionCandidate, receiptWrapped bool) bool {
	next := previous
	advanceAgentDeliveryEventRetentionProgress(&next, candidates, receiptWrapped)
	if next.lastStreamID > previous.lastStreamID || (next.lastStreamID == previous.lastStreamID && next.lastSequence > previous.lastSequence) {
		return true
	}
	if len(prompts) > 0 && prompts[len(prompts)-1].submissionID > previous.lastSubmissionID {
		return true
	}
	if !next.lastCompactionAt.Valid {
		return false
	}
	if !previous.lastCompactionAt.Valid {
		return true
	}
	return next.lastCompactionAt.Time.After(previous.lastCompactionAt.Time) ||
		(next.lastCompactionAt.Time.Equal(previous.lastCompactionAt.Time) &&
			(next.lastCompactionStreamID > previous.lastCompactionStreamID ||
				(next.lastCompactionStreamID == previous.lastCompactionStreamID && next.lastCompactionSequence > previous.lastCompactionSequence)))
}
