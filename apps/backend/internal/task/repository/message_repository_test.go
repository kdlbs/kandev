package repository

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

func setupSQLiteTestSession(ctx context.Context, repo *sqlite.Repository, taskID, sessionID string) string {
	session := &models.TaskSession{
		ID:             sessionID,
		TaskID:         taskID,
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateStarting,
	}
	_ = repo.CreateTaskSession(ctx, session)
	return session.ID
}

func setupSQLiteTestTurn(ctx context.Context, repo *sqlite.Repository, sessionID, taskID, turnID string) string {
	now := time.Now()
	turn := &models.Turn{
		ID:            turnID,
		TaskSessionID: sessionID,
		TaskID:        taskID,
		StartedAt:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_ = repo.CreateTurn(ctx, turn)
	return turn.ID
}

// Message CRUD tests

func TestSQLiteRepository_MessageCRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workflow first (required for foreign key constraints)
	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	// Create a task first
	task := &models.Task{ID: "task-123", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	sessionID := setupSQLiteTestSession(ctx, repo, task.ID, "session-123")
	turnID := setupSQLiteTestTurn(ctx, repo, sessionID, task.ID, "turn-123")

	// Create comment
	comment := &models.Message{
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorUser,
		AuthorID:      "user-123",
		Content:       "This is a test comment",
		RequestsInput: false,
	}
	if err := repo.CreateMessage(ctx, comment); err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}
	if comment.ID == "" {
		t.Error("expected comment ID to be set")
	}
	if comment.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Get comment
	retrieved, err := repo.GetMessage(ctx, comment.ID)
	if err != nil {
		t.Fatalf("failed to get comment: %v", err)
	}
	if retrieved.Content != "This is a test comment" {
		t.Errorf("expected content 'This is a test comment', got %s", retrieved.Content)
	}
	if retrieved.AuthorType != models.MessageAuthorUser {
		t.Errorf("expected author type 'user', got %s", retrieved.AuthorType)
	}

	// Delete comment
	if err := repo.DeleteMessage(ctx, comment.ID); err != nil {
		t.Fatalf("failed to delete comment: %v", err)
	}
	_, err = repo.GetMessage(ctx, comment.ID)
	if err == nil {
		t.Error("expected comment to be deleted")
	}
}

func TestSQLiteRepository_MessageNotFound(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_, err := repo.GetMessage(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent comment")
	}

	err = repo.DeleteMessage(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent comment")
	}
}

func TestSQLiteRepository_ListMessages(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workflow first (required for foreign key constraints)
	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	// Create tasks
	task := &models.Task{ID: "task-123", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	task2 := &models.Task{ID: "task-456", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task 2"}
	_ = repo.CreateTask(ctx, task2)
	sessionID := setupSQLiteTestSession(ctx, repo, task.ID, "session-123")
	session2ID := setupSQLiteTestSession(ctx, repo, task2.ID, "session-456")
	turnID := setupSQLiteTestTurn(ctx, repo, sessionID, task.ID, "turn-123")
	turn2ID := setupSQLiteTestTurn(ctx, repo, session2ID, task2.ID, "turn-456")

	// Create multiple comments
	_ = repo.CreateMessage(ctx, &models.Message{ID: "comment-1", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorUser, Content: "Comment 1"})
	_ = repo.CreateMessage(ctx, &models.Message{ID: "comment-2", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorAgent, Content: "Comment 2"})
	_ = repo.CreateMessage(ctx, &models.Message{ID: "comment-3", TaskSessionID: session2ID, TaskID: "task-456", TurnID: turn2ID, AuthorType: models.MessageAuthorUser, Content: "Comment 3"})

	comments, err := repo.ListMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to list comments: %v", err)
	}
	if len(comments) != 2 {
		t.Errorf("expected 2 comments for task-123, got %d", len(comments))
	}
}

func TestSQLiteRepository_ListMessagesPagination(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	task := &models.Task{ID: "task-123", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	sessionID := setupSQLiteTestSession(ctx, repo, task.ID, "session-123")
	turnID := setupSQLiteTestTurn(ctx, repo, sessionID, task.ID, "turn-123")

	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	_ = repo.CreateMessage(ctx, &models.Message{
		ID:            "comment-1",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorUser,
		Content:       "Comment 1",
		CreatedAt:     baseTime.Add(-2 * time.Minute),
	})
	_ = repo.CreateMessage(ctx, &models.Message{
		ID:            "comment-2",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorUser,
		Content:       "Comment 2",
		CreatedAt:     baseTime.Add(-1 * time.Minute),
	})
	_ = repo.CreateMessage(ctx, &models.Message{
		ID:            "comment-3",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorUser,
		Content:       "Comment 3",
		CreatedAt:     baseTime,
	})

	comments, hasMore, err := repo.ListMessagesPaginated(ctx, sessionID, models.ListMessagesOptions{
		Limit: 2,
		Sort:  "desc",
	})
	if err != nil {
		t.Fatalf("failed to list comments: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}
	if !hasMore {
		t.Error("expected hasMore to be true")
	}
	if comments[0].ID != "comment-3" {
		t.Errorf("expected newest comment first, got %s", comments[0].ID)
	}
}

func TestSQLiteRepository_MessageWithRequestsInput(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workflow first (required for foreign key constraints)
	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	// Create a task
	task := &models.Task{ID: "task-123", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	sessionID := setupSQLiteTestSession(ctx, repo, task.ID, "session-123")
	turnID := setupSQLiteTestTurn(ctx, repo, sessionID, task.ID, "turn-123")

	// Create agent comment requesting input
	comment := &models.Message{
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		AuthorID:      "agent-123",
		Content:       "What should I do next?",
		RequestsInput: true,
	}
	if err := repo.CreateMessage(ctx, comment); err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}

	retrieved, _ := repo.GetMessage(ctx, comment.ID)
	if !retrieved.RequestsInput {
		t.Error("expected RequestsInput to be true")
	}
}

func TestSQLiteRepository_SearchMessages(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	workflow := &models.Workflow{ID: "wf-s", Name: "W"}
	_ = repo.CreateWorkflow(ctx, workflow)
	task := &models.Task{ID: "task-s", WorkflowID: "wf-s", WorkflowStepID: "step-s", Title: "T"}
	_ = repo.CreateTask(ctx, task)
	sid := setupSQLiteTestSession(ctx, repo, task.ID, "sess-s")
	otherSid := setupSQLiteTestSession(ctx, repo, task.ID, "sess-other")
	turnID := setupSQLiteTestTurn(ctx, repo, sid, task.ID, "turn-s")
	otherTurnID := setupSQLiteTestTurn(ctx, repo, otherSid, task.ID, "turn-other")

	base := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	_ = repo.CreateMessage(ctx, &models.Message{
		ID: "m1", TaskSessionID: sid, TaskID: task.ID, TurnID: turnID,
		AuthorType: models.MessageAuthorUser,
		Content:    "hello world from the agent",
		CreatedAt:  base.Add(-2 * time.Minute),
	})
	_ = repo.CreateMessage(ctx, &models.Message{
		ID: "m2", TaskSessionID: sid, TaskID: task.ID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent,
		Content:    "no match here",
		CreatedAt:  base.Add(-1 * time.Minute),
	})
	_ = repo.CreateMessage(ctx, &models.Message{
		ID: "m3", TaskSessionID: sid, TaskID: task.ID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent,
		Content:    "another HELLO from the agent again",
		CreatedAt:  base,
	})
	_ = repo.CreateMessage(ctx, &models.Message{
		ID: "m-other", TaskSessionID: otherSid, TaskID: task.ID, TurnID: otherTurnID,
		AuthorType: models.MessageAuthorUser,
		Content:    "hello from other session",
		CreatedAt:  base,
	})

	hits, err := repo.SearchMessages(ctx, sid, models.SearchMessagesOptions{Query: "hello"})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits in session, got %d", len(hits))
	}
	// Newest-first: m3 before m1
	if hits[0].ID != "m3" || hits[1].ID != "m1" {
		t.Errorf("expected hits [m3, m1], got [%s, %s]", hits[0].ID, hits[1].ID)
	}

	// Respect session boundary — no m-other
	for _, h := range hits {
		if h.TaskSessionID == otherSid {
			t.Errorf("search leaked into other session: %s", h.ID)
		}
	}

	// Empty query returns nothing
	empty, err := repo.SearchMessages(ctx, sid, models.SearchMessagesOptions{Query: "  "})
	if err != nil {
		t.Fatalf("empty search failed: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 hits for empty query, got %d", len(empty))
	}

	// Limit honored
	limited, err := repo.SearchMessages(ctx, sid, models.SearchMessagesOptions{Query: "hello", Limit: 1})
	if err != nil {
		t.Fatalf("limited search failed: %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("expected 1 hit with limit=1, got %d", len(limited))
	}
}
