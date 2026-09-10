package retention

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

// fakeAfter is a deterministic stand-in for time.After, keyed by duration:
// every call for the same duration returns the same channel, so a test can
// fire a specific timer (e.g. firstSweepDelay vs. the configured interval)
// without racing real wall-clock time. armed records every duration the
// scheduler has requested, in order, so a test can prove which timer was
// armed and when — including proving RunCensus already completed
// synchronously before the following after() call.
type fakeAfter struct {
	mu    sync.Mutex
	chans map[time.Duration]chan time.Time
	armed chan time.Duration
}

func newFakeAfter() *fakeAfter {
	return &fakeAfter{chans: map[time.Duration]chan time.Time{}, armed: make(chan time.Duration, 64)}
}

func (f *fakeAfter) after(d time.Duration) <-chan time.Time {
	f.mu.Lock()
	ch, ok := f.chans[d]
	if !ok {
		ch = make(chan time.Time, 1)
		f.chans[d] = ch
	}
	f.mu.Unlock()
	f.armed <- d
	return ch
}

func (f *fakeAfter) fire(t *testing.T, d time.Duration) {
	t.Helper()
	f.mu.Lock()
	ch, ok := f.chans[d]
	f.mu.Unlock()
	if !ok {
		t.Fatalf("fire: no timer ever armed for duration %v", d)
	}
	ch <- time.Now()
}

// waitArmed blocks until after() has been called with duration d,
// draining (and discarding) any other durations seen along the way.
func (f *fakeAfter) waitArmed(t *testing.T, d time.Duration) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case got := <-f.armed:
			if got == d {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for a timer to be armed at %v", d)
		}
	}
}

// assertNotArmed drains any pending arm notifications and fails if d is
// among them.
func (f *fakeAfter) assertNotArmed(t *testing.T, d time.Duration) {
	t.Helper()
	for {
		select {
		case got := <-f.armed:
			if got == d {
				t.Fatalf("timer armed at %v, want it never armed", d)
			}
		default:
			return
		}
	}
}

func newTestScheduler(t *testing.T, settings Settings, fake *fakeAfter) (*Scheduler, *Sweeper) {
	t.Helper()
	sweeper, _ := newTestSweeper(t)
	if _, err := sweeper.settingsStore.SaveSettings(context.Background(), settings); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	scheduler := NewScheduler(sweeper.settingsStore, sweeper, SchedulerOptions{After: fake.after})
	return scheduler, sweeper
}

func TestScheduler_RunsCensusAtStartRegardlessOfEnabled(t *testing.T) {
	fake := newFakeAfter()
	settings := DefaultSettings()
	settings.Enabled = false
	scheduler, sweeper := newTestScheduler(t, settings, fake)

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer scheduler.Stop()

	// The next after() call is census's re-arm, issued only once RunCensus
	// (called synchronously beforehand in run()) has returned.
	fake.waitArmed(t, sweepInterval(settings))

	counts := sweeper.CensusSnapshot()
	if counts.OfficeRoutineRuns.State != CensusFresh {
		t.Fatalf("office_routine_runs census state = %v, want fresh (disabled must not block the census)", counts.OfficeRoutineRuns.State)
	}
	if counts.Runs.State != CensusFresh || counts.RunEvents.State != CensusFresh {
		t.Fatalf("census not fresh for every table: %+v", counts)
	}
}

func TestScheduler_SweepArmedAtFirstDelayWhenEnabledAtStart(t *testing.T) {
	fake := newFakeAfter()
	settings := DefaultSettings()
	settings.Enabled = true
	scheduler, _ := newTestScheduler(t, settings, fake)

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer scheduler.Stop()

	fake.waitArmed(t, firstSweepDelay)
}

func TestScheduler_SweepNotArmedWhenDisabledAtStart(t *testing.T) {
	fake := newFakeAfter()
	settings := DefaultSettings()
	settings.Enabled = false
	scheduler, _ := newTestScheduler(t, settings, fake)

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer scheduler.Stop()

	fake.waitArmed(t, sweepInterval(settings)) // census's arm proves the loop is running
	fake.assertNotArmed(t, firstSweepDelay)
}

func TestScheduler_SweepFiresAndReArmsAtFullIntervalNotFirstDelayAgain(t *testing.T) {
	fake := newFakeAfter()
	settings := DefaultSettings()
	settings.Enabled = true
	settings.SweepIntervalHours = 1
	scheduler, sweeper := newTestScheduler(t, settings, fake)

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer scheduler.Stop()

	fake.waitArmed(t, firstSweepDelay)
	fake.fire(t, firstSweepDelay)

	fake.waitArmed(t, sweepInterval(settings)) // re-armed at the full interval, not another 5-minute delay

	if _, ok := sweeper.LastSweepSnapshot(); !ok {
		t.Fatal("LastSweepSnapshot: ok = false, want true (the fired timer must have run a sweep)")
	}
}

func TestScheduler_EnablingFromDisabledArmsAtFirstDelay(t *testing.T) {
	fake := newFakeAfter()
	settings := DefaultSettings()
	settings.Enabled = false
	scheduler, _ := newTestScheduler(t, settings, fake)

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer scheduler.Stop()
	fake.waitArmed(t, sweepInterval(settings))

	enabled := settings
	enabled.Enabled = true
	scheduler.ApplySettings(enabled)

	fake.waitArmed(t, firstSweepDelay)
}

func TestScheduler_SettingsChangeWhileEnabledReArmsAtNewIntervalNotFirstDelay(t *testing.T) {
	fake := newFakeAfter()
	settings := DefaultSettings()
	settings.Enabled = true
	settings.SweepIntervalHours = 1
	scheduler, _ := newTestScheduler(t, settings, fake)

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer scheduler.Stop()
	fake.waitArmed(t, firstSweepDelay) // initial arm; timer never fired

	changed := settings
	changed.SweepIntervalHours = 2
	scheduler.ApplySettings(changed)

	fake.waitArmed(t, sweepInterval(changed)) // re-armed at the new interval, not firstSweepDelay again
}

func TestScheduler_DisablingStopsArmingSweepButNotCensus(t *testing.T) {
	fake := newFakeAfter()
	settings := DefaultSettings()
	settings.Enabled = true
	scheduler, _ := newTestScheduler(t, settings, fake)

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer scheduler.Stop()
	fake.waitArmed(t, firstSweepDelay)

	disabled := settings
	disabled.Enabled = false
	scheduler.ApplySettings(disabled)

	fake.waitArmed(t, sweepInterval(disabled)) // census's re-arm proves the wake was processed
	fake.assertNotArmed(t, firstSweepDelay)
}

func TestScheduler_StartTwiceIsNoop(t *testing.T) {
	fake := newFakeAfter()
	settings := DefaultSettings()
	settings.Enabled = false
	scheduler, _ := newTestScheduler(t, settings, fake)

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer scheduler.Stop()
	fake.waitArmed(t, sweepInterval(settings))

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	// A second run() goroutine would double-arm; draining once more must
	// time out rather than find another immediate arm.
	select {
	case d := <-fake.armed:
		t.Fatalf("second Start armed another timer at %v", d)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestScheduler_StartsOnDefaultsWhenStoredSettingsUnparseable proves
// AC-OFFICE-RUN-HISTORY-RETENTION-004.4: an unreadable stored settings
// document must not disable retention silently. GetSettings already falls
// back to DefaultSettings on such a document (see settings_store_test.go);
// this proves Start actually uses that fallback and runs the loop instead
// of aborting before the goroutine ever spawns.
func TestScheduler_StartsOnDefaultsWhenStoredSettingsUnparseable(t *testing.T) {
	fake := newFakeAfter()
	conn, err := sqlx.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := officesqlite.NewWithDB(conn, conn, nil); err != nil {
		t.Fatalf("init office schema: %v", err)
	}
	pool := db.NewPool(conn, conn)
	settingsRaw, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatalf("init settings schema: %v", err)
	}
	if err := settingsRaw.Save(context.Background(), settingsKey, []byte("not json")); err != nil {
		t.Fatalf("seed unparseable settings: %v", err)
	}

	settingsStore := NewSettingsStore(settingsRaw)
	sweeper := NewSweeper(pool, NewStore(pool), settingsStore, NewPreviewMarkerStore(settingsRaw))
	scheduler := NewScheduler(settingsStore, sweeper, SchedulerOptions{After: fake.after})

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer scheduler.Stop()

	// DefaultSettings().Enabled is true, so both timers must arm off the
	// fallback defaults rather than the loop never starting at all.
	fake.waitArmed(t, sweepInterval(DefaultSettings()))
	fake.waitArmed(t, firstSweepDelay)

	counts := sweeper.CensusSnapshot()
	if counts.OfficeRoutineRuns.State != CensusFresh {
		t.Fatalf("office_routine_runs census state = %v, want fresh (Start must run the census off defaults)", counts.OfficeRoutineRuns.State)
	}
}

func TestScheduler_StopJoinsWithoutHanging(t *testing.T) {
	fake := newFakeAfter()
	settings := DefaultSettings()
	settings.Enabled = true
	scheduler, _ := newTestScheduler(t, settings, fake)

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.waitArmed(t, firstSweepDelay)

	done := make(chan struct{})
	go func() {
		scheduler.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return")
	}
}
