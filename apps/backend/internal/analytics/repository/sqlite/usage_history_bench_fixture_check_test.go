//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/stretchr/testify/require"
)

// The small fixture must exercise the real API, retain old published work,
// preserve publication identity across pages, and leave recent amounts intact.
func TestUsageHistoryBenchmarkFixture(t *testing.T) {
	checkUsageBenchFixture(t, "sqlite")
}

func TestUsageHistoryBenchmarkPostgresFixture(t *testing.T) {
	checkUsageBenchFixture(t, "postgres")
}

func checkUsageBenchFixture(t *testing.T, engine string) {
	t.Helper()
	fixture := newUsageBenchFixture(t, engine, usageBenchSize{Native: 100, External: 50})
	fixture.seedRecent(t)
	fixture.validateStorage(t, false)
	recent := fixture.cases[0]
	before := fixture.readAndValidate(t, recent)
	require.NotEmpty(t, before.ProjectionGeneration)
	if engine == "sqlite" {
		observer := &usageBenchObserver{}
		require.NoError(t, attachUsageBenchSQLite(context.Background(), fixture.database.DB, observer))
		observer.start()
		fixture.readAndValidate(t, recent)
		statements, err := observer.finish()
		require.NoError(t, err)
		require.NoError(t, validateUsageBenchStatements("sqlite", statements))
	}
	plain := runUsageBenchProcess(t, fixture, recent, false)
	instrumented := runUsageBenchProcess(t, fixture, recent, true)
	require.Equal(t, plain.Publication, instrumented.Publication)
	require.Positive(t, plain.ProcessPeakRSS)
	if engine == "sqlite" {
		require.Positive(t, instrumented.Scans.SQLiteVisits)
		require.NotNil(t, plain.NativePeak)
		require.GreaterOrEqual(t, *plain.NativePeak, *plain.NativeBaseline)
	} else {
		require.Positive(t, instrumented.Scans.PostgresRows)
		require.NotEmpty(t, plain.PostgresVersion)
		t.Logf("actual PostgreSQL fixture: %s; instrumented statements=%d", plain.PostgresVersion, len(instrumented.Statements))
	}
	fixture.seedOlder(t)
	after := fixture.readAndValidate(t, recent)
	require.Equal(t, before.Totals.TotalTokens, after.Totals.TotalTokens)
	require.Equal(t, before.Providers, after.Providers)
	require.NotEqual(t, before.ProjectionGeneration, after.ProjectionGeneration)
	for _, scenario := range fixture.cases {
		fixture.readAndValidate(t, scenario)
	}
	fixture.validateStorage(t, true)
	assertUsageBenchAdmissionParity(t, fixture)
}

// The query-only snapshot must agree with the same small producer recipe sent
// through actual native and SDK admission. Both variants use the real worker;
// neither variant installs ready markers or preselected canonical payloads.
func assertUsageBenchAdmissionParity(t *testing.T, imported *usageBenchFixture) {
	t.Helper()
	admitted := newUsageBenchFixture(t, imported.engine, imported.size)
	admitted.ordinaryAdmission = true
	admitted.seedRecent(t)
	admitted.seedOlder(t)
	admitted.validateStorage(t, true)
	for i, scenario := range imported.cases {
		expected, err := fetchUsageBenchReport(t.Context(), imported.repo, scenario.Filter)
		require.NoError(t, err)
		actual, err := fetchUsageBenchReport(t.Context(), admitted.repo, admitted.cases[i].Filter)
		require.NoError(t, err)
		validateUsageBenchResult(t, actual, admitted.cases[i])
		expected = usageBenchComparableReport(t, imported, expected, scenario.Filter.GroupBy)
		actual = usageBenchComparableReport(t, admitted, actual, scenario.Filter.GroupBy)
		require.Equal(t, usageBenchSelectionDigest(t, expected), usageBenchSelectionDigest(t, actual), scenario.Name)
		require.Equal(t, usageBenchSavedRows(t, imported, scenario), usageBenchSavedRows(t, admitted, admitted.cases[i]), scenario.Name)
	}
	t.Logf("snapshot/admission API parity native=%d external=%d published=%d contributions=%d; no synthetic migration/publication watermarks", imported.storage.RetainedNative, imported.storage.RetainedExternal, imported.storage.PublishedFacts, imported.storage.PublishedContributions)
}

func usageBenchSavedRows(t testing.TB, fixture *usageBenchFixture, scenario *usageBenchCase) []models.SessionUsageMeasurement {
	t.Helper()
	filter := scenario.Filter
	filter.Limit = projectionLimit(filter.Limit)
	var rows []models.SessionUsageMeasurement
	for {
		page, state, err := fixture.repo.ListRetainedCanonicalUsagePage(t.Context(), filter)
		usageBenchMust(t, err)
		if state.Status != projectionReady || state.Pending || state.Generation == "" {
			t.Fatal("saved fixture read lacks a complete publication")
		}
		if filter.ProjectionGeneration == "" {
			filter.ProjectionGeneration = state.Generation
		}
		if filter.ProjectionGeneration != state.Generation {
			t.Fatal("saved fixture publication changed across pages")
		}
		for _, row := range page {
			// Host collection times and opaque binding digests differ between import
			// and RPC admission. Consumption time, values, roles and actors must agree.
			row.CollectedAt, row.UsageBindingID, row.PayloadDigest = time.Time{}, "", ""
			row.SubjectID = usageBenchComparableSubject(t, fixture, row.SubjectID)
			rows = append(rows, row)
		}
		if len(page) < filter.Limit {
			break
		}
		filter.Offset += len(page)
	}
	slices.SortFunc(rows, func(a, b models.SessionUsageMeasurement) int { return cmp.Compare(a.ID, b.ID) })
	return rows
}

// Separate databases receive distinct host incarnation IDs. Compare their
// captured owners by fixture identity after checking the actual retained row.
func usageBenchComparableSubject(t testing.TB, fixture *usageBenchFixture, id string) string {
	t.Helper()
	if id == "" {
		return ""
	}
	for _, subject := range fixture.subjects {
		if subject.subject != id {
			continue
		}
		var stored struct{ Scope, Task, Session, TaskIncarnation, SessionIncarnation string }
		usageBenchMust(t, fixture.database.Get(&stored, fixture.database.Rebind(`SELECT scope_id AS scope,task_id AS task,session_id AS session,
 task_incarnation_id AS taskincarnation,session_incarnation_id AS sessionincarnation FROM usage_subjects WHERE subject_id=?`), id))
		if stored.Scope != subject.scope || stored.Task != subject.task || stored.Session != subject.session ||
			stored.TaskIncarnation != subject.nativeScope.TaskIncarnationID || stored.SessionIncarnation != subject.nativeScope.SessionIncarnationID {
			t.Fatal("benchmark subject does not match its retained scope and admitted incarnations")
		}
		return fmt.Sprintf("fixture-subject:%s:%s:%s", stored.Scope, stored.Task, stored.Session)
	}
	t.Fatalf("benchmark returned an uncaptured subject: %s", id)
	return ""
}

func usageBenchComparableReport(t testing.TB, fixture *usageBenchFixture, result usageBenchResult, group string) usageBenchResult {
	t.Helper()
	result.Rows = slices.Clone(result.Rows)
	for i := range result.Rows {
		row := &result.Rows[i]
		row.SubjectID = usageBenchComparableSubject(t, fixture, row.SubjectID)
		entity := ""
		switch group {
		case usageWorkFallback:
			entity = "fixture-task:" + row.TaskID
		case usageSubjectSession:
			entity = row.SubjectID
		}
		if entity != "" {
			// These opaque group keys include the host's task/session incarnation.
			key := usageReportAggregate{Entity: entity, Period: row.Period, CalendarBasis: row.CalendarBasis,
				UsageDailySummary: models.UsageDailySummary{Model: row.Model, Provider: row.Provider, Calendar: row.Calendar, Currency: row.Currency}}
			row.Key = usageReportRow(key).Key
		}
	}
	slices.SortFunc(result.Rows, func(a, b models.TokenUsageRow) int { return cmp.Compare(a.Key, b.Key) })
	return result
}
