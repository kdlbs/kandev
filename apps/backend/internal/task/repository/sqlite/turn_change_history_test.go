package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestTurnChangeHistoryPagesInStableTurnAndPathOrder(t *testing.T) {
	repo := newRepoForSessionTests(t)
	runTurnChangeHistoryPagesInStableTurnAndPathOrder(t, repo)
}

func TestPostgresTurnChangeHistoryPagesInStableTurnAndPathOrder(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres schema: %v", err)
	}
	runTurnChangeHistoryPagesInStableTurnAndPathOrder(t, repo)
}

func runTurnChangeHistoryPagesInStableTurnAndPathOrder(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	const taskID, envID, sessionID = "task-history-page", "env-history-page", "session-history-page"
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "History"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: envID, TaskID: taskID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, TaskEnvironmentID: envID}); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	changeSetIDs := make([]string, 0, 2)
	for index, turnID := range []string{"turn-history-a", "turn-history-b"} {
		startedAt := base.Add(time.Duration(index) * time.Minute)
		if err := repo.CreateTurn(ctx, &models.Turn{ID: turnID, TaskID: taskID, TaskSessionID: sessionID, StartedAt: startedAt, CreatedAt: startedAt}); err != nil {
			t.Fatal(err)
		}
		changeSetID := "set-" + turnID
		changeSet := &models.TurnChangeSet{
			ID: changeSetID, TaskID: taskID, TaskSessionID: sessionID, TurnID: turnID, TaskEnvironmentID: envID,
			Availability: models.TurnChangeAvailabilityUnavailable, Reason: models.TurnChangeReasonCaptureDisabled,
		}
		if err := repo.CreateTurnChangeSet(ctx, changeSet); err != nil {
			t.Fatal(err)
		}
		changeSetIDs = append(changeSetIDs, changeSetID)
	}
	page, total, err := repo.ListTurnChangeSets(ctx, taskID, sessionID, 0, 1)
	if err != nil || total != 2 || len(page) != 1 {
		t.Fatalf("first history page = %#v total %d err %v", page, total, err)
	}
	if page[0].TurnID != "turn-history-b" || page[0].TurnOrdinal != 2 {
		t.Fatalf("first history item = turn %s ordinal %d, want newest turn ordinal 2", page[0].TurnID, page[0].TurnOrdinal)
	}
	page, total, err = repo.ListTurnChangeSets(ctx, taskID, sessionID, 1, 1)
	if err != nil || total != 2 || len(page) != 1 || page[0].TurnID != "turn-history-a" || page[0].TurnOrdinal != 1 {
		t.Fatalf("second history page = %#v total %d err %v", page, total, err)
	}

	const repositoryChangeID = "repo-change-history-page"
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO turn_repository_changes (id, change_set_id, checkout_id, availability, created_at, updated_at)
		VALUES (?, ?, ?, 'ready', ?, ?)
	`), repositoryChangeID, changeSetIDs[0], "checkout-history", base, base); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"src/z.go", "src/a.go"} {
		if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
			INSERT INTO turn_file_changes (id, repository_change_id, checkout_id, path, path_bytes, kind, created_at)
			VALUES (?, ?, ?, ?, ?, 'modified', ?)
		`), "file-"+path, repositoryChangeID, "checkout-history", path, []byte(path), base); err != nil {
			t.Fatal(err)
		}
	}
	files, fileTotal, err := repo.ListTurnFileChanges(ctx, changeSetIDs[0], repositoryChangeID, 0, 1)
	if err != nil || fileTotal != 2 || len(files) != 1 || files[0].Path != "src/a.go" {
		t.Fatalf("first file page = %#v total %d err %v", files, fileTotal, err)
	}
	if _, err := repo.GetTurnRepositoryChange(ctx, changeSetIDs[1], repositoryChangeID); err == nil {
		t.Fatal("repository change from another set was accepted")
	}
	if _, _, err := repo.ListTurnFileChanges(ctx, changeSetIDs[1], repositoryChangeID, 0, 10); err != nil {
		t.Fatalf("empty cross-owned page should be valid but contain no rows: %v", err)
	}
}
