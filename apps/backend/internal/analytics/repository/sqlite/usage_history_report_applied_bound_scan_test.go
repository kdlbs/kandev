//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryPostgresAppliedBuildsFollowSelectedSubjects(t *testing.T) {
	if os.Getenv("KANDEV_TEST_POSTGRES_DSN") == "" {
		t.Skip("actual PostgreSQL plan checks require the private test DSN")
	}
	fixture := newUsageBenchFixture(t, "postgres", usageBenchSize{})
	subject := fixture.subjects[2]
	unrelated := usageHistoryAppliedBoundSubjects(t, fixture, 128)
	fixture.admitSeedBatch(t, usageHistoryAppliedBoundRecords(t, fixture, subject, unrelated))
	fixture.publish(t)
	subject = fixture.subjects[2]
	require.NoError(t, fixture.tasks.DeleteTask(t.Context(), subject.task))
	totalApplied, selectedWorks, selectedFacts := usageHistoryAppliedBoundCardinality(t, fixture, subject)
	t.Logf("unrelated sessions=%d", len(unrelated))
	t.Logf("published applied work items=%d; selected task facts/work items=%d/%d", totalApplied, selectedFacts, selectedWorks)
	require.GreaterOrEqual(t, totalApplied, int64(2048), "the fixture must include thousands of unrelated published work items")
	require.Equal(t, int64(2), selectedWorks)
	require.Equal(t, int64(2048), totalApplied-selectedWorks)
	require.Equal(t, int64(2), selectedFacts)

	filters := usageHistoryAppliedBoundFilters(subject)
	for _, filter := range filters {
		t.Run(filter.name, func(t *testing.T) {
			candidate := usageReportSourceForEngine(subject.scope, subject.generation, filter.filter, true, true)
			fallback := usageReportSourceForEngine(subject.scope, subject.generation, filter.filter, true, false)
			requireUsageReportSourcesEqual(t, fixture, filter.filter, fallback, candidate)
			plan := usageHistoryAppliedBoundPlan(t, fixture, subject, filter.filter)
			visits := usageBenchPostgresRelationVisits(plan, "usage_projection_applied")
			require.Positive(t, visits)
			require.LessOrEqual(t, visits, float64(selectedWorks*3+2), "applied lookups must scale with selected work and its report checks")
			require.Less(t, visits, float64(totalApplied)/4, "unrelated historical work must not enter the materialized applied set")
			require.True(t, usageBenchPostgresHasIndex(plan, "usage_projection_applied", "usage_projection_applied_pkey"),
				"selected applied builds must use primary-key lookups")
			factVisits := usageBenchPostgresRelationVisits(plan, "usage_canonical_facts")
			subjectVisits := usageBenchPostgresRelationVisits(plan, "usage_subjects")
			legacyVisits := usageHistoryLegacyAppliedBuildVisits(t, fixture, subject)
			require.GreaterOrEqual(t, legacyVisits, float64(totalApplied), "the old CTE must scan the full published applied set")
			t.Logf("%s applied visits new=%g old=%g; matching visits subjects=%g facts=%g", filter.name, visits, legacyVisits, subjectVisits, factVisits)
			require.Greater(t, legacyVisits, visits, "the former workspace-wide materialization must visit unrelated applied builds")

			report, err := fixture.repo.ListSessionUsage(t.Context(), filter.filter)
			require.NoError(t, err)
			require.NotNil(t, report.Totals.TotalTokens)
			require.EqualValues(t, 40, *report.Totals.TotalTokens)
			require.NotNil(t, report.Totals.CostSubcents)
			require.EqualValues(t, 5, *report.Totals.CostSubcents)
			require.NotEmpty(t, report.Rows)
			if filter.deletedTask {
				require.Equal(t, subject.task, report.Rows[0].TaskID)
				require.False(t, report.Rows[0].TaskAvailable, "deleted selected subjects remain reportable")
			}
		})
	}
}

type usageHistoryAppliedBoundFilter struct {
	name        string
	filter      models.SessionUsageFilter
	deletedTask bool
}

func usageHistoryAppliedBoundFilters(subject usageBenchSubject) []usageHistoryAppliedBoundFilter {
	start, end := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	base := models.SessionUsageFilter{WorkspaceID: subject.workspace, GroupBy: "model", Timezone: "UTC", Start: &start, End: &end, IncludeUndated: true, Limit: 100}
	task, session, both := base, base, base
	task.TaskIDs, session.SessionIDs = []string{subject.task}, []string{subject.session}
	both.TaskIDs, both.SessionIDs = []string{subject.task}, []string{subject.session}
	lifetime := models.SessionUsageFilter{WorkspaceID: subject.workspace, GroupBy: "task", Timezone: "UTC", TaskIDs: []string{subject.task}, SessionIDs: []string{subject.session}, Limit: 100}
	return []usageHistoryAppliedBoundFilter{
		{name: "dated_include_undated_task", filter: task},
		{name: "dated_include_undated_session", filter: session},
		{name: "dated_include_undated_task_and_session", filter: both},
		{name: "all_time_lifetime_task_and_session", filter: lifetime, deletedTask: true},
	}
}

func usageHistoryAppliedBoundRecords(t *testing.T, fixture *usageBenchFixture, selected usageBenchSubject, unrelated []usageBenchSubject) []usageBenchSeedRecord {
	t.Helper()
	date := time.Date(2025, 12, 15, 12, 0, 0, 0, time.UTC)
	dated := usageBenchMeasurement(selected, "selected-dated", date, "plugin:bench")
	dated.Model, dated.Provider, dated.CostSubcents = "selected-dated-model", "selected-provider", models.Int64(2)
	lifetime := usageBenchMeasurement(selected, "selected-lifetime", date, "plugin:bench")
	lifetime.Model, lifetime.Provider, lifetime.CostSubcents = "selected-lifetime-model", "selected-provider", models.Int64(3)
	lifetime.InputTokens, lifetime.OutputTokens = models.Int64(20), models.Int64(10)
	lifetime.TotalTokens, lifetime.SourceTotalTokens = models.Int64(30), models.Int64(30)
	lifetime.CoverageStart, lifetime.CoverageEnd = nil, nil
	lifetime.SourceDate, lifetime.SourceTimezone, lifetime.CoverageKey = "", "", ""
	records := []usageBenchSeedRecord{
		usageHistoryAppliedBoundRecord(t, fixture, selected, dated, "selected-dated-work"),
		usageHistoryAppliedBoundRecord(t, fixture, selected, lifetime, "selected-lifetime-work"),
	}
	for index := range 2048 {
		subject := unrelated[index%len(unrelated)]
		id := fmt.Sprintf("unrelated-work-%04d", index)
		measurement := usageBenchMeasurement(subject, id, date.Add(time.Duration(index)*time.Minute), "plugin:bench")
		measurement.Model, measurement.Provider = "unrelated-model", "unrelated-provider"
		records = append(records, usageHistoryAppliedBoundRecord(t, fixture, subject, measurement, id))
	}
	return records
}

func usageHistoryAppliedBoundSubjects(t *testing.T, fixture *usageBenchFixture, count int) []usageBenchSubject {
	t.Helper()
	created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	workspace, organization := usageBenchWorkspace, usageBenchWorkspace+"-org"
	subjects := make([]usageBenchSubject, 0, count)
	for index := range count {
		taskID := fmt.Sprintf("usage-bench-unrelated-task-%03d", index)
		task := &taskmodels.Task{ID: taskID, WorkspaceID: workspace, Title: taskID, CreatedAt: created, UpdatedAt: created}
		require.NoError(t, fixture.tasks.CreateTask(t.Context(), task))
		task, err := fixture.tasks.GetTask(t.Context(), taskID)
		require.NoError(t, err)
		sessionID, transcript := taskID+"-session", taskID+"-producer-thread"
		session := &taskmodels.TaskSession{ID: sessionID, TaskID: taskID, QueueIncarnationID: sessionID, StartedAt: created,
			Metadata: map[string]any{"acp": map[string]any{"session_id": transcript}}}
		require.NoError(t, fixture.tasks.CreateTaskSession(t.Context(), session))
		session, err = fixture.tasks.GetTaskSession(t.Context(), sessionID)
		require.NoError(t, err)
		binding := usagehistory.Binding{ID: taskID + "-binding", Kind: usagehistory.BindingKindExecution, WorkspaceID: workspace, OrganizationID: organization,
			TaskID: taskID, TaskIncarnationID: usageTaskIncarnationID(task.CreatedAt), SessionID: sessionID, SessionIncarnationID: session.QueueIncarnationID,
			ActorID: "user-unrelated", ActorStatus: "attributed", ActorKind: "user", ActorSource: "execution", CapturedAt: created}
		tx, err := fixture.database.BeginTxx(t.Context(), nil)
		require.NoError(t, err)
		require.NoError(t, fixture.repo.AdmitBindingTx(t.Context(), tx, binding))
		require.NoError(t, tx.Commit())
		scope := usageScopeID(workspace, organization)
		subjectID := usageSubjectID(scope, taskID, binding.TaskIncarnationID, sessionID, binding.SessionIncarnationID)
		entry := usageBenchSubject{workspace: workspace, scope: scope, subject: subjectID, task: taskID, session: sessionID, binding: binding.ID,
			actor: binding.ActorID, actorStatus: binding.ActorStatus, transcript: transcript}
		entry.work = transcriptUsageWork(scope, subjectID, transcript)
		fixture.subjects = append(fixture.subjects, entry)
		fixture.collectionTarget(t, entry, "plugin:bench")
		subjects = append(subjects, entry)
	}
	return subjects
}

func usageHistoryAppliedBoundRecord(t *testing.T, fixture *usageBenchFixture, subject usageBenchSubject, measurement models.SessionUsageMeasurement, identity string) usageBenchSeedRecord {
	t.Helper()
	record := fixture.seedRecord(t, subject, measurement)
	record.input.WorkIdentity = &pluginsdk.UsageWorkIdentityV1{Scheme: "provider_event", ID: identity}
	usageBenchSeal(t, record.input)
	return record
}

func usageHistoryAppliedBoundCardinality(t *testing.T, fixture *usageBenchFixture, subject usageBenchSubject) (int64, int64, int64) {
	t.Helper()
	var total, works, facts int64
	query := `SELECT COUNT(*) FROM usage_projection_applied WHERE scope_id=? AND generation=?`
	require.NoError(t, fixture.database.Get(&total, fixture.database.Rebind(query), subject.scope, subject.generation))
	query = `SELECT COUNT(DISTINCT a.work_key) FROM usage_projection_applied a JOIN usage_canonical_facts f
		ON f.scope_id=a.scope_id AND f.generation=a.generation AND f.work_key=a.work_key AND f.build_id=a.build_id
		WHERE a.scope_id=? AND a.generation=? AND f.subject_id=?`
	require.NoError(t, fixture.database.Get(&works, fixture.database.Rebind(query), subject.scope, subject.generation, subject.subject))
	query = `SELECT COUNT(*) FROM usage_canonical_facts f JOIN usage_projection_applied a
		ON a.scope_id=f.scope_id AND a.generation=f.generation AND a.work_key=f.work_key AND a.build_id=f.build_id
		WHERE f.scope_id=? AND f.generation=? AND f.subject_id=?`
	require.NoError(t, fixture.database.Get(&facts, fixture.database.Rebind(query), subject.scope, subject.generation, subject.subject))
	return total, works, facts
}

func usageHistoryAppliedBoundPlan(t *testing.T, fixture *usageBenchFixture, subject usageBenchSubject, filter models.SessionUsageFilter) usageBenchPostgresNode {
	t.Helper()
	tx, err := fixture.database.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	metadata, err := fixture.repo.usageProjectionMetadataTx(t.Context(), tx, subject.scope)
	require.NoError(t, err)
	require.NoError(t, fixture.repo.usageReportGrid(t.Context(), tx, filter, metadata))
	source := usageReportSourceForEngine(subject.scope, subject.generation, filter, true, true)
	query := `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) SELECT COUNT(*) FROM (` + source.sql + `) usage_applied_bound_source`
	var encoded []byte
	require.NoError(t, tx.GetContext(t.Context(), &encoded, fixture.database.Rebind(query), source.args...))
	var roots []struct {
		Plan usageBenchPostgresNode `json:"Plan"`
	}
	require.NoError(t, json.Unmarshal(encoded, &roots))
	require.Len(t, roots, 1)
	return roots[0].Plan
}

func usageHistoryLegacyAppliedBuildVisits(t *testing.T, fixture *usageBenchFixture, subject usageBenchSubject) float64 {
	t.Helper()
	query := `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)
		WITH usage_report_applied AS MATERIALIZED (
			SELECT scope_id,generation,work_key,build_id FROM usage_projection_applied WHERE scope_id=? AND generation=?
		) SELECT COUNT(*) FROM usage_report_applied`
	var encoded []byte
	require.NoError(t, fixture.database.GetContext(t.Context(), &encoded, fixture.database.Rebind(query), subject.scope, subject.generation))
	var roots []struct {
		Plan usageBenchPostgresNode `json:"Plan"`
	}
	require.NoError(t, json.Unmarshal(encoded, &roots))
	require.Len(t, roots, 1)
	return usageBenchPostgresRelationVisits(roots[0].Plan, "usage_projection_applied")
}
