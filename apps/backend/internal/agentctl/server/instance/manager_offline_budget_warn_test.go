package instance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
)

func budgetWarnings(t *testing.T, cfg *config.Config, budget time.Duration) []observer.LoggedEntry {
	t.Helper()
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	m := &Manager{config: cfg, logger: log}
	m.warnBudgetExceedsReapers("inst-1", budget)
	return logs.All()
}

func TestWarnBudgetExceedsReapersNamesBudgetAndPeriod(t *testing.T) {
	cfg := &config.Config{UnownedPeriod: 10 * time.Minute, IdleTimeout: 0, AgentSurvivalEnabled: true}

	entries := budgetWarnings(t, cfg, 15*time.Minute)
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Contains(t, fields, "offline_budget")
	require.Contains(t, fields, "unowned_period")

	require.Empty(t, budgetWarnings(t, cfg, 5*time.Minute))
}

// Without agent survival the unowned reaper never runs, so a budget longer
// than the unowned period cannot be reaped by it and must not warn.
func TestWarnBudgetExceedsReapersIgnoresUnownedPeriodWithoutSurvival(t *testing.T) {
	cfg := &config.Config{UnownedPeriod: 10 * time.Minute, IdleTimeout: 0, AgentSurvivalEnabled: false}

	require.Empty(t, budgetWarnings(t, cfg, 15*time.Minute))
}
