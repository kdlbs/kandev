package api

import (
	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/service"
)

// SetupRoutes configures the task API routes
func SetupRoutes(router *gin.RouterGroup, svc *service.Service, log *logger.Logger) {
	handler := NewHandler(svc, log)

	// Board routes
	boards := router.Group("/boards")
	{
		boards.POST("", handler.CreateBoard)
		boards.GET("", handler.ListBoards)
		boards.GET("/:boardId", handler.GetBoard)
		boards.PUT("/:boardId", handler.UpdateBoard)
		boards.DELETE("/:boardId", handler.DeleteBoard)

		// Board sub-resources
		boards.GET("/:boardId/tasks", handler.ListTasks)
		boards.POST("/:boardId/columns", handler.CreateColumn)
		boards.GET("/:boardId/columns", handler.ListColumns)
	}

	// Task routes
	tasks := router.Group("/tasks")
	{
		tasks.POST("", handler.CreateTask)
		tasks.GET("/:taskId", handler.GetTask)
		tasks.PUT("/:taskId", handler.UpdateTask)
		tasks.DELETE("/:taskId", handler.DeleteTask)
		tasks.PUT("/:taskId/state", handler.UpdateTaskState)
		tasks.PUT("/:taskId/move", handler.MoveTask)
	}

	// Column routes
	columns := router.Group("/columns")
	{
		columns.GET("/:columnId", handler.GetColumn)
	}
}

