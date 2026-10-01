package recorder

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

const (
	scanQueueCap   = 1000
	chainCap       = 1000
	scanBatchSize  = 200
	endedChainKeep = time.Hour
)

var defaultRetryDelays = []time.Duration{2 * time.Minute, 10 * time.Minute, time.Hour}

// chain is the retry chain of one task.moved event: history rows are written
// asynchronously, so it rescans until a row lands in the event's window.
type chain struct {
	task         string
	at           time.Time
	attempt      int
	timerPending bool
	ended        bool
	endedAt      time.Time
	timer        *time.Timer
}

// Scanner runs moved-back scans of tasks on one worker, keyed by task id.
type Scanner struct {
	capture *Capture
	db      dbReader
	log     *zap.Logger
	now     func() time.Time
	delays  []time.Duration

	mu      sync.Mutex
	queued  map[string]bool
	order   chan string
	chains  map[string][]*chain
	active  int
	stopCh  chan struct{}
	stopped bool
	wg      sync.WaitGroup
}

// dbReader is the slice of sqlx.DB the scanner reads task activity through.
type dbReader interface {
	SelectContext(ctx context.Context, dest any, query string, args ...any) error
	Rebind(query string) string
}

// NewScanner builds a stopped scanner; db is the connection activity rows are
// read through.
func NewScanner(capture *Capture, db dbReader, log *zap.Logger) *Scanner {
	if log == nil {
		log = zap.NewNop()
	}
	return &Scanner{capture: capture, db: db, log: log, now: capture.now, delays: defaultRetryDelays,
		queued: map[string]bool{}, order: make(chan string, scanQueueCap), chains: map[string][]*chain{}, stopCh: make(chan struct{})}
}

// Start runs the scan worker and the daily scan of recently acted-on tasks.
func (s *Scanner) Start(ctx context.Context) {
	s.wg.Add(2)
	go s.run(ctx)
	go s.runDaily(ctx)
}

// Stop ends the worker, cancels every chain timer and waits; it is idempotent.
func (s *Scanner) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	close(s.stopCh)
	for _, chains := range s.chains {
		for _, c := range chains {
			if c.timer != nil {
				c.timer.Stop()
			}
		}
	}
	s.mu.Unlock()
	s.wg.Wait()
}

// EnqueueScan asks for a scan of taskID; a full queue drops it and counts it.
func (s *Scanner) EnqueueScan(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || s.queued[taskID] {
		return
	}
	select {
	case s.order <- taskID:
		s.queued[taskID] = true
	default:
		bump(scanDroppedTotal, ScanDroppedQueueFull)
	}
}

func (s *Scanner) run(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		case task := <-s.order:
			s.mu.Lock()
			delete(s.queued, task)
			s.mu.Unlock()
			s.scan(ctx, task)
		}
	}
}

func (s *Scanner) scan(ctx context.Context, taskID string) {
	defer func() {
		if r := recover(); r != nil {
			bump(observerPanicTotal, ObserverOverride)
			s.log.Error("coordinator override scan: panic", zap.String("task_id", taskID), zap.Any("panic", r))
		}
	}()
	times, err := s.capture.ScanTask(ctx, taskID)
	if err != nil {
		s.log.Warn("coordinator override scan failed", zap.String("task_id", taskID), zap.Error(err))
	}
	s.evaluate(taskID, times)
}

// OnTaskMoved starts a retry chain for a task.moved event of a task the
// coordinator created or moved, and scans it at once.
func (s *Scanner) OnTaskMoved(ctx context.Context, taskID string, at time.Time) {
	var one []int
	if err := s.db.SelectContext(ctx, &one, s.db.Rebind(`SELECT 1 FROM coordinator_activity
		WHERE target_task_id = ? AND outcome = 'approved' AND action_class IN ('create_task', 'move') LIMIT 1`), taskID); err != nil || len(one) == 0 {
		return
	}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.pruneEnded()
	if s.active >= chainCap {
		s.mu.Unlock()
		bump(scanDroppedTotal, ScanDroppedChainCap)
		return
	}
	s.chains[taskID] = append(s.chains[taskID], &chain{task: taskID, at: at})
	s.active++
	s.mu.Unlock()
	s.EnqueueScan(taskID)
}

func (s *Scanner) pruneEnded() {
	cutoff := s.now().Add(-endedChainKeep)
	for task, chains := range s.chains {
		kept := chains[:0]
		for _, c := range chains {
			if !c.ended || c.endedAt.After(cutoff) {
				kept = append(kept, c)
			}
		}
		if len(kept) == 0 {
			delete(s.chains, task)
		} else {
			s.chains[task] = kept
		}
	}
}

// evaluate ends the chains of taskID whose window holds a history row and
// schedules the next rescan of the others.
func (s *Scanner) evaluate(taskID string, rows []time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	chains := s.chains[taskID]
	for i, c := range chains {
		if c.ended {
			continue
		}
		var end time.Time
		for _, next := range chains[i+1:] {
			if next.at.After(c.at) && (end.IsZero() || next.at.Before(end)) {
				end = next.at
			}
		}
		if windowHasRow(rows, c.at, end) {
			s.endChain(c)
			continue
		}
		if c.timerPending {
			continue
		}
		if c.attempt >= len(s.delays) {
			s.endChain(c)
			continue
		}
		c.timerPending = true
		delay := s.delays[c.attempt]
		c.attempt++
		c.timer = time.AfterFunc(delay, func() { s.fire(c) })
	}
}

func windowHasRow(rows []time.Time, from, to time.Time) bool {
	for _, r := range rows {
		if !r.Before(from) && (to.IsZero() || r.Before(to)) {
			return true
		}
	}
	return false
}

func (s *Scanner) endChain(c *chain) {
	c.ended, c.endedAt = true, s.now()
	if c.timer != nil {
		c.timer.Stop()
	}
	c.timerPending = false
	s.active--
}

func (s *Scanner) fire(c *chain) {
	s.mu.Lock()
	c.timerPending = false
	ended := c.ended
	s.mu.Unlock()
	if !ended {
		s.EnqueueScan(c.task)
	}
}

// Subscribe starts a chain for every task.moved event. The returned function
// unsubscribes.
func (s *Scanner) Subscribe(eventBus bus.EventBus) (func(), error) {
	sub, err := eventBus.Subscribe(events.TaskMoved, func(ctx context.Context, e *bus.Event) error {
		data, _ := e.Data.(map[string]any)
		if id, _ := data["task_id"].(string); id != "" {
			s.OnTaskMoved(ctx, id, e.Timestamp)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { _ = sub.Unsubscribe() }) }, nil
}

func (s *Scanner) runDaily(ctx context.Context) {
	defer s.wg.Done()
	s.enqueueRecentTasks(ctx)
	t := time.NewTicker(sweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-t.C:
			s.enqueueRecentTasks(ctx)
		}
	}
}

// enqueueRecentTasks queues every task with a coordinator created or moved
// action in the last 30 days, in batches ordered by task id. It includes tasks
// whose outcome rows are final.
func (s *Scanner) enqueueRecentTasks(ctx context.Context) {
	since := s.now().Add(-moveBackWindow).UTC()
	cursor := ""
	for ctx.Err() == nil {
		var ids []string
		if err := s.db.SelectContext(ctx, &ids, s.db.Rebind(`SELECT DISTINCT target_task_id FROM coordinator_activity
			WHERE outcome = 'approved' AND action_class IN ('create_task', 'move') AND created_at >= ? AND target_task_id > ?
			ORDER BY target_task_id LIMIT ?`), since, cursor, scanBatchSize); err != nil {
			bump(readFailedTotal, ReadFailedHistoryRows)
			return
		}
		for _, id := range ids {
			s.EnqueueScan(id)
		}
		if len(ids) < scanBatchSize {
			return
		}
		cursor = ids[len(ids)-1]
	}
}
