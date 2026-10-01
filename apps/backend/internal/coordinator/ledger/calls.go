package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	callQueueSize  = 1000
	callCap        = 100
	callMaxRetries = 3
	drainTimeout   = 2 * time.Second
)

type callEntry struct {
	seq       uint64
	sessionID string
	action    string
	target    string
	allowed   bool
	at        time.Time
	attempts  int
}

type parkedCall struct {
	entry callEntry
	next  time.Time
}

// callQueue is the bounded in-process queue between the guarded-call layer and
// the single writer. seq is assigned under mu together with the send, so queue
// order is sequence order.
type callQueue struct {
	ch chan callEntry

	mu      sync.Mutex
	seq     uint64
	written uint64
	// advanced is closed and replaced each time written moves.
	advanced chan struct{}
	// truncated holds ledger rows whose digest lost a call to a full queue.
	truncated map[string]struct{}
	// parked is the retry list; read by the verdict and drain.
	parked map[string]int
}

func (q *callQueue) init() {
	q.ch = make(chan callEntry, callQueueSize)
	q.advanced = make(chan struct{})
	q.truncated = make(map[string]struct{})
	q.parked = make(map[string]int)
}

// Call appends one guarded-call decision to the digest queue. It never blocks,
// never returns an error and carries no arguments or results.
func (l *Ledger) Call(sessionID, action, targetTaskID string, allowed bool) {
	if sessionID == "" {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			countFailure(StageCall)
		}
	}()
	q := &l.calls
	q.mu.Lock()
	q.seq++
	e := callEntry{seq: q.seq, sessionID: sessionID, action: action, target: targetTaskID, allowed: allowed, at: l.now()}
	select {
	case q.ch <- e:
		q.mu.Unlock()
		return
	default:
	}
	q.written = e.seq
	q.signalLocked()
	if entry, ok := l.activeEntryFor(sessionID); ok {
		q.truncated[entry.rowID] = struct{}{}
	}
	q.mu.Unlock()
	countFailure(StageCallQueueFull)
}

func (q *callQueue) signalLocked() {
	close(q.advanced)
	q.advanced = make(chan struct{})
}

func (q *callQueue) settle(seq uint64) {
	q.mu.Lock()
	if seq > q.written {
		q.written = seq
		q.signalLocked()
	}
	q.mu.Unlock()
}

func (q *callQueue) takeTruncated() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.truncated) == 0 {
		return nil
	}
	out := make([]string, 0, len(q.truncated))
	for id := range q.truncated {
		out = append(out, id)
	}
	q.truncated = make(map[string]struct{})
	return out
}

func (q *callQueue) retryPending(sessionID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.parked[sessionID] > 0
}

func (q *callQueue) setParked(sessionID string, delta int) {
	q.mu.Lock()
	q.parked[sessionID] += delta
	if q.parked[sessionID] <= 0 {
		delete(q.parked, sessionID)
	}
	q.mu.Unlock()
}

// drainCalls waits until every call enqueued before it started was written,
// dropped or parked, for at most drainTimeout. It runs on the completion
// handler, never on a turn.
func (l *Ledger) drainCalls(ctx context.Context) {
	q := &l.calls
	q.mu.Lock()
	target := q.seq
	q.mu.Unlock()
	deadline := time.NewTimer(drainTimeout)
	defer deadline.Stop()
	for {
		q.mu.Lock()
		done, wait := q.written >= target, q.advanced
		q.mu.Unlock()
		if done {
			return
		}
		select {
		case <-wait:
		case <-deadline.C:
			return
		case <-ctx.Done():
			return
		case <-l.stopCh:
			return
		}
	}
}

func (l *Ledger) runCallWriter() {
	defer l.wg.Done()
	var parked []parkedCall
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		l.arm(timer, parked)
		select {
		case <-l.stopCh:
			return
		case e := <-l.calls.ch:
			parked = l.writeOrPark(e, parked)
			l.applyTruncations()
		case <-timer.C:
			parked = l.retryDue(parked)
			l.applyTruncations()
		}
	}
}

func (l *Ledger) arm(timer *time.Timer, parked []parkedCall) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	d := time.Hour
	if len(parked) > 0 {
		d = parked[0].next.Sub(l.now())
		for _, p := range parked {
			if w := p.next.Sub(l.now()); w < d {
				d = w
			}
		}
		if d < 0 {
			d = 0
		}
	}
	timer.Reset(d)
}

func (l *Ledger) retryDue(parked []parkedCall) []parkedCall {
	now := l.now()
	var keep []parkedCall
	for _, p := range parked {
		if p.next.After(now) {
			keep = append(keep, p)
			continue
		}
		keep = l.writeOrPark(p.entry, keep)
	}
	return keep
}

// writeOrPark resolves the entry's turn at write time. An entry that resolves
// to no turn moves to the retry list, so a start race never stalls the queue.
func (l *Ledger) writeOrPark(e callEntry, parked []parkedCall) []parkedCall {
	first := e.attempts == 0
	if first {
		defer l.calls.settle(e.seq)
	}
	var turnID string
	l.safe(StageCall, func() error {
		id, err := l.resolveTurn(context.Background(), e)
		if err != nil {
			return err
		}
		turnID = id
		if id == "" {
			return nil
		}
		return l.insertCall(context.Background(), id, e)
	})
	if turnID != "" {
		if !first {
			l.calls.setParked(e.sessionID, -1)
		}
		return parked
	}
	if e.attempts >= callMaxRetries {
		l.calls.setParked(e.sessionID, -1)
		countFailure(StageCallUnattributed)
		return parked
	}
	if first {
		l.calls.setParked(e.sessionID, 1)
	}
	e.attempts++
	return append(parked, parkedCall{entry: e, next: l.now().Add(l.retryEvery)})
}

func (l *Ledger) resolveTurn(ctx context.Context, e callEntry) (string, error) {
	var id string
	err := l.deps.DB.GetContext(ctx, &id, l.deps.DB.Rebind(`
		SELECT id FROM coordinator_turns
		WHERE session_id = ? AND started_at <= ? AND (finished_at IS NULL OR finished_at >= ?)
		ORDER BY started_at DESC, id DESC LIMIT 1`), e.sessionID, e.at, e.at)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve call turn: %w", err)
	}
	return id, nil
}

// insertCall is single-writer, so the count-then-insert of the call cap cannot
// race and call ids follow recording order.
func (l *Ledger) insertCall(ctx context.Context, turnID string, e callEntry) error {
	var n int
	if err := l.deps.DB.GetContext(ctx, &n, l.deps.DB.Rebind(`SELECT COUNT(*) FROM coordinator_turn_calls WHERE turn_id = ?`), turnID); err != nil {
		return fmt.Errorf("count calls: %w", err)
	}
	if n >= callCap {
		_, err := l.deps.DB.ExecContext(ctx, l.deps.DB.Rebind(`UPDATE coordinator_turns SET calls_truncated = ? WHERE id = ?`), true, turnID)
		return err
	}
	var target any
	if e.target != "" {
		target = e.target
	}
	_, err := l.deps.DB.ExecContext(ctx, l.deps.DB.Rebind(`
		INSERT INTO coordinator_turn_calls (turn_id, action, target_task_id, allowed, recorded_at) VALUES (?, ?, ?, ?, ?)`),
		turnID, e.action, target, e.allowed, e.at.UTC())
	return err
}

// applyTruncations persists the calls_truncated marks a full queue left.
func (l *Ledger) applyTruncations() {
	l.truncMu.Lock()
	defer l.truncMu.Unlock()
	for _, id := range l.calls.takeTruncated() {
		l.safe(StageCall, func() error {
			_, err := l.deps.DB.ExecContext(context.Background(), l.deps.DB.Rebind(`UPDATE coordinator_turns SET calls_truncated = ? WHERE id = ?`), true, id)
			return err
		})
	}
}
