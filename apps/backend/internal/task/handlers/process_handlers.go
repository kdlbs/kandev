package handlers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	agentctlclient "github.com/kandev/kandev/internal/agentctl/client"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

type ProcessHandlers struct {
	service      *service.Service
	lifecycleMgr *lifecycle.Manager
	logger       *logger.Logger
}

func RegisterProcessRoutes(
	router *gin.Engine,
	svc *service.Service,
	lifecycleMgr *lifecycle.Manager,
	log *logger.Logger,
) {
	handlers := &ProcessHandlers{
		service:      svc,
		lifecycleMgr: lifecycleMgr,
		logger:       log.WithFields(zap.String("component", "task-process-handlers")),
	}
	api := router.Group("/api/v1")
	processes := api.Group("/task-sessions/:id/processes")
	processes.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		handlers.logger.Debug("process route",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("route", c.FullPath()),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("duration", time.Since(start)),
		)
	})
	processes.POST("/start", handlers.httpStartProcess)
	processes.POST("/:processId/stop", handlers.httpStopProcessByID)
	processes.GET("", handlers.httpListProcesses)
	processes.GET("/:processId", handlers.httpGetProcess)
}

type httpStartProcessRequest struct {
	Kind         string `json:"kind"`
	ScriptName   string `json:"script_name,omitempty"`
	RepositoryID string `json:"repo_id,omitempty"`
}

func (h *ProcessHandlers) httpStartProcess(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		h.logger.Warn("start process missing session id")
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}

	var body httpStartProcessRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		h.logger.Warn("start process invalid request body", zap.String("session_id", sessionID), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	h.logger.Debug("start process request",
		zap.String("session_id", sessionID),
		zap.String("kind", body.Kind),
		zap.String("script_name", body.ScriptName),
		zap.String("repo_id", body.RepositoryID),
	)

	session, err := h.service.GetTaskSession(c.Request.Context(), sessionID)
	if err != nil {
		h.logger.Warn("start process session not found", zap.String("session_id", sessionID), zap.Error(err))
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}

	executorID := session.ExecutorID
	// TODO: fix session, should always have executorID
	if executorID == "" {
		if task, err := h.service.GetTask(c.Request.Context(), session.TaskID); err == nil {
			if workspace, err := h.service.GetWorkspace(c.Request.Context(), task.WorkspaceID); err == nil {
				if workspace.DefaultExecutorID != nil {
					executorID = *workspace.DefaultExecutorID
				}
			}
		}
		if executorID == "" {
			executorID = models.ExecutorIDLocalPC
		}
	}
	executor, err := h.service.GetExecutor(c.Request.Context(), executorID)
	if err != nil {
		h.logger.Warn("start process executor not found",
			zap.String("session_id", sessionID),
			zap.String("executor_id", executorID),
			zap.Error(err),
		)
		handleNotFound(c, h.logger, err, "executor not found")
		return
	}

	repoID := body.RepositoryID
	if repoID == "" {
		repoID = session.RepositoryID
	}
	if repoID == "" {
		if task, err := h.service.GetTask(c.Request.Context(), session.TaskID); err == nil {
			if len(task.Repositories) > 0 {
				repoID = task.Repositories[0].RepositoryID
			}
		}
	}
	if repoID == "" {
		h.logger.Warn("start process missing repository",
			zap.String("session_id", sessionID),
			zap.String("task_id", session.TaskID),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": "session has no repository"})
		return
	}
	repo, err := h.service.GetRepository(c.Request.Context(), repoID)
	if err != nil {
		h.logger.Warn("start process repository not found",
			zap.String("session_id", sessionID),
			zap.String("repo_id", repoID),
			zap.Error(err),
		)
		handleNotFound(c, h.logger, err, "repository not found")
		return
	}

	command, kind, scriptName, err := resolveScriptCommand(c.Request.Context(), h.service, repo, body.Kind, body.ScriptName)
	if err != nil {
		h.logger.Warn("start process script resolution failed",
			zap.String("session_id", sessionID),
			zap.String("repo_id", repoID),
			zap.String("kind", body.Kind),
			zap.String("script_name", body.ScriptName),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	workingDir := repo.LocalPath
	if len(session.Worktrees) > 0 && session.Worktrees[0].WorktreePath != "" {
		workingDir = session.Worktrees[0].WorktreePath
	}
	h.logger.Info("start process resolved command",
		zap.String("session_id", sessionID),
		zap.String("repo_id", repoID),
		zap.String("kind", kind),
		zap.String("script_name", scriptName),
		zap.String("working_dir", workingDir),
		zap.String("command", command),
	)

	if _, err := h.lifecycleMgr.EnsureWorkspaceExecutionForSession(
		c.Request.Context(),
		session.TaskID,
		session.ID,
	); err != nil {
		h.logger.Error("failed to ensure workspace execution for process", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	switch executor.Type {
	case models.ExecutorTypeLocalPC, models.ExecutorTypeLocalDocker:
		if streams.ProcessKind(kind) == streams.ProcessKindDev {
			if existing, err := h.lifecycleMgr.ListProcesses(c.Request.Context(), sessionID); err == nil {
				for _, proc := range existing {
					if proc.Kind != streams.ProcessKindDev {
						continue
					}
					if proc.Status == agentctltypes.ProcessStatusRunning || proc.Status == agentctltypes.ProcessStatusStarting {
						c.JSON(http.StatusOK, gin.H{"process": proc})
						return
					}
				}
			} else {
				h.logger.Warn("failed to list processes for dev check",
					zap.String("session_id", sessionID),
					zap.Error(err),
				)
			}
		}
		proc, err := h.lifecycleMgr.StartProcess(c.Request.Context(), lifecycle.StartProcessRequest{
			SessionID:  sessionID,
			Kind:       kind,
			ScriptName: scriptName,
			Command:    command,
			WorkingDir: workingDir,
		})
		if err != nil {
			h.logger.Error("failed to start process",
				zap.String("session_id", sessionID),
				zap.String("repo_id", repoID),
				zap.String("kind", kind),
				zap.String("script_name", scriptName),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		h.logger.Info("process started",
			zap.String("session_id", sessionID),
			zap.String("process_id", proc.ID),
			zap.String("kind", kind),
			zap.String("script_name", scriptName),
		)
		c.JSON(http.StatusOK, gin.H{"process": proc})
	default:
		h.logger.Warn("start process unsupported executor type",
			zap.String("session_id", sessionID),
			zap.String("executor_id", executorID),
			zap.String("executor_type", string(executor.Type)),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": "executor type not supported for process runner"})
	}
}

func (h *ProcessHandlers) httpStopProcessByID(c *gin.Context) {
	sessionID := c.Param("id")
	processID := c.Param("processId")
	if sessionID == "" || processID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id and process_id are required"})
		return
	}
	h.logger.Debug("stop process request (path)",
		zap.String("session_id", sessionID),
		zap.String("process_id", processID),
	)

	session, err := h.service.GetTaskSession(c.Request.Context(), sessionID)
	if err != nil {
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}

	executorID := session.ExecutorID
	if executorID == "" {
		if task, err := h.service.GetTask(c.Request.Context(), session.TaskID); err == nil {
			if workspace, err := h.service.GetWorkspace(c.Request.Context(), task.WorkspaceID); err == nil {
				if workspace.DefaultExecutorID != nil {
					executorID = *workspace.DefaultExecutorID
				}
			}
		}
		if executorID == "" {
			executorID = models.ExecutorIDLocalPC
		}
	}
	executor, err := h.service.GetExecutor(c.Request.Context(), executorID)
	if err != nil {
		handleNotFound(c, h.logger, err, "executor not found")
		return
	}

	switch executor.Type {
	case models.ExecutorTypeLocalPC, models.ExecutorTypeLocalDocker:
		proc, err := h.lifecycleMgr.GetProcess(c.Request.Context(), processID, false)
		if err == nil && proc.SessionID != sessionID {
			c.Status(http.StatusNoContent)
			return
		}
		if err := h.lifecycleMgr.StopProcess(c.Request.Context(), processID); err != nil {
			h.logger.Warn("failed to stop process (path)",
				zap.String("session_id", sessionID),
				zap.String("process_id", processID),
				zap.Error(err),
			)
		}
		c.Status(http.StatusNoContent)
	default:
		c.Status(http.StatusNoContent)
	}
}

func (h *ProcessHandlers) httpListProcesses(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}
	session, err := h.service.GetTaskSession(c.Request.Context(), sessionID)
	if err != nil {
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}

	executorID := session.ExecutorID
	// TODO: fix session, should always have executorID
	if executorID == "" {
		if task, err := h.service.GetTask(c.Request.Context(), session.TaskID); err == nil {
			if workspace, err := h.service.GetWorkspace(c.Request.Context(), task.WorkspaceID); err == nil {
				if workspace.DefaultExecutorID != nil {
					executorID = *workspace.DefaultExecutorID
				}
			}
		}
		if executorID == "" {
			executorID = models.ExecutorIDLocalPC
		}
	}
	executor, err := h.service.GetExecutor(c.Request.Context(), executorID)
	if err != nil {
		handleNotFound(c, h.logger, err, "executor not found")
		return
	}
	switch executor.Type {
	case models.ExecutorTypeLocalPC, models.ExecutorTypeLocalDocker:
		listCtx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		procs, err := h.lifecycleMgr.ListProcesses(listCtx, sessionID)
		if err != nil {
			var netErr net.Error
			if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) || strings.Contains(err.Error(), "connection refused") {
				h.logger.Warn("process list unavailable", zap.String("session_id", sessionID), zap.Error(err))
				c.JSON(http.StatusOK, []agentctlclient.ProcessInfo{})
				return
			}
			h.logger.Error("failed to list processes", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, procs)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "executor type not supported for process runner"})
	}
}

func (h *ProcessHandlers) httpGetProcess(c *gin.Context) {
	sessionID := c.Param("id")
	processID := c.Param("processId")
	if sessionID == "" || processID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id and process_id are required"})
		return
	}
	session, err := h.service.GetTaskSession(c.Request.Context(), sessionID)
	if err != nil {
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}

	executorID := session.ExecutorID
	// TODO: fix session, should always have executorID
	if executorID == "" {
		if task, err := h.service.GetTask(c.Request.Context(), session.TaskID); err == nil {
			if workspace, err := h.service.GetWorkspace(c.Request.Context(), task.WorkspaceID); err == nil {
				if workspace.DefaultExecutorID != nil {
					executorID = *workspace.DefaultExecutorID
				}
			}
		}
		if executorID == "" {
			executorID = models.ExecutorIDLocalPC
		}
	}
	executor, err := h.service.GetExecutor(c.Request.Context(), executorID)
	if err != nil {
		handleNotFound(c, h.logger, err, "executor not found")
		return
	}
	includeOutput := c.Query("include_output") == "true"
	switch executor.Type {
	case models.ExecutorTypeLocalPC, models.ExecutorTypeLocalDocker:
		proc, err := h.lifecycleMgr.GetProcess(c.Request.Context(), processID, includeOutput)
		if err != nil {
			handleNotFound(c, h.logger, err, "process not found")
			return
		}
		if proc.SessionID != sessionID {
			handleNotFound(c, h.logger, fmt.Errorf("process not found"), "process not found")
			return
		}
		c.JSON(http.StatusOK, proc)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "executor type not supported for process runner"})
	}
}

func resolveScriptCommand(
	ctx context.Context,
	svc *service.Service,
	repo *models.Repository,
	kind string,
	scriptName string,
) (string, string, string, error) {
	switch strings.ToLower(kind) {
	case "setup":
		if strings.TrimSpace(repo.SetupScript) == "" {
			return "", "", "", fmt.Errorf("setup script not configured")
		}
		return repo.SetupScript, "setup", "", nil
	case "cleanup":
		if strings.TrimSpace(repo.CleanupScript) == "" {
			return "", "", "", fmt.Errorf("cleanup script not configured")
		}
		return repo.CleanupScript, "cleanup", "", nil
	case "dev":
		if strings.TrimSpace(repo.DevScript) == "" {
			return "", "", "", fmt.Errorf("dev script not configured")
		}
		return repo.DevScript, "dev", "", nil
	case "custom":
		if strings.TrimSpace(scriptName) == "" {
			return "", "", "", fmt.Errorf("script_name is required for custom scripts")
		}
		scripts, err := svc.ListRepositoryScripts(ctx, repo.ID)
		if err != nil {
			return "", "", "", err
		}
		for _, script := range scripts {
			if script.Name == scriptName {
				if strings.TrimSpace(script.Command) == "" {
					return "", "", "", fmt.Errorf("script command is empty")
				}
				return script.Command, "custom", script.Name, nil
			}
		}
		return "", "", "", fmt.Errorf("custom script not found")
	default:
		return "", "", "", fmt.Errorf("invalid script kind")
	}
}
