package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	agentDeliveryRetentionPhaseEvents       = "events"
	agentDeliveryRetentionPhasePrompts      = "prompts"
	agentDeliveryRetentionCandidateLimit    = 200
	agentDeliveryRetentionPayloadByteLimit  = 4 << 20
	agentDeliveryRetentionWorkTimeout       = 250 * time.Millisecond
	agentDeliveryRetentionPayloadGrace      = time.Hour
	agentDeliveryRetentionCompactEventLimit = 10_000
	agentDeliveryRetentionCompactMaxAge     = 24 * time.Hour
)

type agentDeliveryRetentionCandidate struct {
	event                 models.AgentDeliveryEvent
	compactionSource      bool
	directRetirement      bool
	projectedAt           sql.NullTime
	payloadPrunedAt       sql.NullTime
	payloadLength         int64
	payloadLoaded         bool
	genericEffectComplete bool
	hasPendingEffect      bool
	hasUnknownEffect      bool
	canPrune              bool
	canRetire             bool
	cursorExists          bool
	cursorIdentityValid   bool
	projectedSequence     int64
	retiredSequence       int64
	firstTerminalSequence sql.NullInt64
}

type agentDeliveryPromptRetentionCandidate struct {
	submissionID      string
	sessionID         string
	incarnationID     string
	harnessGeneration int64
	dispatchAttemptID string
	payloadHash       string
	payloadBytes      int64
	payloadLength     int64
	terminalStreamID  string
	terminalSequence  int64
	terminalTurnID    string
	outcome           string
	fullySettledAt    sql.NullTime
	canPrune          bool
}

// RunAgentDeliveryRetentionBatch compacts projected canonical output and
// retires only contiguous, replay-expired event prefixes. Every mutation and
// progress update commits atomically in one bounded writer transaction.
func (r *Repository) RunAgentDeliveryRetentionBatch(
	ctx context.Context,
	options models.AgentDeliveryRetentionBatchOptions,
) (models.AgentDeliveryRetentionBatchResult, error) {
	options = normalizeAgentDeliveryRetentionOptions(options)
	workCtx, cancel := context.WithTimeout(ctx, options.WorkTimeout)
	defer cancel()
	if err := workCtx.Err(); err != nil {
		return models.AgentDeliveryRetentionBatchResult{}, err
	}
	if err := r.prepareAgentDeliveryReloadStatement(workCtx); err != nil {
		return models.AgentDeliveryRetentionBatchResult{}, err
	}
	now := r.nowUTC()
	tx, err := r.db.BeginTxx(workCtx, nil)
	if err != nil {
		return models.AgentDeliveryRetentionBatchResult{}, fmt.Errorf("begin agent delivery retention batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(workCtx, `
		UPDATE agent_delivery_retention_state SET updated_at = updated_at WHERE id = 1`); err != nil {
		return models.AgentDeliveryRetentionBatchResult{}, fmt.Errorf("lock agent delivery retention state: %w", err)
	}
	progress, err := loadAgentDeliveryRetentionProgress(workCtx, tx, r.db.Rebind)
	if err != nil {
		return models.AgentDeliveryRetentionBatchResult{}, err
	}
	cutoff := now.Add(-options.PayloadGrace)
	selected, promptSelected, wrapped, promptWrapped, nextPhase, err := r.selectAgentDeliveryRetentionBatch(
		workCtx, tx, progress, options, cutoff,
	)
	if err != nil {
		return models.AgentDeliveryRetentionBatchResult{}, err
	}
	if err := workCtx.Err(); err != nil {
		return models.AgentDeliveryRetentionBatchResult{}, err
	}
	streamsToLock, sessionsToLock := retentionLockIdentities(selected)
	for _, candidate := range promptSelected {
		sessionsToLock = append(sessionsToLock, candidate.sessionID)
		streamsToLock = append(streamsToLock, candidate.terminalStreamID)
	}
	if err := lockAgentDeliveryRetentionWrites(workCtx, tx, r.db.DriverName(), sessionsToLock, streamsToLock, promptSelected); err != nil {
		return models.AgentDeliveryRetentionBatchResult{}, fmt.Errorf("lock delivery rows for retention: %w", err)
	}
	if err := workCtx.Err(); err != nil {
		return models.AgentDeliveryRetentionBatchResult{}, err
	}
	eventByteLimit := options.MaxPayloadBytes - promptRetentionPayloadBytes(promptSelected)
	if eventByteLimit < 0 {
		eventByteLimit = 0
	}
	candidates, err := r.reloadAgentDeliverySelectedCandidates(workCtx, tx, selected, eventByteLimit, cutoff)
	if err != nil {
		return models.AgentDeliveryRetentionBatchResult{}, err
	}
	prompts, err := r.reloadAgentDeliveryPromptRetentionCandidates(workCtx, tx, promptSelected, cutoff)
	if err != nil {
		return models.AgentDeliveryRetentionBatchResult{}, err
	}
	result := models.AgentDeliveryRetentionBatchResult{Candidates: len(candidates) + len(prompts)}
	result.EligibleCandidates, result.OldestEligibleAgeSeconds = retentionBacklogStats(candidates, prompts, now)
	if err := r.commitAgentDeliveryRetentionBatch(
		workCtx, tx, progress, candidates, prompts, now, wrapped, promptWrapped, nextPhase, options, &result,
	); err != nil {
		return models.AgentDeliveryRetentionBatchResult{}, err
	}
	result.DiscoveryAdvanced = agentDeliveryDiscoveryAdvanced(progress, candidates, prompts, wrapped)
	result.DiscoveryBefore = agentDeliveryDiscoveryVector(progress)
	next := nextAgentDeliveryRetentionProgress(progress, candidates, prompts, wrapped, promptWrapped)
	result.DiscoveryAfter = agentDeliveryDiscoveryVector(next)
	return result, nil
}

func (r *Repository) selectAgentDeliveryRetentionBatch(
	ctx context.Context,
	tx *sqlx.Tx,
	progress agentDeliveryRetentionProgress,
	options models.AgentDeliveryRetentionBatchOptions,
	cutoff time.Time,
) ([]agentDeliveryRetentionCandidate, []agentDeliveryPromptRetentionCandidate, bool, bool, string, error) {
	if progress.sweepPhase != agentDeliveryRetentionPhaseEvents {
		return r.selectAgentDeliveryRetentionPromptsFirst(ctx, tx, progress, options, cutoff)
	}
	events, eventWrapped, err := r.selectAgentDeliveryEventRetentionPages(
		ctx, tx, progress, options.MaxCandidates, cutoff,
	)
	if err != nil {
		return nil, nil, false, false, "", err
	}
	prompts, promptWrapped, err := r.selectAgentDeliveryPromptRetentionCandidates(
		ctx, tx, progress.lastSubmissionID, options.MaxCandidates-len(events), options.MaxPayloadBytes, cutoff,
	)
	if err != nil {
		return nil, nil, false, false, "", err
	}
	return events, prompts, eventWrapped, promptWrapped, agentDeliveryRetentionPhasePrompts, nil
}

func (r *Repository) selectAgentDeliveryRetentionPromptsFirst(
	ctx context.Context,
	tx *sqlx.Tx,
	progress agentDeliveryRetentionProgress,
	options models.AgentDeliveryRetentionBatchOptions,
	cutoff time.Time,
) ([]agentDeliveryRetentionCandidate, []agentDeliveryPromptRetentionCandidate, bool, bool, string, error) {
	prompts, promptWrapped, err := r.selectAgentDeliveryPromptRetentionCandidates(
		ctx, tx, progress.lastSubmissionID, options.MaxCandidates, options.MaxPayloadBytes, cutoff,
	)
	if err != nil {
		return nil, nil, false, false, "", err
	}
	events, eventWrapped, err := r.selectAgentDeliveryEventRetentionPages(
		ctx, tx, progress, options.MaxCandidates-len(prompts), cutoff,
	)
	if err != nil {
		return nil, nil, false, false, "", err
	}
	return events, prompts, eventWrapped, promptWrapped, agentDeliveryRetentionPhaseEvents, nil
}

func promptRetentionPayloadBytes(candidates []agentDeliveryPromptRetentionCandidate) int64 {
	var total int64
	for _, candidate := range candidates {
		if candidate.canPrune && candidate.payloadLength > 0 {
			total += candidate.payloadLength
		}
	}
	return total
}

func retentionBacklogStats(
	events []agentDeliveryRetentionCandidate,
	prompts []agentDeliveryPromptRetentionCandidate,
	now time.Time,
) (int, int64) {
	eligible := 0
	oldestAge := int64(-1)
	consider := func(at time.Time) {
		age := int64(now.Sub(at).Seconds())
		if age < 0 {
			age = 0
		}
		if oldestAge < 0 || age > oldestAge {
			oldestAge = age
		}
	}
	for _, candidate := range events {
		if candidate.canPrune && candidate.projectedAt.Valid {
			eligible++
			consider(candidate.projectedAt.Time)
		}
	}
	for _, candidate := range prompts {
		if candidate.canPrune && candidate.fullySettledAt.Valid {
			eligible++
			consider(candidate.fullySettledAt.Time)
		}
	}
	return eligible, oldestAge
}

func (r *Repository) commitAgentDeliveryRetentionBatch(
	ctx context.Context,
	tx *sqlx.Tx,
	progress agentDeliveryRetentionProgress,
	candidates []agentDeliveryRetentionCandidate,
	prompts []agentDeliveryPromptRetentionCandidate,
	now time.Time,
	wrapped bool,
	promptWrapped bool,
	nextPhase string,
	options models.AgentDeliveryRetentionBatchOptions,
	result *models.AgentDeliveryRetentionBatchResult,
) error {
	if err := r.compactAgentDeliveryCandidates(ctx, tx, candidates, now, options.MaxPayloadBytes, options.ReplayAge, options.ReplayEvents, result); err != nil {
		return err
	}
	if err := r.retireAgentDeliveryPrefixes(ctx, tx, candidates, now, options.ReplayAge, options.ReplayEvents, result); err != nil {
		return err
	}
	if err := r.pruneAgentDeliveryPromptPayloads(
		ctx, tx, prompts, now, options.PayloadGrace, options.MaxPayloadBytes, result,
	); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.writeAgentDeliveryRetentionProgress(
		ctx, tx, progress, candidates, prompts, now, wrapped, promptWrapped, nextPhase,
	); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent delivery retention batch: %w", err)
	}
	return nil
}

type agentDeliveryRetentionProgress struct {
	lastSessionID          string
	eventSweepPhase        string
	lastCompactionAt       sql.NullTime
	lastCompactionStreamID string
	lastCompactionSequence int64
	lastStreamID           string
	lastSequence           int64
	lastSubmissionID       string
	sweepPhase             string
}

func loadAgentDeliveryRetentionProgress(
	ctx context.Context,
	tx *sqlx.Tx,
	rebind func(string) string,
) (agentDeliveryRetentionProgress, error) {
	var progress agentDeliveryRetentionProgress
	var formatVersion int
	if err := tx.QueryRowxContext(ctx, rebind(`
		SELECT format_version, last_session_id, last_stream_id, last_sequence,
		       last_submission_id, sweep_phase, event_sweep_phase, last_compaction_at, last_compaction_stream_id, last_compaction_sequence
		FROM agent_delivery_retention_state WHERE id = ?`), 1).Scan(
		&formatVersion, &progress.lastSessionID, &progress.lastStreamID, &progress.lastSequence,
		&progress.lastSubmissionID, &progress.sweepPhase, &progress.eventSweepPhase, &progress.lastCompactionAt, &progress.lastCompactionStreamID, &progress.lastCompactionSequence,
	); err != nil {
		return progress, fmt.Errorf("load agent delivery retention progress: %w", err)
	}
	if formatVersion != agentDeliveryRetentionFormatVersion {
		return progress, fmt.Errorf("unsupported agent delivery retention format %d", formatVersion)
	}
	return progress, nil
}

func normalizeAgentDeliveryRetentionOptions(options models.AgentDeliveryRetentionBatchOptions) models.AgentDeliveryRetentionBatchOptions {
	if options.MaxCandidates <= 0 || options.MaxCandidates > agentDeliveryRetentionCandidateLimit {
		options.MaxCandidates = agentDeliveryRetentionCandidateLimit
	}
	if options.MaxPayloadBytes <= 0 || options.MaxPayloadBytes > agentDeliveryRetentionPayloadByteLimit {
		options.MaxPayloadBytes = agentDeliveryRetentionPayloadByteLimit
	}
	if options.WorkTimeout <= 0 || options.WorkTimeout > agentDeliveryRetentionWorkTimeout {
		options.WorkTimeout = agentDeliveryRetentionWorkTimeout
	}
	if options.PayloadGrace <= 0 || options.PayloadGrace > 30*24*time.Hour {
		options.PayloadGrace = agentDeliveryRetentionPayloadGrace
	}
	if options.ReplayAge <= 0 || options.ReplayAge > 30*24*time.Hour {
		options.ReplayAge = agentDeliveryRetentionCompactMaxAge
	}
	if options.ReplayEvents <= 0 || options.ReplayEvents > 100000 {
		options.ReplayEvents = agentDeliveryRetentionCompactEventLimit
	}
	return options
}

func retentionLockIdentities(candidates []agentDeliveryRetentionCandidate) ([]string, []string) {
	sessions := make([]string, 0, len(candidates))
	streamsToLock := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		sessions = append(sessions, candidate.event.SessionID)
		streamsToLock = append(streamsToLock, candidate.event.StreamID)
	}
	return streamsToLock, sessions
}

func (r *Repository) writeAgentDeliveryRetentionProgress(
	ctx context.Context,
	tx *sqlx.Tx,
	previous agentDeliveryRetentionProgress,
	candidates []agentDeliveryRetentionCandidate,
	prompts []agentDeliveryPromptRetentionCandidate,
	now time.Time,
	wrapped bool,
	promptWrapped bool,
	nextPhase string,
) error {
	progress := nextAgentDeliveryRetentionProgress(previous, candidates, prompts, wrapped, promptWrapped)
	if err := r.failAgentDeliveryRetention("before_progress"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_retention_state
		SET pruning_started_at = COALESCE(pruning_started_at, ?), sweep_phase = ?,
			last_stream_id = ?, last_sequence = ?, last_submission_id = ?, updated_at = ?,
			event_sweep_phase = ?, last_compaction_at = ?, last_compaction_stream_id = ?, last_compaction_sequence = ?
		WHERE id = 1`), now, nextPhase, progress.lastStreamID,
		progress.lastSequence, progress.lastSubmissionID, now, progress.eventSweepPhase, progress.lastCompactionAt, progress.lastCompactionStreamID, progress.lastCompactionSequence); err != nil {
		return fmt.Errorf("save agent delivery retention progress: %w", err)
	}
	if err := r.failAgentDeliveryRetention("after_progress"); err != nil {
		return err
	}
	return nil
}

func (r *Repository) failAgentDeliveryRetention(point string) error {
	if r.agentDeliveryRetentionFailpoint == nil {
		return nil
	}
	return r.agentDeliveryRetentionFailpoint(point)
}

func nullTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	copy := value.Time
	return &copy
}
