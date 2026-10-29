//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"fmt"
	"os"
	"testing"

	"github.com/kandev/kandev/internal/analytics/models"
)

// Snapshot import never sets migration readiness or publication watermarks.
// The real worker completes migration, source scans, bounded selection (including
// oversized work), contributions, summaries, applied pointers and cutover.
func (f *usageBenchFixture) publish(t testing.TB) {
	t.Helper()
	ctx := t.Context()
	f.finishRetention(t)
	for _, index := range []int{0, 12} {
		scope := f.subjects[index].scope
		generation, err := f.repo.StartUsageHistoryRebuild(ctx, scope)
		usageBenchMust(t, err)
		for i := range f.subjects {
			if f.subjects[i].scope == scope {
				f.subjects[i].generation = generation
			}
		}
		// Each operation retains the production admission gate and deadline. The
		// construction deadline is the enclosing test/benchmark timeout, not a new
		// per-report budget. No timer or allocation sample includes construction.
		for {
			done, err := f.repo.AdvanceUsageHistoryProjection(ctx, scope, generation, 200)
			usageBenchMust(t, err)
			f.projectionSteps++
			if done {
				break
			}
			if f.projectionSteps%10000 == 0 {
				t.Logf("bounded fixture projection pages=%d", f.projectionSteps)
			}
			if err := ctx.Err(); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, table := range []string{usageBucketsTable, usageMeasurementsTable, "usage_canonical_facts", "usage_projection_contributions", "usage_projection_applied", "usage_subjects", "usage_daily_summaries", "usage_lifetime_summaries", "usage_projection_history"} {
		_, err := f.database.Exec(`ANALYZE ` + table)
		usageBenchMust(t, err)
	}
}

func (f *usageBenchFixture) finishRetention(t testing.TB) {
	t.Helper()
	for {
		done, err := f.repo.BackfillUsageHistory(t.Context(), 200)
		usageBenchMust(t, err)
		if done {
			return
		}
		if err := t.Context().Err(); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *usageBenchFixture) validateStorage(t testing.TB, full bool) {
	t.Helper()
	wantNative, wantExternal := f.size.Native/5, f.size.External/5
	if full {
		wantNative, wantExternal = f.size.Native, f.size.External
	}
	for _, item := range []struct {
		predicate string
		minimum   int
	}{{`source='native'`, wantNative}, {`source<>'native'`, wantExternal}} {
		var count int
		query := `SELECT COUNT(*) FROM (SELECT source FROM session_usage_buckets WHERE scope_id=? UNION ALL SELECT source FROM session_usage_measurements WHERE scope_id=?) retained WHERE ` + item.predicate
		usageBenchMust(t, f.database.Get(&count, f.database.Rebind(query), f.subjects[0].scope, f.subjects[0].scope))
		kind := "external"
		if item.predicate == `source='native'` {
			kind = usageSourceNative
		}
		if count != f.retained[f.subjects[0].scope+":"+kind] || count < item.minimum {
			t.Fatalf("retained fixture has %d rows, recipe requires exactly %d and at least %d", count, f.retained[f.subjects[0].scope+":"+kind], item.minimum)
		}
		if item.predicate == `source='native'` {
			f.storage.RetainedNative = count
		} else {
			f.storage.RetainedExternal = count
		}
	}
	var count, declared int64
	usageBenchMust(t, f.database.Get(&count, f.database.Rebind(`SELECT COUNT(*) FROM usage_canonical_facts f JOIN usage_projection_applied a ON a.scope_id=f.scope_id AND a.generation=f.generation AND a.work_key=f.work_key AND a.build_id=f.build_id WHERE f.scope_id=? AND f.generation=?`), f.subjects[0].scope, f.subjects[0].generation))
	usageBenchMust(t, f.database.Get(&declared, f.database.Rebind(`SELECT fact_count FROM usage_projection_generations WHERE scope_id=? AND generation=?`), f.subjects[0].scope, f.subjects[0].generation))
	if count != declared || count != f.expectedFacts[f.subjects[0].scope] || count < int64(wantNative) {
		t.Fatalf("canonical publication count=%d declared=%d native minimum=%d", count, declared, wantNative)
	}
	f.storage.PublishedFacts = count
	usageBenchMust(t, f.database.Get(&count, f.database.Rebind(`SELECT COUNT(*) FROM usage_projection_contributions c JOIN usage_projection_applied a ON a.scope_id=c.scope_id AND a.generation=c.generation AND a.work_key=c.work_key AND a.build_id=c.build_id WHERE c.scope_id=? AND c.generation=?`), f.subjects[0].scope, f.subjects[0].generation))
	if count != f.expectedContributions[f.subjects[0].scope] {
		t.Fatalf("published contributions=%d scalar recipe=%d", count, f.expectedContributions[f.subjects[0].scope])
	}
	f.storage.PublishedContributions = count
	for _, target := range []struct {
		table string
		value *int64
	}{{"usage_daily_summaries", &f.storage.DailySummaries}, {"usage_lifetime_summaries", &f.storage.LifetimeSummaries}} {
		usageBenchMust(t, f.database.Get(target.value, f.database.Rebind(`SELECT COUNT(*) FROM `+target.table+` WHERE scope_id=? AND generation=?`), f.subjects[0].scope, f.subjects[0].generation))
	}
	f.validateReadiness(t)
	var payload string
	usageBenchMust(t, f.database.Get(&payload, f.database.Rebind(`SELECT f.payload FROM usage_canonical_facts f JOIN usage_projection_applied a ON a.scope_id=f.scope_id AND a.generation=f.generation AND a.work_key=f.work_key AND a.build_id=f.build_id WHERE f.scope_id=? AND f.generation=? ORDER BY f.fact_key LIMIT 1`), f.subjects[0].scope, f.subjects[0].generation))
	var fact models.UsageCanonicalFact
	usageBenchMust(t, decodeUsageProjection(payload, &fact))
	if fact.Key == "" || fact.Measurement.SubjectID == "" {
		t.Fatal(fmt.Errorf("canonical fixture payload is incomplete"))
	}
	if f.engine == "postgres" {
		usageBenchMust(t, f.database.Get(&f.storage.DiskBytes, `SELECT COALESCE(SUM(pg_total_relation_size(c.oid)),0) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relkind IN ('r','m')`))
		f.storage.DiskScope = "isolated PostgreSQL schema tables including indexes and toast"
	} else {
		f.storage.DiskBytes = 0
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if info, err := os.Stat(f.path + suffix); err == nil {
				f.storage.DiskBytes += info.Size()
			}
		}
		f.storage.DiskScope = "SQLite database plus WAL and shared-memory files"
	}
}

func (f *usageBenchFixture) validateReadiness(t testing.TB) {
	t.Helper()
	for _, index := range []int{0, 12} {
		subject := f.subjects[index]
		var bad int
		for _, table := range []string{usageBucketsTable, usageMeasurementsTable} {
			query := `SELECT COUNT(*) FROM ` + table + ` r JOIN usage_scopes s ON s.scope_id=r.scope_id WHERE r.scope_id=? AND
    (r.selection_partition='' OR r.selection_bundle='' OR r.selection_scan_key='' OR r.selection_version<>s.work_version
    OR r.work_status<>'proven' OR (r.source='native' AND (r.turns IS NOT NULL OR r.native_accounting_version<?)))`
			usageBenchMust(t, f.database.Get(&bad, f.database.Rebind(query), subject.scope, nativeUsageAccountingRepairVersion))
			if bad != 0 {
				t.Fatalf("%s has %d unindexed/unproven/unrepaired source rows", table, bad)
			}
		}
		usageBenchMust(t, f.database.Get(&bad, f.database.Rebind(`SELECT COUNT(*) FROM usage_projection_checkpoints WHERE scope_id=? AND generation=? AND complete=0`), subject.scope, subject.generation))
		if bad != 0 {
			t.Fatal("fixture source checkpoints did not complete")
		}
		usageBenchMust(t, f.database.Get(&bad, f.database.Rebind(`SELECT COUNT(*) FROM usage_projection_work WHERE scope_id=? AND generation=?`), subject.scope, subject.generation))
		if bad != 0 {
			t.Fatal("fixture still has unpublished work")
		}
		tx, err := f.database.BeginTxx(t.Context(), nil)
		usageBenchMust(t, err)
		snapshot, err := f.repo.usageReportingSnapshotTx(t.Context(), tx, subject.scope)
		var gap bool
		if err == nil {
			err = f.repo.usageReportGap(t.Context(), tx, subject.workspace, &gap)
		}
		usageBenchMust(t, tx.Rollback())
		usageBenchMust(t, err)
		if snapshot.Status != projectionReady || snapshot.Pending || snapshot.Generation == "" || gap {
			t.Fatalf("fixture lacks honest completed migration/publication: %+v", snapshot.UsageProjectionMetadata)
		}
	}
	var complete int
	usageBenchMust(t, f.database.Get(&complete, `SELECT COUNT(*) FROM usage_history_migration_state WHERE complete=1`))
	if complete != len(usageHistoryMigrationSources) {
		t.Fatal("fixture retention migration is incomplete")
	}
}
