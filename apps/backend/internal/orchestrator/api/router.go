package api

import (
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
)

// SetupRoutes configures the orchestrator API routes
func SetupRoutes(router *gin.RouterGroup, service *orchestrator.Service, log *logger.Logger) {
	handler := NewHandler(service, log)

	// Orchestrator control
	router.GET("/status", handler.GetStatus)
	router.GET("/queue", handler.GetQueue)
	router.POST("/trigger", handler.TriggerTask)

	// Task execution control
	tasks := router.Group("/tasks/:taskId")
	{
		tasks.POST("/start", handler.StartTask)
		tasks.POST("/stop", handler.StopTask)
		tasks.POST("/pause", handler.PauseTask)
		tasks.POST("/resume", handler.ResumeTask)
		tasks.GET("/status", handler.GetTaskStatus)
		tasks.GET("/logs", handler.GetTaskLogs)
		tasks.GET("/artifacts", handler.GetTaskArtifacts)
	}
}

