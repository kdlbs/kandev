// Package main is the unified entry point for Kandev.
// This single binary runs all services together with shared infrastructure.
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
	"github.com/kandev/kandev/internal/events/bus"

	// Agent Manager packages
	agentapi "github.com/kandev/kandev/internal/agent/api"
	"github.com/kandev/kandev/internal/agent/credentials"
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"

	// Orchestrator packages
	"github.com/kandev/kandev/internal/orchestrator"
	orchestratorapi "github.com/kandev/kandev/internal/orchestrator/api"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	orchestratorstreaming "github.com/kandev/kandev/internal/orchestrator/streaming"

	// Task Service packages
	taskapi "github.com/kandev/kandev/internal/task/api"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"

	// ACP
	"github.com/kandev/kandev/pkg/acp/protocol"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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

	// WebSocket hub for ACP streaming
	wsHub := orchestratorstreaming.NewHub(log)
	go wsHub.Run(ctx)

	// Wire ACP handler to broadcast to WebSocket clients
	orchestratorSvc.RegisterACPHandler(func(taskID string, msg *protocol.Message) {
		wsHub.Broadcast(taskID, msg)
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

	if err := orchestratorSvc.Start(ctx); err != nil {
		log.Fatal("Failed to start orchestrator", zap.Error(err))
	}
	log.Info("Orchestrator initialized")

	// ============================================
	// HTTP SERVER
	// ============================================
	if cfg.Logging.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(orchestratorapi.RequestLogger(log))
	router.Use(orchestratorapi.CORS())

	// API v1 routes
	v1Group := router.Group("/api/v1")

	// Task Service routes
	taskapi.SetupRoutes(v1Group, taskSvc, log)

	// Orchestrator routes
	orchGroup := v1Group.Group("/orchestrator")
	orchestratorapi.SetupRoutes(orchGroup, orchestratorSvc, log)

	// WebSocket routes for orchestrator
	wsHandler := orchestratorstreaming.NewWSHandler(wsHub, orchestratorSvc, log)
	orchestratorstreaming.SetupWebSocketRoutes(orchGroup, wsHandler)

	// Agent Manager routes (if available)
	if lifecycleMgr != nil && agentRegistry != nil && dockerClient != nil {
		// Note: ACP is now handled via agentctl inside containers, so we pass nil for the ACP manager
		agentapi.SetupRoutes(v1Group, lifecycleMgr, agentRegistry, dockerClient, nil, log)
	}

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "kandev",
			"mode":    "unified",
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
		log.Info("HTTP server listening", zap.Int("port", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Failed to start HTTP server", zap.Error(err))
		}
	}()

	// Print routes summary
	log.Info("Routes configured",
		zap.String("tasks", "/api/v1/tasks, /api/v1/boards"),
		zap.String("orchestrator", "/api/v1/orchestrator"),
		zap.String("agents", "/api/v1/agents"),
		zap.String("health", "/health"),
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
func (a *lifecycleAdapter) PromptAgent(ctx context.Context, agentInstanceID string, prompt string) error {
	return a.mgr.PromptAgent(ctx, agentInstanceID, prompt)
}
