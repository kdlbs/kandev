//nolint:revive // This file groups the durable stream state machine and its protocol validation.
package plugins

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

const (
	SessionEventProtocolVersion = 1
	SessionPoisonPending        = "pending"
	SessionPoisonLeased         = "leased"
	SessionPoisonExhausted      = "exhausted"
	SessionPoisonRequeued       = "requeued"
	SessionPoisonMaxAttempts    = 5
	SessionPoisonLease          = 30 * time.Second
	SessionReconnectGrace       = 10 * time.Minute
	SessionEventRetention       = conversationTokenTTL + SessionReconnectGrace
	sessionRemovedEventType     = "session.removed"
)

var (
	ErrSessionRemoved  = errors.New("session removed")
	ErrPoisonEvent     = errors.New("poison session event")
	ErrForwardGap      = errors.New("session event forward gap")
	ErrStaleOwnerEpoch = errors.New("stale poison owner epoch")
)

// SessionDeliveryDisposition is the atomic poison-delivery claim outcome.
type SessionDeliveryDisposition string

const (
	SessionDeliveryUntracked SessionDeliveryDisposition = "untracked"
	SessionDeliveryClaimed   SessionDeliveryDisposition = "claimed"
	SessionDeliveryLeased    SessionDeliveryDisposition = "leased"
	SessionDeliveryBackoff   SessionDeliveryDisposition = "backoff"
	SessionDeliveryExhausted SessionDeliveryDisposition = "exhausted"
)

// SessionDeliveryClaim is an opaque reservation. Complete must receive the
// exact claim so a stale sender cannot mutate a newer lease or owner epoch.
type SessionDeliveryClaim struct {
	Disposition SessionDeliveryDisposition
	sessionID   string
	eventID     string
	ownerEpoch  uint64
	leaseUntil  time.Time
}

// SessionEvent is the immutable Host-only ordered stream envelope.
type SessionEvent struct {
	ProtocolVersion int             `json:"protocol_version"`
	EventType       string          `json:"event_type"`
	SessionID       string          `json:"session_id"`
	TaskID          *string         `json:"task_id"`
	Sequence        uint64          `json:"sequence"`
	ID              string          `json:"event_id"`
	Payload         json.RawMessage `json:"payload"`
	CreatedAt       time.Time       `json:"created_at"`
}

// SessionDeliveryCursorKey is the complete durable consumer binding.
type SessionDeliveryCursorKey struct {
	SessionID    string `json:"session_id"`
	ConsumerKind string `json:"consumer_kind"`
	ConsumerID   string `json:"consumer_id"`
	WireID       string `json:"wire_id"`
	PluginID     string `json:"plugin_id"`
	Generation   int64  `json:"generation"`
	UserID       string `json:"user_id"`
}

type SessionDeliveryCursor struct {
	Key                  SessionDeliveryCursorKey `json:"key"`
	AcknowledgedSequence uint64                   `json:"acknowledged_sequence"`
	OwnerEpoch           uint64                   `json:"owner_epoch"`
	LeaseUntil           time.Time                `json:"lease_until,omitempty"`
	RetryAt              time.Time                `json:"retry_at,omitempty"`
	UpdatedAt            time.Time                `json:"updated_at"`
}

type SessionPoisonRecord struct {
	SessionID       string    `json:"session_id"`
	EventID         string    `json:"event_id"`
	Sequence        uint64    `json:"sequence"`
	ProtocolVersion int       `json:"protocol_version"`
	LastError       string    `json:"last_error"`
	State           string    `json:"state"`
	Attempts        int       `json:"attempts"`
	OwnerEpoch      uint64    `json:"owner_epoch"`
	LeaseUntil      time.Time `json:"lease_until,omitempty"`
	NextRetry       time.Time `json:"next_retry,omitempty"`
	LastActor       string    `json:"last_actor,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type SessionPoisonAudit struct {
	SessionID  string    `json:"session_id"`
	EventID    string    `json:"event_id"`
	Actor      string    `json:"actor"`
	PriorState string    `json:"prior_state"`
	Attempts   int       `json:"attempts"`
	At         time.Time `json:"at"`
}

type sessionEventPartition struct {
	Events    []SessionEvent `json:"events"`
	Watermark uint64         `json:"watermark"`
	Terminal  bool           `json:"terminal"`
}

type sessionEventLogState struct {
	Sessions map[string]*sessionEventPartition `json:"sessions"`
	Cursors  map[string]*SessionDeliveryCursor `json:"cursors"`
	Poison   map[string]*SessionPoisonRecord   `json:"poison"`
	Audits   []SessionPoisonAudit              `json:"audits"`
}

func cloneSessionEventLogState(state sessionEventLogState) (sessionEventLogState, error) {
	encoded, err := json.Marshal(state)
	if err != nil {
		return sessionEventLogState{}, fmt.Errorf("clone session event log state: %w", err)
	}
	var clone sessionEventLogState
	if err := json.Unmarshal(encoded, &clone); err != nil {
		return sessionEventLogState{}, fmt.Errorf("clone session event log state: %w", err)
	}
	return clone, nil
}

// SessionEventLog serializes append, versioning, poison, terminal tombstones,
// and consumer cursor advancement behind one persistence boundary.
type SessionEventLog struct {
	mu    sync.Mutex
	path  string
	db    *sql.DB
	state sessionEventLogState
	now   func() time.Time
}

// SessionDeliveryDispatcher owns poison delivery transitions and recovery.
type SessionDeliveryDispatcher struct {
	events *SessionEventLog
}

func NewSessionDeliveryDispatcher(events *SessionEventLog) *SessionDeliveryDispatcher {
	return &SessionDeliveryDispatcher{events: events}
}

// Claim atomically reserves a poison delivery. Untracked events need no
// completion; claimed poison must be completed after the queue attempt.
func (d *SessionDeliveryDispatcher) Claim(sessionID, eventID string, now time.Time) (SessionDeliveryClaim, error) {
	return d.events.claimPoisonDelivery(sessionID, eventID, now)
}

// Complete records whether a claimed poison frame reached any client queue.
func (d *SessionDeliveryDispatcher) Complete(claim SessionDeliveryClaim, queued bool, now time.Time) error {
	return d.events.completePoisonDelivery(claim, queued, now)
}

func (d *SessionDeliveryDispatcher) Requeue(
	sessionID string,
	eventID string,
	expectedOwnerEpoch uint64,
	actor string,
	now time.Time,
) error {
	return d.events.requeuePoison(sessionID, eventID, expectedOwnerEpoch, actor, now)
}

func (d *SessionDeliveryDispatcher) ReclaimExpiredLeases(now time.Time) error {
	return d.events.reclaimExpiredLeases(now)
}

func NewSessionEventLog(path string) (*SessionEventLog, error) {
	log := &SessionEventLog{
		state: sessionEventLogState{
			Sessions: make(map[string]*sessionEventPartition),
			Cursors:  make(map[string]*SessionDeliveryCursor),
			Poison:   make(map[string]*SessionPoisonRecord),
		},
		now: time.Now,
	}
	if path == "" {
		return log, nil
	}
	log.path = path
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create session event log directory: %w", err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open session event log: %w", err)
	}
	db.SetMaxOpenConns(1)
	log.db = db
	if err := log.initializeDatabase(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := log.loadDatabase(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return log, nil
}

// Close releases the durable event log database.
func (l *SessionEventLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.db == nil {
		return nil
	}
	err := l.db.Close()
	l.db = nil
	return err
}
func mustSessionEventLog() *SessionEventLog {
	log, err := NewSessionEventLog("")
	if err != nil {
		panic(err)
	}
	return log
}

func (l *SessionEventLog) Append(
	sessionID string,
	taskID *string,
	eventType string,
	payload json.RawMessage,
) (SessionEvent, error) {
	if !json.Valid(payload) {
		return SessionEvent{}, fmt.Errorf("%w: malformed payload", ErrPoisonEvent)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	partition := l.state.Sessions[sessionID]
	newPartition := partition == nil
	if newPartition {
		partition = &sessionEventPartition{}
		l.state.Sessions[sessionID] = partition
	}
	previousTerminal := partition.Terminal
	previousWatermark := partition.Watermark
	if partition.Terminal {
		return SessionEvent{}, ErrSessionRemoved
	}
	event := SessionEvent{
		ProtocolVersion: SessionEventProtocolVersion,
		EventType:       eventType,
		SessionID:       sessionID,
		TaskID:          taskID,
		Sequence:        partition.Watermark + 1,
		ID:              uuid.NewString(),
		Payload:         append(json.RawMessage(nil), payload...),
		CreatedAt:       l.now().UTC(),
	}
	partition.Events = append(partition.Events, event)
	partition.Watermark = event.Sequence
	if eventType == sessionRemovedEventType {
		partition.Terminal = true
	}
	var poison *SessionPoisonRecord
	if _, projectionErr := ProjectSessionEvent(event); projectionErr != nil {
		poison = &SessionPoisonRecord{
			SessionID: sessionID, EventID: event.ID, Sequence: event.Sequence,
			ProtocolVersion: event.ProtocolVersion, LastError: projectionErr.Error(),
			State: SessionPoisonPending, OwnerEpoch: 1, UpdatedAt: event.CreatedAt,
		}
		l.state.Poison[poisonKey(sessionID, event.ID)] = poison
	}
	if err := l.persistAppendLocked(event, poison); err != nil {
		partition.Events = partition.Events[:len(partition.Events)-1]
		partition.Terminal = previousTerminal
		partition.Watermark = previousWatermark
		delete(l.state.Poison, poisonKey(sessionID, event.ID))
		if newPartition {
			delete(l.state.Sessions, sessionID)
		}
		return SessionEvent{}, err
	}
	return event, nil
}

// AppendCommitted mirrors an event whose sequence and identity were allocated
// in the source mutation's primary-database transaction.
func (l *SessionEventLog) AppendCommitted(event SessionEvent) (bool, error) {
	if !json.Valid(event.Payload) {
		return false, fmt.Errorf("%w: malformed payload", ErrPoisonEvent)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	partition := l.state.Sessions[event.SessionID]
	newPartition := partition == nil
	if newPartition {
		partition = &sessionEventPartition{}
		l.state.Sessions[event.SessionID] = partition
	}
	previousWatermark := partition.Watermark
	if event.Sequence <= partition.Watermark {
		for _, existing := range partition.Events {
			if existing.Sequence == event.Sequence {
				if existing.ID != event.ID || existing.EventType != event.EventType {
					return false, fmt.Errorf("%w: sequence identity mismatch", ErrPoisonEvent)
				}
				return false, nil
			}
		}
		// The local replay row may already have aged out while its monotonic
		// watermark remains. The primary journal is authoritative.
		return false, nil
	}
	if partition.Terminal {
		return false, ErrSessionRemoved
	}
	if event.Sequence != partition.Watermark+1 {
		if !mirrorGapHealable(newPartition, partition) {
			return false, ErrForwardGap
		}
		// Collection can drop a partition (terminal, after retention) or
		// truncate its rows (idle, non-terminal) once no cursor or poison
		// protects the session. The primary journal then prunes the same
		// aged rows, so the first surviving row may start past the local
		// watermark. The primary is authoritative for mirrored rows: jump
		// the replay boundary and accept the row.
		partition.Watermark = event.Sequence - 1
	}
	previousTerminal := partition.Terminal
	partition.Events = append(partition.Events, event)
	partition.Watermark = event.Sequence
	if event.EventType == sessionRemovedEventType {
		partition.Terminal = true
	}
	var poison *SessionPoisonRecord
	if _, projectionErr := ProjectSessionEvent(event); projectionErr != nil {
		poison = &SessionPoisonRecord{
			SessionID: event.SessionID, EventID: event.ID, Sequence: event.Sequence,
			ProtocolVersion: event.ProtocolVersion, LastError: projectionErr.Error(),
			State: SessionPoisonPending, OwnerEpoch: 1, UpdatedAt: event.CreatedAt,
		}
		l.state.Poison[poisonKey(event.SessionID, event.ID)] = poison
	}
	if err := l.persistAppendLocked(event, poison); err != nil {
		partition.Events = partition.Events[:len(partition.Events)-1]
		partition.Watermark = previousWatermark
		partition.Terminal = previousTerminal
		delete(l.state.Poison, poisonKey(event.SessionID, event.ID))
		if newPartition {
			delete(l.state.Sessions, event.SessionID)
		}
		return false, err
	}
	return true, nil
}

// mirrorGapHealable reports whether a mirror row that starts past the local
// watermark can be accepted by jumping the replay boundary. Cursor/poison-
// protected sessions always keep their rows, so a gap into a non-empty (or
// still-terminal) partition means real history is missing and must stay an
// error; an empty partition can only mean both sides already discarded the
// intermediate rows.
func mirrorGapHealable(newPartition bool, partition *sessionEventPartition) bool {
	if newPartition {
		return true
	}
	return len(partition.Events) == 0 && !partition.Terminal
}

func (l *SessionEventLog) EventsAfter(sessionID string, sequence uint64) ([]SessionEvent, bool) {
	events, _, terminal := l.ReplayState(sessionID, sequence)
	return events, terminal
}

// ReplayState captures replay rows, the current watermark, and terminal state
// under one lock so subscription registration cannot construct a torn cutoff.
func (l *SessionEventLog) ReplayState(
	sessionID string,
	sequence uint64,
) ([]SessionEvent, uint64, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	partition := l.state.Sessions[sessionID]
	if partition == nil {
		return nil, 0, false
	}
	start := len(partition.Events)
	for index := range partition.Events {
		if partition.Events[index].Sequence > sequence {
			start = index
			break
		}
	}
	result := append([]SessionEvent(nil), partition.Events[start:]...)
	return result, partition.Watermark, partition.Terminal
}

func (l *SessionEventLog) Watermark(sessionID string) uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	partition := l.state.Sessions[sessionID]
	if partition == nil {
		return 0
	}
	return partition.Watermark
}

func (l *SessionEventLog) RegisterCursor(key SessionDeliveryCursorKey, sequence uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	partition := l.state.Sessions[key.SessionID]
	if partition != nil && sequence > partition.Watermark {
		return ErrForwardGap
	}
	encodedKey := deliveryCursorKey(key)
	previous := l.state.Cursors[encodedKey]
	if previous != nil {
		previousCopy := *previous
		previous.UpdatedAt = l.now().UTC()
		if err := l.persistLocked(); err != nil {
			*previous = previousCopy
			return err
		}
		return nil
	}
	l.state.Cursors[encodedKey] = &SessionDeliveryCursor{
		Key: key, AcknowledgedSequence: sequence, OwnerEpoch: 1, UpdatedAt: l.now().UTC(),
	}
	if err := l.persistLocked(); err != nil {
		delete(l.state.Cursors, encodedKey)
		return err
	}
	return nil
}

func (l *SessionEventLog) ReleaseCursor(key SessionDeliveryCursorKey) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	encodedKey := deliveryCursorKey(key)
	cursor := l.state.Cursors[encodedKey]
	if cursor == nil {
		return nil
	}
	delete(l.state.Cursors, encodedKey)
	if err := l.persistLocked(); err != nil {
		l.state.Cursors[encodedKey] = cursor
		return err
	}
	return nil
}

// ReplaceCursor atomically advances a consumer to a new replay boundary.
// Rebinds use this after invalid resume or poison recovery; RegisterCursor
// intentionally preserves an existing acknowledgement for ordinary reconnects.
func (l *SessionEventLog) ReplaceCursor(key SessionDeliveryCursorKey, sequence uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	partition := l.state.Sessions[key.SessionID]
	if partition != nil && sequence > partition.Watermark {
		return ErrForwardGap
	}
	encodedKey := deliveryCursorKey(key)
	previous, err := cloneSessionEventLogState(l.state)
	if err != nil {
		return err
	}
	cursor := l.state.Cursors[encodedKey]
	if cursor == nil {
		cursor = &SessionDeliveryCursor{Key: key}
		l.state.Cursors[encodedKey] = cursor
	}
	cursor.AcknowledgedSequence = sequence
	cursor.OwnerEpoch++
	cursor.UpdatedAt = l.now().UTC()
	if err := l.persistLocked(); err != nil {
		l.state = previous
		return err
	}
	return nil
}
func (l *SessionEventLog) HasCursor(key SessionDeliveryCursorKey) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, exists := l.state.Cursors[deliveryCursorKey(key)]
	return exists
}

func (l *SessionEventLog) Acknowledge(key SessionDeliveryCursorKey, sequence uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	partition := l.state.Sessions[key.SessionID]
	watermark := uint64(0)
	if partition != nil {
		watermark = partition.Watermark
	}
	if sequence > watermark {
		return ErrForwardGap
	}
	encodedKey := deliveryCursorKey(key)
	cursor := l.state.Cursors[encodedKey]
	if cursor == nil {
		cursor = &SessionDeliveryCursor{Key: key}
		l.state.Cursors[encodedKey] = cursor
	}
	if sequence <= cursor.AcknowledgedSequence {
		return nil
	}
	for _, event := range partition.Events {
		if event.Sequence <= cursor.AcknowledgedSequence || event.Sequence > sequence {
			continue
		}
		if _, poisoned := l.state.Poison[poisonKey(key.SessionID, event.ID)]; poisoned {
			return ErrPoisonEvent
		}
	}
	previous := cursor.AcknowledgedSequence
	cursor.AcknowledgedSequence = sequence
	cursor.UpdatedAt = l.now().UTC()
	// ACKs are per contiguous event, so persist only the advanced cursor row
	// (one upsert) instead of rewriting every retained partition, event,
	// cursor, poison record, and audit with the full-state transaction.
	if err := l.persistCursorRowLocked(encodedKey, cursor); err != nil {
		cursor.AcknowledgedSequence = previous
		return err
	}
	return nil
}

// persistCursorRowLocked writes exactly one session_delivery_cursors row,
// leaving every other row untouched. Called on the hot ACK path; full-state
// rewrites remain for structural changes (collection, poison transitions,
// registration of the first cursor).
func (l *SessionEventLog) persistCursorRowLocked(encodedKey string, cursor *SessionDeliveryCursor) error {
	if l.path == "" {
		return nil
	}
	if l.db == nil {
		return errors.New("session event log database is closed")
	}
	tx, err := l.db.Begin()
	if err != nil {
		return fmt.Errorf("begin session event cursor transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	binding, err := json.Marshal(cursor.Key)
	if err != nil {
		return fmt.Errorf("encode delivery cursor: %w", err)
	}
	query := sqlx.Rebind(sqlx.QUESTION, `
		INSERT INTO session_delivery_cursors(cursor_key, binding, acknowledged_sequence, owner_epoch, lease_until, retry_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cursor_key) DO UPDATE SET
			binding = excluded.binding,
			acknowledged_sequence = excluded.acknowledged_sequence,
			owner_epoch = excluded.owner_epoch,
			lease_until = excluded.lease_until,
			retry_at = excluded.retry_at,
			updated_at = excluded.updated_at`)
	if _, err := tx.Exec(query,
		encodedKey, binding, cursor.AcknowledgedSequence, cursor.OwnerEpoch,
		nullableTime(cursor.LeaseUntil), nullableTime(cursor.RetryAt),
		cursor.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("persist delivery cursor row: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session event cursor transaction: %w", err)
	}
	return nil
}

// CollectExpired removes replay state only after both the binding-token and
// reconnect-grace windows have elapsed. A live cursor retains its complete
// partition so reconnect never observes a partially collected history.
//
//nolint:cyclop // Collection coordinates cursor, poison, tombstone, and partition retention.
func (l *SessionEventLog) CollectExpired(now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	previous, err := cloneSessionEventLogState(l.state)
	if err != nil {
		return err
	}
	cutoff := now.UTC().Add(-SessionEventRetention)
	activeSessions := make(map[string]struct{})
	for encodedKey, cursor := range l.state.Cursors {
		if cursor.UpdatedAt.After(cutoff) {
			activeSessions[cursor.Key.SessionID] = struct{}{}
			continue
		}
		delete(l.state.Cursors, encodedKey)
	}
	for key, record := range l.state.Poison {
		if !record.UpdatedAt.After(cutoff) {
			delete(l.state.Poison, key)
		}
	}
	keptAudits := l.state.Audits[:0]
	for _, audit := range l.state.Audits {
		if audit.At.After(cutoff) {
			keptAudits = append(keptAudits, audit)
		}
	}
	l.state.Audits = keptAudits
	for sessionID, partition := range l.state.Sessions {
		if _, active := activeSessions[sessionID]; active || l.sessionHasPoisonLocked(sessionID) {
			continue
		}
		kept := partition.Events[:0]
		for _, event := range partition.Events {
			if event.CreatedAt.After(cutoff) {
				kept = append(kept, event)
			}
		}
		partition.Events = kept
		if partition.Terminal && len(partition.Events) == 0 {
			delete(l.state.Sessions, sessionID)
		}
	}
	if err := l.persistLocked(); err != nil {
		l.state = previous
		return err
	}
	return nil
}

func (l *SessionEventLog) sessionHasPoisonLocked(sessionID string) bool {
	for _, record := range l.state.Poison {
		if record.SessionID == sessionID {
			return true
		}
	}
	return false
}

// RetainedSessionIDs returns sessions whose sidecar state still protects
// reconnect or poison delivery. Primary journal history may be pruned only
// for sessions outside this set.
func (l *SessionEventLog) RetainedSessionIDs(now time.Time) map[string]struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.UTC().Add(-SessionEventRetention)
	retained := make(map[string]struct{})
	for _, cursor := range l.state.Cursors {
		if cursor.UpdatedAt.After(cutoff) {
			retained[cursor.Key.SessionID] = struct{}{}
		}
	}
	for _, record := range l.state.Poison {
		if record.UpdatedAt.After(cutoff) {
			retained[record.SessionID] = struct{}{}
		}
	}
	return retained
}

func (l *SessionEventLog) Poison(sessionID, eventID string) (SessionPoisonRecord, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	record, ok := l.state.Poison[poisonKey(sessionID, eventID)]
	if !ok {
		return SessionPoisonRecord{}, false
	}
	return *record, true
}

func (l *SessionEventLog) claimPoisonDelivery(
	sessionID string,
	eventID string,
	now time.Time,
) (SessionDeliveryClaim, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	record := l.state.Poison[poisonKey(sessionID, eventID)]
	if record == nil {
		return SessionDeliveryClaim{Disposition: SessionDeliveryUntracked}, nil
	}
	now = now.UTC()
	disposition := SessionDeliveryClaimed
	switch {
	case record.State == SessionPoisonExhausted:
		disposition = SessionDeliveryExhausted
	case record.State == SessionPoisonLeased && record.LeaseUntil.After(now):
		disposition = SessionDeliveryLeased
	case record.NextRetry.After(now):
		disposition = SessionDeliveryBackoff
	}
	if disposition != SessionDeliveryClaimed {
		return SessionDeliveryClaim{Disposition: disposition}, nil
	}
	previous, err := cloneSessionEventLogState(l.state)
	if err != nil {
		return SessionDeliveryClaim{}, err
	}
	record.State = SessionPoisonLeased
	record.LeaseUntil = now.Add(SessionPoisonLease)
	record.UpdatedAt = now
	claim := SessionDeliveryClaim{
		Disposition: SessionDeliveryClaimed,
		sessionID:   sessionID, eventID: eventID,
		ownerEpoch: record.OwnerEpoch, leaseUntil: record.LeaseUntil,
	}
	if err := l.persistLocked(); err != nil {
		l.state = previous
		return SessionDeliveryClaim{}, err
	}
	return claim, nil
}

func (l *SessionEventLog) completePoisonDelivery(
	claim SessionDeliveryClaim,
	queued bool,
	now time.Time,
) error {
	if claim.Disposition != SessionDeliveryClaimed {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	record := l.state.Poison[poisonKey(claim.sessionID, claim.eventID)]
	if record == nil {
		return ErrPoisonEvent
	}
	if record.State != SessionPoisonLeased ||
		record.OwnerEpoch != claim.ownerEpoch ||
		!record.LeaseUntil.Equal(claim.leaseUntil) {
		return ErrStaleOwnerEpoch
	}
	previous, err := cloneSessionEventLogState(l.state)
	if err != nil {
		return err
	}
	now = now.UTC()
	if !queued {
		record.State = SessionPoisonPending
		record.LeaseUntil = time.Time{}
		record.NextRetry = time.Time{}
		record.UpdatedAt = now
	} else {
		record.Attempts++
		backoff := time.Second << min(record.Attempts-1, 5)
		if backoff > SessionPoisonLease {
			backoff = SessionPoisonLease
		}
		record.NextRetry = record.LeaseUntil.Add(backoff)
		if record.Attempts >= SessionPoisonMaxAttempts {
			record.State = SessionPoisonExhausted
			record.LeaseUntil = time.Time{}
			record.NextRetry = time.Time{}
		}
		record.UpdatedAt = now
	}
	if err := l.persistLocked(); err != nil {
		l.state = previous
		return err
	}
	return nil
}

func (l *SessionEventLog) PendingPoison(now time.Time) []SessionPoisonRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	now = now.UTC()
	result := make([]SessionPoisonRecord, 0)
	for _, record := range l.state.Poison {
		if record.State == SessionPoisonPending && !record.NextRetry.After(now) {
			result = append(result, *record)
		}
	}
	return result
}

func (l *SessionEventLog) requeuePoison(
	sessionID string,
	eventID string,
	expectedOwnerEpoch uint64,
	actor string,
	now time.Time,
) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	record := l.state.Poison[poisonKey(sessionID, eventID)]
	if record == nil {
		return ErrPoisonEvent
	}
	if record.State == SessionPoisonPending && record.OwnerEpoch == expectedOwnerEpoch+1 {
		return nil
	}
	if record.OwnerEpoch != expectedOwnerEpoch {
		return ErrStaleOwnerEpoch
	}
	previous, err := cloneSessionEventLogState(l.state)
	if err != nil {
		return err
	}
	priorState := record.State
	priorAttempts := record.Attempts
	record.State = SessionPoisonRequeued
	record.OwnerEpoch++
	record.Attempts = 0
	record.LeaseUntil = time.Time{}
	record.NextRetry = time.Time{}
	record.LastActor = actor
	record.UpdatedAt = now.UTC()
	l.state.Audits = append(l.state.Audits, SessionPoisonAudit{
		SessionID: sessionID, EventID: eventID, Actor: actor,
		PriorState: priorState, Attempts: priorAttempts, At: now.UTC(),
	})
	record.State = SessionPoisonPending
	if err := l.persistLocked(); err != nil {
		l.state = previous
		return err
	}
	return nil
}

func (l *SessionEventLog) reclaimExpiredLeases(now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	previous, err := cloneSessionEventLogState(l.state)
	if err != nil {
		return err
	}
	changed := false
	for _, record := range l.state.Poison {
		if record.State == SessionPoisonLeased && !record.LeaseUntil.After(now) {
			record.State = SessionPoisonPending
			record.LeaseUntil = time.Time{}
			record.UpdatedAt = now.UTC()
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := l.persistLocked(); err != nil {
		l.state = previous
		return err
	}
	return nil
}

//nolint:goconst // Event names are protocol literals.
func ProjectSessionEvent(event SessionEvent) (json.RawMessage, error) {
	if event.ProtocolVersion != SessionEventProtocolVersion {
		return nil, fmt.Errorf("%w: unsupported protocol version", ErrPoisonEvent)
	}
	switch event.EventType {
	case "message.added", "message.updated", "message.deleted",
		"session.turn.started", "session.turn.completed", "session.turn.removed", "session.removed":
	default:
		return nil, fmt.Errorf("%w: unsupported event type", ErrPoisonEvent)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return nil, fmt.Errorf("%w: malformed payload", ErrPoisonEvent)
	}
	if payload["type"] != event.EventType {
		return nil, fmt.Errorf("%w: payload event type mismatch", ErrPoisonEvent)
	}
	if sessionID, ok := payload["session_id"].(string); !ok || sessionID == "" || sessionID != event.SessionID {
		return nil, fmt.Errorf("%w: payload session identity mismatch", ErrPoisonEvent)
	}
	if err := validateSessionEventTask(event, payload); err != nil {
		return nil, err
	}
	if err := validateSessionEventPayload(event.EventType, payload); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), event.Payload...), nil
}

func validateSessionEventTask(event SessionEvent, payload map[string]any) error {
	taskID, present := payload["task_id"]
	if event.TaskID == nil {
		if !present || taskID == nil {
			return nil
		}
		return fmt.Errorf("%w: payload task identity mismatch", ErrPoisonEvent)
	}
	value, ok := taskID.(string)
	if !ok || value == "" || value != *event.TaskID {
		return fmt.Errorf("%w: payload task identity mismatch", ErrPoisonEvent)
	}
	return nil
}

//nolint:cyclop,goconst // Event validation is intentionally explicit per protocol event type.
func validateSessionEventPayload(eventType string, payload map[string]any) error {
	requiredString := func(field string) (string, error) {
		value, ok := payload[field].(string)
		if !ok || value == "" {
			return "", fmt.Errorf("%w: missing %s", ErrPoisonEvent, field)
		}
		return value, nil
	}
	requiredTime := func(field string) error {
		value, err := requiredString(field)
		if err != nil {
			return err
		}
		if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
			return fmt.Errorf("%w: invalid %s", ErrPoisonEvent, field)
		}
		return nil
	}
	switch eventType {
	case "message.added", "message.updated":
		if _, err := requiredString("message_id"); err != nil {
			return err
		}
		author, err := requiredString("author_type")
		if err != nil {
			return err
		}
		if author != "user" && author != "agent" {
			return fmt.Errorf("%w: invalid author_type", ErrPoisonEvent)
		}
		if _, ok := payload["content"].(string); !ok {
			return fmt.Errorf("%w: missing content", ErrPoisonEvent)
		}
		if err := requiredTime("created_at"); err != nil {
			return err
		}
		if eventType == "message.updated" {
			return requiredTime("updated_at")
		}
	case "message.deleted":
		_, err := requiredString("message_id")
		return err
	case "session.turn.removed":
		_, err := requiredString("id")
		return err
	case "session.turn.started", "session.turn.completed":
		if _, err := requiredString("id"); err != nil {
			return err
		}
		if err := requiredTime("started_at"); err != nil {
			return err
		}
		if eventType == "session.turn.completed" {
			if err := requiredTime("completed_at"); err != nil {
				return err
			}
			return requiredTime("updated_at")
		}
	case "session.removed":
		return nil
	}
	return nil
}

func (l *SessionEventLog) persistAppendLocked(event SessionEvent, poison *SessionPoisonRecord) error {
	if l.path == "" {
		return nil
	}
	if l.db == nil {
		return errors.New("session event log database is closed")
	}
	tx, err := l.db.Begin()
	if err != nil {
		return fmt.Errorf("begin session event append: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(sqlx.Rebind(sqlx.QUESTION, `INSERT INTO session_event_partitions(session_id, watermark, terminal) VALUES (?, ?, ?) ON CONFLICT(session_id) DO UPDATE SET watermark = excluded.watermark, terminal = excluded.terminal`),
		event.SessionID, event.Sequence, event.EventType == sessionRemovedEventType); err != nil {
		return fmt.Errorf("persist session partition: %w", err)
	}
	if _, err := tx.Exec(sqlx.Rebind(sqlx.QUESTION, `INSERT INTO session_events(session_id, sequence, event_id, protocol_version, event_type, task_id, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		event.SessionID, event.Sequence, event.ID, event.ProtocolVersion, event.EventType, nullableString(event.TaskID), []byte(event.Payload), event.CreatedAt.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("persist session event: %w", err)
	}
	if poison != nil {
		if _, err := tx.Exec(sqlx.Rebind(sqlx.QUESTION, `INSERT INTO session_poison(poison_key, record) VALUES (?, ?)`), poisonKey(event.SessionID, event.ID), mustJSON(poison)); err != nil {
			return fmt.Errorf("persist poison record: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session event append: %w", err)
	}
	return nil
}

func (l *SessionEventLog) persistLocked() error {
	if l.path == "" {
		return nil
	}
	if l.db == nil {
		return errors.New("session event log database is closed")
	}
	tx, err := l.db.Begin()
	if err != nil {
		return fmt.Errorf("begin session event transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := resetSessionEventTables(tx); err != nil {
		return err
	}
	if err := l.persistSessions(tx); err != nil {
		return err
	}
	if err := l.persistConsumerState(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session event transaction: %w", err)
	}
	return nil
}

func resetSessionEventTables(tx *sql.Tx) error {
	for _, table := range []string{"session_poison_audits", "session_poison", "session_delivery_cursors", "session_events", "session_event_partitions"} {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("reset %s: %w", table, err)
		}
	}
	return nil
}

func (l *SessionEventLog) persistSessions(tx *sql.Tx) error {
	for sessionID, partition := range l.state.Sessions {
		if _, err := tx.Exec(sqlx.Rebind(sqlx.QUESTION, `INSERT INTO session_event_partitions(session_id, watermark, terminal) VALUES (?, ?, ?)`), sessionID, partition.Watermark, partition.Terminal); err != nil {
			return fmt.Errorf("persist session partition: %w", err)
		}
		for _, event := range partition.Events {
			if _, err := tx.Exec(sqlx.Rebind(sqlx.QUESTION, `INSERT INTO session_events(session_id, sequence, event_id, protocol_version, event_type, task_id, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
				event.SessionID, event.Sequence, event.ID, event.ProtocolVersion, event.EventType, nullableString(event.TaskID), []byte(event.Payload), event.CreatedAt.Format(time.RFC3339Nano)); err != nil {
				return fmt.Errorf("persist session event: %w", err)
			}
		}
	}
	return nil
}

func (l *SessionEventLog) persistConsumerState(tx *sql.Tx) error {
	for encodedKey, cursor := range l.state.Cursors {
		key, err := json.Marshal(cursor.Key)
		if err != nil {
			return fmt.Errorf("encode delivery cursor: %w", err)
		}
		if _, err := tx.Exec(sqlx.Rebind(sqlx.QUESTION, `INSERT INTO session_delivery_cursors(cursor_key, binding, acknowledged_sequence, owner_epoch, lease_until, retry_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`),
			encodedKey, key, cursor.AcknowledgedSequence, cursor.OwnerEpoch, nullableTime(cursor.LeaseUntil), nullableTime(cursor.RetryAt), cursor.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("persist delivery cursor: %w", err)
		}
	}
	for encodedKey, record := range l.state.Poison {
		if _, err := tx.Exec(sqlx.Rebind(sqlx.QUESTION, `INSERT INTO session_poison(poison_key, record) VALUES (?, ?)`), encodedKey, mustJSON(record)); err != nil {
			return fmt.Errorf("persist poison record: %w", err)
		}
	}
	for _, audit := range l.state.Audits {
		if _, err := tx.Exec(sqlx.Rebind(sqlx.QUESTION, `INSERT INTO session_poison_audits(record) VALUES (?)`), mustJSON(audit)); err != nil {
			return fmt.Errorf("persist poison audit: %w", err)
		}
	}
	return nil
}

func (l *SessionEventLog) initializeDatabase() error {
	const schema = `
CREATE TABLE IF NOT EXISTS session_event_partitions (
	session_id TEXT PRIMARY KEY,
	watermark INTEGER NOT NULL DEFAULT 0,
	terminal INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS session_events (
	session_id TEXT NOT NULL,
	sequence INTEGER NOT NULL,
	event_id TEXT NOT NULL UNIQUE,
	protocol_version INTEGER NOT NULL,
	event_type TEXT NOT NULL,
	task_id TEXT,
	payload BLOB NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY(session_id, sequence)
);
CREATE TABLE IF NOT EXISTS session_delivery_cursors (
	cursor_key TEXT PRIMARY KEY,
	binding BLOB NOT NULL,
	acknowledged_sequence INTEGER NOT NULL,
	owner_epoch INTEGER NOT NULL DEFAULT 1,
	lease_until TEXT NOT NULL DEFAULT '',
	retry_at TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS session_poison (
	poison_key TEXT PRIMARY KEY,
	record BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS session_poison_audits (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	record BLOB NOT NULL
);`
	if _, err := l.db.Exec(schema); err != nil {
		return fmt.Errorf("initialize session event log: %w", err)
	}
	if _, err := l.db.Exec(`ALTER TABLE session_event_partitions ADD COLUMN watermark INTEGER NOT NULL DEFAULT 0`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return fmt.Errorf("migrate session event partition watermark: %w", err)
	}
	for _, migration := range []struct {
		column     string
		definition string
	}{
		{"owner_epoch", "INTEGER NOT NULL DEFAULT 1"},
		{"lease_until", "TEXT NOT NULL DEFAULT ''"},
		{"retry_at", "TEXT NOT NULL DEFAULT ''"},
	} {
		query := fmt.Sprintf("ALTER TABLE session_delivery_cursors ADD COLUMN %s %s", migration.column, migration.definition)
		if _, err := l.db.Exec(query); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("migrate session delivery cursor %s: %w", migration.column, err)
		}
	}
	return nil
}

func (l *SessionEventLog) loadDatabase() error {
	if err := l.loadPartitions(); err != nil {
		return err
	}
	if err := l.loadEvents(); err != nil {
		return err
	}
	if err := l.loadCursors(); err != nil {
		return err
	}
	if err := l.loadPoisonState(); err != nil {
		return err
	}
	return nil
}

func (l *SessionEventLog) loadPartitions() error {
	rows, err := l.db.Query(`SELECT session_id, watermark, terminal FROM session_event_partitions`)
	if err != nil {
		return fmt.Errorf("load session partitions: %w", err)
	}
	for rows.Next() {
		var sessionID string
		var watermark uint64
		var terminal bool
		if err := rows.Scan(&sessionID, &watermark, &terminal); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan session partition: %w", err)
		}
		l.state.Sessions[sessionID] = &sessionEventPartition{Watermark: watermark, Terminal: terminal}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate session partitions: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close session partitions: %w", err)
	}
	return nil
}

func (l *SessionEventLog) loadEvents() error {
	rows, err := l.db.Query(`SELECT session_id, sequence, event_id, protocol_version, event_type, task_id, payload, created_at FROM session_events ORDER BY session_id, sequence`)
	if err != nil {
		return fmt.Errorf("load session events: %w", err)
	}
	for rows.Next() {
		event, err := scanSessionEvent(rows)
		if err != nil {
			_ = rows.Close()
			return err
		}
		partition := l.state.Sessions[event.SessionID]
		if partition == nil {
			partition = &sessionEventPartition{}
			l.state.Sessions[event.SessionID] = partition
		}
		partition.Events = append(partition.Events, event)
		if event.Sequence > partition.Watermark {
			partition.Watermark = event.Sequence
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate session events: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close session events: %w", err)
	}
	return nil
}

func scanSessionEvent(rows *sql.Rows) (SessionEvent, error) {
	var event SessionEvent
	var taskID sql.NullString
	var createdAt string
	if err := rows.Scan(&event.SessionID, &event.Sequence, &event.ID, &event.ProtocolVersion, &event.EventType, &taskID, &event.Payload, &createdAt); err != nil {
		return SessionEvent{}, fmt.Errorf("scan session event: %w", err)
	}
	if taskID.Valid {
		event.TaskID = &taskID.String
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return SessionEvent{}, fmt.Errorf("parse session event time: %w", err)
	}
	event.CreatedAt = parsedCreatedAt
	return event, nil
}

func (l *SessionEventLog) loadCursors() error {
	err := l.loadJSONRows(`SELECT cursor_key, binding, acknowledged_sequence, owner_epoch, lease_until, retry_at, updated_at FROM session_delivery_cursors`, func(columns []any) error {
		encodedKey := columns[0].(string)
		var key SessionDeliveryCursorKey
		if err := json.Unmarshal(columns[1].([]byte), &key); err != nil {
			return err
		}
		leaseUntil, err := parseOptionalTime(columns[4])
		if err != nil {
			return err
		}
		retryAt, err := parseOptionalTime(columns[5])
		if err != nil {
			return err
		}
		updatedAt, err := time.Parse(time.RFC3339Nano, columns[6].(string))
		if err != nil {
			return err
		}
		l.state.Cursors[encodedKey] = &SessionDeliveryCursor{
			Key: key, AcknowledgedSequence: uint64(columns[2].(int64)), OwnerEpoch: uint64(columns[3].(int64)),
			LeaseUntil: leaseUntil, RetryAt: retryAt, UpdatedAt: updatedAt,
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("load delivery cursors: %w", err)
	}
	return nil
}

func (l *SessionEventLog) loadPoisonState() error {
	if err := l.loadJSONRows(`SELECT poison_key, record FROM session_poison`, func(columns []any) error {
		var record SessionPoisonRecord
		if err := json.Unmarshal(columns[1].([]byte), &record); err != nil {
			return err
		}
		l.state.Poison[columns[0].(string)] = &record
		return nil
	}); err != nil {
		return fmt.Errorf("load poison records: %w", err)
	}
	if err := l.loadJSONRows(`SELECT record FROM session_poison_audits ORDER BY id`, func(columns []any) error {
		var audit SessionPoisonAudit
		if err := json.Unmarshal(columns[0].([]byte), &audit); err != nil {
			return err
		}
		l.state.Audits = append(l.state.Audits, audit)
		return nil
	}); err != nil {
		return fmt.Errorf("load poison audits: %w", err)
	}
	return nil
}

func (l *SessionEventLog) loadJSONRows(query string, consume func([]any) error) error {
	rows, err := l.db.Query(query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	columnNames, err := rows.Columns()
	if err != nil {
		return err
	}
	for rows.Next() {
		values := make([]any, len(columnNames))
		destinations := make([]any, len(columnNames))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return err
		}
		if err := consume(values); err != nil {
			return err
		}
	}
	return rows.Err()
}
func nullableTime(value time.Time) any {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseOptionalTime(value any) (time.Time, error) {
	var raw string
	switch typed := value.(type) {
	case string:
		raw = typed
	case []byte:
		raw = string(typed)
	case nil:
		return time.Time{}, nil
	default:
		return time.Time{}, fmt.Errorf("unexpected optional time type %T", value)
	}
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, raw)
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func poisonKey(sessionID, eventID string) string {
	return sessionID + "\x00" + eventID
}

func deliveryCursorKey(key SessionDeliveryCursorKey) string {
	return strings.Join([]string{
		key.SessionID,
		key.ConsumerKind,
		key.ConsumerID,
		key.WireID,
		key.PluginID,
		fmt.Sprintf("%d", key.Generation),
		key.UserID,
	}, "\x00")
}
