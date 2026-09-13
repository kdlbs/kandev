package sqlite

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/analytics/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestStatsTaskAggregateAvoidsTurnsMessagesProduct keeps a deliberately small
// session count while making the turns/messages relationship large. The query
// must finish from the independently aggregated inputs before the database
// reaches the timeout; joining both child tables by session produces 2 million
// intermediate rows for this fixture.
func TestStatsTaskAggregateAvoidsTurnsMessagesProduct(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	now := time.Now().UTC()
	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-heavy', 'Heavy', ?, ?)`, now, now)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-heavy', 'ws-heavy', 'board-heavy', 'Heavy', 0, ?, ?)`, now, now)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('session-heavy', 'task-heavy', 'agent', 'COMPLETED', ?, ?)`, now, now)
	seedHeavyStatsChildRows(t, dbConn, "task-heavy", "session-heavy", now)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	stats, err := repo.GetTaskStats(ctx, "ws-heavy", nil, 10)
	if err != nil {
		t.Fatalf("GetTaskStats failed on independent aggregates: %v", err)
	}
	if len(stats) != 1 || stats[0].TurnCount != 200 || stats[0].MessageCount != 10000 {
		t.Fatalf("unexpected heavy aggregate result: %+v", stats)
	}
}

// TestStatsRepositoryAndDailyAggregatesAvoidTurnsMessagesProduct applies the
// same pressure to the repository and daily queries before their rewrites.
func TestStatsRepositoryAndDailyAggregatesAvoidTurnsMessagesProduct(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	now := time.Now().UTC()
	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-heavy', 'Heavy', ?, ?)`, now, now)
	execOrFatal(t, dbConn, `INSERT INTO repositories (id, workspace_id, name, created_at, updated_at) VALUES ('repo-heavy', 'ws-heavy', 'Heavy', ?, ?)`, now, now)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-heavy', 'ws-heavy', 'board-heavy', 'Heavy', 0, ?, ?)`, now, now)
	execOrFatal(t, dbConn, `INSERT INTO task_repositories (id, task_id, repository_id, created_at, updated_at) VALUES ('task-repo-heavy', 'task-heavy', 'repo-heavy', ?, ?)`, now, now)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, repository_id, state, started_at, updated_at) VALUES ('session-heavy', 'task-heavy', 'agent', 'repo-heavy', 'COMPLETED', ?, ?)`, now, now)
	seedHeavyStatsChildRows(t, dbConn, "task-heavy", "session-heavy", now)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	repositoryStats, err := repo.GetRepositoryStats(ctx, "ws-heavy", nil)
	if err != nil {
		t.Fatalf("GetRepositoryStats failed on independent aggregates: %v", err)
	}
	if len(repositoryStats) != 1 || repositoryStats[0].TurnCount != 200 || repositoryStats[0].MessageCount != 10000 {
		t.Fatalf("unexpected heavy repository aggregate result: %+v", repositoryStats)
	}

	daily, err := repo.GetDailyActivity(ctx, "ws-heavy", 7)
	if err != nil {
		t.Fatalf("GetDailyActivity failed on independent aggregates: %v", err)
	}
	if len(daily) != 7 || daily[len(daily)-1].TurnCount != 200 || daily[len(daily)-1].MessageCount != 10000 {
		t.Fatalf("unexpected heavy daily aggregate result: %+v", daily)
	}
}

func seedHeavyStatsChildRows(t testing.TB, dbConn *sqlx.DB, taskID, sessionID string, now time.Time) {
	t.Helper()

	tx, err := dbConn.Beginx()
	if err != nil {
		t.Fatalf("begin fixture transaction: %v", err)
	}
	turnStmt, err := tx.Prepare(`INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatalf("prepare turn insert: %v", err)
	}
	defer func() { _ = turnStmt.Close() }()
	messageStmt, err := tx.Prepare(`INSERT INTO task_session_messages (id, task_session_id, turn_id, author_type, type, content, created_at) VALUES (?, ?, '', 'user', 'message', 'x', ?)`)
	if err != nil {
		t.Fatalf("prepare message insert: %v", err)
	}
	defer func() { _ = messageStmt.Close() }()

	for i := 0; i < 200; i++ {
		startedAt := now.Add(time.Duration(i) * time.Second)
		if _, err := turnStmt.Exec(fmt.Sprintf("turn-heavy-%d", i), sessionID, taskID, startedAt, startedAt.Add(time.Second), startedAt, startedAt); err != nil {
			t.Fatalf("insert turn %d: %v", i, err)
		}
	}
	for i := 0; i < 10000; i++ {
		if _, err := messageStmt.Exec(fmt.Sprintf("message-heavy-%d", i), sessionID, now); err != nil {
			t.Fatalf("insert message %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit fixture transaction: %v", err)
	}
}

type statsParityFixture struct {
	today      time.Time
	rangeStart time.Time
}

// TestStatsAggregateParity is the small numeric oracle for the three queries
// in this work order. The same fixture and assertions run on both database
// engines when KANDEV_TEST_POSTGRES_DSN is available.
func TestStatsAggregateParity(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		dbConn := createTestDB(t)
		repo, err := NewWithDB(dbConn, dbConn)
		if err != nil {
			t.Fatalf("NewWithDB failed: %v", err)
		}
		fixture := seedStatsParityFixture(t, dbConn)
		assertStatsAggregateParity(t, repo, fixture)
	})

	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("KANDEV_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("set KANDEV_TEST_POSTGRES_DSN to run PostgreSQL parity")
		}
		dbConn := testutil.OpenIsolatedPostgres(t, dsn)
		if _, err := dbConn.Exec(`SET TIME ZONE 'Pacific/Honolulu'`); err != nil {
			t.Fatalf("set PostgreSQL test timezone: %v", err)
		}
		if _, err := dbConn.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
			t.Fatalf("create kandev_meta: %v", err)
		}
		if _, err := tasksqlite.NewWithDB(dbConn, dbConn, nil); err != nil {
			t.Fatalf("initialize PostgreSQL task schema: %v", err)
		}
		repo, err := NewWithDB(dbConn, dbConn)
		if err != nil {
			t.Fatalf("initialize PostgreSQL analytics repository: %v", err)
		}
		fixture := seedStatsParityFixture(t, dbConn)
		assertStatsAggregateParity(t, repo, fixture)
	})
}

func seedStatsParityFixture(t testing.TB, dbConn *sqlx.DB) statsParityFixture {
	t.Helper()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	rangeStart := today.Add(-48 * time.Hour)
	dayBefore := today.Add(-24 * time.Hour)
	dayWithoutTurns := today.Add(-72 * time.Hour)

	statsExec(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, "ws-main", "Main", rangeStart, rangeStart)
	statsExec(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, "ws-other", "Other", rangeStart, rangeStart)
	statsExec(t, dbConn, `INSERT INTO repositories (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, "repo-a", "ws-main", "A", rangeStart, rangeStart)
	statsExec(t, dbConn, `INSERT INTO repositories (id, workspace_id, name, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?)`, "repo-deleted", "ws-main", "Deleted", rangeStart, rangeStart, rangeStart)
	statsExec(t, dbConn, `INSERT INTO repositories (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, "repo-b", "ws-main", "B", rangeStart, rangeStart)
	statsExec(t, dbConn, `INSERT INTO repositories (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, "repo-other", "ws-other", "Other", rangeStart, rangeStart)

	insertStatsTask(t, dbConn, "task-main", "ws-main", "Main", "IN_PROGRESS", false, "", rangeStart, today)
	insertStatsTask(t, dbConn, "task-empty", "ws-main", "Empty", "TODO", false, "", dayBefore, today.Add(-time.Hour))
	insertStatsTask(t, dbConn, "task-old", "ws-main", "Old", "TODO", false, "", rangeStart.Add(-time.Hour), today.Add(-2*time.Hour))
	insertStatsTask(t, dbConn, "task-archived", "ws-main", "Archived", "IN_PROGRESS", false, "", rangeStart, today.Add(-30*time.Minute))
	statsExec(t, dbConn, `UPDATE tasks SET archived_at = ? WHERE id = ?`, today, "task-archived")
	insertStatsTask(t, dbConn, "task-ephemeral", "ws-main", "Ephemeral", "TODO", true, "", dayBefore, dayBefore)
	insertStatsTask(t, dbConn, "task-automation", "ws-main", "Automation", "TODO", false, taskmodels.TaskOriginAutomationRun, dayBefore, dayBefore)
	insertStatsTask(t, dbConn, "task-other", "ws-other", "Other", "TODO", false, "", dayBefore, dayBefore)

	insertStatsTaskRepository(t, dbConn, "tr-main-a", "task-main", "repo-a", rangeStart)
	insertStatsTaskRepository(t, dbConn, "tr-main-b", "task-main", "repo-b", rangeStart)
	insertStatsTaskRepository(t, dbConn, "tr-empty-a", "task-empty", "repo-a", dayBefore)
	insertStatsTaskRepository(t, dbConn, "tr-old-a", "task-old", "repo-a", rangeStart.Add(-time.Hour))
	insertStatsTaskRepository(t, dbConn, "tr-archived-a", "task-archived", "repo-a", rangeStart)
	insertStatsTaskRepository(t, dbConn, "tr-ephemeral-a", "task-ephemeral", "repo-a", dayBefore)
	insertStatsTaskRepository(t, dbConn, "tr-automation-a", "task-automation", "repo-a", dayBefore)
	insertStatsTaskRepository(t, dbConn, "tr-other", "task-other", "repo-other", dayBefore)

	insertStatsSession(t, dbConn, "session-main-a", "task-main", "repo-a", rangeStart.Add(time.Hour), statsTimePtr(rangeStart.Add(5*time.Hour)))
	insertStatsSession(t, dbConn, "session-main-b", "task-main", "repo-b", rangeStart.Add(2*time.Hour), statsTimePtr(rangeStart.Add(6*time.Hour)))
	insertStatsSession(t, dbConn, "session-empty", "task-empty", "repo-a", dayBefore.Add(time.Hour), nil)
	insertStatsSession(t, dbConn, "session-old", "task-old", "repo-a", dayBefore.Add(2*time.Hour), statsTimePtr(dayBefore.Add(5*time.Hour)))
	insertStatsSession(t, dbConn, "session-ephemeral", "task-ephemeral", "repo-a", dayBefore, statsTimePtr(dayBefore.Add(time.Hour)))
	insertStatsSession(t, dbConn, "session-automation", "task-automation", "repo-a", dayBefore, statsTimePtr(dayBefore.Add(time.Hour)))
	insertStatsSession(t, dbConn, "session-other", "task-other", "repo-other", dayBefore, statsTimePtr(dayBefore.Add(time.Hour)))

	insertStatsTurn(t, dbConn, "turn-main-a-1", "session-main-a", "task-main", rangeStart.Add(time.Hour), rangeStart.Add(2*time.Hour))
	insertStatsTurn(t, dbConn, "turn-main-a-2", "session-main-a", "task-main", rangeStart.Add(2*time.Hour), rangeStart.Add(4*time.Hour))
	insertStatsTurn(t, dbConn, "turn-main-b-1", "session-main-b", "task-main", rangeStart.Add(3*time.Hour), rangeStart.Add(3*time.Hour+30*time.Minute))
	insertStatsTurn(t, dbConn, "turn-old", "session-old", "task-old", dayBefore.Add(2*time.Hour), dayBefore.Add(3*time.Hour))
	insertStatsTurn(t, dbConn, "turn-ephemeral", "session-ephemeral", "task-ephemeral", dayBefore, dayBefore.Add(time.Hour))
	insertStatsTurn(t, dbConn, "turn-automation", "session-automation", "task-automation", dayBefore, dayBefore.Add(time.Hour))
	insertStatsTurn(t, dbConn, "turn-other", "session-other", "task-other", dayBefore, dayBefore.Add(time.Hour))

	insertStatsMessage(t, dbConn, "message-main-1", "session-main-a", "turn-main-a-1", "user", "message", rangeStart.Add(time.Hour+10*time.Minute))
	insertStatsMessage(t, dbConn, "message-main-2", "session-main-a", "turn-main-a-1", "agent", "tool_call", rangeStart.Add(time.Hour+20*time.Minute))
	insertStatsMessage(t, dbConn, "message-main-3", "session-main-a", "turn-main-a-2", "agent", "tool_edit", rangeStart.Add(2*time.Hour+20*time.Minute))
	insertStatsMessage(t, dbConn, "message-main-4", "session-main-a", "", "user", "message", rangeStart.Add(2*time.Hour+30*time.Minute))
	insertStatsMessage(t, dbConn, "message-main-5", "session-main-b", "turn-main-b-1", "agent", "message", rangeStart.Add(3*time.Hour+15*time.Minute))
	insertStatsMessage(t, dbConn, "message-empty-day", "session-empty", "", "user", "message", dayWithoutTurns.Add(2*time.Hour))
	insertStatsMessage(t, dbConn, "message-session-day-without-turn", "session-main-a", "", "user", "message", dayBefore.Add(10*time.Hour))
	insertStatsMessage(t, dbConn, "message-old-1", "session-old", "turn-old", "user", "message", dayBefore.Add(2*time.Hour+10*time.Minute))
	insertStatsMessage(t, dbConn, "message-old-2", "session-old", "", "user", "message", dayBefore.Add(2*time.Hour+20*time.Minute))
	insertStatsMessage(t, dbConn, "message-ephemeral", "session-ephemeral", "turn-ephemeral", "user", "message", dayBefore)
	insertStatsMessage(t, dbConn, "message-automation", "session-automation", "turn-automation", "user", "message", dayBefore)
	insertStatsMessage(t, dbConn, "message-other", "session-other", "turn-other", "user", "message", dayBefore)

	insertStatsCommit(t, dbConn, "commit-main-a", "session-main-a", "sha-main-a", rangeStart.Add(7*time.Hour), 2, 10, 3)
	insertStatsCommit(t, dbConn, "commit-old-before", "session-old", "sha-old-before", rangeStart.Add(-time.Hour), 1, 5, 1)
	insertStatsCommit(t, dbConn, "commit-old-after", "session-old", "sha-old-after", dayBefore.Add(6*time.Hour), 4, 30, 6)
	insertStatsCommit(t, dbConn, "commit-main-b", "session-main-b", "sha-main-b", rangeStart.Add(8*time.Hour), 3, 20, 5)
	insertStatsCommit(t, dbConn, "commit-ephemeral", "session-ephemeral", "sha-ephemeral", dayBefore, 9, 90, 9)
	insertStatsCommit(t, dbConn, "commit-automation", "session-automation", "sha-automation", dayBefore, 9, 90, 9)
	insertStatsCommit(t, dbConn, "commit-other", "session-other", "sha-other", dayBefore, 9, 90, 9)

	return statsParityFixture{today: today, rangeStart: rangeStart}
}

func insertStatsTask(t testing.TB, dbConn *sqlx.DB, id, workspaceID, title, state string, ephemeral bool, origin string, createdAt, updatedAt time.Time) {
	t.Helper()
	eph := 0
	if ephemeral {
		eph = 1
	}
	if dbConn.DriverName() == "sqlite3" {
		statsExec(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_id, workflow_step_id, title, state, is_ephemeral, origin, created_at, updated_at) VALUES (?, ?, 'board', '', '', ?, ?, ?, ?, ?, ?)`, id, workspaceID, title, state, eph, origin, createdAt, updatedAt)
		return
	}
	statsExec(t, dbConn, `INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, state, is_ephemeral, origin, created_at, updated_at) VALUES (?, ?, '', '', ?, ?, ?, ?, ?, ?)`, id, workspaceID, title, state, eph, origin, createdAt, updatedAt)
}

func insertStatsTaskRepository(t testing.TB, dbConn *sqlx.DB, id, taskID, repositoryID string, createdAt time.Time) {
	t.Helper()
	statsExec(t, dbConn, `INSERT INTO task_repositories (id, task_id, repository_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, id, taskID, repositoryID, createdAt, createdAt)
}

func insertStatsSession(t testing.TB, dbConn *sqlx.DB, id, taskID, repositoryID string, startedAt time.Time, completedAt *time.Time) {
	t.Helper()
	statsExec(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, repository_id, state, started_at, completed_at, updated_at) VALUES (?, ?, 'agent', ?, 'COMPLETED', ?, ?, ?)`, id, taskID, repositoryID, startedAt, completedAt, startedAt)
}

func statsTimePtr(value time.Time) *time.Time {
	return &value
}

func insertStatsTurn(t testing.TB, dbConn *sqlx.DB, id, sessionID, taskID string, startedAt, completedAt time.Time) {
	t.Helper()
	statsExec(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, id, sessionID, taskID, startedAt, completedAt, startedAt, completedAt)
}

func insertStatsMessage(t testing.TB, dbConn *sqlx.DB, id, sessionID, turnID, authorType, messageType string, createdAt time.Time) {
	t.Helper()
	// The compact SQLite fixture intentionally permits an empty turn_id. The
	// production PostgreSQL schema retains a foreign key for this legacy text
	// column, so use an existing turn there while keeping the message outside
	// the session/day pair whose eligibility the query must determine.
	if turnID == "" && dbConn.DriverName() != "sqlite3" {
		turnID = "turn-main-a-1"
	}
	statsExec(t, dbConn, `INSERT INTO task_session_messages (id, task_session_id, turn_id, author_type, type, content, created_at) VALUES (?, ?, ?, ?, ?, 'fixture', ?)`, id, sessionID, turnID, authorType, messageType, createdAt)
}

func insertStatsCommit(t testing.TB, dbConn *sqlx.DB, id, sessionID, sha string, committedAt time.Time, files, insertions, deletions int) {
	t.Helper()
	statsExec(t, dbConn, `INSERT INTO task_session_commits (id, session_id, commit_sha, committed_at, files_changed, insertions, deletions, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, sessionID, sha, committedAt, files, insertions, deletions, committedAt)
}

func statsExec(t testing.TB, dbConn *sqlx.DB, query string, args ...any) {
	t.Helper()
	if _, err := dbConn.Exec(dbConn.Rebind(query), args...); err != nil {
		t.Fatalf("fixture exec failed: %v", err)
	}
}

func assertStatsAggregateParity(t *testing.T, repo *Repository, fixture statsParityFixture) {
	t.Helper()
	ctx := context.Background()

	taskStats, err := repo.GetTaskStats(ctx, "ws-main", nil, 10)
	if err != nil {
		t.Fatalf("GetTaskStats failed: %v", err)
	}
	tasksByID := make(map[string]*models.TaskStats, len(taskStats))
	for _, stat := range taskStats {
		tasksByID[stat.TaskID] = stat
	}
	if len(tasksByID) != 4 {
		t.Fatalf("task stats count = %d, want 4: %+v", len(tasksByID), taskStats)
	}
	assertTaskStats(t, tasksByID["task-main"], 2, 3, 6, 3, 2, 12600000, 10800000)
	assertTaskStats(t, tasksByID["task-empty"], 1, 0, 1, 1, 0, 0, 0)
	assertTaskStats(t, tasksByID["task-old"], 1, 1, 2, 2, 0, 3600000, 3600000)
	assertTaskStats(t, tasksByID["task-archived"], 0, 0, 0, 0, 0, 0, 0)

	rangedTaskStats, err := repo.GetTaskStats(ctx, "ws-main", &fixture.rangeStart, 10)
	if err != nil {
		t.Fatalf("GetTaskStats with range failed: %v", err)
	}
	if len(rangedTaskStats) != 3 || rangedTaskStats[0].TaskID != "task-main" || rangedTaskStats[1].TaskID != "task-archived" || rangedTaskStats[2].TaskID != "task-empty" {
		t.Fatalf("ranged task stats = %+v, want task-main, task-archived, then task-empty", rangedTaskStats)
	}

	repositoryStats, err := repo.GetRepositoryStats(ctx, "ws-main", nil)
	if err != nil {
		t.Fatalf("GetRepositoryStats failed: %v", err)
	}
	repositoriesByID := make(map[string]*models.RepositoryStats, len(repositoryStats))
	for _, stat := range repositoryStats {
		repositoriesByID[stat.RepositoryID] = stat
	}
	if len(repositoriesByID) != 2 {
		t.Fatalf("repository stats count = %d, want 2: %+v", len(repositoriesByID), repositoryStats)
	}
	assertRepositoryStats(t, repositoriesByID["repo-a"], 4, 1, 1, 4, 4, 9, 6, 2, 16200000, 3, 7, 45, 10)
	assertRepositoryStats(t, repositoriesByID["repo-b"], 1, 0, 1, 2, 3, 6, 3, 2, 12600000, 1, 3, 20, 5)

	rangedRepositoryStats, err := repo.GetRepositoryStats(ctx, "ws-main", &fixture.rangeStart)
	if err != nil {
		t.Fatalf("GetRepositoryStats with range failed: %v", err)
	}
	rangedRepositoriesByID := make(map[string]*models.RepositoryStats, len(rangedRepositoryStats))
	for _, stat := range rangedRepositoryStats {
		rangedRepositoriesByID[stat.RepositoryID] = stat
	}
	assertRepositoryStats(t, rangedRepositoriesByID["repo-a"], 3, 1, 1, 4, 4, 9, 6, 2, 16200000, 2, 6, 40, 9)
	assertRepositoryStats(t, rangedRepositoriesByID["repo-b"], 1, 0, 1, 2, 3, 6, 3, 2, 12600000, 1, 3, 20, 5)

	daily, err := repo.GetDailyActivity(ctx, "ws-main", 7)
	if err != nil {
		t.Fatalf("GetDailyActivity failed: %v", err)
	}
	activityByDate := make(map[string]*models.DailyActivity, len(daily))
	for _, activity := range daily {
		activityByDate[activity.Date] = activity
	}
	assertDailyActivity(t, activityByDate[fixture.rangeStart.Format("2006-01-02")], 3, 5, 1)
	assertDailyActivity(t, activityByDate[fixture.today.Add(-24*time.Hour).Format("2006-01-02")], 1, 2, 1)
	assertDailyActivity(t, activityByDate[fixture.today.Add(-72*time.Hour).Format("2006-01-02")], 0, 0, 0)
}

func assertTaskStats(t *testing.T, stat *models.TaskStats, sessions, turns, messages, userMessages, toolCalls int, totalDuration, elapsedSpan int64) {
	t.Helper()
	if stat == nil {
		t.Fatal("task stats row is nil")
	}
	if stat.SessionCount != sessions || stat.TurnCount != turns || stat.MessageCount != messages || stat.UserMessageCount != userMessages || stat.ToolCallCount != toolCalls {
		t.Fatalf("task %s counts = %+v, want sessions=%d turns=%d messages=%d user=%d tools=%d", stat.TaskID, stat, sessions, turns, messages, userMessages, toolCalls)
	}
	assertDurationAlmostEqual(t, stat.TotalDurationMs, totalDuration, "task total duration")
	assertDurationAlmostEqual(t, stat.ActiveDurationMs, totalDuration, "task active duration")
	assertDurationAlmostEqual(t, stat.ElapsedSpanMs, elapsedSpan, "task elapsed span")
}

func assertRepositoryStats(t *testing.T, stat *models.RepositoryStats, tasks, completed, inProgress, sessions, turns, messages, userMessages, toolCalls int, duration int64, commits, files, insertions, deletions int) {
	t.Helper()
	if stat == nil {
		t.Fatal("repository stats row is nil")
	}
	if stat.TotalTasks != tasks || stat.CompletedTasks != completed || stat.InProgressTasks != inProgress || stat.SessionCount != sessions || stat.TurnCount != turns || stat.MessageCount != messages || stat.UserMessageCount != userMessages || stat.ToolCallCount != toolCalls || stat.TotalCommits != commits || stat.TotalFilesChanged != files || stat.TotalInsertions != insertions || stat.TotalDeletions != deletions {
		t.Fatalf("repository %s stats = %+v, want tasks=%d completed=%d in_progress=%d sessions=%d turns=%d messages=%d user=%d tools=%d commits=%d files=%d insertions=%d deletions=%d", stat.RepositoryID, stat, tasks, completed, inProgress, sessions, turns, messages, userMessages, toolCalls, commits, files, insertions, deletions)
	}
	assertDurationAlmostEqual(t, stat.TotalDurationMs, duration, "repository total duration")
}

func assertDailyActivity(t *testing.T, activity *models.DailyActivity, turns, messages, tasks int) {
	t.Helper()
	if activity == nil {
		t.Fatalf("daily activity row is nil, want turns=%d messages=%d tasks=%d", turns, messages, tasks)
	}
	if activity.TurnCount != turns || activity.MessageCount != messages || activity.TaskCount != tasks {
		t.Fatalf("daily activity %s = %+v, want turns=%d messages=%d tasks=%d", activity.Date, activity, turns, messages, tasks)
	}
}
