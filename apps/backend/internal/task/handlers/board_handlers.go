package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// WorkflowStepLister provides access to workflow steps for boards with workflow templates.
type WorkflowStepLister interface {
	ListStepsByBoard(ctx context.Context, boardID string) ([]*workflowmodels.WorkflowStep, error)
}

type BoardHandlers struct {
	service            *service.Service
	workflowStepLister WorkflowStepLister
	logger             *logger.Logger
}

func NewBoardHandlers(svc *service.Service, log *logger.Logger) *BoardHandlers {
	return &BoardHandlers{
		service: svc,
		logger:  log.WithFields(zap.String("component", "task-board-handlers")),
	}
}

// SetWorkflowStepLister sets the workflow step lister for returning workflow steps.
func (h *BoardHandlers) SetWorkflowStepLister(lister WorkflowStepLister) {
	h.workflowStepLister = lister
}

func RegisterBoardRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *service.Service, log *logger.Logger) *BoardHandlers {
	handlers := NewBoardHandlers(svc, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
	return handlers
}

func (h *BoardHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/boards", h.httpListBoards)
	api.GET("/workspaces/:id/boards", h.httpListBoardsByWorkspace)
	api.GET("/workspaces/:id/board-snapshot", h.httpGetWorkspaceBoardSnapshot)
	api.GET("/boards/:id", h.httpGetBoard)
	api.GET("/boards/:id/snapshot", h.httpGetBoardSnapshot)
	api.POST("/boards", h.httpCreateBoard)
	api.PATCH("/boards/:id", h.httpUpdateBoard)
	api.DELETE("/boards/:id", h.httpDeleteBoard)
}

func (h *BoardHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionBoardList, h.wsListBoards)
	dispatcher.RegisterFunc(ws.ActionBoardCreate, h.wsCreateBoard)
	dispatcher.RegisterFunc(ws.ActionBoardGet, h.wsGetBoard)
	dispatcher.RegisterFunc(ws.ActionBoardUpdate, h.wsUpdateBoard)
	dispatcher.RegisterFunc(ws.ActionBoardDelete, h.wsDeleteBoard)
}

// toWorkflowStepDTO converts a workflow step to a WorkflowStepDTO.
func boardWorkflowStepToDTO(step *workflowmodels.WorkflowStep) dto.WorkflowStepDTO {
	return dto.WorkflowStepDTO{
		ID:              step.ID,
		BoardID:         step.BoardID,
		Name:            step.Name,
		StepType:        step.StepType,
		Position:        step.Position,
		State:           step.TaskState,
		Color:           step.Color,
		AutoStartAgent:  step.AutoStartAgent,
		PlanMode:        step.PlanMode,
		RequireApproval: step.RequireApproval,
		PromptPrefix:    step.PromptPrefix,
		PromptSuffix:    step.PromptSuffix,
		AllowManualMove: step.AllowManualMove,
		CreatedAt:       step.CreatedAt,
		UpdatedAt:       step.UpdatedAt,
	}
}

// getStepsForBoard returns workflow steps for a board.
func (h *BoardHandlers) getStepsForBoard(ctx context.Context, board *models.Board) ([]dto.WorkflowStepDTO, error) {
	if h.workflowStepLister == nil {
		return nil, fmt.Errorf("workflow step lister not configured")
	}
	steps, err := h.workflowStepLister.ListStepsByBoard(ctx, board.ID)
	if err != nil {
		return nil, err
	}
	result := make([]dto.WorkflowStepDTO, 0, len(steps))
	for _, step := range steps {
		result = append(result, boardWorkflowStepToDTO(step))
	}
	return result, nil
}

// convertTasksWithPrimarySessions converts task models to DTOs with primary session IDs.
func (h *BoardHandlers) convertTasksWithPrimarySessions(ctx context.Context, tasks []*models.Task) ([]dto.TaskDTO, error) {
	if len(tasks) == 0 {
		return []dto.TaskDTO{}, nil
	}

	taskIDs := make([]string, len(tasks))
	for i, task := range tasks {
		taskIDs[i] = task.ID
	}

	primarySessionMap, err := h.service.GetPrimarySessionIDsForTasks(ctx, taskIDs)
	if err != nil {
		return nil, err
	}

	sessionCountMap, err := h.service.GetSessionCountsForTasks(ctx, taskIDs)
	if err != nil {
		return nil, err
	}

	primarySessionInfoMap, err := h.service.GetPrimarySessionInfoForTasks(ctx, taskIDs)
	if err != nil {
		return nil, err
	}

	result := make([]dto.TaskDTO, 0, len(tasks))
	for _, task := range tasks {
		var primarySessionID *string
		if sid, ok := primarySessionMap[task.ID]; ok {
			primarySessionID = &sid
		}

		var sessionCount *int
		if count, ok := sessionCountMap[task.ID]; ok {
			sessionCount = &count
		}

		var reviewStatus *string
		if sessionInfo, ok := primarySessionInfoMap[task.ID]; ok && sessionInfo.ReviewStatus != nil {
			reviewStatus = sessionInfo.ReviewStatus
		}

		result = append(result, dto.FromTaskWithSessionInfo(task, primarySessionID, sessionCount, reviewStatus))
	}
	return result, nil
}

// HTTP handlers

func (h *BoardHandlers) httpListBoards(c *gin.Context) {
	boards, err := h.service.ListBoards(c.Request.Context(), c.Query("workspace_id"))
	if err != nil {
		h.logger.Error("failed to list boards", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list boards"})
		return
	}
	resp := dto.ListBoardsResponse{
		Boards: make([]dto.BoardDTO, 0, len(boards)),
		Total:  len(boards),
	}
	for _, board := range boards {
		resp.Boards = append(resp.Boards, dto.FromBoard(board))
	}
	c.JSON(http.StatusOK, resp)
}

func (h *BoardHandlers) httpListBoardsByWorkspace(c *gin.Context) {
	boards, err := h.service.ListBoards(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.logger.Error("failed to list boards", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list boards"})
		return
	}
	resp := dto.ListBoardsResponse{
		Boards: make([]dto.BoardDTO, 0, len(boards)),
		Total:  len(boards),
	}
	for _, board := range boards {
		resp.Boards = append(resp.Boards, dto.FromBoard(board))
	}
	c.JSON(http.StatusOK, resp)
}

func (h *BoardHandlers) httpGetBoard(c *gin.Context) {
	board, err := h.service.GetBoard(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleNotFound(c, h.logger, err, "board not found")
		return
	}
	c.JSON(http.StatusOK, dto.FromBoard(board))
}

type httpCreateBoardRequest struct {
	WorkspaceID        string  `json:"workspace_id"`
	Name               string  `json:"name"`
	Description        string  `json:"description,omitempty"`
	WorkflowTemplateID *string `json:"workflow_template_id,omitempty"`
}

func (h *BoardHandlers) httpCreateBoard(c *gin.Context) {
	var body httpCreateBoardRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if body.Name == "" || body.WorkspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id and name are required"})
		return
	}
	board, err := h.service.CreateBoard(c.Request.Context(), &service.CreateBoardRequest{
		WorkspaceID:        body.WorkspaceID,
		Name:               body.Name,
		Description:        body.Description,
		WorkflowTemplateID: body.WorkflowTemplateID,
	})
	if err != nil {
		h.logger.Error("failed to create board", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create board"})
		return
	}
	c.JSON(http.StatusCreated, dto.FromBoard(board))
}

type httpUpdateBoardRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (h *BoardHandlers) httpUpdateBoard(c *gin.Context) {
	var body httpUpdateBoardRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	board, err := h.service.UpdateBoard(c.Request.Context(), c.Param("id"), &service.UpdateBoardRequest{
		Name:        body.Name,
		Description: body.Description,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "board not found")
		return
	}
	c.JSON(http.StatusOK, dto.FromBoard(board))
}

func (h *BoardHandlers) httpDeleteBoard(c *gin.Context) {
	if err := h.service.DeleteBoard(c.Request.Context(), c.Param("id")); err != nil {
		handleNotFound(c, h.logger, err, "board not found")
		return
	}
	c.JSON(http.StatusOK, dto.SuccessResponse{Success: true})
}

func (h *BoardHandlers) httpGetBoardSnapshot(c *gin.Context) {
	ctx := c.Request.Context()
	boardID := c.Param("id")

	board, err := h.service.GetBoard(ctx, boardID)
	if err != nil {
		handleNotFound(c, h.logger, err, "board not found")
		return
	}
	steps, err := h.getStepsForBoard(ctx, board)
	if err != nil {
		h.logger.Error("failed to get workflow steps", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get workflow steps"})
		return
	}
	tasks, err := h.service.ListTasks(ctx, boardID)
	if err != nil {
		h.logger.Error("failed to list tasks", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tasks"})
		return
	}

	taskDTOs, err := h.convertTasksWithPrimarySessions(ctx, tasks)
	if err != nil {
		h.logger.Error("failed to convert tasks", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to convert tasks"})
		return
	}

	c.JSON(http.StatusOK, dto.BoardSnapshotDTO{
		Board: dto.FromBoard(board),
		Steps: steps,
		Tasks: taskDTOs,
	})
}

func (h *BoardHandlers) httpGetWorkspaceBoardSnapshot(c *gin.Context) {
	ctx := c.Request.Context()
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace id is required"})
		return
	}

	boardID := c.Query("board_id")
	if boardID == "" {
		boards, err := h.service.ListBoards(ctx, workspaceID)
		if err != nil {
			h.logger.Error("failed to list boards", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list boards"})
			return
		}
		if len(boards) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "no boards found for workspace"})
			return
		}
		boardID = boards[0].ID
	}

	board, err := h.service.GetBoard(ctx, boardID)
	if err != nil {
		handleNotFound(c, h.logger, err, "board not found")
		return
	}
	if board.WorkspaceID != workspaceID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "board does not belong to workspace"})
		return
	}

	steps, err := h.getStepsForBoard(ctx, board)
	if err != nil {
		h.logger.Error("failed to get workflow steps", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get workflow steps"})
		return
	}
	tasks, err := h.service.ListTasks(ctx, boardID)
	if err != nil {
		h.logger.Error("failed to list tasks", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tasks"})
		return
	}

	taskDTOs, err := h.convertTasksWithPrimarySessions(ctx, tasks)
	if err != nil {
		h.logger.Error("failed to convert tasks", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to convert tasks"})
		return
	}

	c.JSON(http.StatusOK, dto.BoardSnapshotDTO{
		Board: dto.FromBoard(board),
		Steps: steps,
		Tasks: taskDTOs,
	})
}

// WS handlers

func (h *BoardHandlers) wsListBoards(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		WorkspaceID string `json:"workspace_id,omitempty"`
	}
	if msg.Payload != nil {
		if err := msg.ParsePayload(&req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		}
	}
	boards, err := h.service.ListBoards(ctx, req.WorkspaceID)
	if err != nil {
		h.logger.Error("failed to list boards", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list boards", nil)
	}
	resp := dto.ListBoardsResponse{
		Boards: make([]dto.BoardDTO, 0, len(boards)),
		Total:  len(boards),
	}
	for _, board := range boards {
		resp.Boards = append(resp.Boards, dto.FromBoard(board))
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCreateBoardRequest struct {
	WorkspaceID        string  `json:"workspace_id"`
	Name               string  `json:"name"`
	Description        string  `json:"description,omitempty"`
	WorkflowTemplateID *string `json:"workflow_template_id,omitempty"`
}

func (h *BoardHandlers) wsCreateBoard(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateBoardRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name is required", nil)
	}
	if req.WorkspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required", nil)
	}

	board, err := h.service.CreateBoard(ctx, &service.CreateBoardRequest{
		WorkspaceID:        req.WorkspaceID,
		Name:               req.Name,
		Description:        req.Description,
		WorkflowTemplateID: req.WorkflowTemplateID,
	})
	if err != nil {
		h.logger.Error("failed to create board", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create board", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromBoard(board))
}

type wsGetBoardRequest struct {
	ID string `json:"id"`
}

func (h *BoardHandlers) wsGetBoard(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetBoardRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	board, err := h.service.GetBoard(ctx, req.ID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Board not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromBoard(board))
}

type wsUpdateBoardRequest struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (h *BoardHandlers) wsUpdateBoard(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateBoardRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	board, err := h.service.UpdateBoard(ctx, req.ID, &service.UpdateBoardRequest{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		h.logger.Error("failed to update board", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update board", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromBoard(board))
}

type wsDeleteBoardRequest struct {
	ID string `json:"id"`
}

func (h *BoardHandlers) wsDeleteBoard(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsDeleteBoardRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	if err := h.service.DeleteBoard(ctx, req.ID); err != nil {
		h.logger.Error("failed to delete board", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete board", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.SuccessResponse{Success: true})
}
