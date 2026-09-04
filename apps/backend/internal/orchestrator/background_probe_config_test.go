package orchestrator

import (
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	commonlogger "github.com/kandev/kandev/internal/common/logger"
)

func observingTestLogger(t *testing.T) (*commonlogger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, observed := observer.New(zap.WarnLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log, observed
}

func TestLoadBackgroundProbeConfig_Defaults(t *testing.T) {
	cfg := LoadBackgroundProbeConfig(nil)

	if cfg.Budget != defaultParkedProbeBudget {
		t.Errorf("Budget = %v, want default %v", cfg.Budget, defaultParkedProbeBudget)
	}
	if cfg.Interval != defaultParkedProbeInterval {
		t.Errorf("Interval = %v, want default %v", cfg.Interval, defaultParkedProbeInterval)
	}
}

func TestLoadBackgroundProbeConfig_ValidValuesRespected(t *testing.T) {
	t.Setenv(envParkedProbeBudget, "500ms")
	t.Setenv(envParkedProbeInterval, "10s")

	cfg := LoadBackgroundProbeConfig(nil)

	if cfg.Budget != 500*time.Millisecond {
		t.Errorf("Budget = %v, want 500ms", cfg.Budget)
	}
	if cfg.Interval != 10*time.Second {
		t.Errorf("Interval = %v, want 10s", cfg.Interval)
	}
}

// AC-81: a zero or negative budget is a deliberate exception to the
// "0 disables" idiom — it would enable an unbounded blocking call on the
// turn-settle path — so both are rejected, default used, warning logged.
func TestLoadBackgroundProbeConfig_ZeroOrNegativeBudgetRejected(t *testing.T) {
	for _, raw := range []string{"0", "0s", "-1s"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv(envParkedProbeBudget, raw)
			log, observed := observingTestLogger(t)

			cfg := LoadBackgroundProbeConfig(log)

			if cfg.Budget != defaultParkedProbeBudget {
				t.Errorf("Budget = %v, want default %v for input %q", cfg.Budget, defaultParkedProbeBudget, raw)
			}
			if observed.Len() == 0 {
				t.Error("expected a warning to be logged for a rejected budget value")
			}
		})
	}
}

// Round-5 F10: KANDEV_PARKED_PROBE_INTERVAL=0 is valid and meaningful
// (disables periodic sampling) — unlike Budget, zero must NOT be rejected.
func TestLoadBackgroundProbeConfig_ZeroIntervalAccepted(t *testing.T) {
	t.Setenv(envParkedProbeInterval, "0")
	log, observed := observingTestLogger(t)

	cfg := LoadBackgroundProbeConfig(log)

	if cfg.Interval != 0 {
		t.Errorf("Interval = %v, want 0", cfg.Interval)
	}
	if observed.Len() != 0 {
		t.Errorf("expected no warning for a valid zero interval, got %d", observed.Len())
	}
}

// Round-5 F10: time.NewTicker panics on a non-positive duration, so a
// negative interval must be rejected the same way as Budget.
func TestLoadBackgroundProbeConfig_NegativeIntervalRejected(t *testing.T) {
	t.Setenv(envParkedProbeInterval, "-5s")
	log, observed := observingTestLogger(t)

	cfg := LoadBackgroundProbeConfig(log)

	if cfg.Interval != defaultParkedProbeInterval {
		t.Errorf("Interval = %v, want default %v", cfg.Interval, defaultParkedProbeInterval)
	}
	if observed.Len() == 0 {
		t.Error("expected a warning to be logged for a rejected negative interval")
	}
}

func TestLoadBackgroundProbeConfig_UnparseableValueFallsBackToDefault(t *testing.T) {
	t.Setenv(envParkedProbeBudget, "not-a-duration")
	log, observed := observingTestLogger(t)

	cfg := LoadBackgroundProbeConfig(log)

	if cfg.Budget != defaultParkedProbeBudget {
		t.Errorf("Budget = %v, want default %v", cfg.Budget, defaultParkedProbeBudget)
	}
	if observed.Len() == 0 {
		t.Error("expected a warning to be logged for an unparseable value")
	}
}

func TestLoadBackgroundProbeConfig_NilLoggerDoesNotPanic(t *testing.T) {
	t.Setenv(envParkedProbeBudget, "-1s")
	t.Setenv(envParkedProbeInterval, "-1s")

	cfg := LoadBackgroundProbeConfig(nil)

	if cfg.Budget != defaultParkedProbeBudget || cfg.Interval != defaultParkedProbeInterval {
		t.Errorf("expected both to fall back to defaults, got %+v", cfg)
	}
}
