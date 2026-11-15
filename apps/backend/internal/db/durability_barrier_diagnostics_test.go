package db

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

var errControlledRunFailure = errors.New("controlled durability run failure")

// testSlowRunThreshold makes every completed run qualify as slow without a
// wall-clock delay.
const testSlowRunThreshold = time.Nanosecond

func newBarrierLogFixture(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, observed := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	return log, observed
}

// waitForBarrierDiagnostics waits for the expected run records. A caller can
// return from Await before its owned goroutine emits the warning, so tests
// must not read the observer immediately after Await.
func waitForBarrierDiagnostics(t *testing.T, observed *observer.ObservedLogs, count int) []observer.LoggedEntry {
	t.Helper()
	require.Eventually(t, func() bool { return observed.Len() >= count },
		5*time.Second, 5*time.Millisecond, "expected %d run diagnostics", count)
	return observed.All()
}

// releaseGate closes a blocking test gate exactly once, from the success path
// or from cleanup.
func releaseGate(gate chan struct{}) func() {
	var once sync.Once
	return func() { once.Do(func() { close(gate) }) }
}

func requirePhaseField(t *testing.T, fields map[string]any, key string) {
	t.Helper()
	if _, ok := fields[key]; !ok {
		t.Fatalf("durability diagnostic omitted %q: %#v", key, fields)
	}
}

func requireAbsentField(t *testing.T, fields map[string]any, key string) {
	t.Helper()
	if value, ok := fields[key]; ok {
		t.Fatalf("durability diagnostic included unreached phase %q: %#v", key, value)
	}
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.8
func TestDurabilityBarrierRunDiagnosticPhases(t *testing.T) {
	t.Run("success records every reached phase and stays quiet", func(t *testing.T) {
		database := openBarrierTestDB(t)
		log, observed := newBarrierLogFixture(t)
		barrier := newDurabilityBarrier(database, 0, log)

		record := newDurabilityRunDiagnostic(log)
		require.NoError(t, barrier.commit(t.Context(), record))
		record.finish(nil)

		require.Equal(t, durabilityRunOutcomeSuccess, record.outcome)
		require.Equal(t, durabilityRunErrorNone, record.errorClass)
		require.Equal(t, durabilityRunStageComplete, record.stage)
		for name, phase := range map[string]*time.Duration{
			"acquire": record.acquire, "hold": record.hold,
			"begin": record.begin, "write": record.write, "commit": record.commit,
		} {
			if phase == nil {
				t.Fatalf("successful run omitted %s timing", name)
			}
		}
		record.logRun(durabilityRunSlowThreshold)
		if record.elapsed < durabilityRunSlowThreshold {
			require.Zero(t, observed.Len(), "a fast successful run must not log")
		}
	})

	t.Run("failed generation write omits the commit phase", func(t *testing.T) {
		database := openBarrierTestDB(t)
		_, err := database.Exec(`DROP TABLE kandev_meta`)
		require.NoError(t, err)
		log, observed := newBarrierLogFixture(t)
		barrier := newDurabilityBarrier(database, 0, log)

		record := newDurabilityRunDiagnostic(log)
		commitErr := barrier.commit(t.Context(), record)
		record.finish(commitErr)
		require.Error(t, commitErr)

		require.Equal(t, durabilityRunOutcomeError, record.outcome)
		require.Equal(t, durabilityRunErrorOperation, record.errorClass)
		require.Equal(t, durabilityRunStageGenerationWrite, record.stage)
		if record.acquire == nil || record.hold == nil || record.begin == nil || record.write == nil {
			t.Fatalf("failed run omitted a reached phase: %#v", record)
		}
		require.Nil(t, record.commit, "an unreached commit has no timing")

		record.logRun(durabilityRunSlowThreshold)
		entries := observed.All()
		require.Len(t, entries, 1)
		fields := entries[0].ContextMap()
		require.Equal(t, durabilityRunOperation, fields["operation"])
		require.Equal(t, durabilityRunOutcomeError, fields["outcome"])
		require.Equal(t, durabilityRunStageGenerationWrite, fields["stage"])
		require.Equal(t, durabilityRunErrorOperation, fields["error_class"])
		for _, key := range []string{"elapsed_ms", "acquire_ms", "hold_ms", "begin_ms", "write_ms"} {
			requirePhaseField(t, fields, key)
		}
		requireAbsentField(t, fields, "commit_ms")
		if _, ok := fields["error"]; ok {
			t.Fatalf("durability diagnostic exposed the raw error: %#v", fields)
		}
	})
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.8
func TestDurabilityBarrierRunDiagnosticAcquireTimeout(t *testing.T) {
	database := openBarrierTestDB(t)
	database.SetMaxOpenConns(1)
	held, err := database.Conn(t.Context())
	require.NoError(t, err)
	defer func() { require.NoError(t, held.Close()) }()

	log, _ := newBarrierLogFixture(t)
	barrier := newDurabilityBarrier(database, 0, log)
	record := newDurabilityRunDiagnostic(log)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	commitErr := barrier.commit(ctx, record)
	record.finish(commitErr)
	require.ErrorIs(t, commitErr, context.DeadlineExceeded)
	require.Equal(t, durabilityRunStageWriterAcquire, record.stage)
	require.Equal(t, durabilityRunErrorDeadline, record.errorClass)
	if record.acquire == nil {
		t.Fatal("acquisition timeout did not record acquire_ms")
	}
	require.Nil(t, record.hold, "an unacquired connection has no hold phase")
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.8
func TestDurabilityBarrierRunDiagnosticSlowSuccess(t *testing.T) {
	database := openBarrierTestDB(t)
	log, observed := newBarrierLogFixture(t)
	barrier := newDurabilityBarrier(database, 0, log)
	// Every completed run exceeds this threshold, so the success record is
	// exercised without a real-time wait.
	barrier.slowRunThreshold = testSlowRunThreshold

	require.NoError(t, barrier.Await(t.Context()))
	entries := waitForBarrierDiagnostics(t, observed, 1)
	require.Len(t, entries, 1, "a slow successful run emits one diagnostic")
	fields := entries[0].ContextMap()
	require.Equal(t, durabilityRunOutcomeSuccess, fields["outcome"])
	require.Equal(t, durabilityRunStageComplete, fields["stage"])
	require.Equal(t, durabilityRunErrorNone, fields["error_class"])
	for _, key := range []string{"elapsed_ms", "acquire_ms", "hold_ms", "begin_ms", "write_ms", "commit_ms"} {
		requirePhaseField(t, fields, key)
	}
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.8
func TestDurabilityBarrierRunDiagnosticCoalesces(t *testing.T) {
	database := openBarrierTestDB(t)
	log, observed := newBarrierLogFixture(t)
	barrier := newDurabilityBarrier(database, 150*time.Millisecond, log)
	barrier.slowRunThreshold = testSlowRunThreshold

	var runs atomic.Int32
	barrier.observe = func(context.Context, *sql.Conn) error {
		runs.Add(1)
		return nil
	}

	const callers = 4
	var registered atomic.Int32
	allRegistered := make(chan struct{})
	barrier.awaitHook = func() {
		if registered.Add(1) == callers {
			close(allRegistered)
		}
		// Hold every caller until all of them have registered, so no leader
		// can take its covered snapshot before the fourth arrival.
		<-allRegistered
	}

	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- barrier.Await(t.Context())
		}()
	}
	wg.Wait()
	close(errs)
	for callerErr := range errs {
		require.NoError(t, callerErr)
	}
	entries := waitForBarrierDiagnostics(t, observed, 1)
	require.Len(t, entries, 1, "coalesced callers share one run diagnostic")
	require.Equal(t, int32(1), runs.Load(), "coalesced callers share one actual run")
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.8
func TestDurabilityBarrierRunDiagnosticCancellation(t *testing.T) {
	database := openBarrierTestDB(t)
	log, observed := newBarrierLogFixture(t)
	barrier := newDurabilityBarrier(database, 0, log)

	var runs atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	releaseRun := releaseGate(release)
	t.Cleanup(releaseRun)
	barrier.observe = func(context.Context, *sql.Conn) error {
		if runs.Add(1) == 1 {
			close(started)
			<-release
		}
		return errControlledRunFailure
	}
	registered := make(chan struct{}, 2)
	barrier.awaitHook = func() { registered <- struct{}{} }

	canceledCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	canceledResult := make(chan error, 1)
	go func() { canceledResult <- barrier.Await(canceledCtx) }()
	// The canceled caller is the only caller so far, so it owns the run.
	<-registered
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("barrier run did not start")
	}

	followerResult := make(chan error, 1)
	go func() { followerResult <- barrier.Await(t.Context()) }()
	<-registered

	cancel()
	select {
	case callerErr := <-canceledResult:
		require.ErrorIs(t, callerErr, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("canceled caller did not stop waiting")
	}

	releaseRun()
	require.ErrorIs(t, <-followerResult, errControlledRunFailure,
		"the shared run keeps its own lifetime after a caller cancels")
	entries := waitForBarrierDiagnostics(t, observed, 2)
	require.Len(t, entries, 2, "each actual run emits exactly one diagnostic")
	require.Equal(t, int32(2), runs.Load())
	for _, entry := range entries {
		require.Equal(t, durabilityRunOutcomeError, entry.ContextMap()["outcome"])
	}
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.8
func TestDurabilityBarrierRunDiagnosticResultBeforeLogRelease(t *testing.T) {
	database := openBarrierTestDB(t)
	database.SetMaxOpenConns(1)
	hookEntered := make(chan struct{})
	releaseHook := make(chan struct{})
	releaseLog := releaseGate(releaseHook)
	t.Cleanup(releaseLog)
	var entered sync.Once
	core, observed := observer.New(zapcore.WarnLevel)
	core = zapcore.RegisterHooks(core, func(zapcore.Entry) error {
		entered.Do(func() { close(hookEntered) })
		<-releaseHook
		return nil
	})
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)

	barrier := newDurabilityBarrier(database, 0, log)
	barrier.observe = func(context.Context, *sql.Conn) error {
		return errControlledRunFailure
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- barrier.Await(ctx) }()

	select {
	case <-hookEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("run diagnostic was not emitted")
	}
	select {
	case runErr := <-result:
		require.ErrorIs(t, runErr, errControlledRunFailure,
			"the initiating caller must receive the run result, not its deadline")
	case <-time.After(2 * time.Second):
		t.Fatal("initiating caller waited for the blocked log sink")
	}

	probeCtx, probeCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer probeCancel()
	var value int
	require.NoError(t, database.QueryRowContext(probeCtx, "SELECT 1").Scan(&value),
		"the writer stays available while the diagnostic sink blocks")

	releaseLog()
	require.Len(t, waitForBarrierDiagnostics(t, observed, 1), 1)
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.8
func TestDurabilityBarrierRunDiagnosticReleaseBeforeLog(t *testing.T) {
	database := openBarrierTestDB(t)
	probeErr := make(chan error, 1)
	core, _ := observer.New(zapcore.WarnLevel)
	core = zapcore.RegisterHooks(core, func(zapcore.Entry) error {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		var value int
		probeErr <- database.QueryRowContext(ctx, "SELECT 1").Scan(&value)
		return nil
	})
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)

	barrier := newDurabilityBarrier(database, 0, log)
	barrier.observe = func(context.Context, *sql.Conn) error {
		return errControlledRunFailure
	}
	require.Error(t, barrier.Await(t.Context()))
	select {
	case err := <-probeErr:
		require.NoError(t, err, "the writer must be released before the diagnostic is emitted")
	case <-time.After(2 * time.Second):
		t.Fatal("diagnostic was not emitted")
	}
}
