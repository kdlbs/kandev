package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

type taskMentionSearcher interface {
	SearchMentionTasks(
		ctx context.Context,
		workspaceID, query, excludeTaskID string,
		limit int,
	) ([]*models.Task, error)
}

func TestSearchMentionTasksFiltersAndRanksByTitle(t *testing.T) {
	repo := newTaskMentionTestRepository(t)
	searcher, ok := any(repo).(taskMentionSearcher)
	require.True(t, ok, "Repository must implement task mention search")

	now := time.Now().UTC()
	seedMentionTask(t, repo, mentionTaskSeed{ID: "prefix-a", WorkspaceID: "ws-a", Title: "Auth Alpha", UpdatedAt: now})
	seedMentionTask(t, repo, mentionTaskSeed{ID: "prefix-b", WorkspaceID: "ws-a", Title: "Auth Beta", UpdatedAt: now.Add(time.Minute)})
	seedMentionTask(t, repo, mentionTaskSeed{ID: "tie-b", WorkspaceID: "ws-a", Title: "Auth Tie", UpdatedAt: now.Add(2 * time.Minute)})
	seedMentionTask(t, repo, mentionTaskSeed{ID: "tie-a", WorkspaceID: "ws-a", Title: "Auth Tie", UpdatedAt: now.Add(3 * time.Minute)})
	seedMentionTask(t, repo, mentionTaskSeed{ID: "contains", WorkspaceID: "ws-a", Title: "Fix auth redirect", UpdatedAt: now.Add(4 * time.Minute)})
	seedMentionTask(t, repo, mentionTaskSeed{ID: "current", WorkspaceID: "ws-a", Title: "Auth Current", UpdatedAt: now})
	seedMentionTask(t, repo, mentionTaskSeed{ID: "archived", WorkspaceID: "ws-a", Title: "Auth Archived", Archived: true, UpdatedAt: now})
	seedMentionTask(t, repo, mentionTaskSeed{ID: "ephemeral", WorkspaceID: "ws-a", Title: "Auth Ephemeral", Ephemeral: true, UpdatedAt: now})
	seedMentionTask(t, repo, mentionTaskSeed{ID: "other-workspace", WorkspaceID: "ws-b", Title: "Auth Elsewhere", UpdatedAt: now})
	seedMentionTask(t, repo, mentionTaskSeed{ID: "description-only", WorkspaceID: "ws-a", Title: "Unrelated", Description: "auth", UpdatedAt: now})
	seedMentionTask(t, repo, mentionTaskSeed{ID: "identifier-only", WorkspaceID: "ws-a", Identifier: "AUTH-99", Title: "Nothing here", UpdatedAt: now})

	got, err := searcher.SearchMentionTasks(context.Background(), "ws-a", "AUTH", "current", 10)
	require.NoError(t, err)
	require.Equal(t, []string{"prefix-a", "prefix-b", "tie-a", "tie-b", "contains"}, mentionTaskIDs(got))
	require.Equal(t, "ws-a", got[0].WorkspaceID)
	require.Equal(t, "Auth Alpha", got[0].Title)

	limited, err := searcher.SearchMentionTasks(context.Background(), "ws-a", "auth", "current", 3)
	require.NoError(t, err)
	require.Equal(t, []string{"prefix-a", "prefix-b", "tie-a"}, mentionTaskIDs(limited))
}

func TestSearchMentionTasksTreatsLikeWildcardsAsLiteralText(t *testing.T) {
	repo := newTaskMentionTestRepository(t)
	searcher, ok := any(repo).(taskMentionSearcher)
	require.True(t, ok, "Repository must implement task mention search")

	now := time.Now().UTC()
	seedMentionTask(t, repo, mentionTaskSeed{ID: "literal", WorkspaceID: "ws", Title: "Fix 100% login", UpdatedAt: now})
	seedMentionTask(t, repo, mentionTaskSeed{ID: "wildcard-match", WorkspaceID: "ws", Title: "Fix 1000 login", UpdatedAt: now})

	got, err := searcher.SearchMentionTasks(context.Background(), "ws", "100%", "", 10)
	require.NoError(t, err)
	require.Equal(t, []string{"literal"}, mentionTaskIDs(got))
}

type mentionTaskSeed struct {
	ID          string
	WorkspaceID string
	Identifier  string
	Title       string
	Description string
	Ephemeral   bool
	Archived    bool
	UpdatedAt   time.Time
}

func newTaskMentionTestRepository(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "task-mentions.db")
	dbConn, err := db.OpenSQLite(dbPath)
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlxDB.Close()) })
	return repo
}

func seedMentionTask(t *testing.T, repo *Repository, task mentionTaskSeed) {
	t.Helper()
	var archivedAt *time.Time
	if task.Archived {
		archived := task.UpdatedAt
		archivedAt = &archived
	}
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (
			id, workspace_id, identifier, title, description, is_ephemeral,
			archived_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), task.ID, task.WorkspaceID, task.Identifier, task.Title, task.Description,
		task.Ephemeral, archivedAt, task.UpdatedAt, task.UpdatedAt)
	require.NoError(t, err)
}

func mentionTaskIDs(tasks []*models.Task) []string {
	ids := make([]string, len(tasks))
	for i, task := range tasks {
		ids[i] = task.ID
	}
	return ids
}
