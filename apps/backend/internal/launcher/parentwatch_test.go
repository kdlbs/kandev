package launcher

import (
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestParentWatchdogShutsDownWhenParentDies(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var alive atomic.Bool
		alive.Store(true)
		var shutdowns atomic.Int32
		var exits atomic.Int32
		watchdog := newParentWatchdog(1234, func(string) {
			shutdowns.Add(1)
		}, func(code int) {
			if code == 0 {
				exits.Add(1)
			}
		})
		watchdog.interval = time.Second
		watchdog.probe = func(int64) (bool, bool) {
			return alive.Load(), true
		}
		t.Cleanup(watchdog.stop)

		watchdog.start()
		time.Sleep(time.Second)
		synctest.Wait()
		if shutdowns.Load() != 0 {
			t.Fatal("watchdog shut down while parent was alive")
		}

		alive.Store(false)
		time.Sleep(time.Second)
		synctest.Wait()
		if shutdowns.Load() != 1 {
			t.Fatalf("shutdown count = %d, want 1", shutdowns.Load())
		}
		if exits.Load() != 1 {
			t.Fatalf("exit count = %d, want 1", exits.Load())
		}
	})
}

func TestParentWatchdogIgnoresUnknownLiveness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var shutdowns atomic.Int32
		var exits atomic.Int32
		watchdog := newParentWatchdog(1234, func(string) {
			shutdowns.Add(1)
		}, func(int) {
			exits.Add(1)
		})
		watchdog.interval = time.Second
		watchdog.probe = func(int64) (bool, bool) {
			return false, false
		}
		t.Cleanup(watchdog.stop)

		watchdog.start()
		time.Sleep(3 * time.Second)
		synctest.Wait()
		if shutdowns.Load() != 0 || exits.Load() != 0 {
			t.Fatalf("unknown liveness caused shutdown=%d exit=%d", shutdowns.Load(), exits.Load())
		}
	})
}

func TestParentWatchdogStopIsIdempotent(t *testing.T) {
	watchdog := newParentWatchdog(1234, func(string) {
		t.Fatal("unexpected shutdown")
	}, func(int) {
		t.Fatal("unexpected exit")
	})
	watchdog.probe = func(int64) (bool, bool) { return true, true }
	watchdog.start()
	watchdog.stop()
	watchdog.stop()
}

func TestParentPIDFromEnvRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "not-a-pid"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(parentPIDEnv, value)
			if got := parentPIDFromEnv(); got != 0 {
				t.Fatalf("parentPIDFromEnv() = %d for %q, want 0", got, value)
			}
		})
	}
}

func TestParentPIDFromEnvAcceptsPositivePID(t *testing.T) {
	t.Setenv(parentPIDEnv, "1234")
	if got := parentPIDFromEnv(); got != 1234 {
		t.Fatalf("parentPIDFromEnv() = %d, want 1234", got)
	}
}
