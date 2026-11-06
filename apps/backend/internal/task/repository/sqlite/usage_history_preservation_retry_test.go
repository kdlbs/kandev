package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const preservationPageFailure = "usage preservation injected failure"

func (f *preservationPageFixture) failAndResume(t *testing.T, source string) {
	t.Helper()
	dropFault := f.installPageFault(t, source)
	err := f.removeLiveTarget()
	require.ErrorContains(t, err, preservationPageFailure, "a second-page error must stop destructive cleanup")
	var count int
	require.NoError(t, f.db.Get(&count, f.db.Rebind(`SELECT COUNT(*) FROM tasks WHERE id = ?`), preservationPageTask))
	require.Equal(t, 1, count, "task must remain retryable after preservation rollback")
	require.NoError(t, f.db.Get(&count, f.db.Rebind(`SELECT COUNT(*) FROM task_sessions WHERE id = ?`), preservationPageSession))
	require.Equal(t, 1, count, "session must remain retryable after preservation rollback")
	require.NoError(t, f.db.Get(&count, f.db.Rebind(`SELECT COUNT(*) FROM task_usage_events
		WHERE task_id = ? AND session_id = ?`), preservationPageTask, preservationPageSession))
	require.Equal(t, preservationPageRecords, count, "native source tail must remain live after failed preservation")
	f.assertCheckpoints(t, source, false)
	f.assertScopedProgress(t, source)
	f.assertAdmissionFenced(t)
	dropFault()
	f.reloadRepositories(t)
	f.assertCheckpoints(t, source, false)
}

func (f *preservationPageFixture) installPageFault(t *testing.T, source string) func() {
	t.Helper()
	table, action := "session_usage_"+source, "UPDATE OF scope_id"
	key := f.checkpointKey(source, preservationPageSize+50)
	condition := "NEW.id = '" + strings.ReplaceAll(key, "'", "''") + "'"
	if source == "native" {
		table, action = "session_usage_buckets", "INSERT"
		condition = fmt.Sprintf("NEW.source = 'native' AND NEW.source_record_id = 'legacy-native-%03d'", preservationPageSize+50)
	}
	const trigger = "usage_preservation_page_fault"
	var create string
	var drop func()
	if f.db.DriverName() == "pgx" {
		_, err := f.db.Exec(`CREATE FUNCTION usage_preservation_page_fault_fn() RETURNS trigger
			LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'usage preservation injected failure'; END $$`)
		require.NoError(t, err)
		create = `CREATE TRIGGER ` + trigger + ` BEFORE ` + action + ` ON ` + table +
			` FOR EACH ROW WHEN (` + condition + `) EXECUTE FUNCTION usage_preservation_page_fault_fn()`
		drop = func() {
			_, err := f.db.Exec(`DROP TRIGGER IF EXISTS ` + trigger + ` ON ` + table)
			require.NoError(t, err)
			_, err = f.db.Exec(`DROP FUNCTION IF EXISTS usage_preservation_page_fault_fn()`)
			require.NoError(t, err)
		}
	} else {
		create = `CREATE TRIGGER ` + trigger + ` BEFORE ` + action + ` ON ` + table + ` WHEN ` + condition +
			` BEGIN SELECT RAISE(ABORT, 'usage preservation injected failure'); END`
		drop = func() {
			_, err := f.db.Exec(`DROP TRIGGER IF EXISTS ` + trigger)
			require.NoError(t, err)
		}
	}
	t.Cleanup(drop)
	_, err := f.db.Exec(create)
	require.NoError(t, err)
	return drop
}

type preservationCheckpoint struct {
	Source    string `db:"source_key"`
	HighWater string `db:"high_water"`
	LastKey   string `db:"last_key"`
	Complete  int    `db:"complete"`
	State     string `db:"state"`
}

func (f *preservationPageFixture) assertCheckpoints(t *testing.T, failedSource string, complete bool) {
	t.Helper()
	kind := "task"
	if f.prune {
		kind = "session"
	}
	var rows []preservationCheckpoint
	err := f.db.Select(&rows, f.db.Rebind(`SELECT c.source_key, c.high_water, c.last_key, c.complete, p.state
		FROM usage_preservation_cursors c JOIN usage_preservation_fences p ON p.fence_id = c.fence_id
		WHERE p.task_id = ? AND p.fence_kind = ?`), preservationPageTask, kind)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	stages := map[string]int{"native": 0, "measurements": 1, "buckets": 2}
	for _, row := range rows {
		require.Equal(t, f.checkpointKey(row.Source, preservationPageRecords-1), row.HighWater, "%s high-water", row.Source)
		switch {
		case complete:
			require.Equal(t, "deleted", row.State)
			require.Equal(t, 1, row.Complete, "%s completion", row.Source)
			require.Equal(t, row.HighWater, row.LastKey, "%s completed cursor", row.Source)
		case stages[row.Source] < stages[failedSource]:
			require.Equal(t, "preserving", row.State)
			require.Equal(t, 1, row.Complete, "%s prior source completion", row.Source)
			require.Equal(t, row.HighWater, row.LastKey)
		case row.Source == failedSource:
			require.Equal(t, "preserving", row.State)
			require.Zero(t, row.Complete, "%s failed page must not complete", row.Source)
			require.Equal(t, f.checkpointKey(row.Source, preservationPageSize-1), row.LastKey,
				"%s failed page must retain exactly the first committed page's cursor", row.Source)
		default:
			require.Equal(t, "preserving", row.State)
			require.Zero(t, row.Complete)
			require.Empty(t, row.LastKey, "%s has not started", row.Source)
		}
	}
}

func (f *preservationPageFixture) assertScopedProgress(t *testing.T, failedSource string) {
	t.Helper()
	stages := map[string]int{"native": 0, "measurements": 1, "buckets": 2}
	for _, source := range []string{"native", "measurements", "buckets"} {
		count := 0
		if stages[source] < stages[failedSource] {
			count = preservationPageRecords
		} else if source == failedSource {
			count = preservationPageSize
		}
		table, provenance := "session_usage_"+source, preservationLegacySource
		if source == "native" {
			table, provenance = "session_usage_buckets", "native"
		}
		var records []string
		err := f.db.Select(&records, f.db.Rebind(`SELECT source_record_id FROM `+table+`
			WHERE task_id = ? AND session_id = ? AND source = ? AND scope_id <> '' AND subject_id <> ''`),
			preservationPageTask, preservationPageSession, provenance)
		require.NoError(t, err)
		require.Len(t, records, count, "%s committed rows after page rollback", source)
		committed := make(map[string]bool, len(records))
		for _, record := range records {
			committed[record] = true
		}
		for i := 0; i < preservationPageRecords; i++ {
			record, expected := fmt.Sprintf("legacy-native-%03d", i), i < count
			if source != "native" {
				record = fmt.Sprintf("legacy-%s-%03d", source, i)
				expected = count > 0 && preservationExternalKey(source, i) <= f.externalKeys[source][count-1]
			}
			require.Equal(t, expected, committed[record], "%s/%s atomic page membership", source, record)
		}
	}
}

func (f *preservationPageFixture) assertAdmissionFenced(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	err := f.tasks.CreateTaskUsageEvent(ctx, newTestUsageEvent("preservation-fenced-native", preservationPageTask, preservationPageSession))
	require.ErrorContains(t, err, "usage writes are fenced")
	err = reportExternalUsageWithBinding(ctx, f.analytics, "preservation", f.binding, "preservation-fenced-external", "exact-model", 9, time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC))
	require.ErrorContains(t, err, "usage writes are fenced")
	var rows int
	require.NoError(t, f.db.Get(&rows, `SELECT COUNT(*) FROM task_usage_events WHERE usage_event_id = 'preservation-fenced-native'`))
	require.Zero(t, rows)
	require.NoError(t, f.db.Get(&rows, `SELECT COUNT(*) FROM session_usage_measurements WHERE source_record_id = 'preservation-fenced-external'`))
	require.Zero(t, rows)
}
