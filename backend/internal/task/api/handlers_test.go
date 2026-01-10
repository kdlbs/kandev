package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// MockEventBus implements bus.EventBus for testing
type MockEventBus struct {
	mu sync.Mutex
}

func NewMockEventBus() *MockEventBus {
	return &MockEventBus{}
}

func (m *MockEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	return nil
}

func (m *MockEventBus) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBus) QueueSubscribe(subject, queue string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBus) Request(ctx context.Context, subject string, event *bus.Event, timeout time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (m *MockEventBus) Close() {}

func (m *MockEventBus) IsConnected() bool {
	return true
}

func setupTestHandler(t *testing.T) (*Handler, *repository.MemoryRepository, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := repository.NewMemoryRepository()
	eventBus := NewMockEventBus()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := service.NewService(repo, eventBus, log)
	handler := NewHandler(svc, log)

	router := gin.New()
	return handler, repo, router
}

// Task handler tests

func TestHandler_CreateTask(t *testing.T) {
	handler, repo, router := setupTestHandler(t)
	ctx := context.Background()

	// Create board and column first
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "To Do", State: v1.TaskStateTODO}
	_ = repo.CreateColumn(ctx, column)

	router.POST("/tasks", handler.CreateTask)

	body := CreateTaskRequest{
		BoardID:     "board-123",
		ColumnID:    "col-123",
		Title:       "Test Task",
		Description: "A test task",
		Priority:    5,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp TaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Title != "Test Task" {
		t.Errorf("expected title 'Test Task', got %s", resp.Title)
	}
}

func TestHandler_GetTask(t *testing.T) {
	handler, repo, router := setupTestHandler(t)
	ctx := context.Background()

	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)

	router.GET("/tasks/:taskId", handler.GetTask)

	req := httptest.NewRequest(http.MethodGet, "/tasks/task-123", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Title != "Test Task" {
		t.Errorf("expected title 'Test Task', got %s", resp.Title)
	}
}

func TestHandler_GetTaskNotFound(t *testing.T) {
	handler, _, router := setupTestHandler(t)

	router.GET("/tasks/:taskId", handler.GetTask)

	req := httptest.NewRequest(http.MethodGet, "/tasks/nonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandler_UpdateTask(t *testing.T) {
	handler, repo, router := setupTestHandler(t)
	ctx := context.Background()

	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Original"}
	_ = repo.CreateTask(ctx, task)

	router.PUT("/tasks/:taskId", handler.UpdateTask)

	body := UpdateTaskRequest{Title: stringPtr("Updated Title")}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/tasks/task-123", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %s", resp.Title)
	}
}

func TestHandler_DeleteTask(t *testing.T) {
	handler, repo, router := setupTestHandler(t)
	ctx := context.Background()

	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test"}
	_ = repo.CreateTask(ctx, task)

	router.DELETE("/tasks/:taskId", handler.DeleteTask)

	req := httptest.NewRequest(http.MethodDelete, "/tasks/task-123", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}

	// Verify task is deleted
	_, err := repo.GetTask(ctx, "task-123")
	if err == nil {
		t.Error("expected task to be deleted")
	}
}

func TestHandler_ListTasks(t *testing.T) {
	handler, repo, router := setupTestHandler(t)
	ctx := context.Background()

	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", BoardID: "board-123", ColumnID: "col-123", Title: "Task 1"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", BoardID: "board-123", ColumnID: "col-123", Title: "Task 2"})

	router.GET("/boards/:boardId/tasks", handler.ListTasks)

	req := httptest.NewRequest(http.MethodGet, "/boards/board-123/tasks", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TasksListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(resp.Tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(resp.Tasks))
	}
}

// Board handler tests

func TestHandler_CreateBoard(t *testing.T) {
	handler, _, router := setupTestHandler(t)

	router.POST("/boards", handler.CreateBoard)

	body := CreateBoardRequest{
		Name:        "Test Board",
		Description: "A test board",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/boards", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp BoardResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Name != "Test Board" {
		t.Errorf("expected name 'Test Board', got %s", resp.Name)
	}
}

func TestHandler_GetBoard(t *testing.T) {
	handler, repo, router := setupTestHandler(t)
	ctx := context.Background()

	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)

	router.GET("/boards/:boardId", handler.GetBoard)

	req := httptest.NewRequest(http.MethodGet, "/boards/board-123", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp BoardResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Name != "Test Board" {
		t.Errorf("expected name 'Test Board', got %s", resp.Name)
	}
}

func TestHandler_GetBoardNotFound(t *testing.T) {
	handler, _, router := setupTestHandler(t)

	router.GET("/boards/:boardId", handler.GetBoard)

	req := httptest.NewRequest(http.MethodGet, "/boards/nonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandler_UpdateBoard(t *testing.T) {
	handler, repo, router := setupTestHandler(t)
	ctx := context.Background()

	board := &models.Board{ID: "board-123", Name: "Original"}
	_ = repo.CreateBoard(ctx, board)

	router.PUT("/boards/:boardId", handler.UpdateBoard)

	body := UpdateBoardRequest{Name: stringPtr("Updated")}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/boards/board-123", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp BoardResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Name != "Updated" {
		t.Errorf("expected name 'Updated', got %s", resp.Name)
	}
}

func TestHandler_DeleteBoard(t *testing.T) {
	handler, repo, router := setupTestHandler(t)
	ctx := context.Background()

	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)

	router.DELETE("/boards/:boardId", handler.DeleteBoard)

	req := httptest.NewRequest(http.MethodDelete, "/boards/board-123", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}

	// Verify board is deleted
	_, err := repo.GetBoard(ctx, "board-123")
	if err == nil {
		t.Error("expected board to be deleted")
	}
}

func TestHandler_ListBoards(t *testing.T) {
	handler, repo, router := setupTestHandler(t)
	ctx := context.Background()

	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-1", Name: "Board 1"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-2", Name: "Board 2"})

	router.GET("/boards", handler.ListBoards)

	req := httptest.NewRequest(http.MethodGet, "/boards", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp BoardsListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(resp.Boards) != 2 {
		t.Errorf("expected 2 boards, got %d", len(resp.Boards))
	}
}

// Column handler tests

func TestHandler_CreateColumn(t *testing.T) {
	handler, repo, router := setupTestHandler(t)
	ctx := context.Background()

	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)

	router.POST("/boards/:boardId/columns", handler.CreateColumn)

	body := CreateColumnRequest{
		Name:     "To Do",
		Position: 0,
		State:    "TODO",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/boards/board-123/columns", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp ColumnResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Name != "To Do" {
		t.Errorf("expected name 'To Do', got %s", resp.Name)
	}
}

func TestHandler_GetColumn(t *testing.T) {
	handler, repo, router := setupTestHandler(t)
	ctx := context.Background()

	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "To Do"}
	_ = repo.CreateColumn(ctx, column)

	router.GET("/columns/:columnId", handler.GetColumn)

	req := httptest.NewRequest(http.MethodGet, "/columns/col-123", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ColumnResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Name != "To Do" {
		t.Errorf("expected name 'To Do', got %s", resp.Name)
	}
}

func TestHandler_ListColumns(t *testing.T) {
	handler, repo, router := setupTestHandler(t)
	ctx := context.Background()

	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-1", BoardID: "board-123", Name: "To Do"})
	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-2", BoardID: "board-123", Name: "Done"})

	router.GET("/boards/:boardId/columns", handler.ListColumns)

	req := httptest.NewRequest(http.MethodGet, "/boards/board-123/columns", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ColumnsListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(resp.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(resp.Columns))
	}
}

// Helper function
func stringPtr(s string) *string {
	return &s
}

