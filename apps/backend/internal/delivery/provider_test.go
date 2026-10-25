package delivery

import (
	"github.com/kandev/kandev/internal/startup"
	"testing"
)

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-001.2
func TestHousekeepingLedgerIsInitializedBeforeSweepActivation(t *testing.T) {
	_, db := newInternalTestRepo(t)
	reporter := startup.New(nil)
	ctx := startup.WithReporter(t.Context(), reporter)
	sweep, stop, err := ProvideWithContext(ctx, db, db, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stop(); err != nil {
			t.Error(err)
		}
	}()
	sweep.mu.Lock()
	started := sweep.started
	sweep.mu.Unlock()
	if started {
		t.Fatal("delivery sweep started before housekeeping completed")
	}
	reporter.Set(startup.Ready)
	if err := startup.Activate(ctx); err != nil {
		t.Fatal(err)
	}
	sweep.mu.Lock()
	started = sweep.started
	sweep.mu.Unlock()
	if !started {
		t.Fatal("delivery sweep never activated")
	}
}
