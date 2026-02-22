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
	"github.com/kandev/kandev/internal/common/portutil"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

// queryValueTrue is the string "true" used in query parameter comparisons.
const queryValueTrue = "true"

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

	repoID, repo, ok := h.resolveProcessRepository(c, session, body.RepositoryID, sessionID)
	if !ok {
		return
	}

	target, ok := h.resolveScriptTarget(c, session, body, repoID, repo, sessionID)
	if !ok {
		return
	}

	command, portEnv, ok := h.prepareCommandWithPorts(c, sessionID, target.command)
	if !ok {
		return
	}

	if !h.ensureAgentctlReady(c, session, sessionID) {
		return
	}

	workingDir, ok := h.resolveWorkingDirForStart(c, sessionID, target.workingDir)
	if !ok {
		return
	}

	h.startSessionProcess(c, sessionID, repoID, target.kind, target.scriptName, command, workingDir, portEnv)
}

// resolvedScript holds the resolved script command and metadata for a process start request.
type resolvedScript struct {
	command    string
	kind       string
	scriptName string
	workingDir string
}

// resolveScriptTarget resolves the script command and working directory for a process start request.
// Returns the resolved script and true on success; on failure writes the HTTP error response and returns false.
func (h *ProcessHandlers) resolveScriptTarget(
	c *gin.Context,
	session *models.TaskSession,
	body httpStartProcessRequest,
	repoID string,
	repo *models.Repository,
	sessionID string,
) (*resolvedScript, bool) {
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
		return nil, false
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
	return &resolvedScript{command: command, kind: kind, scriptName: scriptName, workingDir: workingDir}, true
}

// prepareCommandWithPorts transforms a command by allocating port placeholders.
// Returns the transformed command, the port environment map, and true on success.
// On failure it writes the HTTP error response itself and returns false.
func (h *ProcessHandlers) prepareCommandWithPorts(c *gin.Context, sessionID, command string) (string, map[string]string, bool) {
	transformed, portEnv, err := portutil.TransformCommand(command)
	if err != nil {
		h.logger.Error("failed to transform command with port placeholders",
			zap.String("session_id", sessionID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "port allocation failed"})
		return "", nil, false
	}
	if len(portEnv) > 0 {
		h.logger.Info("allocated ports for dev process",
			zap.String("session_id", sessionID),
			zap.Any("ports", portEnv))
	}
	return transformed, portEnv, true
}

// ensureAgentctlReady ensures the workspace execution exists and agentctl is responsive.
// Returns true when ready; on failure it writes the HTTP error response itself and returns false.
func (h *ProcessHandlers) ensureAgentctlReady(c *gin.Context, session *models.TaskSession, sessionID string) bool {
	if _, err := h.lifecycleMgr.EnsureWorkspaceExecutionForSession(
		c.Request.Context(),
		session.TaskID,
		session.ID,
	); err != nil {
		h.logger.Error("failed to ensure workspace execution for process", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	readyCtx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := h.lifecycleMgr.WaitForAgentctlReadyForSession(readyCtx, session.ID); err != nil {
		h.logger.Warn("agentctl not ready for process start",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agentctl not ready"})
		return false
	}
	return true
}

// resolveProcessRepository resolves the repository ID and fetches the repository for a
// start-process request. Returns the repo ID, repo model, and false (writing the HTTP
// error itself) when the repository cannot be determined.
func (h *ProcessHandlers) resolveProcessRepository(
	c *gin.Context,
	session *models.TaskSession,
	requestedRepoID, sessionID string,
) (string, *models.Repository, bool) {
	repoID := requestedRepoID
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
		return "", nil, false
	}
	repo, err := h.service.GetRepository(c.Request.Context(), repoID)
	if err != nil {
		h.logger.Warn("start process repository not found",
			zap.String("session_id", sessionID),
			zap.String("repo_id", repoID),
			zap.Error(err),
		)
		handleNotFound(c, h.logger, err, "repository not found")
		return "", nil, false
	}
	return repoID, repo, true
}

// getRunningDevProcess returns an already-running dev process for the session, or nil.
func (h *ProcessHandlers) getRunningDevProcess(ctx context.Context, sessionID string) *agentctlclient.ProcessInfo {
	existing, err := h.lifecycleMgr.ListProcesses(ctx, sessionID)
	if err != nil {
		h.logger.Warn("failed to list processes for dev check",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil
	}
	for i := range existing {
		proc := &existing[i]
		if proc.Kind != streams.ProcessKindDev {
			continue
		}
		if proc.Status == agentctltypes.ProcessStatusRunning || proc.Status == agentctltypes.ProcessStatusStarting {
			return proc
		}
	}
	return nil
}

// startLocalProcess handles starting a process on a local/worktree/docker executor.
// It deduplicates running dev processes and starts the new process via the lifecycle manager.
func (h *ProcessHandlers) startSessionProcess(
	c *gin.Context,
	sessionID, repoID, kind, scriptName, command, workingDir string,
	portEnv map[string]string,
) {
	if streams.ProcessKind(kind) == streams.ProcessKindDev {
		if proc := h.getRunningDevProcess(c.Request.Context(), sessionID); proc != nil {
			c.JSON(http.StatusOK, gin.H{"process": proc})
			return
		}
	}
	proc, err := h.lifecycleMgr.StartProcess(c.Request.Context(), lifecycle.StartProcessRequest{
		SessionID:  sessionID,
		Kind:       kind,
		ScriptName: scriptName,
		Command:    command,
		WorkingDir: workingDir,
		Env:        portEnv,
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
}

// resolveWorkingDirForStart picks the working directory for a process start.
// It prefers the active execution workspace path (works for local and remote runtimes),
// and falls back to legacy local repo/worktree path when execution data is unavailable.
func (h *ProcessHandlers) resolveWorkingDirForStart(c *gin.Context, sessionID, fallback string) (string, bool) {
	if execution, found := h.lifecycleMgr.GetExecutionBySessionID(sessionID); found {
		if workspace := strings.TrimSpace(execution.WorkspacePath); workspace != "" {
			return workspace, true
		}
	}
	if trimmed := strings.TrimSpace(fallback); trimmed != "" {
		return trimmed, true
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "workspace path unavailable for session"})
	return "", false
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

	// Idempotent stop for sessions with no active execution.
	if _, found := h.lifecycleMgr.GetExecutionBySessionID(sessionID); !found {
		c.Status(http.StatusNoContent)
		return
	}

	proc, err := h.lifecycleMgr.GetProcess(c.Request.Context(), processID, false)
	if err != nil {
		handleNotFound(c, h.logger, err, "process not found")
		return
	}
	if proc.SessionID != sessionID {
		handleNotFound(c, h.logger, fmt.Errorf("process not found"), "process not found")
		return
	}
	if err := h.lifecycleMgr.StopProcessForSession(c.Request.Context(), sessionID, processID); err != nil {
		h.logger.Warn("failed to stop process (path)",
			zap.String("session_id", sessionID),
			zap.String("process_id", processID),
			zap.Error(err))
	}
	c.Status(http.StatusNoContent)
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

	listCtx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	procs, err := h.lifecycleMgr.ListProcesses(listCtx, session.ID)
	if err != nil {
		var netErr net.Error
		// Handle expected "no processes" conditions gracefully:
		// - no execution found: agent hasn't started yet (async launch)
		// - connection refused: agent not running
		// - deadline exceeded: agent not responding
		if errors.Is(err, context.DeadlineExceeded) ||
			errors.As(err, &netErr) ||
			strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "no execution found") {
			h.logger.Debug("process list unavailable (agent not ready)", zap.String("session_id", sessionID), zap.Error(err))
			c.JSON(http.StatusOK, []agentctlclient.ProcessInfo{})
			return
		}
		h.logger.Error("failed to list processes", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, procs)
}

func (h *ProcessHandlers) httpGetProcess(c *gin.Context) {
	sessionID := c.Param("id")
	processID := c.Param("processId")
	if sessionID == "" || processID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id and process_id are required"})
		return
	}
	if _, err := h.service.GetTaskSession(c.Request.Context(), sessionID); err != nil {
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}
	includeOutput := c.Query("include_output") == queryValueTrue
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
