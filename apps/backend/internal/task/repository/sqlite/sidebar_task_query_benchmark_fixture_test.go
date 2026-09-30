package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

func seedSidebarBenchmarkRepositories(b *testing.B, repo *Repository, workspaceID string) {
	b.Helper()
	for _, id := range []string{"benchmark-a", "benchmark-b"} {
		if err := repo.CreateRepository(b.Context(), &models.Repository{ID: id, WorkspaceID: workspaceID, Name: id, LocalPath: "/fixture/" + id}); err != nil {
			b.Fatal(err)
		}
	}
}

func insertSidebarBenchmarkRepositories(ctx context.Context, tx *sqlx.Tx, database *sqlx.DB, workspaceID string) error {
	_, err := tx.ExecContext(ctx, database.Rebind(`INSERT INTO task_repositories
		(id, task_id, repository_id, position, created_at, updated_at)
		SELECT t.id || '-a', t.id, 'benchmark-a', 0, t.created_at, t.updated_at
		FROM tasks t WHERE workspace_id = ? AND CAST(substr(t.id, 12) AS INTEGER) % 4 IN (0, 1)
		UNION ALL
		SELECT t.id || '-b', t.id, 'benchmark-b', 1, t.created_at, t.updated_at
		FROM tasks t WHERE workspace_id = ? AND CAST(substr(t.id, 12) AS INTEGER) % 4 IN (1, 2)`), workspaceID, workspaceID)
	return err
}

func benchmarkSidebarColdRead(b *testing.B, repo *Repository, workspaceID string, query models.SidebarTaskViewQuery) (*models.SidebarTaskPageResult, error) {
	b.Helper()
	previous := time.Now()
	stages := make(map[string]time.Duration)
	repo.sidebarQueryStage = func(stage string, _ *sqlx.Tx) error {
		now := time.Now()
		stages[stage] = now.Sub(previous)
		previous = now
		return nil
	}
	defer func() { repo.sidebarQueryStage = nil }()
	ctx, cancel := context.WithTimeout(b.Context(), 30*time.Second)
	defer cancel()
	result, err := repo.QuerySidebarTaskPage(ctx, workspaceID, query, models.SidebarTaskViewPreferences{})
	b.Logf("cold_preparation_and_candidates=%s cold_page_execution=%s cold_headers=%s cold_hydration=%s dialect=%s",
		stages["created"]+stages["indexed"]+stages["preferences"], stages["page"], stages["headers"], stages["hydrated"], repo.ro.DriverName())
	return result, err
}

func logSidebarPostgresExecution(b *testing.B, repo *Repository, workspaceID string, query models.SidebarTaskViewQuery) {
	b.Helper()
	ctx, cancel := context.WithTimeout(b.Context(), 30*time.Second)
	defer cancel()
	snapshot, err := beginSidebarQuerySnapshot(ctx, repo.ro)
	if err != nil {
		b.Fatal(err)
	}
	defer snapshot.close()
	baseSQL, baseArgs, err := snapshot.prepare(ctx, repo.ro.DriverName(), workspaceID, query)
	if err != nil {
		b.Fatal(err)
	}
	ctes, args := sidebarPageCTEs(repo.ro.DriverName(), query, models.SidebarTaskViewPreferences{})
	statement := "EXPLAIN (ANALYZE, FORMAT JSON) " + baseSQL + ctes + sidebarPageSelectSQL(query.Group == sidebarGroupNone)
	var raw []byte
	if err := snapshot.tx.QueryRowContext(ctx, repo.ro.Rebind(statement), append(baseArgs, args...)...).Scan(&raw); err != nil {
		b.Fatal(err)
	}
	b.Logf("postgres_execution_plan group=%s plan=%s", query.Group, raw)
	if query.Group != sidebarGroupNone {
		return
	}
	if _, err := snapshot.tx.ExecContext(ctx, "SET LOCAL jit = on"); err != nil {
		b.Fatal(err)
	}
	if err := snapshot.tx.QueryRowContext(ctx, repo.ro.Rebind(statement), append(baseArgs, args...)...).Scan(&raw); err != nil {
		b.Fatal(err)
	}
	var plans []struct {
		JIT       json.RawMessage `json:"JIT"`
		Execution float64         `json:"Execution Time"`
	}
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		b.Fatalf("read sidebar JIT timing: plans=%d error=%v", len(plans), err)
	}
	b.Logf("postgres_jit_baseline_execution_ms=%f jit=%s", plans[0].Execution, plans[0].JIT)
}
