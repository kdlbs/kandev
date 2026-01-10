// Package wshandlers provides WebSocket message handlers for the task service.
package wshandlers

import (
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// Handlers contains WebSocket handlers for the task API
type Handlers struct {
	service *service.Service
	logger  *logger.Logger
}

// NewHandlers creates a new WebSocket handlers instance
func NewHandlers(svc *service.Service, log *logger.Logger) *Handlers {
	return &Handlers{
		service: svc,
		logger:  log.WithFields(zap.String("component", "task-ws-handlers")),
	}
}

// RegisterHandlers registers all task handlers with the dispatcher
func (h *Handlers) RegisterHandlers(d *ws.Dispatcher) {
	// Board handlers
	d.RegisterFunc(ws.ActionBoardList, h.ListBoards)
	d.RegisterFunc(ws.ActionBoardCreate, h.CreateBoard)
	d.RegisterFunc(ws.ActionBoardGet, h.GetBoard)
	d.RegisterFunc(ws.ActionBoardUpdate, h.UpdateBoard)
	d.RegisterFunc(ws.ActionBoardDelete, h.DeleteBoard)

	// Column handlers
	d.RegisterFunc(ws.ActionColumnList, h.ListColumns)
	d.RegisterFunc(ws.ActionColumnCreate, h.CreateColumn)
	d.RegisterFunc(ws.ActionColumnGet, h.GetColumn)

	// Task handlers
	d.RegisterFunc(ws.ActionTaskList, h.ListTasks)
	d.RegisterFunc(ws.ActionTaskCreate, h.CreateTask)
	d.RegisterFunc(ws.ActionTaskGet, h.GetTask)
	d.RegisterFunc(ws.ActionTaskUpdate, h.UpdateTask)
	d.RegisterFunc(ws.ActionTaskDelete, h.DeleteTask)
	d.RegisterFunc(ws.ActionTaskMove, h.MoveTask)
	d.RegisterFunc(ws.ActionTaskState, h.UpdateTaskState)
}

