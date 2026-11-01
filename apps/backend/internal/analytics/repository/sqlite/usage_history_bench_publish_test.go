//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
)

// Snapshot import never sets migration readiness or publication watermarks.
// The real worker completes migration, source scans, bounded selection (including
// oversized work), contributions, summaries, applied pointers and cutover.
func (f *usageBenchFixture) publish(t testing.TB) {
	t.Helper()
	ctx := t.Context()
	f.finishRetention(t)
	t.Logf("usage fixture retention backfill complete pages=%d", f.retentionPages)
	for _, index := range []int{0, 12} {
		scope := f.subjects[index].scope
		generation, err := f.repo.StartUsageHistoryRebuild(ctx, scope)
		usageBenchMust(t, err)
		pages := 0
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
			if err != nil {
				f.logUsageBenchBuildFailure(t, scope, generation, pages+1)
				t.Fatalf("usage projection advance scope_index=%d page=%d generation=%s: %v", index, pages+1, generation, err)
			}
			pages++
			f.projectionSteps++
			if done {
				break
			}
			if pages%100 == 0 {
				t.Logf("bounded fixture projection scope_index=%d pages=%d total_pages=%d", index, pages, f.projectionSteps)
			}
			if err := ctx.Err(); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, table := range []string{usageBucketsTable, usageMeasurementsTable, "usage_canonical_facts", "usage_projection_contributions", "usage_projection_provider_options", "usage_projection_applied", "usage_subjects", "usage_daily_summaries", "usage_lifetime_summaries", "usage_projection_history"} {
		_, err := f.database.Exec(`ANALYZE ` + table)
		usageBenchMust(t, err)
	}
}

func (f *usageBenchFixture) logUsageBenchBuildFailure(t testing.TB, scope, generation string, page int) {
	traceEnabled := (f.engine == "postgres" && usageBenchPostgresTraceEnabled()) || (f.engine == "sqlite" && usageBenchSQLiteTraceEnabled())
	if !traceEnabled {
		return
	}
	statements := f.observer.snapshot()
	start := max(0, len(statements)-20)
	t.Logf("%s construction trace page=%d statements=%d", f.engine, page, len(statements))
	for index := start; index < len(statements); index++ {
		t.Logf("%s construction SQL index=%d duration_ns=%d error=%q: %s", f.engine, index+1, statements[index].DurationNS, statements[index].Error, statements[index].SQL)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var counts struct {
		State         string `db:"state"`
		Replay        int64  `db:"replay_sequence"`
		Items         int64  `db:"items"`
		PendingWork   int64  `db:"pending_work"`
		Builds        int64  `db:"builds"`
		Resolutions   int64  `db:"resolutions"`
		Applied       int64  `db:"applied"`
		Facts         int64  `db:"facts"`
		Contributions int64  `db:"contributions"`
	}
	query := `SELECT g.state,g.replay_sequence,
		(SELECT COUNT(*) FROM usage_projection_items WHERE scope_id=g.scope_id AND generation=g.generation) AS items,
		(SELECT COUNT(*) FROM usage_projection_work WHERE scope_id=g.scope_id AND generation=g.generation) AS pending_work,
		(SELECT COUNT(*) FROM usage_projection_builds WHERE scope_id=g.scope_id AND generation=g.generation) AS builds,
		(SELECT COUNT(*) FROM usage_projection_resolutions WHERE scope_id=g.scope_id AND generation=g.generation) AS resolutions,
		(SELECT COUNT(*) FROM usage_projection_applied WHERE scope_id=g.scope_id AND generation=g.generation) AS applied,
		(SELECT COUNT(*) FROM usage_canonical_facts WHERE scope_id=g.scope_id AND generation=g.generation) AS facts,
		(SELECT COUNT(*) FROM usage_projection_contributions WHERE scope_id=g.scope_id AND generation=g.generation) AS contributions
		FROM usage_projection_generations g WHERE g.scope_id=? AND g.generation=?`
	if err := f.database.GetContext(ctx, &counts, f.database.Rebind(query), scope, generation); err != nil {
		t.Logf("%s construction state query failed: %v", f.engine, err)
	} else {
		t.Logf("%s construction state=%s replay=%d items=%d work=%d builds=%d resolutions=%d applied=%d facts=%d contributions=%d", f.engine, counts.State, counts.Replay, counts.Items, counts.PendingWork, counts.Builds, counts.Resolutions, counts.Applied, counts.Facts, counts.Contributions)
	}
	if f.engine == "sqlite" {
		var checkpoints []struct {
			Source   string `db:"source_table"`
			KeySize  int    `db:"key_size"`
			Complete int    `db:"complete"`
		}
		checkpointQuery := `SELECT source_table,LENGTH(last_key) AS key_size,complete FROM usage_projection_checkpoints WHERE scope_id=? AND generation=? ORDER BY source_table`
		if err := f.database.SelectContext(ctx, &checkpoints, f.database.Rebind(checkpointQuery), scope, generation); err != nil {
			t.Logf("SQLite construction checkpoint query failed: %v", err)
		} else {
			t.Logf("SQLite construction checkpoints: %+v", checkpoints)
		}
		var repairState struct {
			WorkVersion                int64 `db:"work_version"`
			PendingContextSubjects     int64 `db:"pending_context_subjects"`
			UnprovenMeasurements       int64 `db:"unproven_measurements"`
			UnprovenBuckets            int64 `db:"unproven_buckets"`
			BlankMeasurementPartitions int64 `db:"blank_measurement_partitions"`
			BlankBucketPartitions      int64 `db:"blank_bucket_partitions"`
			BlankMeasurementBundles    int64 `db:"blank_measurement_bundles"`
			BlankBucketBundles         int64 `db:"blank_bucket_bundles"`
			BlankMeasurementScanKeys   int64 `db:"blank_measurement_scan_keys"`
			BlankBucketScanKeys        int64 `db:"blank_bucket_scan_keys"`
			StaleMeasurementVersions   int64 `db:"stale_measurement_versions"`
			StaleBucketVersions        int64 `db:"stale_bucket_versions"`
			PendingNativeMeasurements  int64 `db:"pending_native_measurements"`
			PendingNativeBuckets       int64 `db:"pending_native_buckets"`
		}
		repairQuery := `SELECT s.work_version,
			(SELECT COUNT(*) FROM usage_subjects u WHERE u.scope_id=s.scope_id AND u.work_repair_pending=1) AS pending_context_subjects,
			(SELECT COUNT(*) FROM session_usage_measurements r WHERE r.scope_id=s.scope_id AND r.source<>'native' AND r.work_status='unproven') AS unproven_measurements,
			(SELECT COUNT(*) FROM session_usage_buckets r WHERE r.scope_id=s.scope_id AND r.source<>'native' AND r.work_status='unproven') AS unproven_buckets,
			(SELECT COUNT(*) FROM session_usage_measurements r WHERE r.scope_id=s.scope_id AND r.selection_partition='') AS blank_measurement_partitions,
			(SELECT COUNT(*) FROM session_usage_buckets r WHERE r.scope_id=s.scope_id AND r.selection_partition='') AS blank_bucket_partitions,
			(SELECT COUNT(*) FROM session_usage_measurements r WHERE r.scope_id=s.scope_id AND r.selection_bundle='') AS blank_measurement_bundles,
			(SELECT COUNT(*) FROM session_usage_buckets r WHERE r.scope_id=s.scope_id AND r.selection_bundle='') AS blank_bucket_bundles,
			(SELECT COUNT(*) FROM session_usage_measurements r WHERE r.scope_id=s.scope_id AND r.selection_scan_key='') AS blank_measurement_scan_keys,
			(SELECT COUNT(*) FROM session_usage_buckets r WHERE r.scope_id=s.scope_id AND r.selection_scan_key='') AS blank_bucket_scan_keys,
			(SELECT COUNT(*) FROM session_usage_measurements r WHERE r.scope_id=s.scope_id AND r.selection_version<s.work_version) AS stale_measurement_versions,
			(SELECT COUNT(*) FROM session_usage_buckets r WHERE r.scope_id=s.scope_id AND r.selection_version<s.work_version) AS stale_bucket_versions,
			(SELECT COUNT(*) FROM session_usage_measurements r WHERE r.scope_id=s.scope_id AND r.source='native' AND (r.turns IS NOT NULL OR r.native_accounting_version<1)) AS pending_native_measurements,
			(SELECT COUNT(*) FROM session_usage_buckets r WHERE r.scope_id=s.scope_id AND r.source='native' AND (r.turns IS NOT NULL OR r.native_accounting_version<1)) AS pending_native_buckets
			FROM usage_scopes s WHERE s.scope_id=?`
		if err := f.database.GetContext(ctx, &repairState, f.database.Rebind(repairQuery), scope); err != nil {
			t.Logf("SQLite construction repair-state query failed: %v", err)
		} else {
			t.Logf("SQLite construction repair-state: %+v", repairState)
		}
		return
	}
	var work usageProjectionWork
	workQuery := `SELECT * FROM usage_projection_work WHERE scope_id=? AND generation=? ORDER BY CASE WHEN state='complete' THEN 1 ELSE 0 END,work_key LIMIT 1`
	if err := f.database.GetContext(ctx, &work, f.database.Rebind(workQuery), scope, generation); err != nil {
		t.Logf("PostgreSQL construction next-work query failed: %v", err)
	} else {
		t.Logf("PostgreSQL construction next work state=%s cursor_bytes=%d build_present=%t selected_sequence=%d", work.State, len(work.Cursor), work.Build != "", work.Selected)
	}
	if len(statements) == 0 {
		return
	}
	last := ""
	for index := len(statements) - 1; index >= 0; index-- {
		candidate := statements[index].SQL
		upper := strings.ToUpper(strings.TrimSpace(candidate))
		if strings.HasPrefix(upper, "INSERT INTO USAGE_DAILY_SUMMARIES") || strings.HasPrefix(upper, "INSERT INTO USAGE_LIFETIME_SUMMARIES") {
			last = candidate
			break
		}
	}
	if last == "" {
		t.Logf("PostgreSQL construction last statement is not a summary insert; no ANALYZE replay")
		return
	}
	planCtx, planCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer planCancel()
	tx, err := f.database.BeginTxx(planCtx, nil)
	if err != nil {
		t.Logf("PostgreSQL summary plan transaction failed: %v", err)
		return
	}
	var plan []byte
	planSQL := "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) " + last
	if err := tx.QueryRowxContext(planCtx, planSQL, scope, generation).Scan(&plan); err != nil {
		t.Logf("PostgreSQL summary EXPLAIN ANALYZE failed: %v", err)
	} else {
		t.Logf("PostgreSQL summary EXPLAIN ANALYZE BUFFERS: %s", plan)
	}
	if err := tx.Rollback(); err != nil {
		t.Logf("PostgreSQL summary plan rollback failed: %v", err)
	}
}

func (f *usageBenchFixture) finishRetention(t testing.TB) {
	t.Helper()
	for {
		done, err := f.repo.BackfillUsageHistory(t.Context(), 200)
		if err != nil {
			t.Fatalf("usage retention backfill page=%d: %v", f.retentionPages+1, err)
		}
		f.retentionPages++
		if f.retentionPages%100 == 0 {
			t.Logf("usage fixture retention backfill pages=%d", f.retentionPages)
		}
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
	}{{"usage_daily_summaries", &f.storage.DailySummaries}, {"usage_lifetime_summaries", &f.storage.LifetimeSummaries}, {"usage_projection_provider_options", &f.storage.ProviderOptionDimensions}} {
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
	if complete != len(usageHistoryMigrationSources)+1 {
		t.Fatal("fixture retention migration is incomplete")
	}
}
