// Package main is the unified entry point for Kandev.
// This single binary runs all services together with shared infrastructure.
// The server exposes WebSocket and HTTP endpoints.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	// Common packages
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"

	// Event bus
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"

	// WebSocket gateway
	gateways "github.com/kandev/kandev/internal/gateway/websocket"

	// Agent Manager packages
	agentcontroller "github.com/kandev/kandev/internal/agent/controller"
	"github.com/kandev/kandev/internal/agent/credentials"
	"github.com/kandev/kandev/internal/agent/docker"
	agenthandlers "github.com/kandev/kandev/internal/agent/handlers"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/worktree"

	// Orchestrator packages
	"github.com/kandev/kandev/internal/orchestrator"
	orchestratorcontroller "github.com/kandev/kandev/internal/orchestrator/controller"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	orchestratorhandlers "github.com/kandev/kandev/internal/orchestrator/handlers"

	// Task Service packages
	taskcontroller "github.com/kandev/kandev/internal/task/controller"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"

	// ACP protocol
	"github.com/kandev/kandev/pkg/acp/protocol"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func main() {
	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize logger
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()
	logger.SetDefault(log)

	log.Info("Starting Kandev (unified mode)...")

	// 3. Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 4. Initialize event bus (in-memory for unified mode, or NATS if configured)
	var eventBus bus.EventBus
	if cfg.NATS.URL != "" {
		log.Info("Connecting to NATS...", zap.String("url", cfg.NATS.URL))
		natsEventBus, err := bus.NewNATSEventBus(cfg.NATS, log)
		if err != nil {
			log.Fatal("Failed to connect to NATS", zap.Error(err))
		}
		eventBus = natsEventBus
		defer natsEventBus.Close()
		log.Info("Connected to NATS event bus")
	} else {
		log.Info("Using in-memory event bus")
		eventBus = bus.NewMemoryEventBus(log)
	}

	// 5. Initialize Docker client
	dockerClient, err := docker.NewClient(cfg.Docker, log)
	if err != nil {
		log.Warn("Failed to initialize Docker client - agent features will be disabled", zap.Error(err))
		dockerClient = nil
	} else {
		defer dockerClient.Close()
		if err := dockerClient.Ping(ctx); err != nil {
			log.Warn("Docker daemon not available - agent features will be disabled", zap.Error(err))
			dockerClient = nil
		} else {
			log.Info("Connected to Docker daemon")
		}
	}

	// ============================================
	// TASK SERVICE
	// ============================================
	log.Info("Initializing Task Service...")

	// Get database path from environment or use default
	dbPath := os.Getenv("KANDEV_DB_PATH")
	if dbPath == "" {
		dbPath = "./kandev.db"
	}

	// Initialize SQLite repository
	var taskRepo repository.Repository
	taskRepo, err = repository.NewSQLiteRepository(dbPath)
	if err != nil {
		log.Fatal("Failed to initialize SQLite database", zap.Error(err), zap.String("db_path", dbPath))
	}
	defer taskRepo.Close()
	log.Info("SQLite database initialized", zap.String("db_path", dbPath))

	taskSvc := taskservice.NewService(
		taskRepo,
		eventBus,
		log,
		taskservice.RepositoryDiscoveryConfig{
			Roots:    cfg.RepositoryDiscovery.Roots,
			MaxDepth: cfg.RepositoryDiscovery.MaxDepth,
		},
	)
	log.Info("Task Service initialized")

	// ACP messages are now stored as comments in the task_comments table
	// The comment system provides unified storage for all task-related communication
	log.Info("ACP messages will be stored as comments")

	// ============================================
	// AGENT MANAGER
	// ============================================
	var lifecycleMgr *lifecycle.Manager
	var agentRegistry *registry.Registry

	if dockerClient != nil {
		log.Info("Initializing Agent Manager...")

		// Agent Registry
		agentRegistry = registry.NewRegistry(log)
		agentRegistry.LoadDefaults()

		// Credentials Manager
		credsMgr := credentials.NewManager(log)
		credsMgr.AddProvider(credentials.NewEnvProvider("KANDEV_"))
		credsMgr.AddProvider(credentials.NewAugmentSessionProvider()) // Read ~/.augment/session.json
		if credsFile := os.Getenv("KANDEV_CREDENTIALS_FILE"); credsFile != "" {
			credsMgr.AddProvider(credentials.NewFileProvider(credsFile))
		}

		// Lifecycle Manager (uses agentctl for agent communication)
		lifecycleMgr = lifecycle.NewManager(dockerClient, agentRegistry, eventBus, log)
		lifecycleMgr.SetCredentialsManager(credsMgr)

		if err := lifecycleMgr.Start(ctx); err != nil {
			log.Fatal("Failed to start lifecycle manager", zap.Error(err))
		}

		log.Info("Agent Manager initialized", zap.Int("agent_types", len(agentRegistry.List())))
	} else {
		log.Info("Agent Manager disabled (no Docker)")
	}

	// ============================================
	// WORKTREE MANAGER
	// ============================================
	log.Info("Initializing Worktree Manager...")

	// Create worktree store (uses same database as tasks)
	// Type assert to get the *sql.DB from the SQLite repository
	var worktreeMgr *worktree.Manager
	if sqliteRepo, ok := taskRepo.(*repository.SQLiteRepository); ok {
		worktreeStore, err := worktree.NewSQLiteStore(sqliteRepo.DB())
		if err != nil {
			log.Fatal("Failed to initialize worktree store", zap.Error(err))
		}

		// Create worktree manager with config from common config
		worktreeMgr, err = worktree.NewManager(worktree.Config{
			Enabled:      cfg.Worktree.Enabled,
			BasePath:     cfg.Worktree.BasePath,
			MaxPerRepo:   cfg.Worktree.MaxPerRepo,
			BranchPrefix: "kandev/", // Default branch prefix
		}, worktreeStore, log)
		if err != nil {
			log.Fatal("Failed to initialize worktree manager", zap.Error(err))
		}
		log.Info("Worktree Manager initialized",
			zap.Bool("enabled", cfg.Worktree.Enabled),
			zap.String("base_path", cfg.Worktree.BasePath))

		// Wire worktree manager to lifecycle manager for agent isolation
		if lifecycleMgr != nil {
			lifecycleMgr.SetWorktreeManager(worktreeMgr)
			log.Info("Worktree Manager connected to Lifecycle Manager")
		}
	} else {
		log.Warn("Worktree Manager disabled (requires SQLite repository)")
	}

	// ============================================
	// ORCHESTRATOR
	// ============================================
	log.Info("Initializing Orchestrator...")

	// Create an adapter for the task repository to work with orchestrator
	taskRepoAdapter := &taskRepositoryAdapter{repo: taskRepo, svc: taskSvc}

	// Create agent manager client (adapter or mock)
	var agentManagerClient executor.AgentManagerClient
	if lifecycleMgr != nil {
		agentManagerClient = newLifecycleAdapter(lifecycleMgr, agentRegistry, log)
	} else {
		agentManagerClient = executor.NewMockAgentManagerClient(log)
	}

	serviceCfg := orchestrator.DefaultServiceConfig()
	orchestratorSvc := orchestrator.NewService(serviceCfg, eventBus, nil, agentManagerClient, taskRepoAdapter, taskRepo, log)

	// Set up comment creator for saving agent responses as comments
	orchestratorSvc.SetCommentCreator(&commentCreatorAdapter{svc: taskSvc})

	// ============================================
	// WEBSOCKET GATEWAY (All communication via WebSocket)
	// ============================================
	log.Info("Initializing WebSocket Gateway...")

	// Create the unified WebSocket gateway
	gateway := gateways.NewGateway(log)

	// Prepare Task Service controllers for HTTP + WebSocket handlers
	workspaceController := taskcontroller.NewWorkspaceController(taskSvc)
	boardController := taskcontroller.NewBoardController(taskSvc)
	taskController := taskcontroller.NewTaskController(taskSvc)
	if worktreeMgr != nil {
		taskController.SetWorktreeLookup(worktreeMgr) // Enable worktree info in task responses
	}
	repositoryController := taskcontroller.NewRepositoryController(taskSvc)

	// Create orchestrator controller and handlers (Pattern A)
	orchestratorCtrl := orchestratorcontroller.NewController(orchestratorSvc)
	orchestratorHandlers := orchestratorhandlers.NewHandlers(orchestratorCtrl, log)
	orchestratorHandlers.RegisterHandlers(gateway.Dispatcher)
	log.Info("Registered Orchestrator WebSocket handlers")

	// Create agent controller and handlers (Pattern A)
	if lifecycleMgr != nil && agentRegistry != nil {
		agentCtrl := agentcontroller.NewController(lifecycleMgr, agentRegistry)
		agentHandlers := agenthandlers.NewHandlers(agentCtrl, log)
		agentHandlers.RegisterHandlers(gateway.Dispatcher)
		log.Info("Registered Agent Manager WebSocket handlers")
	}

	// Start the WebSocket hub
	go gateway.Hub.Run(ctx)
	gateways.RegisterTaskNotifications(ctx, eventBus, gateway.Hub, log)

	// Set up historical logs provider for task subscriptions
	// Uses comments instead of execution logs - all agent messages are now comments
	gateway.Hub.SetHistoricalLogsProvider(func(ctx context.Context, taskID string) ([]*ws.Message, error) {
		comments, err := taskSvc.ListComments(ctx, taskID)
		if err != nil {
			return nil, err
		}

		// Convert comments to ws.Message notifications
		result := make([]*ws.Message, 0, len(comments))
		for _, comment := range comments {
			// Determine the action based on comment type
			action := ws.ActionCommentAdded
			payload := map[string]interface{}{
				"comment_id":     comment.ID,
				"task_id":        comment.TaskID,
				"author_type":    string(comment.AuthorType),
				"author_id":      comment.AuthorID,
				"content":        comment.Content,
				"type":           string(comment.Type),
				"requests_input": comment.RequestsInput,
				"created_at":     comment.CreatedAt.Format(time.RFC3339),
			}
			if comment.ACPSessionID != "" {
				payload["acp_session_id"] = comment.ACPSessionID
			}
			if comment.Metadata != nil {
				payload["metadata"] = comment.Metadata
			}
			notification, err := ws.NewNotification(action, payload)
			if err != nil {
				continue
			}
			result = append(result, notification)
		}
		return result, nil
	})
	log.Info("Historical logs provider configured for task subscriptions (using comments)")

	// NOTE: We no longer create comments for each ACP message chunk.
	// Instead, the lifecycle manager accumulates message chunks and:
	// 1. Flushes the buffer as a comment when a tool use starts (step boundary)
	// 2. Flushes the buffer as a comment when the prompt completes (final response)
	// This is handled via the prompt.complete and step.complete events.

	// Wire ACP handler to broadcast to WebSocket clients as notifications
	orchestratorSvc.RegisterACPHandler(func(taskID string, msg *protocol.Message) {
		// Only broadcast message types that the frontend handles
		// Skip internal message types like session/update
		switch msg.Type {
		case protocol.MessageTypeProgress,
			protocol.MessageTypeLog,
			protocol.MessageTypeResult,
			protocol.MessageTypeError,
			protocol.MessageTypeStatus,
			protocol.MessageTypeHeartbeat,
			protocol.MessageTypeInputRequired:
			// Convert ACP message to WebSocket notification
			action := "acp." + string(msg.Type)
			notification, _ := ws.NewNotification(action, map[string]interface{}{
				"task_id":   taskID,
				"type":      msg.Type,
				"data":      msg.Data,
				"timestamp": msg.Timestamp,
			})
			gateway.Hub.BroadcastToTask(taskID, notification)
		default:
			// Skip other message types (session/update, session_info, etc.)
		}
	})

	// Wire ACP handler to extract session_id from session_info messages and store in task metadata
	orchestratorSvc.RegisterACPHandler(func(taskID string, msg *protocol.Message) {
		log.Debug("ACP handler received message",
			zap.String("task_id", taskID),
			zap.String("message_type", string(msg.Type)))

		// Check for session_info message type (custom type from augment-agent)
		if msg.Type == "session_info" && msg.Data != nil {
			log.Debug("session_info message received, extracting session_id")
			if sessionID, ok := msg.Data["session_id"].(string); ok && sessionID != "" {
				metadata := map[string]interface{}{
					"auggie_session_id": sessionID,
				}
				if _, err := taskSvc.UpdateTaskMetadata(context.Background(), taskID, metadata); err != nil {
					log.Error("failed to store auggie session_id in task metadata",
						zap.String("task_id", taskID),
						zap.String("session_id", sessionID),
						zap.Error(err))
				} else {
					log.Info("stored auggie session_id in task metadata",
						zap.String("task_id", taskID),
						zap.String("session_id", sessionID))
				}
			}
		}
	})

	// Wire input request handler to create agent comments when input is requested
	orchestratorSvc.SetInputRequestHandler(func(ctx context.Context, taskID, agentID, message string) error {
		log.Info("agent requesting user input, creating comment",
			zap.String("task_id", taskID),
			zap.String("agent_id", agentID))

		// Create a comment from the agent
		comment, err := taskSvc.CreateComment(ctx, &taskservice.CreateCommentRequest{
			TaskID:        taskID,
			Content:       message,
			AuthorType:    "agent",
			AuthorID:      agentID,
			RequestsInput: true,
		})
		if err != nil {
			log.Error("failed to create agent comment",
				zap.String("task_id", taskID),
				zap.Error(err))
			return err
		}

		// Broadcast comment.added notification to task subscribers
		notification, _ := ws.NewNotification(ws.ActionCommentAdded, map[string]interface{}{
			"task_id":         taskID,
			"comment":         comment.ToAPI(),
			"requests_input":  true,
		})
		gateway.Hub.BroadcastToTask(taskID, notification)

		// Also broadcast input.requested notification
		inputNotification, _ := ws.NewNotification(ws.ActionInputRequested, map[string]interface{}{
			"task_id":    taskID,
			"comment_id": comment.ID,
			"message":    message,
		})
		gateway.Hub.BroadcastToTask(taskID, inputNotification)

		return nil
	})

	if err := orchestratorSvc.Start(ctx); err != nil {
		log.Fatal("Failed to start orchestrator", zap.Error(err))
	}
	log.Info("Orchestrator initialized")

	// Subscribe to comment.added events and broadcast to WebSocket subscribers
	_, err = eventBus.Subscribe(events.CommentAdded, func(ctx context.Context, event *bus.Event) error {
		data := event.Data
		taskID, _ := data["task_id"].(string)
		if taskID == "" {
			return nil
		}

		payload := map[string]interface{}{
			"task_id":        taskID,
			"comment_id":     data["comment_id"],
			"author_type":    data["author_type"],
			"author_id":      data["author_id"],
			"content":        data["content"],
			"type":           data["type"],
			"requests_input": data["requests_input"],
			"created_at":     data["created_at"],
		}
		if metadata, ok := data["metadata"]; ok && metadata != nil {
			payload["metadata"] = metadata
		}
		notification, _ := ws.NewNotification(ws.ActionCommentAdded, payload)
		gateway.Hub.BroadcastToTask(taskID, notification)
		return nil
	})
	if err != nil {
		log.Error("Failed to subscribe to comment.added events", zap.Error(err))
	} else {
		log.Info("Subscribed to comment.added events for WebSocket broadcasting")
	}

	// ============================================
	// HTTP SERVER (WebSocket + HTTP endpoints)
	// ============================================
	if cfg.Logging.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	// WebSocket endpoint - primary realtime transport
	gateway.SetupRoutes(router)

	// Task Service handlers (HTTP + WebSocket)
	taskhandlers.RegisterWorkspaceRoutes(router, gateway.Dispatcher, workspaceController, log)
	taskhandlers.RegisterBoardRoutes(router, gateway.Dispatcher, boardController, log)
	taskhandlers.RegisterTaskRoutes(router, gateway.Dispatcher, taskController, log)
	taskhandlers.RegisterRepositoryRoutes(router, gateway.Dispatcher, repositoryController, log)
	taskhandlers.RegisterCommentRoutes(router, gateway.Dispatcher, taskSvc, &orchestratorAdapter{svc: orchestratorSvc}, log)
	log.Info("Registered Task Service handlers (HTTP + WebSocket)")

	// Health check (simple HTTP for load balancers/monitoring)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "kandev",
			"mode":    "websocket+http",
		})
	})

	// Create HTTP server
	port := cfg.Server.Port
	if port == 0 {
		port = 8080
	}
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeoutDuration(),
		WriteTimeout: cfg.Server.WriteTimeoutDuration(),
	}

	// Start server
	go func() {
		log.Info("WebSocket server listening", zap.Int("port", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Print routes summary
	log.Info("API configured",
		zap.String("websocket", "/ws"),
		zap.String("health", "/health"),
		zap.String("http", "/api/v1"),
	)

	// ============================================
	// GRACEFUL SHUTDOWN
	// ============================================
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down Kandev...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("HTTP server shutdown error", zap.Error(err))
	}

	if err := orchestratorSvc.Stop(); err != nil {
		log.Error("Orchestrator stop error", zap.Error(err))
	}

	if lifecycleMgr != nil {
		if err := lifecycleMgr.Stop(); err != nil {
			log.Error("Lifecycle manager stop error", zap.Error(err))
		}
	}

	log.Info("Kandev stopped")
}

// taskRepositoryAdapter adapts the task repository for the orchestrator's scheduler
type taskRepositoryAdapter struct {
	repo repository.Repository
	svc  *taskservice.Service
}

// GetTask retrieves a task by ID and converts it to API type
func (a *taskRepositoryAdapter) GetTask(ctx context.Context, taskID string) (*v1.Task, error) {
	task, err := a.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return task.ToAPI(), nil
}

// UpdateTaskState updates task state via the service
func (a *taskRepositoryAdapter) UpdateTaskState(ctx context.Context, taskID string, state v1.TaskState) error {
	_, err := a.svc.UpdateTaskState(ctx, taskID, state)
	return err
}

// lifecycleAdapter adapts the lifecycle manager as an AgentManagerClient
type lifecycleAdapter struct {
	mgr      *lifecycle.Manager
	registry *registry.Registry
	logger   *logger.Logger
}

// newLifecycleAdapter creates a new lifecycle adapter
func newLifecycleAdapter(mgr *lifecycle.Manager, reg *registry.Registry, log *logger.Logger) *lifecycleAdapter {
	return &lifecycleAdapter{
		mgr:      mgr,
		registry: reg,
		logger:   log.WithFields(zap.String("component", "lifecycle_adapter")),
	}
}

// LaunchAgent starts a new agent container for a task
func (a *lifecycleAdapter) LaunchAgent(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	// The RepositoryURL field contains a local filesystem path for the workspace
	// If empty, the agent will run without a mounted workspace
	launchReq := &lifecycle.LaunchRequest{
		TaskID:          req.TaskID,
		AgentType:       req.AgentType,
		WorkspacePath:   req.RepositoryURL, // May be empty - lifecycle manager handles this
		TaskDescription: req.TaskDescription,
		Metadata:        req.Metadata,
		// Worktree configuration for concurrent agent execution
		UseWorktree:    req.UseWorktree,
		RepositoryID:   req.RepositoryID,
		RepositoryPath: req.RepositoryPath,
		BaseBranch:     req.BaseBranch,
	}

	instance, err := a.mgr.Launch(ctx, launchReq)
	if err != nil {
		return nil, err
	}

	// Streaming is now handled by the lifecycle manager

	return &executor.LaunchAgentResponse{
		AgentInstanceID: instance.ID,
		ContainerID:     instance.ContainerID,
		Status:          instance.Status,
	}, nil
}

// StopAgent stops a running agent
func (a *lifecycleAdapter) StopAgent(ctx context.Context, agentInstanceID string, force bool) error {
	return a.mgr.StopAgent(ctx, agentInstanceID, force)
}

// GetAgentStatus returns the status of an agent instance
func (a *lifecycleAdapter) GetAgentStatus(ctx context.Context, agentInstanceID string) (*v1.AgentInstance, error) {
	instance, found := a.mgr.GetInstance(agentInstanceID)
	if !found {
		return nil, fmt.Errorf("agent instance %q not found", agentInstanceID)
	}

	containerID := instance.ContainerID
	now := time.Now()
	result := &v1.AgentInstance{
		ID:          instance.ID,
		TaskID:      instance.TaskID,
		AgentType:   instance.AgentType,
		ContainerID: &containerID,
		Status:      instance.Status,
		StartedAt:   &instance.StartedAt,
		StoppedAt:   instance.FinishedAt,
		CreatedAt:   instance.StartedAt,
		UpdatedAt:   now,
	}

	if instance.ExitCode != nil {
		result.ExitCode = instance.ExitCode
	}
	if instance.ErrorMessage != "" {
		result.ErrorMessage = &instance.ErrorMessage
	}

	return result, nil
}

// ListAgentTypes returns available agent types
func (a *lifecycleAdapter) ListAgentTypes(ctx context.Context) ([]*v1.AgentType, error) {
	configs := a.registry.List()
	result := make([]*v1.AgentType, 0, len(configs))
	for _, cfg := range configs {
		result = append(result, cfg.ToAPIType())
	}
	return result, nil
}

// PromptAgent sends a follow-up prompt to a running agent
func (a *lifecycleAdapter) PromptAgent(ctx context.Context, agentInstanceID string, prompt string) (*executor.PromptResult, error) {
	result, err := a.mgr.PromptAgent(ctx, agentInstanceID, prompt)
	if err != nil {
		return nil, err
	}
	return &executor.PromptResult{
		StopReason:   result.StopReason,
		AgentMessage: result.AgentMessage,
	}, nil
}

// RespondToPermissionByTaskID sends a response to a permission request for a task
func (a *lifecycleAdapter) RespondToPermissionByTaskID(ctx context.Context, taskID, pendingID, optionID string, cancelled bool) error {
	return a.mgr.RespondToPermissionByTaskID(taskID, pendingID, optionID, cancelled)
}

// GetRecoveredInstances returns instances recovered from Docker during startup
func (a *lifecycleAdapter) GetRecoveredInstances() []executor.RecoveredInstanceInfo {
	recovered := a.mgr.GetRecoveredInstances()
	result := make([]executor.RecoveredInstanceInfo, len(recovered))
	for i, r := range recovered {
		result[i] = executor.RecoveredInstanceInfo{
			InstanceID:  r.InstanceID,
			TaskID:      r.TaskID,
			ContainerID: r.ContainerID,
			AgentType:   r.AgentType,
		}
	}
	return result
}

// corsMiddleware returns a CORS middleware for HTTP and WebSocket connections
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Upgrade, Connection, Sec-WebSocket-Key, Sec-WebSocket-Version, Sec-WebSocket-Protocol")
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// orchestratorAdapter adapts the orchestrator.Service to the taskhandlers.OrchestratorService interface
type orchestratorAdapter struct {
	svc *orchestrator.Service
}

// PromptTask forwards to the orchestrator service and converts the result type
func (a *orchestratorAdapter) PromptTask(ctx context.Context, taskID string, prompt string) (*taskhandlers.PromptResult, error) {
	result, err := a.svc.PromptTask(ctx, taskID, prompt)
	if err != nil {
		return nil, err
	}
	return &taskhandlers.PromptResult{
		StopReason:   result.StopReason,
		AgentMessage: result.AgentMessage,
	}, nil
}

// commentCreatorAdapter adapts the task service to the orchestrator.CommentCreator interface
type commentCreatorAdapter struct {
	svc *taskservice.Service
}

// CreateAgentComment creates a comment with author_type="agent"
func (a *commentCreatorAdapter) CreateAgentComment(ctx context.Context, taskID, content, agentSessionID string) error {
	_, err := a.svc.CreateComment(ctx, &taskservice.CreateCommentRequest{
		TaskID:         taskID,
		Content:        content,
		AuthorType:     "agent",
		AgentSessionID: agentSessionID,
	})
	return err
}

// CreateToolCallComment creates a comment for a tool call with type="tool_call"
func (a *commentCreatorAdapter) CreateToolCallComment(ctx context.Context, taskID, toolCallID, title, status, agentSessionID string, args map[string]interface{}) error {
	metadata := map[string]interface{}{
		"tool_call_id": toolCallID,
		"title":        title,
		"status":       status,
	}

	// Add args if provided (contains kind, path, locations, raw_input)
	if args != nil && len(args) > 0 {
		metadata["args"] = args

		// Extract kind as tool_name for icon selection in the frontend
		if kind, ok := args["kind"].(string); ok && kind != "" {
			metadata["tool_name"] = kind
		}
	}

	_, err := a.svc.CreateComment(ctx, &taskservice.CreateCommentRequest{
		TaskID:         taskID,
		Content:        title,
		AuthorType:     "agent",
		AgentSessionID: agentSessionID,
		Type:           "tool_call",
		Metadata:       metadata,
	})
	return err
}

// mapACPTypeToCommentType maps ACP message types to comment types
func mapACPTypeToCommentType(msgType protocol.MessageType) string {
	switch msgType {
	case protocol.MessageTypeProgress:
		return "progress"
	case protocol.MessageTypeLog:
		return "content" // Log messages are treated as content
	case protocol.MessageTypeResult:
		return "status"
	case protocol.MessageTypeError:
		return "error"
	case protocol.MessageTypeStatus:
		return "status"
	case protocol.MessageTypeInputRequired:
		return "content"
	case protocol.MessageTypeSessionInfo:
		return "status"
	default:
		return "content"
	}
}

// extractACPContent extracts a human-readable message from ACP data
func extractACPContent(msg *protocol.Message) string {
	if msg.Data == nil {
		return ""
	}

	// Check for nested content.text structure (used by agent_message_chunk)
	if content, ok := msg.Data["content"].(map[string]interface{}); ok {
		if text, ok := content["text"].(string); ok && text != "" {
			return text
		}
	}

	// Try common message fields
	if message, ok := msg.Data["message"].(string); ok && message != "" {
		return message
	}
	if text, ok := msg.Data["text"].(string); ok && text != "" {
		return text
	}
	if errMsg, ok := msg.Data["error"].(string); ok && errMsg != "" {
		return errMsg
	}
	if summary, ok := msg.Data["summary"].(string); ok && summary != "" {
		return summary
	}
	if prompt, ok := msg.Data["prompt"].(string); ok && prompt != "" {
		return prompt
	}

	// For progress updates, generate a message
	if msg.Type == protocol.MessageTypeProgress {
		if progress, ok := msg.Data["progress"].(float64); ok {
			return fmt.Sprintf("Progress: %d%%", int(progress))
		}
	}

	// For status updates
	if msg.Type == protocol.MessageTypeStatus {
		if status, ok := msg.Data["status"].(string); ok && status != "" {
			return fmt.Sprintf("Status: %s", status)
		}
	}

	return ""
}
