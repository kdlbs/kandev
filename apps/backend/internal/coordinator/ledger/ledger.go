package ledger

import (
	"context"
	"expvar"
	"fmt"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator"
)

// Failure stages of coordinator_ledger_write_failed_total; the set is closed.
const (
	StageStart            = "start"
	StageCall             = "call"
	StageComplete         = "complete"
	StageSnapshot         = "snapshot"
	StageLink             = "link"
	StageCallQueueFull    = "call_queue_full"
	StageCallUnattributed = "call_unattributed"
	StageSettle           = "settle"
	StageRetention        = "retention"
	StageModel            = "model"
)

var writeFailedTotal = expvar.NewMap("coordinator_ledger_write_failed_total")

func countFailure(stage string) {
	if v, ok := writeFailedTotal.Get(stage).(*expvar.Int); ok {
		v.Add(1)
		return
	}
	writeFailedTotal.Add(stage, 1)
}

// FailureCount reads one stage's failure counter.
func FailureCount(stage string) int64 {
	if v, ok := writeFailedTotal.Get(stage).(*expvar.Int); ok {
		return v.Value()
	}
	return 0
}

// Deps are the ledger's collaborators. DB is the writer handle; RO the reader.
type Deps struct {
	DB  *sqlx.DB
	RO  *sqlx.DB
	Log *zap.Logger
	// Now is the clock; nil means the wall clock.
	Now func() time.Time
	// BuildVersion is the Kandev build version used in the harness stamp.
	BuildVersion string
	// CoordinatorForTask returns the coordinator whose conversation task is
	// taskID; ok is false for any other task.
	CoordinatorForTask func(ctx context.Context, taskID string) (coordinatorID string, ok bool, err error)
	// PromptHash is the SHA-256 hex of the standing instructions a conversation
	// opened now would receive; empty when they cannot be read.
	PromptHash func(ctx context.Context, coordinatorID string) string
	// WatchSet is the coordinator's effective watch set.
	WatchSet func(ctx context.Context, coordinatorID string) (coordinator.WatchSet, error)
	// ActionPending reports whether the session has a pending interaction.
	ActionPending func(ctx context.Context, sessionID string) (bool, error)
}

// Ledger is the recorder, call queue, active-turn map and reader.
type Ledger struct {
	deps Deps
	log  *zap.Logger
	now  func() time.Time

	retryEvery time.Duration

	activeMu sync.Mutex
	active   map[string]activeEntry

	calls   callQueue
	truncMu sync.Mutex
	events  chan turnJob

	lifeMu  sync.Mutex
	stopped bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

type activeEntry struct {
	rowID         string
	sessionTurnID string
	startedAt     time.Time
}

// New builds a ledger. Call Start to run the call writer.
func New(deps Deps) *Ledger {
	l := &Ledger{
		deps:       deps,
		log:        deps.Log,
		now:        deps.Now,
		retryEvery: time.Second,
		active:     make(map[string]activeEntry),
		stopCh:     make(chan struct{}),
		events:     make(chan turnJob, turnEventQueueSize),
	}
	if l.log == nil {
		l.log = zap.NewNop()
	}
	if l.now == nil {
		l.now = func() time.Time { return time.Now().UTC() }
	}
	l.calls.init()
	return l
}

// Start runs the call writer and rebuilds the active-turn map from unfinished
// rows. It returns nothing the caller must act on: a failed rebuild is counted
// and logged.
func (l *Ledger) Start(ctx context.Context) {
	l.safe(StageStart, func() error { return l.rebuildActive(ctx) })
	l.wg.Add(3)
	go l.runCallWriter()
	go l.runJobs()
	go l.runTurnEvents()
}

// Stop ends the writer and waits for in-flight retries; it is idempotent.
func (l *Ledger) Stop() {
	l.lifeMu.Lock()
	if l.stopped {
		l.lifeMu.Unlock()
		return
	}
	l.stopped = true
	close(l.stopCh)
	l.lifeMu.Unlock()
	l.wg.Wait()
}

// safe runs fn, recovering a panic and counting a failure under stage. It
// returns nothing to its caller, so a recorder fault never reaches a turn.
func (l *Ledger) safe(stage string, fn func() error) {
	defer func() {
		if r := recover(); r != nil {
			countFailure(stage)
			l.log.Error("coordinator ledger: panic", zap.String("stage", stage), zap.Any("panic", r))
		}
	}()
	if err := fn(); err != nil {
		countFailure(stage)
		l.log.Warn("coordinator ledger: write failed", zap.String("stage", stage), zap.Error(err))
	}
}

// ActiveTurnID returns the open ledger row id of the session's current turn, or
// "" when none is known. It never touches the database.
func (l *Ledger) ActiveTurnID(sessionID string) string {
	l.activeMu.Lock()
	defer l.activeMu.Unlock()
	return l.active[sessionID].rowID
}

// setActive installs an entry unless it belongs to a later turn than e.
func (l *Ledger) setActive(sessionID string, e activeEntry) {
	l.activeMu.Lock()
	defer l.activeMu.Unlock()
	if cur, ok := l.active[sessionID]; ok && cur.sessionTurnID != e.sessionTurnID && cur.startedAt.After(e.startedAt) {
		return
	}
	l.active[sessionID] = e
}

func (l *Ledger) activeEntryFor(sessionID string) (activeEntry, bool) {
	l.activeMu.Lock()
	defer l.activeMu.Unlock()
	e, ok := l.active[sessionID]
	return e, ok
}

// swapActive replaces the entry only while it still holds (rowID,
// sessionTurnID) of from.
func (l *Ledger) swapActive(sessionID string, from, to activeEntry) {
	l.activeMu.Lock()
	defer l.activeMu.Unlock()
	if cur, ok := l.active[sessionID]; ok && cur.rowID == from.rowID && cur.sessionTurnID == from.sessionTurnID {
		l.active[sessionID] = to
	}
}

// dropActive removes the entry only while it holds (rowID, sessionTurnID); an
// empty rowID matches on the session turn alone.
func (l *Ledger) dropActive(sessionID, rowID, sessionTurnID string) {
	l.activeMu.Lock()
	defer l.activeMu.Unlock()
	cur, ok := l.active[sessionID]
	if !ok || cur.sessionTurnID != sessionTurnID || (rowID != "" && cur.rowID != rowID) {
		return
	}
	delete(l.active, sessionID)
}

func (l *Ledger) rebuildActive(ctx context.Context) error {
	var rows []struct {
		ID            string    `db:"id"`
		SessionID     string    `db:"session_id"`
		SessionTurnID string    `db:"session_turn_id"`
		StartedAt     time.Time `db:"started_at"`
	}
	if err := l.deps.RO.SelectContext(ctx, &rows, l.deps.RO.Rebind(`
		SELECT id, session_id, session_turn_id, started_at FROM coordinator_turns
		WHERE finished_at IS NULL ORDER BY started_at, id`)); err != nil {
		return fmt.Errorf("rebuild active turns: %w", err)
	}
	for _, r := range rows {
		l.setActive(r.SessionID, activeEntry{rowID: r.ID, sessionTurnID: r.SessionTurnID, startedAt: r.StartedAt})
	}
	return nil
}
