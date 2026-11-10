package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

const (
	// durabilityBarrierWindow is the longest a caller waits for other callers
	// to join the run it is about to lead. It is part of the ACK latency
	// budget, not a throughput knob.
	durabilityBarrierWindow = 10 * time.Millisecond

	// durabilityBarrierTimeout bounds a run that must finish even when the
	// caller that started it is canceled, because other callers can depend on
	// its outcome.
	durabilityBarrierTimeout = 30 * time.Second

	// durabilityBarrierMetaKey is the kandev_meta key touched by every run.
	// internal/persistence owns that table; the barrier only needs a real row
	// change so SQLite writes a WAL frame.
	durabilityBarrierMetaKey = "durability_barrier"

	// durabilityBarrierGenerationSQL upserts a monotonically increasing value.
	durabilityBarrierGenerationSQL = `INSERT INTO kandev_meta (key, value) VALUES (?, '1')
		ON CONFLICT(key) DO UPDATE SET
			value = CAST(CAST(kandev_meta.value AS BIGINT) + 1 AS TEXT)`
)

// DurabilityBarrier makes every earlier commit on a WAL writer durable across
// a host power loss. The writer DSN uses synchronous=NORMAL; Await commits one
// FULL transaction on the same connection, which synchronizes the whole WAL
// file and therefore every earlier NORMAL commit.
//
// Concurrent callers coalesce into one run. A caller that arrives while a run
// is in flight waits for the next run, because the in-flight run may have
// started before that caller's commit. Each run executes in a short-lived
// goroutine, so a canceled caller can stop waiting without canceling a run
// other callers depend on; the barrier owns no long-lived goroutine.
type DurabilityBarrier struct {
	db     *sql.DB
	window time.Duration
	log    *logger.Logger

	// slowRunThreshold is the fixed production threshold. Tests may lower it to
	// exercise the slow-run record without waiting on wall-clock time.
	slowRunThreshold time.Duration

	// observe is a test seam that runs after the FULL pragma is applied and
	// before the barrier commit. It never runs in production.
	observe func(context.Context, *sql.Conn) error
	// awaitHook is a test seam that runs after a caller registers its ticket
	// and before it waits. It never runs in production.
	awaitHook func()

	mu       sync.Mutex
	running  bool
	arrivals uint64
	covered  uint64
	changed  chan struct{}
}

// NewDurabilityBarrier returns a barrier over the shared SQLite writer pool.
// log receives at most one warning for each failed or slow run; nil keeps the
// barrier silent.
func NewDurabilityBarrier(database *sql.DB, log *logger.Logger) *DurabilityBarrier {
	return newDurabilityBarrier(database, durabilityBarrierWindow, log)
}

func newDurabilityBarrier(database *sql.DB, window time.Duration, log *logger.Logger) *DurabilityBarrier {
	if window < 0 {
		window = 0
	}
	return &DurabilityBarrier{
		db:               database,
		window:           window,
		log:              log,
		slowRunThreshold: durabilityRunSlowThreshold,
		changed:          make(chan struct{}),
	}
}

// Await returns after a FULL commit that started after the call. The returned
// error means no such commit is proven; every caller must keep its pending
// progress and retry.
func (b *DurabilityBarrier) Await(ctx context.Context) error {
	if b == nil || b.db == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	ticket := b.arrivals
	b.arrivals++
	hook := b.awaitHook
	b.mu.Unlock()
	if hook != nil {
		hook()
	}
	b.mu.Lock()
	for {
		if b.covered > ticket {
			b.mu.Unlock()
			return nil
		}
		if !b.running {
			b.running = true
			b.mu.Unlock()
			return b.lead(ctx)
		}
		changed := b.changed
		b.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
		b.mu.Lock()
	}
}

// lead starts one coalesced run and lets this caller stop waiting when its own
// context ends. The run keeps its independent lifetime so other callers that
// depend on it are not canceled with this one. The run result reaches the
// caller before the diagnostic is emitted, so a slow log sink cannot replace
// the real outcome with a caller deadline.
func (b *DurabilityBarrier) lead(ctx context.Context) error {
	done := make(chan error, 1)
	go func() {
		record, err := b.run()
		done <- err
		record.logRun(b.slowRunThreshold)
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// run waits the coalescing window and commits one FULL generation. It is
// short-lived and owns no state beyond the barrier's fields. The caller
// publishes the returned result before emitting the record's diagnostic.
func (b *DurabilityBarrier) run() (*durabilityRunDiagnostic, error) {
	timer := time.NewTimer(b.window)
	<-timer.C

	b.mu.Lock()
	covered := b.arrivals
	b.mu.Unlock()

	// The record starts after the coalescing window: it describes this shared
	// run, not the time a caller spent waiting for it.
	record := newDurabilityRunDiagnostic(b.log)
	ctx, cancel := context.WithTimeout(context.Background(), durabilityBarrierTimeout)
	err := b.commit(ctx, record)
	cancel()
	record.finish(err)

	b.mu.Lock()
	if err == nil && covered > b.covered {
		b.covered = covered
	}
	b.running = false
	close(b.changed)
	b.changed = make(chan struct{})
	b.mu.Unlock()

	return record, err
}

func (b *DurabilityBarrier) commit(ctx context.Context, record *durabilityRunDiagnostic) error {
	acquireStarted := time.Now()
	conn, err := b.db.Conn(ctx)
	record.recordAcquire(time.Since(acquireStarted))
	if err != nil {
		record.stage = durabilityRunStageWriterAcquire
		return fmt.Errorf("acquire durability barrier writer: %w", err)
	}
	holdStarted := time.Now()
	var discard bool
	defer func() {
		releaseDurabilityBarrierConnection(conn, discard)
		holdDuration := time.Since(holdStarted)
		record.hold = &holdDuration
	}()
	discard, err = b.commitAcquired(ctx, conn, record)
	return err
}

// commitAcquired runs the FULL transaction phases on an acquired connection.
// It reports whether the connection must be discarded instead of returning to
// the pool.
func (b *DurabilityBarrier) commitAcquired(
	ctx context.Context,
	conn *sql.Conn,
	record *durabilityRunDiagnostic,
) (discard bool, err error) {
	record.stage = durabilityRunStageSetFull
	if _, err := conn.ExecContext(ctx, "PRAGMA synchronous=FULL"); err != nil {
		return true, fmt.Errorf("set durability barrier synchronous level: %w", err)
	}
	if b.observe != nil {
		if err := b.observe(ctx, conn); err != nil {
			return true, err
		}
	}
	if err := commitDurabilityBarrierGeneration(ctx, conn, record); err != nil {
		return true, err
	}
	record.stage = durabilityRunStageRestoreNormal
	if _, err := conn.ExecContext(ctx, "PRAGMA synchronous=NORMAL"); err != nil {
		return true, fmt.Errorf("restore writer synchronous level: %w", err)
	}
	record.stage = durabilityRunStageComplete
	return false, nil
}

func releaseDurabilityBarrierConnection(conn *sql.Conn, discard bool) {
	if discard {
		// Return the connection to the pool as bad so it is reopened with
		// the NORMAL writer DSN instead of a leftover FULL setting.
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	}
	_ = conn.Close()
}

func commitDurabilityBarrierGeneration(
	ctx context.Context,
	conn *sql.Conn,
	record *durabilityRunDiagnostic,
) error {
	beginStarted := time.Now()
	tx, err := conn.BeginTx(ctx, nil)
	beginDuration := time.Since(beginStarted)
	record.begin = &beginDuration
	if err != nil {
		record.stage = durabilityRunStageBegin
		return fmt.Errorf("begin durability barrier: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	writeStarted := time.Now()
	_, err = tx.ExecContext(ctx, durabilityBarrierGenerationSQL, durabilityBarrierMetaKey)
	writeDuration := time.Since(writeStarted)
	record.write = &writeDuration
	if err != nil {
		record.stage = durabilityRunStageGenerationWrite
		return fmt.Errorf("write durability barrier generation: %w", err)
	}
	commitStarted := time.Now()
	err = tx.Commit()
	commitDuration := time.Since(commitStarted)
	record.commit = &commitDuration
	if err != nil {
		record.stage = durabilityRunStageCommit
		return fmt.Errorf("commit durability barrier: %w", err)
	}
	return nil
}
