package statussummary

import (
	"context"
	"slices"
	"sort"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
)

// SessionUnreadStateLoader returns unread ranks for the requested sessions.
// Every requested session that still exists is present; a session without a
// read cursor has a zero Read rank. Sessions that no longer exist are absent.
type SessionUnreadStateLoader func(ctx context.Context, sessionIDs []string) (map[string]SessionUnreadState, error)

// sessionUnreadRanks is one session's unread observation. A session is
// unread when its newest qualifying rank is known and after the cursor rank,
// which is zero for a session that has never been read.
type sessionUnreadRanks struct {
	read        UnreadRank
	newestKnown bool
	newest      UnreadRank
}

func (r sessionUnreadRanks) unread() bool {
	return r.newestKnown && r.newest.After(r.read)
}

func unreadRanksFromState(state *SessionUnreadState) sessionUnreadRanks {
	if state == nil {
		return sessionUnreadRanks{}
	}
	return sessionUnreadRanks{
		read:        normalizeRank(state.Read),
		newestKnown: true,
		newest:      normalizeRank(state.Newest),
	}
}

func normalizeRank(rank UnreadRank) UnreadRank {
	return NewUnreadRank(rank.ID, rank.CreatedAt)
}

// deriveUnreadSessionIDs uses per-session ranks only while they are observed
// task-wide; otherwise the last authoritative list is kept.
func deriveUnreadSessionIDs(state *projectionState) []string {
	if !state.unreadObserved {
		return slices.Clone(state.unreadBaseline)
	}
	var ids []string
	for id, session := range state.sessions {
		if session.unread.unread() {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// unreadDiffersFromCurrent reports whether the derived list differs from the
// last persisted summary, including after a cold restore.
func unreadDiffersFromCurrent(state *projectionState) bool {
	var current []string
	if state.current != nil {
		current = state.current.UnreadSessionIDs
	}
	return !slices.Equal(current, deriveUnreadSessionIDs(state))
}

// refreshUnreadBeforeEventLocked performs the repository reads an event needs
// before its synchronous fold. A failed read records a metric and changes
// nothing else, so the event's other effects still apply.
func (p *Projector) refreshUnreadBeforeEventLocked(
	ctx context.Context,
	taskID string,
	state *projectionState,
	eventType string,
	data map[string]interface{},
) {
	sessionID := stringField(data, "session_id")
	if p.loadSessionUnreadState == nil || sessionID == "" {
		return
	}
	switch eventType {
	case events.MessageAdded:
		p.refreshUnreadForMessageAddedLocked(ctx, taskID, state, sessionID, data)
	case events.TaskSessionReadCursorChanged:
		switch {
		case !state.unreadObserved:
			ensureSessionObservation(state, sessionID)
			p.reloadTaskUnreadLocked(ctx, taskID, state, "")
		case boolValue(data["reset"]) || !state.sessions[sessionID].unread.newestKnown:
			p.reloadSessionUnreadLocked(ctx, taskID, state, sessionID)
		}
	case events.MessageDeleted:
		p.refreshUnreadForMessageDeletedLocked(ctx, taskID, state, sessionID, data)
	case events.SessionRemoved:
		p.reloadTaskUnreadLocked(ctx, taskID, state, sessionID)
	}
}

// refreshUnreadForMessageAddedLocked loads ranks a qualifying message needs:
// every session while unread is unobserved, otherwise the session whose
// newest rank is still unknown.
func (p *Projector) refreshUnreadForMessageAddedLocked(
	ctx context.Context,
	taskID string,
	state *projectionState,
	sessionID string,
	data map[string]interface{},
) {
	if !isQualifyingMessageEvent(data) {
		return
	}
	if !state.unreadObserved {
		ensureSessionObservation(state, sessionID)
		p.reloadTaskUnreadLocked(ctx, taskID, state, "")
		return
	}
	if !state.sessions[sessionID].unread.newestKnown {
		p.reloadSessionUnreadLocked(ctx, taskID, state, sessionID)
	}
}

// refreshUnreadForMessageDeletedLocked reloads ranks only when the deleted
// message is the session's tracked newest qualifying message.
func (p *Projector) refreshUnreadForMessageDeletedLocked(
	ctx context.Context,
	taskID string,
	state *projectionState,
	sessionID string,
	data map[string]interface{},
) {
	ranks := state.sessions[sessionID].unread
	if !ranks.newestKnown || ranks.newest.ID != stringField(data, "message_id") {
		return
	}
	if !state.unreadObserved {
		p.reloadTaskUnreadLocked(ctx, taskID, state, "")
		return
	}
	p.reloadSessionUnreadLocked(ctx, taskID, state, sessionID)
}

// reloadTaskUnreadLocked replaces every known session's ranks. removedID is
// excluded from the read and keeps no contribution on success. On failure the
// previous list becomes the unobserved baseline.
func (p *Projector) reloadTaskUnreadLocked(
	ctx context.Context,
	taskID string,
	state *projectionState,
	removedID string,
) {
	prior := deriveUnreadSessionIDs(state)
	sessionIDs := make([]string, 0, len(state.sessions))
	for id := range state.sessions {
		if id != removedID {
			sessionIDs = append(sessionIDs, id)
		}
	}
	sort.Strings(sessionIDs)
	loaded, err := p.loadSessionUnreadState(ctx, sessionIDs)
	if err != nil {
		p.recordUnreadLoadFailure(taskID, err)
		state.unreadBaseline = prior
		state.unreadObserved = false
		return
	}
	for id, observation := range state.sessions {
		var next sessionUnreadRanks
		if id != removedID {
			if value, ok := loaded[id]; ok {
				next = unreadRanksFromState(&value)
			}
		}
		observation.unread = next
		state.sessions[id] = observation
	}
	state.unreadObserved = true
}

func (p *Projector) reloadSessionUnreadLocked(
	ctx context.Context,
	taskID string,
	state *projectionState,
	sessionID string,
) {
	loaded, err := p.loadSessionUnreadState(ctx, []string{sessionID})
	if err != nil {
		p.recordUnreadLoadFailure(taskID, err)
		state.unreadBaseline = deriveUnreadSessionIDs(state)
		state.unreadObserved = false
		return
	}
	value, exists := loaded[sessionID]
	if _, known := state.sessions[sessionID]; !known && !exists {
		// Removed and unobserved: re-creating it would leave a ghost entry.
		return
	}
	observation := ensureSessionObservation(state, sessionID)
	var next sessionUnreadRanks
	if exists {
		next = unreadRanksFromState(&value)
	}
	observation.unread = next
	state.sessions[sessionID] = observation
}

func (p *Projector) recordUnreadLoadFailure(taskID string, err error) {
	RecordUnreadLoadFailure()
	p.logger.Warn("failed to load unread state for task status summary",
		zap.String("task_id", taskID),
		zap.Error(err))
}

func ensureSessionObservation(state *projectionState, sessionID string) sessionObservation {
	observation, ok := state.sessions[sessionID]
	if !ok {
		observation = sessionObservation{id: sessionID}
		state.sessions[sessionID] = observation
	}
	return observation
}

// applyUnreadEventLocked folds already-loaded event values into the ranks.
// It only raises ranks (except an explicit reset), so replaying it after a
// compare-and-set rebase is idempotent.
func applyUnreadEventLocked(state *projectionState, eventType string, data map[string]interface{}) bool {
	sessionID := stringField(data, "session_id")
	if sessionID == "" {
		return false
	}
	switch eventType {
	case events.MessageAdded:
		return raiseNewestUnreadLocked(state, sessionID, data)
	case events.TaskSessionReadCursorChanged:
		return applyReadCursorLocked(state, sessionID, data)
	default:
		return false
	}
}

func raiseNewestUnreadLocked(state *projectionState, sessionID string, data map[string]interface{}) bool {
	if !isQualifyingMessageEvent(data) {
		return false
	}
	observation, ok := state.sessions[sessionID]
	if !ok || !observation.unread.newestKnown {
		return false
	}
	rank := NewUnreadRank(stringField(data, "message_id"), timeValue(data["created_at"]))
	if rank.ID == "" || rank.CreatedAt.IsZero() || !rank.After(observation.unread.newest) {
		return false
	}
	observation.unread.newest = rank
	state.sessions[sessionID] = observation
	return true
}

// applyReadCursorLocked raises the cursor, or replaces it when reset is set.
// An unparseable cursor timestamp leaves unread state untouched.
func applyReadCursorLocked(state *projectionState, sessionID string, data map[string]interface{}) bool {
	rank := NewUnreadRank(stringField(data, "last_read_message_id"), timeValue(data["last_read_created_at"]))
	if rank.ID == "" || rank.CreatedAt.IsZero() {
		return false
	}
	// The refresh establishes the observation when the DB holds the session. An
	// absent session here was removed, so the payload must not re-create it.
	observation, ok := state.sessions[sessionID]
	if !ok {
		return false
	}
	current := observation.unread.read
	if current == rank || (!boolValue(data["reset"]) && !rank.After(current)) {
		return false
	}
	observation.unread.read = rank
	state.sessions[sessionID] = observation
	return true
}

func isQualifyingMessageEvent(data map[string]interface{}) bool {
	metadata, _ := data["metadata"].(map[string]interface{})
	return IsUnreadQualifyingMessage(stringField(data, "author_type"), stringField(data, "type"),
		stringField(data, "content"), metadata)
}

func (p *Projector) noteUnreadFlip(taskID string, previous *TaskStatusSummary, next TaskStatusSummary) {
	before := previous != nil && len(previous.UnreadSessionIDs) > 0
	after := len(next.UnreadSessionIDs) > 0
	if before == after {
		return
	}
	recordUnreadFlip()
	p.logger.Debug("task unread aggregate changed",
		zap.String("task_id", taskID),
		zap.Bool("unread", after))
}

// unreadRiseOnly reports whether the derived summary differs from the stored
// one only by sessions newly added to the unread list.
func unreadRiseOnly(current *TaskStatusSummary, next TaskStatusSummary) bool {
	if current == nil || len(next.UnreadSessionIDs) <= len(current.UnreadSessionIDs) {
		return false
	}
	for _, id := range current.UnreadSessionIDs {
		if !slices.Contains(next.UnreadSessionIDs, id) {
			return false
		}
	}
	next.UnreadSessionIDs = current.UnreadSessionIDs
	return current.SemanticEqual(next)
}

// DefaultUnreadRiseHold is long enough for a displayed session's mark-read to
// arrive and short enough that a background task's dot still appears promptly.
const DefaultUnreadRiseHold = 1500 * time.Millisecond

// holdUnreadRiseLocked defers a write whose only effect is a new unread
// session. A displayed session is normally marked read within the hold, so the
// rise and its fall coalesce into no write; otherwise the flush persists it.
// Falls and every other field change are never held.
func (p *Projector) holdUnreadRiseLocked(taskID string, state *projectionState) bool {
	if p.unreadRiseHold <= 0 || !unreadRiseOnly(state.current, deriveSummary(state)) {
		return false
	}
	p.flushMu.Lock()
	defer p.flushMu.Unlock()
	if p.closed {
		return false
	}
	if _, pending := p.unreadFlushes[taskID]; !pending {
		p.unreadFlushes[taskID] = p.scheduleUnreadFlush(p.unreadRiseHold, func() { p.flushHeldUnread(taskID) })
	}
	return true
}

// unreadFlushTimeout bounds the repository work of one deferred flush.
const unreadFlushTimeout = 10 * time.Second

func (p *Projector) flushHeldUnread(taskID string) {
	p.flushMu.Lock()
	delete(p.unreadFlushes, taskID)
	closed := p.closed
	p.flushMu.Unlock()
	if closed {
		return
	}
	unlockTask := p.lockTask(taskID)
	defer unlockTask()
	p.mu.Lock()
	state := p.state[taskID]
	p.mu.Unlock()
	if state == nil || !unreadDiffersFromCurrent(state) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), unreadFlushTimeout)
	defer cancel()
	if err := p.persistPendingRefreshLocked(ctx, taskID, state, true, "", nil); err != nil {
		recordHandlerFailure()
		p.logger.Warn("failed to persist held unread task status",
			zap.String("task_id", taskID),
			zap.Error(err))
	}
}

func scheduleWithTimer(delay time.Duration, flush func()) func() {
	timer := time.AfterFunc(delay, flush)
	return func() { timer.Stop() }
}

func (p *Projector) cancelUnreadFlushes() {
	p.flushMu.Lock()
	defer p.flushMu.Unlock()
	p.closed = true
	for taskID, cancel := range p.unreadFlushes {
		cancel()
		delete(p.unreadFlushes, taskID)
	}
}
