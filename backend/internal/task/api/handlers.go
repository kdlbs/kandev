package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/errors"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Handler contains HTTP handlers for the task API
type Handler struct {
	service *service.Service
	logger  *logger.Logger
}

// NewHandler creates a new API handler
func NewHandler(svc *service.Service, log *logger.Logger) *Handler {
	return &Handler{
		service: svc,
		logger:  log,
	}
}

// Task endpoints

// CreateTask creates a new task
// POST /api/v1/tasks
func (h *Handler) CreateTask(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		appErr := errors.BadRequest(err.Error())
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	svcReq := &service.CreateTaskRequest{
		BoardID:     req.BoardID,
		ColumnID:    req.ColumnID,
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		AgentType:   req.AgentType,
		Metadata:    req.Metadata,
	}

	task, err := h.service.CreateTask(c.Request.Context(), svcReq)
	if err != nil {
		h.logger.Error("failed to create task", zap.Error(err))
		appErr := errors.InternalError("failed to create task", err)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	c.JSON(http.StatusCreated, taskToResponse(task))
}

// GetTask retrieves a task by ID
// GET /api/v1/tasks/:taskId
func (h *Handler) GetTask(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		appErr := errors.BadRequest("taskId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	task, err := h.service.GetTask(c.Request.Context(), taskID)
	if err != nil {
		appErr := errors.NotFound("task", taskID)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	c.JSON(http.StatusOK, taskToResponse(task))
}

// UpdateTask updates an existing task
// PUT /api/v1/tasks/:taskId
func (h *Handler) UpdateTask(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		appErr := errors.BadRequest("taskId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	var req UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		appErr := errors.BadRequest(err.Error())
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	svcReq := &service.UpdateTaskRequest{
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		AgentType:   req.AgentType,
		Metadata:    req.Metadata,
	}

	task, err := h.service.UpdateTask(c.Request.Context(), taskID, svcReq)
	if err != nil {
		h.logger.Error("failed to update task", zap.String("task_id", taskID), zap.Error(err))
		appErr := errors.InternalError("failed to update task", err)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	c.JSON(http.StatusOK, taskToResponse(task))
}

// DeleteTask deletes a task
// DELETE /api/v1/tasks/:taskId
func (h *Handler) DeleteTask(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		appErr := errors.BadRequest("taskId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	if err := h.service.DeleteTask(c.Request.Context(), taskID); err != nil {
		h.logger.Error("failed to delete task", zap.String("task_id", taskID), zap.Error(err))
		appErr := errors.InternalError("failed to delete task", err)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListTasks returns all tasks for a board
// GET /api/v1/boards/:boardId/tasks
func (h *Handler) ListTasks(c *gin.Context) {
	boardID := c.Param("boardId")
	if boardID == "" {
		appErr := errors.BadRequest("boardId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	tasks, err := h.service.ListTasks(c.Request.Context(), boardID)
	if err != nil {
		h.logger.Error("failed to list tasks", zap.String("board_id", boardID), zap.Error(err))
		appErr := errors.InternalError("failed to list tasks", err)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	resp := TasksListResponse{
		Tasks: make([]*TaskResponse, len(tasks)),
		Total: len(tasks),
	}
	for i, t := range tasks {
		resp.Tasks[i] = taskToResponse(t)
	}

	c.JSON(http.StatusOK, resp)
}

// UpdateTaskState updates the state of a task
// PUT /api/v1/tasks/:taskId/state
func (h *Handler) UpdateTaskState(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		appErr := errors.BadRequest("taskId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	var req UpdateTaskStateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		appErr := errors.BadRequest(err.Error())
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	state := v1.TaskState(req.State)
	task, err := h.service.UpdateTaskState(c.Request.Context(), taskID, state)
	if err != nil {
		h.logger.Error("failed to update task state", zap.String("task_id", taskID), zap.Error(err))
		appErr := errors.InternalError("failed to update task state", err)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	c.JSON(http.StatusOK, taskToResponse(task))
}

// MoveTask moves a task to a different column
// PUT /api/v1/tasks/:taskId/move
func (h *Handler) MoveTask(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		appErr := errors.BadRequest("taskId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	var req MoveTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		appErr := errors.BadRequest(err.Error())
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	task, err := h.service.MoveTask(c.Request.Context(), taskID, req.ColumnID, req.Position)
	if err != nil {
		h.logger.Error("failed to move task", zap.String("task_id", taskID), zap.Error(err))
		appErr := errors.InternalError("failed to move task", err)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	c.JSON(http.StatusOK, taskToResponse(task))
}

// Board endpoints

// CreateBoard creates a new board
// POST /api/v1/boards
func (h *Handler) CreateBoard(c *gin.Context) {
	var req CreateBoardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		appErr := errors.BadRequest(err.Error())
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	svcReq := &service.CreateBoardRequest{
		Name:        req.Name,
		Description: req.Description,
		OwnerID:     "", // TODO: Get from authenticated user
	}

	board, err := h.service.CreateBoard(c.Request.Context(), svcReq)
	if err != nil {
		h.logger.Error("failed to create board", zap.Error(err))
		appErr := errors.InternalError("failed to create board", err)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	c.JSON(http.StatusCreated, boardToResponse(board))
}

// GetBoard retrieves a board by ID
// GET /api/v1/boards/:boardId
func (h *Handler) GetBoard(c *gin.Context) {
	boardID := c.Param("boardId")
	if boardID == "" {
		appErr := errors.BadRequest("boardId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	board, err := h.service.GetBoard(c.Request.Context(), boardID)
	if err != nil {
		appErr := errors.NotFound("board", boardID)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	c.JSON(http.StatusOK, boardToResponse(board))
}

// UpdateBoard updates an existing board
// PUT /api/v1/boards/:boardId
func (h *Handler) UpdateBoard(c *gin.Context) {
	boardID := c.Param("boardId")
	if boardID == "" {
		appErr := errors.BadRequest("boardId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	var req UpdateBoardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		appErr := errors.BadRequest(err.Error())
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	svcReq := &service.UpdateBoardRequest{
		Name:        req.Name,
		Description: req.Description,
	}

	board, err := h.service.UpdateBoard(c.Request.Context(), boardID, svcReq)
	if err != nil {
		h.logger.Error("failed to update board", zap.String("board_id", boardID), zap.Error(err))
		appErr := errors.InternalError("failed to update board", err)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	c.JSON(http.StatusOK, boardToResponse(board))
}

// DeleteBoard deletes a board
// DELETE /api/v1/boards/:boardId
func (h *Handler) DeleteBoard(c *gin.Context) {
	boardID := c.Param("boardId")
	if boardID == "" {
		appErr := errors.BadRequest("boardId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	if err := h.service.DeleteBoard(c.Request.Context(), boardID); err != nil {
		h.logger.Error("failed to delete board", zap.String("board_id", boardID), zap.Error(err))
		appErr := errors.InternalError("failed to delete board", err)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListBoards returns all boards
// GET /api/v1/boards
func (h *Handler) ListBoards(c *gin.Context) {
	boards, err := h.service.ListBoards(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list boards", zap.Error(err))
		appErr := errors.InternalError("failed to list boards", err)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	resp := BoardsListResponse{
		Boards: make([]*BoardResponse, len(boards)),
		Total:  len(boards),
	}
	for i, b := range boards {
		resp.Boards[i] = boardToResponse(b)
	}

	c.JSON(http.StatusOK, resp)
}

// Column endpoints

// CreateColumn creates a new column
// POST /api/v1/boards/:boardId/columns
func (h *Handler) CreateColumn(c *gin.Context) {
	boardID := c.Param("boardId")
	if boardID == "" {
		appErr := errors.BadRequest("boardId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	var req CreateColumnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		appErr := errors.BadRequest(err.Error())
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	svcReq := &service.CreateColumnRequest{
		BoardID:  boardID,
		Name:     req.Name,
		Position: req.Position,
		State:    v1.TaskState(req.State),
	}

	column, err := h.service.CreateColumn(c.Request.Context(), svcReq)
	if err != nil {
		h.logger.Error("failed to create column", zap.String("board_id", boardID), zap.Error(err))
		appErr := errors.InternalError("failed to create column", err)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	c.JSON(http.StatusCreated, columnToResponse(column))
}

// ListColumns returns all columns for a board
// GET /api/v1/boards/:boardId/columns
func (h *Handler) ListColumns(c *gin.Context) {
	boardID := c.Param("boardId")
	if boardID == "" {
		appErr := errors.BadRequest("boardId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	columns, err := h.service.ListColumns(c.Request.Context(), boardID)
	if err != nil {
		h.logger.Error("failed to list columns", zap.String("board_id", boardID), zap.Error(err))
		appErr := errors.InternalError("failed to list columns", err)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	resp := ColumnsListResponse{
		Columns: make([]*ColumnResponse, len(columns)),
		Total:   len(columns),
	}
	for i, col := range columns {
		resp.Columns[i] = columnToResponse(col)
	}

	c.JSON(http.StatusOK, resp)
}

// GetColumn retrieves a column by ID
// GET /api/v1/columns/:columnId
func (h *Handler) GetColumn(c *gin.Context) {
	columnID := c.Param("columnId")
	if columnID == "" {
		appErr := errors.BadRequest("columnId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	column, err := h.service.GetColumn(c.Request.Context(), columnID)
	if err != nil {
		appErr := errors.NotFound("column", columnID)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	c.JSON(http.StatusOK, columnToResponse(column))
}

// Helper functions to convert models to response types

func taskToResponse(t *models.Task) *TaskResponse {
	return &TaskResponse{
		ID:          t.ID,
		BoardID:     t.BoardID,
		ColumnID:    t.ColumnID,
		Title:       t.Title,
		Description: t.Description,
		State:       string(t.State),
		Priority:    t.Priority,
		AgentType:   t.AgentType,
		Position:    t.Position,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		Metadata:    t.Metadata,
	}
}

func boardToResponse(b *models.Board) *BoardResponse {
	return &BoardResponse{
		ID:          b.ID,
		Name:        b.Name,
		Description: b.Description,
		CreatedAt:   b.CreatedAt,
		UpdatedAt:   b.UpdatedAt,
	}
}

func columnToResponse(c *models.Column) *ColumnResponse {
	return &ColumnResponse{
		ID:        c.ID,
		BoardID:   c.BoardID,
		Name:      c.Name,
		Position:  c.Position,
		State:     string(c.State),
		CreatedAt: c.CreatedAt,
	}
}

