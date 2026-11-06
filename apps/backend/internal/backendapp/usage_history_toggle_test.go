package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-COSTS-USAGE-HISTORY-007.8 AC-COSTS-USAGE-HISTORY-007.9 AC-COSTS-USAGE-HISTORY-007.10
func TestUsageHistoryToggleOffStopsWritesAndKeepsErasure(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{HomeDir: t.TempDir(), Database: config.DatabaseConfig{Driver: "sqlite"}}
	cfg.Features.UsageHistory = true
	firstPool, firstRepos, closeFirst := bootUsageRepositories(t, ctx, cfg)
	at := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	createToggleUsageFixture(t, ctx, firstRepos.Task, "ws-before", "task-before", "session-before", at)
	if usageBindings(t, firstPool, "task-before") == 0 {
		t.Fatal("usage history on must capture a retained binding")
	}
	if firstRepos.usageHistoryControl == nil {
		t.Fatal("usage history on must start the projection worker")
	}
	closeFirst()
	time.Sleep(1100 * time.Millisecond)

	cfg.Features.UsageHistory = false
	secondPool, secondRepos, closeSecond := bootUsageRepositories(t, ctx, cfg)
	if secondRepos.usageHistoryControl != nil {
		t.Fatal("usage history off must not start the projection worker")
	}
	createToggleUsageFixture(t, ctx, secondRepos.Task, "ws-during", "task-during", "session-during", at.Add(time.Hour))
	if usageBindings(t, secondPool, "task-during") != 0 {
		t.Fatal("usage history off must not admit retained rows")
	}
	if usageBindings(t, secondPool, "task-before") == 0 {
		t.Fatal("turning usage history off must keep previously retained rows")
	}
	if err := secondRepos.Task.DeleteTask(ctx, "task-during"); err != nil {
		t.Fatalf("task deletion with usage history off: %v", err)
	}
	if err := secondRepos.Task.DeleteTask(ctx, "task-before"); err != nil {
		t.Fatalf("retained task deletion with usage history off: %v", err)
	}
	if usageBindings(t, secondPool, "task-before") == 0 {
		t.Fatal("task completion must not erase retained history")
	}
	if err := secondRepos.Task.DeleteWorkspace(ctx, "ws-before"); err != nil {
		t.Fatalf("workspace deletion with usage history off: %v", err)
	}
	if usageBindings(t, secondPool, "task-before") != 0 {
		t.Fatal("workspace erasure must remove retained rows while usage history is off")
	}

	closeSecond()
	time.Sleep(1100 * time.Millisecond)
	cfg.Features.UsageHistory = true
	thirdPool, thirdRepos, closeThird := bootUsageRepositories(t, ctx, cfg)
	defer closeThird()
	createToggleUsageFixture(t, ctx, thirdRepos.Task, "ws-after", "task-after", "session-after", at.Add(2*time.Hour))
	if usageBindings(t, thirdPool, "task-after") == 0 {
		t.Fatal("usage history back on must resume retained admission")
	}
}

func bootUsageRepositories(t *testing.T, ctx context.Context, cfg *config.Config) (*db.Pool, *Repositories, func()) {
	t.Helper()
	pool, repos, cleanups, err := provideRepositories(ctx, cfg, nil, "usage-toggle")
	if err != nil {
		t.Fatalf("provideRepositories: %v", err)
	}
	closed := false
	closeFn := func() {
		if closed {
			return
		}
		closed = true
		for i := len(cleanups) - 1; i >= 0; i-- {
			if cleanups[i] != nil {
				_ = cleanups[i]()
			}
		}
	}
	return pool, repos, closeFn
}

func createToggleUsageFixture(t *testing.T, ctx context.Context, tasks *sqliterepo.Repository, workspaceID, taskID, sessionID string, at time.Time) {
	t.Helper()
	if err := tasks.CreateWorkspace(ctx, &taskmodels.Workspace{ID: workspaceID, Name: workspaceID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := tasks.CreateTask(ctx, &taskmodels.Task{ID: taskID, WorkspaceID: workspaceID, Title: taskID, CreatedAt: at, UpdatedAt: at}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := tasks.CreateTaskSession(ctx, &taskmodels.TaskSession{
		ID: sessionID, TaskID: taskID, StartedAt: at,
		Metadata: map[string]any{"acp": map[string]any{"session_id": taskID + "-transcript"}},
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	event := &taskmodels.TaskUsageEvent{
		UsageEventID: taskID + "-event", TaskID: taskID, SessionID: sessionID,
		AgentType: "claude", Model: "model", Provider: "provider",
		TokensIn: 10, TokensTotal: 10, CostSubcents: 3,
		CostSource: "models_dev_list", OccurredAt: at, CreatedAt: at,
	}
	if err := tasks.CreateTaskUsageEvent(ctx, event); err != nil {
		t.Fatalf("create usage event: %v", err)
	}
}

func usageBindings(t *testing.T, pool *db.Pool, taskID string) int {
	t.Helper()
	var count int
	if err := pool.Writer().GetContext(context.Background(), &count,
		`SELECT COUNT(*) FROM usage_collection_bindings WHERE task_id = ?`, taskID); err != nil {
		t.Fatalf("count retained bindings: %v", err)
	}
	return count
}

// @covers AC-COSTS-USAGE-HISTORY-007.10
func TestUsageHistoryToggleReenableCatchesUpOffPeriodWork(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{HomeDir: t.TempDir(), Database: config.DatabaseConfig{Driver: "sqlite"}}
	cfg.Features.UsageHistory = false
	offPool, offRepos, closeOff := bootUsageRepositories(t, ctx, cfg)
	at := time.Date(2026, 2, 2, 3, 4, 0, 0, time.UTC)
	createToggleUsageFixture(t, ctx, offRepos.Task, "ws-off-period", "task-off-period", "session-off-period", at)
	if usageBindings(t, offPool, "task-off-period") != 0 {
		t.Fatal("usage history off must not capture a binding")
	}
	closeOff()
	time.Sleep(1100 * time.Millisecond)

	cfg.Features.UsageHistory = true
	pool, repos, closeOn := bootUsageRepositories(t, ctx, cfg)
	defer closeOn()
	require.Eventually(t, func() bool {
		var count int
		if err := pool.Writer().GetContext(ctx, &count, `SELECT COUNT(*) FROM usage_collection_bindings WHERE task_id = 'task-off-period' AND source_scope_status = 'eligible'`); err != nil {
			return false
		}
		return count > 0
	}, 30*time.Second, 200*time.Millisecond, "re-enabled worker must adopt the off-period session")
	require.Eventually(t, func() bool {
		var count int
		if err := pool.Writer().GetContext(ctx, &count, `SELECT COUNT(*) FROM session_usage_buckets WHERE source = 'native' AND source_record_id = 'task-off-period-event'`); err != nil {
			return false
		}
		return count > 0
	}, 30*time.Second, 200*time.Millisecond, "re-enabled worker must retain the off-period native event")
	if err := repos.Task.DeleteTask(ctx, "task-off-period"); err != nil {
		t.Fatalf("off-period task deletion after catch-up: %v", err)
	}
}
