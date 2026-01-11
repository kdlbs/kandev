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
	"github.com/kandev/kandev/internal/agent/credentials"
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	agentwshandlers "github.com/kandev/kandev/internal/agent/wshandlers"

	// Orchestrator packages
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	orchestratorwshandlers "github.com/kandev/kandev/internal/orchestrator/wshandlers"

	// Task Service packages
	taskcontroller "github.com/kandev/kandev/internal/task/controller"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"

	// ACP
	"github.com/kandev/kandev/internal/orchestrator/acp"
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

	taskSvc := taskservice.NewService(taskRepo, eventBus, log)
	log.Info("Task Service initialized")

	// Create SQLite message store for ACP log persistence
	// Get the underlying *sql.DB from the SQLite repository
	sqliteRepo, ok := taskRepo.(*repository.SQLiteRepository)
	if !ok {
		log.Fatal("Expected SQLiteRepository for ACP message store")
	}
	acpMessageStore := acp.NewSQLiteMessageStore(sqliteRepo.DB())
	acpHandler := acp.NewHandler(acpMessageStore, log)
	log.Info("ACP message store initialized for persistent log storage")

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
	orchestratorSvc := orchestrator.NewService(serviceCfg, eventBus, nil, agentManagerClient, taskRepoAdapter, log)

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

	orchestratorWSHandlers := orchestratorwshandlers.NewHandlers(orchestratorSvc, log)
	orchestratorWSHandlers.SetACPHandler(acpHandler)
	orchestratorWSHandlers.RegisterHandlers(gateway.Dispatcher)
	log.Info("Registered Orchestrator WebSocket handlers")

	if lifecycleMgr != nil && agentRegistry != nil {
		agentWSHandlers := agentwshandlers.NewHandlers(lifecycleMgr, agentRegistry, log)
		agentWSHandlers.RegisterHandlers(gateway.Dispatcher)
		log.Info("Registered Agent Manager WebSocket handlers")
	}

	// Start the WebSocket hub
	go gateway.Hub.Run(ctx)
	gateways.RegisterTaskNotifications(ctx, eventBus, gateway.Hub, log)

	// Set up historical logs provider for task subscriptions
	gateway.Hub.SetHistoricalLogsProvider(func(ctx context.Context, taskID string) ([]*ws.Message, error) {
		messages, err := acpHandler.GetAllMessages(ctx, taskID)
		if err != nil {
			return nil, err
		}

		// Convert protocol.Message to ws.Message notifications
		result := make([]*ws.Message, 0, len(messages))
		for _, msg := range messages {
			action := "acp." + string(msg.Type)
			notification, err := ws.NewNotification(action, map[string]interface{}{
				"task_id":   taskID,
				"type":      msg.Type,
				"data":      msg.Data,
				"timestamp": msg.Timestamp,
			})
			if err != nil {
				continue
			}
			result = append(result, notification)
		}
		return result, nil
	})
	log.Info("Historical logs provider configured for task subscriptions")

	// Wire ACP handler to persist messages to SQLite database
	orchestratorSvc.RegisterACPHandler(func(taskID string, msg *protocol.Message) {
		if err := acpHandler.ProcessMessage(context.Background(), msg); err != nil {
			log.Error("failed to persist ACP message",
				zap.String("task_id", taskID),
				zap.String("message_type", string(msg.Type)),
				zap.Error(err))
		}
	})

	// Wire ACP handler to broadcast to WebSocket clients as notifications
	orchestratorSvc.RegisterACPHandler(func(taskID string, msg *protocol.Message) {
		// Convert ACP message to WebSocket notification
		action := "acp." + string(msg.Type)
		notification, _ := ws.NewNotification(action, map[string]interface{}{
			"task_id":   taskID,
			"type":      msg.Type,
			"data":      msg.Data,
			"timestamp": msg.Timestamp,
		})
		gateway.Hub.BroadcastToTask(taskID, notification)
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

		notification, _ := ws.NewNotification(ws.ActionCommentAdded, map[string]interface{}{
			"task_id":        taskID,
			"comment_id":     data["comment_id"],
			"author_type":    data["author_type"],
			"author_id":      data["author_id"],
			"content":        data["content"],
			"requests_input": data["requests_input"],
			"created_at":     data["created_at"],
		})
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
	launchReq := &lifecycle.LaunchRequest{
		TaskID:          req.TaskID,
		AgentType:       req.AgentType,
		WorkspacePath:   req.RepositoryURL, // Use repository URL as workspace path
		TaskDescription: req.TaskDescription,
		Metadata:        req.Metadata,
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

// corsMiddleware returns a CORS middleware for WebSocket connections
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
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
func (a *commentCreatorAdapter) CreateAgentComment(ctx context.Context, taskID, content string) error {
	_, err := a.svc.CreateComment(ctx, &taskservice.CreateCommentRequest{
		TaskID:     taskID,
		Content:    content,
		AuthorType: "agent",
	})
	return err
}
