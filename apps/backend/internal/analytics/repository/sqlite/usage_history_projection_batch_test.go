//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryProjectionScanBatchesSourceMutations(t *testing.T) {
	t.Setenv("KANDEV_USAGE_BENCH_TRACE_SQLITE", "1")
	fixture := newUsageBenchFixture(t, "sqlite", usageBenchSize{})
	fixture.seedRange(t, "recent", 2025, 2026, 80, 0)
	fixture.finishRetention(t)
	generation, err := fixture.repo.StartUsageHistoryRebuild(t.Context(), fixture.subjects[0].scope)
	require.NoError(t, err)

	fixture.observer.startWithLimit(128, true)
	done, err := fixture.repo.AdvanceUsageHistoryProjection(t.Context(), fixture.subjects[0].scope, generation, 200)
	require.NoError(t, err)
	require.False(t, done)
	statements := fixture.observer.snapshot()
	for _, prefix := range []string{
		"INSERT INTO usage_projection_log",
		"INSERT INTO usage_projection_partitions",
		"INSERT INTO usage_projection_source_versions",
		"INSERT INTO usage_dirty_partitions",
		"INSERT INTO usage_projection_items",
		"INSERT INTO usage_projection_work",
	} {
		require.Equal(t, 1, usageBenchCountSQL(statements, prefix), prefix)
	}
	require.Zero(t, usageBenchCountSQL(statements, "SELECT commit_sequence FROM usage_projection_source_versions"))

	for _, table := range []string{"usage_projection_log", "usage_projection_source_versions", "usage_projection_items"} {
		var count int
		require.NoError(t, fixture.database.Get(&count, fixture.database.Rebind(`SELECT COUNT(*) FROM `+table+` WHERE scope_id=?`), fixture.subjects[0].scope))
		require.Equal(t, 80, count, table)
	}
}

func TestUsageHistoryProjectionStagesFactPagesInBatches(t *testing.T) {
	t.Setenv("KANDEV_USAGE_BENCH_TRACE_SQLITE", "1")
	fixture := newUsageBenchFixture(t, "sqlite", usageBenchSize{})
	work := usageProjectionWork{
		Scope: fixture.subjects[0].scope, Generation: uuid.NewString(), Work: fixture.subjects[0].work,
		Build: uuid.NewString(),
	}
	facts := make([]models.UsageCanonicalFact, 80)
	for index := range facts {
		at := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, index)
		end := at.Add(time.Hour)
		input, output, total, cost := int64(3), int64(2), int64(5), int64(7)
		measurement := models.SessionUsageMeasurement{
			ID: fmt.Sprintf("fact-%03d", index), ScopeID: work.Scope, SubjectID: fixture.subjects[0].subject,
			TaskID: fixture.subjects[0].task, SessionID: fixture.subjects[0].session,
			Source: usageSourceNative, Model: "fixture-model", Provider: "fixture-provider", AgentClient: "fixture-agent",
			SourceDate: at.Format(time.DateOnly), SourceTimezone: "UTC", CoverageStart: &at, CoverageEnd: &end,
			CollectedAt: at, InputTokens: &input, OutputTokens: &output, TotalTokens: &total, SourceTotalTokens: &total,
			CostSubcents: &cost, Currency: "USD", CostBasis: "reported", Coverage: usageCoverageComplete, CostCoverage: usageCoverageComplete,
		}
		facts[index] = models.UsageCanonicalFact{
			Key: measurement.ID, CoverageKind: usageCoveragePoint, Measurement: measurement,
			TokenSource: &models.UsageMetricSource{Source: usageSourceNative, ItemID: measurement.ID, ActorID: "actor", ActorStatus: "attributed", ActorKind: "user"},
			CostSource:  &models.UsageMetricSource{Source: "plugin:fixture", ItemID: measurement.ID, ActorID: "actor", ActorStatus: "attributed", ActorKind: "user"},
		}
	}
	tx, err := fixture.database.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	fixture.observer.startWithLimit(64, true)
	require.NoError(t, fixture.repo.stageUsageProjectionFactsTx(t.Context(), tx, work, facts))
	require.NoError(t, tx.Commit())
	statements := fixture.observer.snapshot()
	require.Equal(t, 2, usageBenchCountSQL(statements, "INSERT INTO usage_canonical_facts"))
	require.Equal(t, 6, usageBenchCountSQL(statements, "INSERT INTO usage_projection_contributions"))

	var canonical, contributions int
	require.NoError(t, fixture.database.Get(&canonical, fixture.database.Rebind(`SELECT COUNT(*) FROM usage_canonical_facts WHERE scope_id=? AND generation=?`), work.Scope, work.Generation))
	require.NoError(t, fixture.database.Get(&contributions, fixture.database.Rebind(`SELECT COUNT(*) FROM usage_projection_contributions WHERE scope_id=? AND generation=?`), work.Scope, work.Generation))
	require.Equal(t, len(facts), canonical)
	require.Equal(t, len(facts)*2, contributions)
}

func usageBenchCountSQL(statements []usageBenchStatement, prefix string) int {
	count := 0
	for _, statement := range statements {
		if strings.HasPrefix(strings.TrimSpace(statement.SQL), prefix) {
			count++
		}
	}
	return count
}
