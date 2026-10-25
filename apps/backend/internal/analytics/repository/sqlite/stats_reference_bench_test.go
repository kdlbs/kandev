package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

// BenchmarkStatsReference measures the seven independent statistics reads on
// the documented reference shape. Fixture construction is completed before
// the timer starts; run with -benchtime=10x for the work-order sample.
func BenchmarkStatsReference(b *testing.B) {
	fixture := newStatsReferenceFixture(b)
	ctx := context.Background()

	operations := []struct {
		name string
		run  func() error
	}{
		{name: "global", run: func() error {
			_, err := fixture.repo.GetGlobalStats(ctx, fixture.workspaceID, nil)
			return err
		}},
		{name: "tasks", run: func() error {
			_, err := fixture.repo.GetTaskStats(ctx, fixture.workspaceID, nil, 201)
			return err
		}},
		{name: "daily", run: func() error {
			_, err := fixture.repo.GetDailyActivity(ctx, fixture.workspaceID, 365)
			return err
		}},
		{name: "completed", run: func() error {
			_, err := fixture.repo.GetCompletedTaskActivity(ctx, fixture.workspaceID, 365)
			return err
		}},
		{name: "models", run: func() error {
			_, err := fixture.repo.GetModelUsage(ctx, fixture.workspaceID, 25, nil)
			return err
		}},
		{name: "repositories", run: func() error {
			_, err := fixture.repo.GetRepositoryStats(ctx, fixture.workspaceID, nil)
			return err
		}},
		{name: "git", run: func() error {
			_, err := fixture.repo.GetGitStats(ctx, fixture.workspaceID, nil)
			return err
		}},
	}

	for _, operation := range operations {
		operation := operation
		b.Run(operation.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if err := operation.run(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}

	b.Run("all-seven", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, operation := range operations {
				if err := operation.run(); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}

type statsReferenceFixture struct {
	repo        *Repository
	workspaceID string
}

func newStatsReferenceFixture(b *testing.B) statsReferenceFixture {
	b.Helper()
	writer, reader := createTestDBPair(b)
	repo, err := NewWithDB(writer, reader)
	if err != nil {
		b.Fatalf("NewWithDB failed: %v", err)
	}

	base := time.Now().UTC().Truncate(24 * time.Hour)
	seedStatsReferenceWorkspace(b, writer, "ws-reference", base)
	seedStatsReferenceWorkspace(b, writer, "ws-other-reference", base)
	return statsReferenceFixture{repo: repo, workspaceID: "ws-reference"}
}

func seedStatsReferenceWorkspace(t testing.TB, dbConn *sqlx.DB, workspaceID string, base time.Time) {
	t.Helper()
	const (
		taskCount    = 700
		sessionCount = 800
		turnCount    = 5000
		messageCount = 650000
		commitCount  = 2200
		repoCount    = 5
	)

	statsExec(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, workspaceID, workspaceID, base, base)
	repositoryIDs := make([]string, repoCount)
	for i := range repositoryIDs {
		repositoryIDs[i] = fmt.Sprintf("%s-repo-%d", workspaceID, i)
		statsExec(t, dbConn, `INSERT INTO repositories (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, repositoryIDs[i], workspaceID, repositoryIDs[i], base, base)
	}

	taskIDs := make([]string, taskCount)
	for i := range taskIDs {
		taskIDs[i] = fmt.Sprintf("%s-task-%d", workspaceID, i)
		createdAt := base.Add(-time.Duration(i%400) * 24 * time.Hour)
		updatedAt := createdAt.Add(time.Duration(i%24) * time.Hour)
		insertStatsTask(t, dbConn, taskIDs[i], workspaceID, "Reference task", "TODO", false, "", createdAt, updatedAt)
		insertStatsTaskRepository(t, dbConn, fmt.Sprintf("%s-task-repo-%d", workspaceID, i), taskIDs[i], repositoryIDs[i%repoCount], createdAt)
		if i%2 == 0 {
			insertStatsTaskRepository(t, dbConn, fmt.Sprintf("%s-task-repo-extra-%d", workspaceID, i), taskIDs[i], repositoryIDs[(i+1)%repoCount], createdAt)
		}
	}

	sessionIDs := make([]string, sessionCount)
	sessionTaskIDs := make([]string, sessionCount)
	for i := range sessionIDs {
		sessionIDs[i] = fmt.Sprintf("%s-session-%d", workspaceID, i)
		sessionTaskIDs[i] = taskIDs[i%taskCount]
		startedAt := base.Add(-time.Duration(i%400) * 24 * time.Hour).Add(time.Duration(i%12) * time.Hour)
		completedAt := statsTimePtr(startedAt.Add(time.Hour))
		insertStatsSessionWithSnapshot(t, dbConn, sessionIDs[i], sessionTaskIDs[i], repositoryIDs[i%repoCount], startedAt, completedAt)
	}

	tx, err := dbConn.Beginx()
	if err != nil {
		t.Fatalf("begin reference fixture transaction: %v", err)
	}
	turnStmt, err := tx.Prepare(`INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatalf("prepare reference turn insert: %v", err)
	}
	messageStmt, err := tx.Prepare(`INSERT INTO task_session_messages (id, task_session_id, turn_id, author_type, type, content, created_at) VALUES (?, ?, ?, ?, ?, 'reference', ?)`)
	if err != nil {
		_ = turnStmt.Close()
		t.Fatalf("prepare reference message insert: %v", err)
	}
	commitStmt, err := tx.Prepare(`INSERT INTO task_session_commits (id, session_id, commit_sha, committed_at, files_changed, insertions, deletions, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = turnStmt.Close()
		_ = messageStmt.Close()
		t.Fatalf("prepare reference commit insert: %v", err)
	}

	turnIDs := make([]string, turnCount)
	for i := 0; i < turnCount; i++ {
		sessionIndex := i % sessionCount
		if i >= 200 {
			sessionIndex = (i - 200) % sessionCount
		}
		turnIDs[i] = fmt.Sprintf("%s-turn-%d", workspaceID, i)
		startedAt := base.Add(-time.Duration(i%400) * 24 * time.Hour).Add(time.Duration(i%12) * time.Hour)
		if _, err := turnStmt.Exec(turnIDs[i], sessionIDs[sessionIndex], sessionTaskIDs[sessionIndex], startedAt, startedAt.Add(time.Minute), startedAt, startedAt); err != nil {
			_ = turnStmt.Close()
			_ = messageStmt.Close()
			_ = commitStmt.Close()
			_ = tx.Rollback()
			t.Fatalf("insert reference turn %d: %v", i, err)
		}
	}
	for i := 0; i < messageCount; i++ {
		sessionIndex := i % sessionCount
		if i < 10000 {
			sessionIndex = 0
		}
		createdAt := base.Add(-time.Duration(i%400) * 24 * time.Hour).Add(time.Duration(i%12) * time.Hour)
		if _, err := messageStmt.Exec(
			fmt.Sprintf("%s-message-%d", workspaceID, i),
			sessionIDs[sessionIndex],
			turnIDs[i%turnCount],
			"user",
			"message",
			createdAt,
		); err != nil {
			_ = turnStmt.Close()
			_ = messageStmt.Close()
			_ = commitStmt.Close()
			_ = tx.Rollback()
			t.Fatalf("insert reference message %d: %v", i, err)
		}
	}
	for i := 0; i < commitCount; i++ {
		sessionIndex := i % sessionCount
		committedAt := base.Add(-time.Duration(i%400) * 24 * time.Hour).Add(time.Duration(i%12) * time.Hour)
		if _, err := commitStmt.Exec(
			fmt.Sprintf("%s-commit-%d", workspaceID, i),
			sessionIDs[sessionIndex],
			fmt.Sprintf("%s-sha-%d", workspaceID, i),
			committedAt,
			1,
			2,
			1,
			committedAt,
		); err != nil {
			_ = turnStmt.Close()
			_ = messageStmt.Close()
			_ = commitStmt.Close()
			_ = tx.Rollback()
			t.Fatalf("insert reference commit %d: %v", i, err)
		}
	}
	if err := turnStmt.Close(); err != nil {
		_ = messageStmt.Close()
		_ = commitStmt.Close()
		_ = tx.Rollback()
		t.Fatalf("close reference turn statement: %v", err)
	}
	if err := messageStmt.Close(); err != nil {
		_ = commitStmt.Close()
		_ = tx.Rollback()
		t.Fatalf("close reference message statement: %v", err)
	}
	if err := commitStmt.Close(); err != nil {
		_ = tx.Rollback()
		t.Fatalf("close reference commit statement: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit reference fixture transaction: %v", err)
	}
}

func insertStatsSessionWithSnapshot(t testing.TB, dbConn *sqlx.DB, id, taskID, repositoryID string, startedAt time.Time, completedAt *time.Time) {
	t.Helper()
	statsExec(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, agent_profile_snapshot, repository_id, state, started_at, completed_at, updated_at) VALUES (?, ?, 'agent', '{"model":"reference-model"}', ?, 'COMPLETED', ?, ?, ?)`, id, taskID, repositoryID, startedAt, completedAt, startedAt)
}
