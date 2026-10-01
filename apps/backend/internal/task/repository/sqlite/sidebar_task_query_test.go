package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestQuerySidebarTaskPageFiltersBeforePaging(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sidebar-page")
	base := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 101; index++ {
		title := fmt.Sprintf("Task %03d", index)
		if index == 100 {
			title = "Needle on later source row"
		}
		if err := repo.CreateTask(ctx, &models.Task{
			ID: fmt.Sprintf("task-%03d", index), WorkspaceID: "ws-sidebar-page", Title: title,
			CreatedAt: base.Add(time.Duration(index) * time.Minute),
			UpdatedAt: base.Add(time.Duration(index) * time.Minute),
		}); err != nil {
			t.Fatalf("create task %d: %v", index, err)
		}
	}

	query := sidebarTaskQuery(1)
	first, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-page", query, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query first page: %v", err)
	}
	if first.TotalVisibleTasks != 101 || first.TotalEntries != 102 || len(first.Tasks) != 100 || !first.HasNext {
		t.Fatalf("first page metadata = visible %d rows %d next %v", first.TotalVisibleTasks, len(first.Tasks), first.HasNext)
	}
	query.Page = 2
	second, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-page", query, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query second page: %v", err)
	}
	if second.QueryKey != first.QueryKey || second.Page != 2 || len(second.Tasks) != 1 || second.HasNext {
		t.Fatalf("second page key/page/tasks/next = %q/%d/%d/%v", second.QueryKey, second.Page, len(second.Tasks), second.HasNext)
	}

	query.Filters = []models.SidebarTaskViewClause{{
		Dimension: "titleMatch", Op: "matches", Value: json.RawMessage(`"needle"`),
	}}
	filtered, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-page", query, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query filtered page: %v", err)
	}
	if filtered.TotalTasks != 1 || len(filtered.Tasks) != 1 || filtered.Tasks[0].ID != "task-100" {
		t.Fatalf("filtered tasks = total %d tasks %+v, want the match beyond source row 100", filtered.TotalTasks, sidebarTaskIDs(filtered.Tasks))
	}
}

func TestSidebarTaskViewConformance(t *testing.T) {
	fixtureBytes, err := os.ReadFile(filepath.Join("..", "testdata", "sidebar-task-views.json"))
	if err != nil {
		t.Fatalf("read sidebar view conformance fixture: %v", err)
	}
	var fixtureSet struct {
		Fixtures []struct {
			Name              string                      `json:"name"`
			WorkspaceID       string                      `json:"workspace_id"`
			TaskIDPrefix      string                      `json:"task_id_prefix"`
			SourceCount       int                         `json:"source_count"`
			MatchingTaskIndex int                         `json:"matching_task_index"`
			TitlePrefix       string                      `json:"title_prefix"`
			MatchingTitle     string                      `json:"matching_title"`
			Query             models.SidebarTaskViewQuery `json:"query"`
			ExpectedTaskIDs   []string                    `json:"expected_task_ids"`
		} `json:"fixtures"`
	}
	if err := json.Unmarshal(fixtureBytes, &fixtureSet); err != nil {
		t.Fatalf("decode sidebar view conformance fixture: %v", err)
	}

	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			ctx := context.Background()
			for _, fixture := range fixtureSet.Fixtures {
				workspaceID := fixture.WorkspaceID + "-" + backend
				seedWorkspace(t, repo, workspaceID)
				base := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
				for index := 0; index < fixture.SourceCount; index++ {
					title := fixture.TitlePrefix
					if index == fixture.MatchingTaskIndex {
						title = fixture.MatchingTitle
					} else {
						title = fmt.Sprintf("%s %03d", title, index)
					}
					if err := repo.CreateTask(ctx, &models.Task{
						ID:          fmt.Sprintf("%s%03d", fixture.TaskIDPrefix, index),
						WorkspaceID: workspaceID,
						Title:       title,
						CreatedAt:   base.Add(time.Duration(index) * time.Minute),
						UpdatedAt:   base.Add(time.Duration(index) * time.Minute),
					}); err != nil {
						t.Fatalf("%s: create source task %d: %v", fixture.Name, index, err)
					}
				}

				page, err := repo.QuerySidebarTaskPage(ctx, workspaceID, fixture.Query, models.SidebarTaskViewPreferences{})
				if err != nil {
					t.Fatalf("%s: query complete view: %v", fixture.Name, err)
				}
				if got := sidebarTaskIDs(page.Tasks); fmt.Sprint(got) != fmt.Sprint(fixture.ExpectedTaskIDs) {
					t.Fatalf("%s: task IDs = %v, want %v", fixture.Name, got, fixture.ExpectedTaskIDs)
				}
			}
		})
	}
}

func newRepoForSidebarConformance(t *testing.T, backend string) *Repository {
	t.Helper()
	var database *sqlx.DB
	if backend == "postgres" {
		database = testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	} else {
		connection, err := db.OpenSQLite(filepath.Join(t.TempDir(), "sidebar-view-conformance.db"))
		if err != nil {
			t.Fatalf("open conformance sqlite: %v", err)
		}
		database = sqlx.NewDb(connection, "sqlite3")
		t.Cleanup(func() { _ = database.Close() })
	}
	repo, err := NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("initialize %s conformance repository: %v", backend, err)
	}
	return repo
}

func TestQuerySidebarTaskPageUsesReaderPoolForSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sidebar-reader-pool.db")
	writerConn, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open writer pool: %v", err)
	}
	writer := sqlx.NewDb(writerConn, "sqlite3")
	readerConn, err := db.OpenSQLiteReader(path)
	if err != nil {
		_ = writer.Close()
		t.Fatalf("open reader pool: %v", err)
	}
	reader := sqlx.NewDb(readerConn, "sqlite3")
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	repo, err := NewWithDB(writer, reader, nil)
	if err != nil {
		t.Fatalf("initialize repository: %v", err)
	}
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sidebar-reader-pool")
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "reader-task", WorkspaceID: "ws-sidebar-reader-pool", Title: "Reader task",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	// The production writer pool has one connection. Holding it must not stall
	// the read-only snapshot query.
	heldWriterConn, err := writer.Conn(ctx)
	if err != nil {
		t.Fatalf("hold writer pool connection: %v", err)
	}
	defer func() { _ = heldWriterConn.Close() }()
	queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, queryErr := repo.QuerySidebarTaskPage(queryCtx, "ws-sidebar-reader-pool", sidebarTaskQuery(1), models.SidebarTaskViewPreferences{})
		done <- queryErr
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("query while writer is occupied: %v", err)
		}
	case <-queryCtx.Done():
		t.Fatalf("sidebar snapshot waited for the occupied writer connection: %v", queryCtx.Err())
	}
}

func TestQuerySidebarTaskPageBoundsTenThousandTasks(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sidebar-ten-thousand")
	insertSidebarScaleTasks(t, repo, "ws-sidebar-ten-thousand", "scale", 10000)

	started := time.Now()
	page, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-ten-thousand", sidebarTaskQuery(1), models.SidebarTaskViewPreferences{})
	t.Logf("10,000 active task query: %s", time.Since(started))
	if err != nil {
		t.Fatalf("query first scale page: %v", err)
	}
	if len(page.Tasks) != models.MaxSidebarTaskPageSize || page.TotalTasks != 10000 || page.TotalVisibleTasks != 10000 {
		t.Fatalf("scale page returned %d task rows, total %d, visible %d", len(page.Tasks), page.TotalTasks, page.TotalVisibleTasks)
	}
	archivedQuery := sidebarTaskQuery(1)
	archivedAt := time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET archived_at = ? WHERE workspace_id = ?`), archivedAt, "ws-sidebar-ten-thousand"); err != nil {
		t.Fatalf("archive scale tasks: %v", err)
	}
	archivedQuery.Filters = []models.SidebarTaskViewClause{{
		Dimension: "archived", Op: "is", Value: json.RawMessage(`true`),
	}}
	started = time.Now()
	archivedPage, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-ten-thousand", archivedQuery, models.SidebarTaskViewPreferences{})
	t.Logf("10,000 archived task query: %s", time.Since(started))
	if err != nil {
		t.Fatalf("query archived scale page: %v", err)
	}
	if len(archivedPage.Tasks) != models.MaxSidebarTaskPageSize || archivedPage.TotalTasks != 10000 || archivedPage.TotalVisibleTasks != 10000 {
		t.Fatalf("archived scale page returned %d task rows, total %d, visible %d", len(archivedPage.Tasks), archivedPage.TotalTasks, archivedPage.TotalVisibleTasks)
	}
}

func insertSidebarScaleTasks(t testing.TB, repo *Repository, workspaceID, prefix string, count int) {
	t.Helper()
	tx, err := repo.db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin sidebar scale fixture: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	base := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	const batchSize = 200
	for start := 0; start < count; start += batchSize {
		end := min(start+batchSize, count)
		values := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*7)
		for index := start; index < end; index++ {
			values = append(values, "(?, ?, ?, ?, ?, ?, ?)")
			parentID := ""
			if index%2 == 1 {
				parentID = fmt.Sprintf("%s-task-%06d", prefix, index-1)
			}
			args = append(args,
				fmt.Sprintf("%s-task-%06d", prefix, index), workspaceID,
				fmt.Sprintf("Scale %s task %06d", prefix, index), parentID, nil, base, base,
			)
		}
		query := `INSERT INTO tasks (id, workspace_id, title, parent_id, archived_at, created_at, updated_at) VALUES ` + strings.Join(values, ", ")
		if _, err := tx.ExecContext(context.Background(), repo.db.Rebind(query), args...); err != nil {
			t.Fatalf("insert sidebar scale tasks %d through %d: %v", start, end-1, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit sidebar scale fixture: %v", err)
	}
}

func BenchmarkSidebarTaskPage100K(b *testing.B) {
	for _, backend := range []string{"sqlite", "postgres"} {
		b.Run(backend, func(b *testing.B) {
			repo := newRepoForSidebarBenchmark(b, backend)
			workspaceID := "ws-sidebar-benchmark-100k"
			seedWorkspace(b, repo, workspaceID)
			insertSidebarBenchmarkTasks(b, repo, workspaceID, 100000)
			for _, group := range []string{"none", "state", "repository"} {
				b.Run(group, func(b *testing.B) {
					for _, page := range []struct {
						name   string
						number int
					}{{"first", 1}, {"middle", 500}, {"final", 1000}} {
						b.Run(page.name, func(b *testing.B) {
							query := sidebarTaskQuery(page.number)
							query.Sort = models.SidebarTaskViewSort{Key: "lastActivityAt", Direction: "desc"}
							query.Group = group
							benchmarkSidebarPage(b, repo, workspaceID, query)
						})
					}
				})
			}
		})
	}
}

func benchmarkSidebarPage(b *testing.B, repo *Repository, workspaceID string, query models.SidebarTaskViewQuery) {
	b.Helper()
	if dialect.IsPostgres(repo.ro.DriverName()) {
		logSidebarTaskPageQueryPlan(b, repo, workspaceID, query)
	}
	baselineNative, baselineRSS := sidebarBenchmarkMemory(b, true)
	started := time.Now()
	warmup, err := benchmarkSidebarColdRead(b, repo, workspaceID, query)
	if err != nil {
		b.Fatal(err)
	}
	cold := time.Since(started)
	if len(warmup.Tasks) != 100 || warmup.TotalVisibleTasks != 100000 || warmup.Page != query.Page {
		b.Fatalf("unexpected benchmark page bounds: tasks=%d total=%d page=%d", len(warmup.Tasks), warmup.TotalVisibleTasks, warmup.Page)
	}
	b.Logf("cold_execution=%s dialect=%s group=%s page=%d", cold, repo.ro.DriverName(), query.Group, query.Page)
	b.ReportAllocs()
	b.ResetTimer()
	var responseBytes int
	for range b.N {
		ctx, cancel := context.WithTimeout(b.Context(), 30*time.Second)
		result, err := repo.QuerySidebarTaskPage(ctx, workspaceID, query, models.SidebarTaskViewPreferences{})
		cancel()
		if err != nil {
			b.Fatal(err)
		}
		payload, err := marshalSidebarBenchmarkResponse(result)
		if err != nil {
			b.Fatal(err)
		}
		responseBytes = len(payload)
	}
	b.StopTimer()
	peak, rss := sidebarBenchmarkMemory(b, false)
	b.ReportMetric(float64(responseBytes), "response-B")
	b.ReportMetric(float64(cold.Nanoseconds()), "cold-ns")
	b.ReportMetric(float64(peak-baselineNative), "native-peak-B")
	b.ReportMetric(float64(rss-baselineRSS), "rss-delta-B")
	if !dialect.IsPostgres(repo.ro.DriverName()) {
		logSidebarTaskPageQueryPlan(b, repo, workspaceID, query)
	} else if query.Page == 1 {
		logSidebarPostgresExecution(b, repo, workspaceID, query)
	}
}

type sidebarBenchmarkEntry struct {
	models.SidebarTaskPageEntry
	Task *models.Task `json:"task,omitempty"`
}

type sidebarBenchmarkResponse struct {
	QueryKey          string                  `json:"query_key"`
	Page              int                     `json:"page"`
	PageSize          int                     `json:"page_size"`
	TotalEntries      int                     `json:"total_entries"`
	TotalTasks        int                     `json:"total_tasks"`
	TotalVisibleTasks int                     `json:"total_visible_tasks"`
	HasPrevious       bool                    `json:"has_previous"`
	HasNext           bool                    `json:"has_next"`
	Entries           []sidebarBenchmarkEntry `json:"entries"`
}

func marshalSidebarBenchmarkResponse(result *models.SidebarTaskPageResult) ([]byte, error) {
	tasks := make(map[string]*models.Task, len(result.Tasks))
	for _, task := range result.Tasks {
		tasks[task.ID] = task
	}
	entries := make([]sidebarBenchmarkEntry, 0, len(result.Entries))
	for _, entry := range result.Entries {
		entries = append(entries, sidebarBenchmarkEntry{SidebarTaskPageEntry: entry, Task: tasks[entry.TaskID]})
	}
	return json.Marshal(sidebarBenchmarkResponse{
		QueryKey: result.QueryKey, Page: result.Page, PageSize: result.PageSize,
		TotalEntries: result.TotalEntries, TotalTasks: result.TotalTasks,
		TotalVisibleTasks: result.TotalVisibleTasks, HasPrevious: result.HasPrevious,
		HasNext: result.HasNext, Entries: entries,
	})
}

func newRepoForSidebarBenchmark(b *testing.B, backend string) *Repository {
	b.Helper()
	var database *sqlx.DB
	if backend == "postgres" {
		database = testutil.OpenIsolatedPostgres(b, testutil.PostgresDSNFromEnv(b))
	} else {
		connection, err := db.OpenSQLite(filepath.Join(b.TempDir(), "sidebar-benchmark.db"))
		if err != nil {
			b.Fatalf("open benchmark sqlite: %v", err)
		}
		database = sqlx.NewDb(connection, "sqlite3")
		b.Cleanup(func() { _ = database.Close() })
	}
	repo, err := NewWithDB(database, database, nil)
	if err != nil {
		b.Fatalf("initialize %s benchmark repository: %v", backend, err)
	}
	return repo
}

func insertSidebarBenchmarkTasks(b *testing.B, repo *Repository, workspaceID string, count int) {
	b.Helper()
	ctx := context.Background()
	base := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	seedSidebarBenchmarkRepositories(b, repo, workspaceID)
	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		b.Fatalf("begin sidebar benchmark fixture: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	const batchSize = 200
	for start := 0; start < count; start += batchSize {
		end := min(start+batchSize, count)
		taskValues := make([]string, 0, end-start)
		taskArgs := make([]any, 0, (end-start)*8)
		summaryValues := make([]string, 0, end-start)
		summaryArgs := make([]any, 0, (end-start)*5)
		for index := start; index < end; index++ {
			id := fmt.Sprintf("bench-task-%06d", index)
			parentID := ""
			if index%10 != 0 {
				parentID = fmt.Sprintf("bench-task-%06d", index-index%10)
			}
			activity := base.Add(time.Duration(index) * time.Second)
			taskValues = append(taskValues, "(?, ?, ?, ?, ?, ?, ?, ?)")
			state := []string{"TODO", "IN_PROGRESS", "COMPLETED"}[(index/10)%3]
			taskArgs = append(taskArgs, id, workspaceID, fmt.Sprintf("Sidebar benchmark task %06d", index), parentID, nil, activity, activity, state)
			summaryValues = append(summaryValues, "(?, ?, ?, ?, ?)")
			summaryArgs = append(summaryArgs, id, workspaceID, 1, fmt.Sprintf(`{"last_activity_at":%q}`, activity.Format(time.RFC3339Nano)), activity)
		}
		if _, err := tx.ExecContext(ctx, repo.db.Rebind(`INSERT INTO tasks (id, workspace_id, title, parent_id, archived_at, created_at, updated_at, state) VALUES `+strings.Join(taskValues, ", ")), taskArgs...); err != nil {
			b.Fatalf("insert sidebar benchmark tasks %d through %d: %v", start, end-1, err)
		}
		if _, err := tx.ExecContext(ctx, repo.db.Rebind(`INSERT INTO task_status_summaries (task_id, workspace_id, revision, summary, updated_at) VALUES `+strings.Join(summaryValues, ", ")), summaryArgs...); err != nil {
			b.Fatalf("insert sidebar benchmark summaries %d through %d: %v", start, end-1, err)
		}
	}
	if err := insertSidebarBenchmarkRepositories(b.Context(), tx, repo.db, workspaceID); err != nil {
		b.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		b.Fatalf("commit sidebar benchmark fixture: %v", err)
	}
	if dialect.IsPostgres(repo.db.DriverName()) {
		for _, table := range []string{"tasks", "task_status_summaries", "repositories", "task_repositories"} {
			if _, err := repo.db.ExecContext(ctx, "ANALYZE "+table); err != nil {
				b.Fatalf("analyze sidebar benchmark %s: %v", table, err)
			}
		}
	}
}

func logSidebarTaskPageQueryPlan(b *testing.B, repo *Repository, workspaceID string, query models.SidebarTaskViewQuery) {
	b.Helper()
	driver := repo.ro.DriverName()
	snapshot, err := beginSidebarQuerySnapshot(b.Context(), repo.ro)
	if err != nil {
		b.Fatal(err)
	}
	defer snapshot.close()
	baseSQL, baseArgs, err := snapshot.prepare(b.Context(), driver, workspaceID, query)
	if err != nil {
		b.Fatal(err)
	}
	pageCTEs, cteArgs := sidebarPageCTEs(driver, query, models.SidebarTaskViewPreferences{})
	args := append(append([]any(nil), baseArgs...), cteArgs...)
	explain := "EXPLAIN "
	if !dialect.IsPostgres(driver) {
		explain = "EXPLAIN QUERY PLAN "
	}
	pageSQL := sidebarPageSelectSQL(query.Group == "none")
	rows, err := snapshot.tx.QueryContext(b.Context(), repo.ro.Rebind(explain+baseSQL+pageCTEs+pageSQL), args...)
	if err != nil {
		b.Fatalf("explain sidebar benchmark page query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() && len(plan) < 500 {
		if dialect.IsPostgres(driver) {
			var line string
			if err := rows.Scan(&line); err != nil {
				b.Fatalf("scan postgres sidebar query plan: %v", err)
			}
			plan = append(plan, line)
			continue
		}
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			b.Fatalf("scan sqlite sidebar query plan: %v", err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		b.Fatalf("read sidebar benchmark query plan: %v", err)
	}
	b.Logf("%s page query plan (first 500 nodes):\n%s", driver, strings.Join(plan, "\n"))
}

func TestQuerySidebarTaskPageClampsAndAppliesCollapsedVisibility(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sidebar-collapse")
	for _, task := range []*models.Task{
		{ID: "parent", WorkspaceID: "ws-sidebar-collapse", Title: "Parent"},
		{ID: "child", WorkspaceID: "ws-sidebar-collapse", ParentID: "parent", Title: "Child"},
		{ID: "grandchild", WorkspaceID: "ws-sidebar-collapse", ParentID: "child", Title: "Grandchild"},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}
	query := sidebarTaskQuery(99)
	query.CollapsedTaskIDs = []string{"parent"}
	page, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-collapse", query, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query collapsed page: %v", err)
	}
	if page.Page != 1 || page.TotalTasks != 3 || page.TotalVisibleTasks != 1 || len(page.Tasks) != 1 || page.Tasks[0].ID != "parent" {
		t.Fatalf("collapsed page = page %d total %d visible %d tasks %v", page.Page, page.TotalTasks, page.TotalVisibleTasks, sidebarTaskIDs(page.Tasks))
	}
	if len(page.Entries) != 2 || page.Entries[1].Kind != "task" || page.Entries[1].TaskID != "parent" {
		t.Fatalf("collapsed entries = %+v", page.Entries)
	}
}

func TestQuerySidebarTaskPageKeepsCollapsedUngroupedHeaderCount(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sidebar-ungrouped-collapse")
	for _, task := range []*models.Task{
		{ID: "root", WorkspaceID: "ws-sidebar-ungrouped-collapse", Title: "Root"},
		{ID: "child", WorkspaceID: "ws-sidebar-ungrouped-collapse", ParentID: "root", Title: "Child"},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}
	query := sidebarTaskQuery(1)
	query.CollapsedGroupKeys = []string{"__all__"}
	page, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-ungrouped-collapse", query, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query collapsed ungrouped page: %v", err)
	}
	if page.TotalTasks != 2 || page.TotalVisibleTasks != 0 || page.TotalEntries != 1 || len(page.Tasks) != 0 {
		t.Fatalf("collapsed ungrouped page totals = tasks %d visible %d entries %d tasks %v",
			page.TotalTasks, page.TotalVisibleTasks, page.TotalEntries, sidebarTaskIDs(page.Tasks))
	}
	if len(page.Entries) != 1 || page.Entries[0].Kind != "group" || page.Entries[0].MatchingCount != 2 {
		t.Fatalf("collapsed ungrouped header = %+v, want group count 2", page.Entries)
	}
}

func TestQuerySidebarTaskPagePromotesFilteredChildWithoutContinuationContext(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sidebar-filtered-parent")
	for _, task := range []*models.Task{
		{ID: "parent", WorkspaceID: "ws-sidebar-filtered-parent", Title: "Parent"},
		{ID: "child", WorkspaceID: "ws-sidebar-filtered-parent", ParentID: "parent", Title: "Needle child"},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}
	query := sidebarTaskQuery(1)
	query.Filters = []models.SidebarTaskViewClause{{
		Dimension: "titleMatch", Op: "matches", Value: json.RawMessage(`"needle"`),
	}}
	page, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-filtered-parent", query, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query filtered child: %v", err)
	}
	if len(page.Tasks) != 1 || page.Tasks[0].ID != "child" {
		t.Fatalf("filtered child tasks = %v, want child promoted to the view root", sidebarTaskIDs(page.Tasks))
	}
	if len(page.Entries) != 2 || page.Entries[1].Kind != "task" || page.Entries[1].Depth != 0 || page.Entries[1].ParentID != "" {
		t.Fatalf("filtered child entries = %+v, want only group and promoted root", page.Entries)
	}
}
