//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryProviderOptionDenseWorkPagePlans(t *testing.T) {
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			if engine == "postgres" {
				require.NotEmpty(t, os.Getenv("KANDEV_TEST_POSTGRES_DSN"), "dense-work plans require the actual PostgreSQL test database")
				t.Setenv("KANDEV_USAGE_BENCH_TRACE_POSTGRES", "1")
			} else {
				t.Setenv("KANDEV_USAGE_BENCH_TRACE_SQLITE", "1")
			}
			fixture := newUsageBenchFixture(t, engine, usageBenchSize{})
			const factCount = 401
			var records []usageBenchSeedRecord
			for index := range factCount {
				itemID := fmt.Sprintf("dense-provider-%03d", index)
				at := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(index) * time.Minute)
				measurement := usageBenchMeasurement(fixture.subjects[6], itemID, at, "plugin:bench")
				measurement.Model, measurement.Provider = "dense-provider-model", fmt.Sprintf("provider-%03d", index)
				record := fixture.seedRecord(t, fixture.subjects[6], measurement)
				record.input.Segment = &pluginsdk.UsageSegmentIdentityV1{DecompositionID: "dense-provider-v1", SegmentID: itemID}
				usageBenchSeal(t, record.input)
				records = append(records, record)
				if len(records) == 200 {
					fixture.admitSeedBatch(t, records)
					records = nil
				}
			}
			fixture.admitSeedBatch(t, records)
			fixture.finishRetention(t)
			generation, err := fixture.repo.StartUsageHistoryRebuild(t.Context(), fixture.subjects[6].scope)
			require.NoError(t, err)
			work := fixture.subjects[6].work
			var build string
			for range 5000 {
				_, err := fixture.repo.AdvanceUsageHistoryProjection(t.Context(), fixture.subjects[6].scope, generation, 200)
				require.NoError(t, err)
				var ready int
				err = fixture.database.Get(&ready, fixture.database.Rebind(`SELECT provider_options_ready FROM usage_projection_history WHERE scope_id=? AND generation=? AND work_key=?`), fixture.subjects[6].scope, generation, work)
				if err == nil {
					err = fixture.database.Get(&build, fixture.database.Rebind(`SELECT build_id FROM usage_projection_applied WHERE scope_id=? AND generation=? AND work_key=?`), fixture.subjects[6].scope, generation, work)
					require.NoError(t, err)
					var phase string
					require.NoError(t, fixture.database.Get(&phase, fixture.database.Rebind(`SELECT provider_options_phase FROM usage_projection_history WHERE scope_id=? AND generation=? AND work_key=?`), fixture.subjects[6].scope, generation, work))
					if ready == 0 && phase == "build" {
						break
					}
				} else {
					require.ErrorIs(t, err, sql.ErrNoRows)
				}
			}
			var retained struct {
				Facts         int64 `db:"facts"`
				Works         int64 `db:"works"`
				Contributions int64 `db:"contributions"`
			}
			require.NoError(t, fixture.database.Get(&retained, fixture.database.Rebind(`SELECT
				(SELECT COUNT(*) FROM usage_canonical_facts WHERE scope_id=? AND generation=? AND work_key=?) AS facts,
				(SELECT COUNT(DISTINCT work_key) FROM usage_canonical_facts WHERE scope_id=? AND generation=?) AS works,
				(SELECT COUNT(*) FROM usage_projection_contributions WHERE scope_id=? AND generation=? AND work_key=? AND build_id=?) AS contributions`),
				fixture.subjects[6].scope, generation, work, fixture.subjects[6].scope, generation,
				fixture.subjects[6].scope, generation, work, build))
			require.EqualValues(t, factCount, retained.Facts)
			require.EqualValues(t, 1, retained.Works)
			require.Greater(t, retained.Contributions, int64(200), "the plan must face a dense work larger than one page")
			const plannedFactCount = 50001
			usageProviderInsertDensePlanRows(t, fixture, fixture.subjects[6].scope, generation, work, build, plannedFactCount-factCount)
			require.NoError(t, fixture.database.Get(&retained.Facts, fixture.database.Rebind(`SELECT COUNT(*) FROM usage_canonical_facts WHERE scope_id=? AND generation=? AND work_key=? AND build_id=?`), fixture.subjects[6].scope, generation, work, build))
			require.NoError(t, fixture.database.Get(&retained.Contributions, fixture.database.Rebind(`SELECT COUNT(*) FROM usage_projection_contributions WHERE scope_id=? AND generation=? AND work_key=? AND build_id=?`), fixture.subjects[6].scope, generation, work, build))
			require.EqualValues(t, plannedFactCount, retained.Facts, "the plan fixture must contain 50,001 canonical facts in one work")
			require.EqualValues(t, plannedFactCount*2, retained.Contributions, "each dense fact must have its two source metric rows")
			t.Logf("dense plan fixture: %d canonical facts / %d contribution rows in one work; %d facts came from normal admission and projection", retained.Facts, retained.Contributions, factCount)

			for _, table := range []string{"usage_canonical_facts", "usage_projection_contributions"} {
				_, err = fixture.database.ExecContext(t.Context(), `ANALYZE `+table)
				require.NoError(t, err)
			}
			fixture.observer.startWithLimit(32, false)
			_, err = fixture.repo.AdvanceUsageHistoryProjection(t.Context(), fixture.subjects[6].scope, generation, 200)
			require.NoError(t, err)
			statements, err := fixture.observer.finish()
			require.NoError(t, err)
			pageQuery := usageProviderFactKeyPageStatement(t, statements)
			insertQuery := usageProviderBuildStatement(t, statements, "INSERT INTO usage_projection_provider_options")
			if engine == "postgres" {
				var indexes []string
				require.NoError(t, fixture.database.Select(&indexes, `SELECT indexname || ': ' || indexdef FROM pg_indexes
					WHERE schemaname=current_schema() AND tablename IN ('usage_canonical_facts','usage_projection_contributions') ORDER BY indexname`))
				t.Logf("PostgreSQL dense source indexes: %s", strings.Join(indexes, " | "))
				t.Logf("PostgreSQL dense fact page SQL: %s", pageQuery.SQL)
				require.NoError(t, explainUsageProviderBuildPostgres(t.Context(), fixture.database, pageQuery, insertQuery))
				factVisits, factIndex := usageProviderPostgresScan(pageQuery.PostgresPlan, "usage_canonical_facts")
				contributionVisits, contributionIndex := usageProviderPostgresScan(insertQuery.PostgresPlan, "usage_projection_contributions")
				t.Logf("PostgreSQL dense fact page plan: %s", string(pageQuery.PostgresPlan))
				t.Logf("PostgreSQL dense provider insert plan: %s", string(insertQuery.PostgresPlan))
				require.True(t, factIndex, "fact-key pages must seek through a canonical fact index")
				require.LessOrEqual(t, factVisits, float64(200), "one fact page must visit at most 200 canonical facts")
				require.True(t, contributionIndex, "page inserts must use a contribution index")
				require.Less(t, contributionVisits, float64(retained.Contributions), "a page insert must not rescan the full work's contributions")
			} else {
				factVisits, factIndex := usageProviderSQLiteScan(t, pageQuery, "usage_canonical_facts")
				var cursor string
				require.NoError(t, fixture.database.Get(&cursor, fixture.database.Rebind(`SELECT provider_options_cursor FROM usage_projection_history WHERE scope_id=? AND generation=? AND work_key=?`), fixture.subjects[6].scope, generation, work))
				var factKeys []string
				require.NoError(t, fixture.database.Select(&factKeys, fixture.database.Rebind(`SELECT fact_key FROM usage_canonical_facts
					WHERE scope_id=? AND generation=? AND work_key=? AND build_id=? AND fact_key<=? ORDER BY fact_key LIMIT 200`), fixture.subjects[6].scope, generation, work, build, cursor))
				require.Len(t, factKeys, 200)
				selectSQL, selectArgs := usageProjectionProviderOptionsSelect(usageProjectionProviderOptionWork{Scope: fixture.subjects[6].scope, Generation: generation, Work: work, AppliedBuild: build}, factKeys)
				fixture.observer.startWithLimit(8, false)
				var selected []struct {
					Scope    string `db:"scope_id"`
					Gen      string `db:"generation"`
					Work     string `db:"work_key"`
					Model    string `db:"model"`
					Source   string `db:"source"`
					Provider string `db:"provider"`
				}
				require.NoError(t, fixture.database.Select(&selected, fixture.database.Rebind(selectSQL), selectArgs...))
				require.Positive(t, len(selected))
				selectStatements, err := fixture.observer.finish()
				require.NoError(t, err)
				providerSelect := usageProviderBuildStatement(t, selectStatements, "SELECT DISTINCT c.scope_id")
				contributionVisits, contributionIndex := usageProviderSQLiteScan(t, providerSelect, "usage_projection_contributions")
				insertPlans, err := usageProviderSQLiteInsertPlan(t.Context(), fixture.database, selectSQL, selectArgs)
				require.NoError(t, err)
				insertPlan := strings.Join(insertPlans, " | ")
				t.Logf("SQLite provider INSERT SELECT plan: %s", insertPlan)
				contributionIndex = contributionIndex || strings.Contains(insertPlan, "USING INDEX") || strings.Contains(insertPlan, "USING COVERING INDEX")
				t.Logf("SQLite dense fact page SQL: %s", pageQuery.SQL)
				t.Logf("SQLite dense provider insert SQL: %s", insertQuery.SQL)
				require.True(t, factIndex, "fact-key pages must seek through a canonical fact index")
				require.LessOrEqual(t, factVisits, int64(200), "one fact page must visit at most 200 canonical facts")
				require.True(t, contributionIndex, "page inserts must use a contribution index")
				require.Less(t, contributionVisits, retained.Contributions, "a page insert must not rescan the full work's contributions")
			}
			var cursor string
			var ready int
			require.NoError(t, fixture.database.Get(&cursor, fixture.database.Rebind(`SELECT provider_options_cursor FROM usage_projection_history WHERE scope_id=? AND generation=? AND work_key=?`), fixture.subjects[6].scope, generation, work))
			require.NoError(t, fixture.database.Get(&ready, fixture.database.Rebind(`SELECT provider_options_ready FROM usage_projection_history WHERE scope_id=? AND generation=? AND work_key=?`), fixture.subjects[6].scope, generation, work))
			require.Zero(t, ready)
			var cursorFacts int
			require.NoError(t, fixture.database.Get(&cursorFacts, fixture.database.Rebind(`SELECT COUNT(*) FROM usage_canonical_facts WHERE scope_id=? AND generation=? AND work_key=? AND build_id=? AND fact_key<=?`), fixture.subjects[6].scope, generation, work, build, cursor))
			require.Equal(t, 200, cursorFacts, "the persisted cursor must advance by exactly one bounded fact page")
		})
	}
}

func usageProviderInsertDensePlanRows(t *testing.T, fixture *usageBenchFixture, scope, generation, work, build string, count int) {
	t.Helper()
	var templateFact string
	require.NoError(t, fixture.database.Get(&templateFact, fixture.database.Rebind(`SELECT fact_key FROM usage_canonical_facts
		WHERE scope_id=? AND generation=? AND work_key=? AND build_id=? ORDER BY fact_key LIMIT 1`), scope, generation, work, build))
	const pageSize = 500
	for start := 0; start < count; start += pageSize {
		end := min(start+pageSize, count)
		keys := make([]string, end-start)
		for index := range keys {
			keys[index] = fmt.Sprintf("z-provider-dense-%05d", start+index)
		}
		values := make([]string, len(keys))
		for index := range values {
			values[index] = "(?)"
		}
		valuesSQL := strings.Join(values, ",")
		arguments := append(stringArgs(keys), scope, generation, work, build)
		canonicalQuery := `WITH extra(fact_key) AS (VALUES ` + valuesSQL + `), base AS (
			SELECT scope_id,generation,build_id,work_key,subject_id,task_id,session_id,coverage_kind,coverage_start,coverage_end,
				source_date,calendar,model,provider,token_source,cost_source,payload,collected_at,segment_key,bundle_key
			FROM usage_canonical_facts WHERE scope_id=? AND generation=? AND work_key=? AND build_id=? ORDER BY fact_key LIMIT 1)
			INSERT INTO usage_canonical_facts(scope_id,generation,build_id,fact_key,work_key,subject_id,task_id,session_id,coverage_kind,
				coverage_start,coverage_end,source_date,calendar,model,provider,token_source,cost_source,payload,collected_at,segment_key,bundle_key)
			SELECT b.scope_id,b.generation,b.build_id,e.fact_key,b.work_key,b.subject_id,b.task_id,b.session_id,b.coverage_kind,
				b.coverage_start,b.coverage_end,b.source_date,b.calendar,b.model,b.provider,b.token_source,b.cost_source,b.payload,b.collected_at,b.segment_key,b.bundle_key
			FROM base b CROSS JOIN extra e`
		_, err := fixture.database.ExecContext(t.Context(), fixture.database.Rebind(canonicalQuery), arguments...)
		require.NoError(t, err)

		arguments = append(append([]any(nil), stringArgs(keys)...), scope, generation, work, build, templateFact)
		contributionQuery := `WITH extra(fact_key) AS (VALUES ` + valuesSQL + `), base AS (
			SELECT scope_id,generation,build_id,work_key,summary_key,coverage_kind,coverage_start,coverage_end,metric_kind,actor_id,
				actor_status,actor_kind,source,model,provider,agent_client,semantics,currency,calendar,usage_date,input_tokens,output_tokens,
				cache_read_tokens,cache_write_tokens,reasoning_tokens,total_tokens,source_total_tokens,turns,cost_subcents,rate_tokens,partial,estimated,stale,cost_basis
			FROM usage_projection_contributions WHERE scope_id=? AND generation=? AND work_key=? AND build_id=? AND fact_key=?
			ORDER BY metric_kind,calendar,usage_date)
			INSERT INTO usage_projection_contributions(scope_id,generation,build_id,fact_key,work_key,summary_key,coverage_kind,coverage_start,coverage_end,
				metric_kind,actor_id,actor_status,actor_kind,source,model,provider,agent_client,semantics,currency,calendar,usage_date,input_tokens,output_tokens,
				cache_read_tokens,cache_write_tokens,reasoning_tokens,total_tokens,source_total_tokens,turns,cost_subcents,rate_tokens,partial,estimated,stale,cost_basis)
			SELECT b.scope_id,b.generation,b.build_id,e.fact_key,b.work_key,b.summary_key,b.coverage_kind,b.coverage_start,b.coverage_end,
				b.metric_kind,b.actor_id,b.actor_status,b.actor_kind,b.source,b.model,b.provider,b.agent_client,b.semantics,b.currency,b.calendar,b.usage_date,
				b.input_tokens,b.output_tokens,b.cache_read_tokens,b.cache_write_tokens,b.reasoning_tokens,b.total_tokens,b.source_total_tokens,b.turns,
				b.cost_subcents,b.rate_tokens,b.partial,b.estimated,b.stale,b.cost_basis FROM base b CROSS JOIN extra e`
		_, err = fixture.database.ExecContext(t.Context(), fixture.database.Rebind(contributionQuery), arguments...)
		require.NoError(t, err)
	}
}

func usageProviderSQLiteInsertPlan(ctx context.Context, database *sqlx.DB, selectSQL string, args []any) ([]string, error) {
	query := `EXPLAIN QUERY PLAN INSERT INTO usage_projection_provider_options(scope_id,generation,work_key,model,source,provider) ` + selectSQL +
		` ON CONFLICT(scope_id,generation,work_key,model,source,provider) DO NOTHING`
	rows, err := database.QueryxContext(ctx, database.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var plans []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			return nil, err
		}
		plans = append(plans, detail)
	}
	return plans, rows.Err()
}

func usageProviderBuildStatement(t *testing.T, statements []usageBenchStatement, prefix string) *usageBenchStatement {
	t.Helper()
	for index := range statements {
		if strings.HasPrefix(strings.TrimSpace(statements[index].SQL), prefix) {
			return &statements[index]
		}
	}
	t.Fatalf("missing provider page statement %q", prefix)
	return nil
}

func usageProviderFactKeyPageStatement(t *testing.T, statements []usageBenchStatement) *usageBenchStatement {
	t.Helper()
	for index := range statements {
		sql := strings.ToLower(strings.Join(strings.Fields(statements[index].SQL), " "))
		if !strings.HasPrefix(sql, "select fact_key from usage_canonical_facts") {
			continue
		}
		where := sql
		if beforeOrder, _, ok := strings.Cut(where, " order by "); ok {
			where = beforeOrder
		}
		if strings.Contains(where, "build_id") && !strings.Contains(where, "work_key") {
			return &statements[index]
		}
	}
	t.Fatal("missing provider-option fact-key page statement")
	return nil
}

func usageProviderSQLiteScan(t *testing.T, statement *usageBenchStatement, relation string) (int64, bool) {
	t.Helper()
	var visits int64
	var indexed bool
	for _, loop := range statement.SQLiteLoops {
		if strings.Contains(loop.Plan, relation) {
			visits += loop.Visits
			indexed = indexed || strings.Contains(loop.Plan, "INDEX")
			t.Logf("SQLite %s scan: %s; NLOOP=%d NVISIT=%d", relation, loop.Plan, loop.Loops, loop.Visits)
		}
	}
	return visits, indexed
}

func usageProviderPostgresScan(encoded json.RawMessage, relation string) (float64, bool) {
	var roots []struct {
		Plan usageBenchPostgresNode `json:"Plan"`
	}
	if json.Unmarshal(encoded, &roots) != nil {
		return 0, false
	}
	var visits float64
	var indexed bool
	var walk func(usageBenchPostgresNode)
	walk = func(node usageBenchPostgresNode) {
		if node.Relation == relation {
			visits += (node.Rows + node.Filtered + node.Rechecked) * node.Loops
		}
		indexed = indexed || node.Relation == relation && node.Index != ""
		for _, child := range node.Plans {
			walk(child)
		}
	}
	for _, root := range roots {
		walk(root.Plan)
	}
	return visits, indexed
}

func explainUsageProviderBuildPostgres(ctx context.Context, database *sqlx.DB, statements ...*usageBenchStatement) error {
	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, statement := range statements {
		args := make([]any, len(statement.Args))
		for index, argument := range statement.Args {
			args[index] = argument.Value
		}
		var plan []byte
		query := `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) ` + statement.SQL
		if err := tx.QueryRowContext(ctx, query, args...).Scan(&plan); err != nil {
			return err
		}
		if !json.Valid(plan) {
			return fmt.Errorf("PostgreSQL provider page returned an invalid execution plan")
		}
		statement.PostgresPlan = append(json.RawMessage(nil), plan...)
	}
	return tx.Rollback()
}
