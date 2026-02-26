package handlers

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// handlerRepo is the minimal repository interface needed by task handlers.
type handlerRepo interface {
	UpsertSessionFileReview(ctx context.Context, review *models.SessionFileReview) error
	GetSessionFileReviews(ctx context.Context, sessionID string) ([]*models.SessionFileReview, error)
	DeleteSessionFileReviews(ctx context.Context, sessionID string) error
	ListTurnsBySession(ctx context.Context, sessionID string) ([]*models.Turn, error)
}

type TaskHandlers struct {
	service      *service.Service
	orchestrator OrchestratorStarter
	repo         handlerRepo
	planService  *service.PlanService
	logger       *logger.Logger
}

type OrchestratorStarter interface {
	// LaunchSession is the unified entry point for all session operations.
	LaunchSession(ctx context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error)
}

func NewTaskHandlers(svc *service.Service, orchestrator OrchestratorStarter, repo handlerRepo, planService *service.PlanService, log *logger.Logger) *TaskHandlers {
	return &TaskHandlers{
		service:      svc,
		orchestrator: orchestrator,
		repo:         repo,
		planService:  planService,
		logger:       log.WithFields(zap.String("component", "task-task-handlers")),
	}
}

func RegisterTaskRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *service.Service, orchestrator OrchestratorStarter, repo handlerRepo, planService *service.PlanService, log *logger.Logger) {
	handlers := NewTaskHandlers(svc, orchestrator, repo, planService, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *TaskHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/workflows/:id/tasks", h.httpListTasks)
	api.GET("/workspaces/:id/tasks", h.httpListTasksByWorkspace)
	api.GET("/tasks/:id", h.httpGetTask)
	api.GET("/task-sessions/:id", h.httpGetTaskSession)
	api.GET("/tasks/:id/sessions", h.httpListTaskSessions)
	api.GET("/task-sessions/:id/turns", h.httpListSessionTurns)
	api.POST("/tasks", h.httpCreateTask)
	api.PATCH("/tasks/:id", h.httpUpdateTask)
	api.POST("/tasks/:id/move", h.httpMoveTask)
	api.DELETE("/tasks/:id", h.httpDeleteTask)
	api.POST("/tasks/:id/archive", h.httpArchiveTask)

	api.POST("/tasks/bulk-move", h.httpBulkMoveTasks)
	api.GET("/workflows/:id/task-count", h.httpGetWorkflowTaskCount)
	api.GET("/workflow/steps/:id/task-count", h.httpGetStepTaskCount)

	// Session workflow review endpoints
	api.POST("/sessions/:id/approve", h.httpApproveSession)
}

func (h *TaskHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionTaskList, h.wsListTasks)
	dispatcher.RegisterFunc(ws.ActionTaskCreate, h.wsCreateTask)
	dispatcher.RegisterFunc(ws.ActionTaskGet, h.wsGetTask)
	dispatcher.RegisterFunc(ws.ActionTaskUpdate, h.wsUpdateTask)
	dispatcher.RegisterFunc(ws.ActionTaskDelete, h.wsDeleteTask)
	dispatcher.RegisterFunc(ws.ActionTaskMove, h.wsMoveTask)
	dispatcher.RegisterFunc(ws.ActionTaskState, h.wsUpdateTaskState)
	dispatcher.RegisterFunc(ws.ActionTaskArchive, h.wsArchiveTask)
	dispatcher.RegisterFunc(ws.ActionTaskSessionList, h.wsListTaskSessions)
	// Git snapshot and commit handlers
	dispatcher.RegisterFunc(ws.ActionSessionGitSnapshots, h.wsGetGitSnapshots)
	dispatcher.RegisterFunc(ws.ActionSessionGitCommits, h.wsGetSessionCommits)
	dispatcher.RegisterFunc(ws.ActionSessionCumulativeDiff, h.wsGetCumulativeDiff)
	// Session file review handlers
	dispatcher.RegisterFunc(ws.ActionSessionFileReviewGet, h.wsGetSessionFileReviews)
	dispatcher.RegisterFunc(ws.ActionSessionFileReviewUpdate, h.wsUpdateSessionFileReview)
	dispatcher.RegisterFunc(ws.ActionSessionFileReviewReset, h.wsResetSessionFileReviews)
	// Task plan handlers
	dispatcher.RegisterFunc(ws.ActionTaskPlanCreate, h.wsCreateTaskPlan)
	dispatcher.RegisterFunc(ws.ActionTaskPlanGet, h.wsGetTaskPlan)
	dispatcher.RegisterFunc(ws.ActionTaskPlanUpdate, h.wsUpdateTaskPlan)
	dispatcher.RegisterFunc(ws.ActionTaskPlanDelete, h.wsDeleteTaskPlan)
}

// convertToServiceRepos converts dto.TaskRepositoryInput slice to service.TaskRepositoryInput slice.
func convertToServiceRepos(repos []dto.TaskRepositoryInput) []service.TaskRepositoryInput {
	result := make([]service.TaskRepositoryInput, len(repos))
	for i, r := range repos {
		result[i] = service.TaskRepositoryInput{
			RepositoryID:  r.RepositoryID,
			BaseBranch:    r.BaseBranch,
			LocalPath:     r.LocalPath,
			Name:          r.Name,
			DefaultBranch: r.DefaultBranch,
		}
	}
	return result
}
