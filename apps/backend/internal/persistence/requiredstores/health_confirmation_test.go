package requiredstores

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/startup"
	"github.com/kandev/kandev/internal/system/maintenance"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type healthLogCapture struct {
	observed *observer.ObservedLogs
	warnings chan string
	deferred chan struct{}
	recorded chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (capture *healthLogCapture) releaseWarnings() {
	capture.once.Do(func() { close(capture.release) })
}

func captureHealthLogs(t *testing.T, health *Health) *healthLogCapture {
	t.Helper()
	core, observed := observer.New(zapcore.DebugLevel)
	capture := &healthLogCapture{
		observed: observed,
		warnings: make(chan string, 4),
		deferred: make(chan struct{}, 1),
		recorded: make(chan struct{}, 1),
		release:  make(chan struct{}),
	}
	core = zapcore.RegisterHooks(core, func(entry zapcore.Entry) error {
		switch entry.Message {
		case "required persistence probe failed", "required persistence probe deadline unconfirmed":
			select {
			case capture.warnings <- entry.Message:
			default:
			}
			<-capture.release
		case "required persistence probe deferred during database maintenance":
			select {
			case capture.deferred <- struct{}{}:
			default:
			}
		case "required persistence state updated":
			select {
			case capture.recorded <- struct{}{}:
			default:
			}
		}
		return nil
	})
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	health.log = log
	return capture
}

func newHealthySQLiteHealth(t *testing.T) (*sqlx.DB, *db.Pool, *Tracker, *Health) {
	t.Helper()
	conn, pool, tracker, health := newSQLiteHealthFixture(t, []Descriptor{{
		ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"}, Sweep: startup.StepStoresRepositories,
	}})
	if _, err := conn.Exec("CREATE TABLE first (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatalf("create required table: %v", err)
	}
	if err := tracker.RecordSuccess("first"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	if err := health.Check(context.Background()); err != nil {
		t.Fatalf("initial Check: %v", err)
	}
	return conn, pool, tracker, health
}

func startHealthLoop(health *Health, interval time.Duration) (context.CancelFunc, <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go health.run(ctx, interval, done)
	return cancel, done
}

func waitForWarning(t *testing.T, capture *healthLogCapture) string {
	t.Helper()
	select {
	case warning := <-capture.warnings:
		return warning
	case <-time.After(5 * time.Second):
		t.Fatal("periodic loop did not report a probe failure")
		return ""
	}
}

func waitForRecordedProbe(t *testing.T, capture *healthLogCapture) {
	t.Helper()
	select {
	case <-capture.recorded:
	case <-time.After(5 * time.Second):
		t.Fatal("health tracker did not record the probe result")
	}
}

func stopHealthLoop(t *testing.T, cancel context.CancelFunc, done <-chan struct{}) {
	t.Helper()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("health loop did not stop after cancellation")
	}
}

func TestRuntimeHealthConfirmsDeadlineOnlyFailure(t *testing.T) {
	conn, _, tracker, health := newHealthySQLiteHealth(t)
	capture := captureHealthLogs(t, health)
	health.SetConfirmationDelay(time.Millisecond)
	initialStatus, _ := tracker.Status("first")
	blocked := holdWriter(t, conn)
	cancel, done := startHealthLoop(health, time.Millisecond)
	defer func() {
		capture.releaseWarnings()
		stopHealthLoop(t, cancel, done)
	}()

	if warning := waitForWarning(t, capture); warning != "required persistence probe deadline unconfirmed" {
		t.Fatalf("warning = %q, want unconfirmed deadline warning", warning)
	}
	status, _ := tracker.Status("first")
	if status.State != StateHealthy || !status.LastCheckedAt.Equal(initialStatus.LastCheckedAt) {
		t.Fatalf("first deadline was recorded before confirmation: initial=%#v current=%#v", initialStatus, status)
	}
	if err := blocked.Rollback(); err != nil {
		t.Fatalf("release blocked connection: %v", err)
	}
	capture.releaseWarnings()

	waitForRecordedProbe(t, capture)
	status, _ = tracker.Status("first")
	if !status.LastCheckedAt.After(initialStatus.LastCheckedAt) {
		t.Fatalf("confirmation timestamp = %s, want after %s", status.LastCheckedAt, initialStatus.LastCheckedAt)
	}
	if !health.Healthy() {
		t.Fatalf("health state after successful confirmation = %q, want healthy", health.State())
	}
	if entries := capture.observed.FilterMessage("required persistence probe deadline unconfirmed").All(); len(entries) != 1 {
		t.Fatalf("unconfirmed deadline warnings = %d, want one", len(entries))
	}
}

func TestRuntimeHealthRecordsFailedConfirmation(t *testing.T) {
	conn, _, tracker, health := newHealthySQLiteHealth(t)
	capture := captureHealthLogs(t, health)
	health.SetConfirmationDelay(time.Millisecond)
	_ = holdWriter(t, conn)
	cancel, done := startHealthLoop(health, time.Millisecond)
	defer func() {
		capture.releaseWarnings()
		stopHealthLoop(t, cancel, done)
	}()

	if warning := waitForWarning(t, capture); warning != "required persistence probe deadline unconfirmed" {
		t.Fatalf("first warning = %q, want unconfirmed deadline warning", warning)
	}
	capture.releaseWarnings()
	if warning := waitForWarning(t, capture); warning != "required persistence probe failed" {
		t.Fatalf("confirmation warning = %q, want probe failure", warning)
	}
	cancel()
	status, _ := tracker.Status("first")
	if status.State != StateUnhealthy || !strings.Contains(status.Error, "context deadline exceeded") {
		t.Fatalf("failed confirmation status = %#v, want its deadline diagnostic", status)
	}
	failed := capture.observed.FilterMessage("required persistence probe failed").All()
	if len(failed) != 1 {
		t.Fatalf("recorded probe failure warnings = %d, want one", len(failed))
	}
	fields := failed[0].ContextMap()
	if fields["stage"] != "writer_acquire" || fields["error_class"] != "deadline_exceeded" {
		t.Fatalf("confirmation diagnostics = %#v, want writer deadline", fields)
	}
	if entries := capture.observed.FilterMessage("required persistence probe deadline unconfirmed").All(); len(entries) != 1 {
		t.Fatalf("unconfirmed deadline warnings = %d, want one per confirmation cycle", len(entries))
	}
}

func TestRuntimeHealthCancellationDuringConfirmationWaitPreservesState(t *testing.T) {
	conn, _, tracker, health := newHealthySQLiteHealth(t)
	capture := captureHealthLogs(t, health)
	health.SetConfirmationDelay(time.Hour)
	initialStatus, _ := tracker.Status("first")
	_ = holdWriter(t, conn)
	cancel, done := startHealthLoop(health, time.Millisecond)
	defer func() {
		capture.releaseWarnings()
		stopHealthLoop(t, cancel, done)
	}()

	if warning := waitForWarning(t, capture); warning != "required persistence probe deadline unconfirmed" {
		t.Fatalf("warning = %q, want unconfirmed deadline warning", warning)
	}
	cancel()
	capture.releaseWarnings()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("health loop did not exit promptly from the confirmation wait")
	}
	status, _ := tracker.Status("first")
	if status.State != StateHealthy || !status.LastCheckedAt.Equal(initialStatus.LastCheckedAt) {
		t.Fatalf("canceled confirmation changed store status: initial=%#v current=%#v", initialStatus, status)
	}
	if entries := capture.observed.FilterMessage("required persistence probe failed").All(); len(entries) != 0 {
		t.Fatalf("canceled confirmation recorded a failure: %+v", entries)
	}
}

func TestRuntimeHealthDefersConfirmationDuringMaintenance(t *testing.T) {
	conn, pool, tracker, health := newHealthySQLiteHealth(t)
	capture := captureHealthLogs(t, health)
	health.SetConfirmationDelay(time.Millisecond)
	initialStatus, _ := tracker.Status("first")
	_ = holdWriter(t, conn)
	cancel, done := startHealthLoop(health, time.Millisecond)
	defer func() {
		capture.releaseWarnings()
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("health loop did not exit after cancellation")
		}
	}()

	if warning := waitForWarning(t, capture); warning != "required persistence probe deadline unconfirmed" {
		t.Fatalf("warning = %q, want unconfirmed deadline warning", warning)
	}
	releaseMaintenance, ok := maintenance.ForPool(pool).TryAcquire()
	if !ok {
		t.Fatal("maintenance lease unavailable after the first probe")
	}
	capture.releaseWarnings()
	select {
	case <-capture.deferred:
	case <-time.After(time.Second):
		t.Fatal("confirmation probe was not deferred during maintenance")
	}
	cancel()
	releaseMaintenance()
	status, _ := tracker.Status("first")
	if status.State != StateHealthy || !status.LastCheckedAt.Equal(initialStatus.LastCheckedAt) {
		t.Fatalf("deferred confirmation changed store status: initial=%#v current=%#v", initialStatus, status)
	}
}

func TestHealthConfirmsOnlyHealthyDeadlineOnlyResults(t *testing.T) {
	conn, _, tracker, health := newSQLiteHealthFixture(t, []Descriptor{
		{ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"}, Sweep: startup.StepStoresRepositories},
		{ID: "second", OwnerPackage: "owner/second", RequiredTables: []string{"second"}, Sweep: startup.StepStoresRepositories},
	})
	for _, table := range []string{"first", "second"} {
		if _, err := conn.Exec("CREATE TABLE " + table + " (id TEXT PRIMARY KEY)"); err != nil {
			t.Fatalf("create %s table: %v", table, err)
		}
		if err := tracker.RecordSuccess(table); err != nil {
			t.Fatalf("RecordSuccess(%s): %v", table, err)
		}
	}
	if err := health.Check(context.Background()); err != nil {
		t.Fatalf("second store Check: %v", err)
	}
	for _, test := range []struct {
		name    string
		results []error
		want    bool
	}{
		{name: "deadline-only", results: []error{nil, context.DeadlineExceeded}, want: true},
		{name: "mixed failures", results: []error{context.DeadlineExceeded, errors.New("driver error")}},
		{name: "missing table", results: []error{nil, errors.New(`required table "missing" is missing`)}},
		{name: "canceled", results: []error{context.Canceled, nil}},
		{name: "driver error", results: []error{errors.New("driver error"), nil}},
		{name: "missing pool", results: []error{errors.New("database pool is unavailable"), nil}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := health.shouldConfirm(probeResult{storeErrors: test.results}); got != test.want {
				t.Fatalf("shouldConfirm() = %v, want %v", got, test.want)
			}
		})
	}
	if err := tracker.RecordProbe("first", errors.New("previous failure")); err != nil {
		t.Fatalf("RecordProbe(first unhealthy): %v", err)
	}
	if health.shouldConfirm(probeResult{storeErrors: []error{context.DeadlineExceeded, nil}}) {
		t.Fatal("already unhealthy stores should not wait for confirmation")
	}
}

func TestRuntimeHealthRecordsBeforeMaintenanceCanStart(t *testing.T) {
	_, pool, _, health := newHealthySQLiteHealth(t)
	deferred, result, releaseProbe, err := health.probeRuntime(context.Background())
	if deferred || err != nil {
		t.Fatalf("runtime probe = deferred %v, err %v; want false, nil", deferred, err)
	}
	if releaseProbe == nil {
		t.Fatal("SQLite probe did not retain maintenance admission")
	}
	t.Cleanup(releaseProbe)

	if releaseMaintenance, ok := maintenance.ForPool(pool).TryAcquire(); ok {
		releaseMaintenance()
		t.Fatal("maintenance started before the runtime result was recorded")
	}
	if err := health.record(result); err != nil {
		t.Fatalf("record the runtime probe result: %v", err)
	}
	releaseProbe()

	releaseMaintenance, ok := maintenance.ForPool(pool).TryAcquire()
	if !ok {
		t.Fatal("maintenance could not start after the runtime result was recorded")
	}
	health.MarkUnavailable()
	releaseMaintenance()
	if health.Healthy() {
		t.Fatal("maintenance did not leave health unavailable after recording its transition")
	}
}
