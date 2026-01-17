// Package main is the unified entry point for Kandev.
// This single binary runs all services together with shared infrastructure.
// The server exposes WebSocket and HTTP endpoints.
package main

import (
	"context"
	"flag"
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
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/client/launcher"
	agentcontroller "github.com/kandev/kandev/internal/agent/controller"
	"github.com/kandev/kandev/internal/agent/credentials"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/docker"
	agenthandlers "github.com/kandev/kandev/internal/agent/handlers"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	agentsettingshandlers "github.com/kandev/kandev/internal/agent/settings/handlers"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agent/worktree"

	// Orchestrator packages
	"github.com/kandev/kandev/internal/orchestrator"
	orchestratorcontroller "github.com/kandev/kandev/internal/orchestrator/controller"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	orchestratorhandlers "github.com/kandev/kandev/internal/orchestrator/handlers"

	// Task Service packages
	taskcontroller "github.com/kandev/kandev/internal/task/controller"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	usercontroller "github.com/kandev/kandev/internal/user/controller"
	userhandlers "github.com/kandev/kandev/internal/user/handlers"
	userservice "github.com/kandev/kandev/internal/user/service"
	userstore "github.com/kandev/kandev/internal/user/store"

	// ACP protocol
	"github.com/kandev/kandev/pkg/acp/protocol"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// Command-line flags
var (
	flagAgentRuntime = flag.String("agent-runtime", "", "Agent runtime mode: 'docker' or 'standalone'")
	flagPort         = flag.Int("port", 0, "HTTP server port (default: 8080)")
	flagLogLevel     = flag.String("log-level", "", "Log level: debug, info, warn, error")
	flagHelp         = flag.Bool("help", false, "Show help message")
	flagVersion      = flag.Bool("version", false, "Show version information")
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: kandev [options]\n\n")
		fmt.Fprintf(os.Stderr, "Kandev is an AI-powered development task orchestrator.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  kandev                              # Start with Docker runtime (default)\n")
		fmt.Fprintf(os.Stderr, "  kandev -agent-runtime=standalone    # Start with standalone agentctl\n")
		fmt.Fprintf(os.Stderr, "  kandev -port=9000 -log-level=debug  # Custom port and log level\n")
	}
}

func main() {
	flag.Parse()

	if *flagHelp {
		flag.Usage()
		os.Exit(0)
	}

	if *flagVersion {
		fmt.Println("kandev version 0.1.0")
		os.Exit(0)
	}

	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Apply command-line flag overrides (flags take precedence over config/env)
	if *flagAgentRuntime != "" {
		cfg.Agent.Runtime = *flagAgentRuntime
	}
	if *flagPort > 0 {
		cfg.Server.Port = *flagPort
	}
	if *flagLogLevel != "" {
		cfg.Logging.Level = *flagLogLevel
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
	defer func() {
		_ = log.Sync()
	}()
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
		if err := dockerClient.Ping(ctx); err != nil {
			log.Warn("Docker daemon not available - agent features will be disabled", zap.Error(err))
			_ = dockerClient.Close()
			dockerClient = nil
		} else {
			defer func() {
				_ = dockerClient.Close()
			}()
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
	defer func() {
		_ = taskRepo.Close()
	}()
	log.Info("SQLite database initialized", zap.String("db_path", dbPath))

	agentSettingsRepo, err := settingsstore.NewSQLiteRepository(dbPath)
	if err != nil {
		log.Fatal("Failed to initialize agent settings store", zap.Error(err), zap.String("db_path", dbPath))
	}
	defer func() {
		_ = agentSettingsRepo.Close()
	}()

	userRepo, err := userstore.NewSQLiteRepository(dbPath)
	if err != nil {
		log.Fatal("Failed to initialize user store", zap.Error(err), zap.String("db_path", dbPath))
	}
	defer func() {
		_ = userRepo.Close()
	}()

	discoveryRegistry, err := discovery.LoadRegistry()
	if err != nil {
		log.Fatal("Failed to load agent discovery config", zap.Error(err))
	}
	agentSettingsController := agentsettingscontroller.NewController(agentSettingsRepo, discoveryRegistry, taskRepo, log)

	userSvc := userservice.NewService(userRepo, eventBus, log)
	userCtrl := usercontroller.NewController(userSvc)

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

	if err := runInitialAgentSetup(ctx, userSvc, agentSettingsController, log); err != nil {
		log.Warn("Failed to run initial agent setup", zap.Error(err))
	}

	// ACP messages are now stored as comments in the task_comments table
	// The comment system provides unified storage for all task-related communication
	log.Info("ACP messages will be stored as comments")

	// ============================================
	// AGENTCTL LAUNCHER (for standalone mode)
	// ============================================
	var agentctlLauncher *launcher.Launcher

	if cfg.Agent.Runtime == "standalone" {
		log.Info("Starting agentctl for standalone mode...")

		agentctlLauncher = launcher.New(launcher.Config{
			Host:                   cfg.Agent.StandaloneHost,
			Port:                   cfg.Agent.StandalonePort,
			AutoApprovePermissions: false, // Let agents request approval from the user
		}, log)

		if err := agentctlLauncher.Start(ctx); err != nil {
			log.Fatal("Failed to start agentctl subprocess", zap.Error(err))
		}

		log.Info("agentctl started successfully",
			zap.Int("port", cfg.Agent.StandalonePort))

		// Ensure agentctl is stopped on panic (in addition to Pdeathsig for crashes)
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered, stopping agentctl", zap.Any("panic", r))
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer stopCancel()
				if err := agentctlLauncher.Stop(stopCtx); err != nil {
					log.Error("failed to stop agentctl on panic", zap.Error(err))
				}
				panic(r) // Re-panic after cleanup
			}
		}()
	}

	// ============================================
	// AGENT MANAGER
	// ============================================
	var lifecycleMgr *lifecycle.Manager
	var agentRegistry *registry.Registry

	// Agent manager is enabled if:
	// - Docker mode and Docker is available, OR
	// - Standalone mode (agentctl was started above)
	agentManagerEnabled := (cfg.Agent.Runtime == "docker" && dockerClient != nil) ||
		cfg.Agent.Runtime == "standalone"

	if agentManagerEnabled {
		log.Info("Initializing Agent Manager...",
			zap.String("runtime", cfg.Agent.Runtime))

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

		// Profile Resolver
		profileResolver := lifecycle.NewStoreProfileResolver(agentSettingsRepo)

		// Create the appropriate runtime based on configuration
		var agentRuntime lifecycle.Runtime
		var containerMgr *lifecycle.ContainerManager
		switch cfg.Agent.Runtime {
		case "standalone":
			standaloneCtl := agentctl.NewStandaloneCtl(
				cfg.Agent.StandaloneHost,
				cfg.Agent.StandalonePort,
				log,
			)
			agentRuntime = lifecycle.NewStandaloneRuntime(
				standaloneCtl,
				cfg.Agent.StandaloneHost,
				cfg.Agent.StandalonePort,
				log,
			)
			log.Info("Using standalone runtime",
				zap.String("host", cfg.Agent.StandaloneHost),
				zap.Int("port", cfg.Agent.StandalonePort))
		default:
			// Docker mode (default)
			if dockerClient != nil {
				containerMgr = lifecycle.NewContainerManager(dockerClient, "", log)
				agentRuntime = lifecycle.NewDockerRuntime(dockerClient, log)
				log.Info("Using Docker runtime")
			}
		}

		// Lifecycle Manager (uses agentctl for agent communication)
		lifecycleMgr = lifecycle.NewManager(agentRegistry, eventBus, agentRuntime, containerMgr, credsMgr, profileResolver, log)

		if err := lifecycleMgr.Start(ctx); err != nil {
			log.Fatal("Failed to start lifecycle manager", zap.Error(err))
		}

		log.Info("Agent Manager initialized",
			zap.String("runtime", cfg.Agent.Runtime),
			zap.Int("agent_types", len(agentRegistry.List())))
	} else {
		log.Info("Agent Manager disabled (no Docker and not in standalone mode)")
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

		// Wire worktree manager to task service for cleanup on task deletion
		taskSvc.SetWorktreeCleanup(worktreeMgr)
		log.Info("Worktree Manager connected to Task Service for cleanup")
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
	orchestratorSvc := orchestrator.NewService(serviceCfg, eventBus, agentManagerClient, taskRepoAdapter, taskRepo, log)
	taskSvc.SetExecutionStopper(orchestratorSvc)

	// Set up comment creator for saving agent responses as comments
	orchestratorSvc.SetMessageCreator(&messageCreatorAdapter{svc: taskSvc})

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
	messageController := taskcontroller.NewMessageController(taskSvc)
	repositoryController := taskcontroller.NewRepositoryController(taskSvc)
	executorController := taskcontroller.NewExecutorController(taskSvc)
	environmentController := taskcontroller.NewEnvironmentController(taskSvc)

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

		// Register workspace file handlers
		workspaceFileHandlers := agenthandlers.NewWorkspaceFileHandlers(lifecycleMgr, log)
		workspaceFileHandlers.RegisterHandlers(gateway.Dispatcher)
		log.Info("Registered Workspace File WebSocket handlers")

		// Register shell handlers
		shellHandlers := agenthandlers.NewShellHandlers(lifecycleMgr, log)
		shellHandlers.SetHub(gateway.Hub)
		shellHandlers.SetSessionResumer(&shellSessionResumerAdapter{svc: orchestratorSvc})
		shellHandlers.RegisterHandlers(gateway.Dispatcher)
		lifecycleMgr.SetShellStreamStarter(shellHandlers)
		log.Info("Registered Shell WebSocket handlers")
	}

	// Start the WebSocket hub
	go gateway.Hub.Run(ctx)
	gateways.RegisterTaskNotifications(ctx, eventBus, gateway.Hub, log)
	gateways.RegisterUserNotifications(ctx, eventBus, gateway.Hub, log)

	// Set up historical logs provider for task subscriptions
	// Uses messages instead of execution logs - all agent messages are now messages
	gateway.Hub.SetHistoricalLogsProvider(func(ctx context.Context, taskID string) ([]*ws.Message, error) {
		var session *models.TaskSession
		session, err = taskRepo.GetActiveTaskSessionByTaskID(ctx, taskID)
		if err != nil {
			session, err = taskRepo.GetTaskSessionByTaskID(ctx, taskID)
			if err != nil {
				return nil, nil
			}
		}

		messages, err := taskSvc.ListMessages(ctx, session.ID)
		if err != nil {
			return nil, err
		}

		result := make([]*ws.Message, 0, len(messages))
		for _, message := range messages {
			action := ws.ActionMessageAdded
			payload := map[string]interface{}{
				"message_id":      message.ID,
				"task_session_id": message.TaskSessionID,
				"task_id":         message.TaskID,
				"author_type":     string(message.AuthorType),
				"author_id":       message.AuthorID,
				"content":         message.Content,
				"type":            string(message.Type),
				"requests_input":  message.RequestsInput,
				"created_at":      message.CreatedAt.Format(time.RFC3339),
			}
			if message.Metadata != nil {
				payload["metadata"] = message.Metadata
			}
			notification, err := ws.NewNotification(action, payload)
			if err != nil {
				continue
			}
			result = append(result, notification)
		}

		// Also send current git status if available
		session, err = taskRepo.GetActiveTaskSessionByTaskID(ctx, taskID)
		if err == nil && session != nil && session.Metadata != nil {
			if gitStatus, ok := session.Metadata["git_status"]; ok {
				// Add task_id to the git status data
				gitStatusData, ok := gitStatus.(map[string]interface{})
				if !ok {
					gitStatusData = map[string]interface{}{"data": gitStatus}
				}
				gitStatusData["task_id"] = taskID

				gitStatusNotification, err := ws.NewNotification("git.status", gitStatusData)
				if err == nil {
					result = append(result, gitStatusNotification)
				}
			}
		}

		return result, nil
	})
	log.Info("Historical logs provider configured for task subscriptions (using messages and git status)")

	// Set up pending permissions provider for task subscriptions
	gateway.Hub.SetPendingPermissionsProvider(func(ctx context.Context, taskID string) ([]*ws.Message, error) {
		if lifecycleMgr == nil {
			return nil, nil
		}
		permissions, err := lifecycleMgr.GetPendingPermissionsForTask(ctx, taskID)
		if err != nil {
			return nil, err
		}

		result := make([]*ws.Message, 0, len(permissions))
		for _, p := range permissions {
			// Build payload matching the permission.requested format
			payload := map[string]interface{}{
				"type":           "permission_request",
				"task_id":        taskID,
				"pending_id":     p.PendingID,
				"session_id":     p.SessionID,
				"tool_call_id":   p.ToolCallID,
				"title":          p.Title,
				"options":        p.Options,
				"action_type":    p.ActionType,
				"action_details": p.ActionDetails,
				"created_at":     p.CreatedAt.Format(time.RFC3339),
			}
			notification, err := ws.NewNotification(ws.ActionPermissionRequested, payload)
			if err != nil {
				continue
			}
			result = append(result, notification)
		}
		return result, nil
	})
	log.Info("Pending permissions provider configured for task subscriptions")

	// NOTE: We no longer create comments for each ACP message chunk.
	// Instead, the lifecycle manager accumulates message chunks and:
	// 1. Flushes the buffer as a comment when a tool use starts (step boundary)
	// 2. Flushes the buffer as a comment when the prompt completes (final response)
	// This is handled via the prompt.complete and step.complete events.

	// Wire ACP handler to broadcast to WebSocket clients as notifications
	orchestratorSvc.RegisterACPHandler(func(taskID string, msg *protocol.Message) {
		// Check for permission_request events (type is in msg.Type, payload in msg.Data)
		if msg.Type == "permission_request" {
			// Build payload for frontend with all necessary fields
			payload := map[string]interface{}{
				"type":              "permission_request",
				"task_id":           taskID,
				"agent_instance_id": msg.AgentID,
				"timestamp":         msg.Timestamp,
			}
			// Copy all fields from msg.Data (pending_id, options, action_type, etc.)
			for k, v := range msg.Data {
				payload[k] = v
			}
			// Broadcast permission.requested notification to task subscribers
			notification, _ := ws.NewNotification(ws.ActionPermissionRequested, payload)
			gateway.Hub.BroadcastToTask(taskID, notification)
			log.Info("broadcasted permission.requested notification",
				zap.String("task_id", taskID),
				zap.Any("pending_id", payload["pending_id"]))
			return
		}

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
			// Skip other message types (session/update, etc.)
		}
	})

	// Wire input request handler to create agent messages when input is requested
	orchestratorSvc.SetInputRequestHandler(func(ctx context.Context, taskID, agentID, message string) error {
		log.Info("agent requesting user input, creating message",
			zap.String("task_id", taskID),
			zap.String("agent_id", agentID))

		session, err := taskRepo.GetActiveTaskSessionByTaskID(ctx, taskID)
		if err != nil {
			log.Error("failed to resolve active agent session for input request",
				zap.String("task_id", taskID),
				zap.Error(err))
			return err
		}

		// Create a message from the agent
		messageRecord, err := taskSvc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
			TaskSessionID: session.ID,
			TaskID:        taskID,
			Content:       message,
			AuthorType:    "agent",
			AuthorID:      agentID,
			RequestsInput: true,
		})
		if err != nil {
			log.Error("failed to create agent message",
				zap.String("task_id", taskID),
				zap.Error(err))
			return err
		}

		// Broadcast message.added notification to task subscribers
		notification, _ := ws.NewNotification(ws.ActionMessageAdded, map[string]interface{}{
			"task_id":         taskID,
			"task_session_id": session.ID,
			"message":         messageRecord.ToAPI(),
			"requests_input":  true,
		})
		gateway.Hub.BroadcastToTask(taskID, notification)

		// Also broadcast input.requested notification
		inputNotification, _ := ws.NewNotification(ws.ActionInputRequested, map[string]interface{}{
			"task_id":    taskID,
			"message_id": messageRecord.ID,
			"message":    message,
		})
		gateway.Hub.BroadcastToTask(taskID, inputNotification)

		return nil
	})

	if err := orchestratorSvc.Start(ctx); err != nil {
		log.Fatal("Failed to start orchestrator", zap.Error(err))
	}
	log.Info("Orchestrator initialized")

	// Subscribe to message.added events and broadcast to WebSocket subscribers
	_, err = eventBus.Subscribe(events.MessageAdded, func(ctx context.Context, event *bus.Event) error {
		data := event.Data
		taskSessionID, _ := data["task_session_id"].(string)
		taskID, _ := data["task_id"].(string)
		if taskSessionID == "" {
			return nil
		}

		payload := map[string]interface{}{
			"task_id":         taskID,
			"task_session_id": taskSessionID,
			"message_id":      data["message_id"],
			"author_type":     data["author_type"],
			"author_id":       data["author_id"],
			"content":         data["content"],
			"type":            data["type"],
			"requests_input":  data["requests_input"],
			"created_at":      data["created_at"],
		}
		if metadata, ok := data["metadata"]; ok && metadata != nil {
			payload["metadata"] = metadata
		}
		notification, err := ws.NewNotification(ws.ActionMessageAdded, payload)
		if err != nil {
			log.Error("Failed to create message.added notification", zap.Error(err))
			return nil
		}
		gateway.Hub.BroadcastToTask(taskID, notification)
		return nil
	})
	if err != nil {
		log.Error("Failed to subscribe to message.added events", zap.Error(err))
	} else {
		log.Info("Subscribed to message.added events for WebSocket broadcasting")
	}

	// Subscribe to message.updated events and broadcast to WebSocket subscribers
	_, err = eventBus.Subscribe(events.MessageUpdated, func(ctx context.Context, event *bus.Event) error {
		data := event.Data
		taskSessionID, _ := data["task_session_id"].(string)
		taskID, _ := data["task_id"].(string)
		if taskSessionID == "" {
			log.Warn("message.updated event has no task_session_id, skipping")
			return nil
		}
		payload := map[string]interface{}{
			"message_id":      data["message_id"],
			"task_session_id": taskSessionID,
			"task_id":         taskID,
			"author_type":     data["author_type"],
			"author_id":       data["author_id"],
			"content":         data["content"],
			"type":            data["type"],
			"requests_input":  data["requests_input"],
			"created_at":      data["created_at"],
		}
		if metadata, ok := data["metadata"]; ok && metadata != nil {
			payload["metadata"] = metadata
		}
		notification, err := ws.NewNotification(ws.ActionMessageUpdated, payload)
		if err != nil {
			log.Error("Failed to create message.updated notification", zap.Error(err))
			return nil
		}
		gateway.Hub.BroadcastToTask(taskID, notification)
		return nil
	})
	if err != nil {
		log.Error("Failed to subscribe to message.updated events", zap.Error(err))
	} else {
		log.Info("Subscribed to message.updated events for WebSocket broadcasting")
	}

	// Subscribe to task_session.state_changed events and broadcast to task subscribers
	_, err = eventBus.Subscribe(events.TaskSessionStateChanged, func(ctx context.Context, event *bus.Event) error {
		data := event.Data
		taskID, _ := data["task_id"].(string)
		if taskID == "" {
			return nil
		}
		notification, err := ws.NewNotification(ws.ActionTaskSessionStateChanged, data)
		if err != nil {
			log.Error("Failed to create task_session.state_changed notification", zap.Error(err))
			return nil
		}
		gateway.Hub.BroadcastToTask(taskID, notification)
		return nil
	})
	if err != nil {
		log.Error("Failed to subscribe to task_session.state_changed events", zap.Error(err))
	} else {
		log.Info("Subscribed to task_session.state_changed events for WebSocket broadcasting")
	}

	// Subscribe to git status events and broadcast to WebSocket subscribers
	_, err = eventBus.Subscribe(events.BuildGitStatusWildcardSubject(), func(ctx context.Context, event *bus.Event) error {
		data := event.Data
		taskID, _ := data["task_id"].(string)
		if taskID == "" {
			return nil
		}

		// Broadcast git status update to task subscribers
		notification, _ := ws.NewNotification("git.status", data)
		gateway.Hub.BroadcastToTask(taskID, notification)
		return nil
	})
	if err != nil {
		log.Error("Failed to subscribe to git status events", zap.Error(err))
	} else {
		log.Info("Subscribed to git status events for WebSocket broadcasting")
	}

	// Subscribe to file change events and broadcast to WebSocket subscribers
	_, err = eventBus.Subscribe(events.BuildFileChangeWildcardSubject(), func(ctx context.Context, event *bus.Event) error {
		data := event.Data
		taskID, _ := data["task_id"].(string)
		if taskID == "" {
			return nil
		}

		// Broadcast file change notification to task subscribers
		notification, _ := ws.NewNotification("workspace.file.changes", data)
		gateway.Hub.BroadcastToTask(taskID, notification)
		return nil
	})
	if err != nil {
		log.Error("Failed to subscribe to file change events", zap.Error(err))
	} else {
		log.Info("Subscribed to file change events for WebSocket broadcasting")
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
	taskhandlers.RegisterExecutorRoutes(router, gateway.Dispatcher, executorController, log)
	taskhandlers.RegisterEnvironmentRoutes(router, gateway.Dispatcher, environmentController, log)
	taskhandlers.RegisterMessageRoutes(
		router,
		gateway.Dispatcher,
		messageController,
		taskController,
		&orchestratorAdapter{svc: orchestratorSvc},
		log,
	)
	log.Info("Registered Task Service handlers (HTTP + WebSocket)")

	agentsettingshandlers.RegisterRoutes(router, agentSettingsController, gateway.Hub, log)
	log.Info("Registered Agent Settings handlers (HTTP)")

	userhandlers.RegisterRoutes(router, gateway.Dispatcher, userCtrl, log)
	log.Info("Registered User handlers (HTTP + WebSocket)")

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
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	sig := <-quit

	log.Info("Received shutdown signal", zap.String("signal", sig.String()))

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
		log.Info("Stopping agents gracefully...")
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 20*time.Second)
		if err := lifecycleMgr.StopAllAgents(stopCtx); err != nil {
			log.Error("Graceful agent stop error", zap.Error(err))
		}
		stopCancel()

		if err := lifecycleMgr.Stop(); err != nil {
			log.Error("Lifecycle manager stop error", zap.Error(err))
		}
	}

	// Stop agentctl subprocess (must be after lifecycle manager to allow cleanup)
	if agentctlLauncher != nil {
		log.Info("Stopping agentctl subprocess...")
		if err := agentctlLauncher.Stop(shutdownCtx); err != nil {
			log.Error("agentctl stop error", zap.Error(err))
		}
	}

	log.Info("Kandev stopped")
}

func runInitialAgentSetup(
	ctx context.Context,
	userSvc *userservice.Service,
	agentSettingsController *agentsettingscontroller.Controller,
	log *logger.Logger,
) error {
	settings, err := userSvc.GetUserSettings(ctx)
	if err != nil {
		return err
	}
	if settings.InitialSetupComplete {
		return nil
	}
	if err := agentSettingsController.EnsureInitialAgentProfiles(ctx); err != nil {
		return err
	}
	complete := true
	if _, err := userSvc.UpdateUserSettings(ctx, &userservice.UpdateUserSettingsRequest{
		InitialSetupComplete: &complete,
	}); err != nil {
		return err
	}
	log.Info("Initial agent setup complete")
	return nil
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

// LaunchAgent creates a new agentctl instance for a task.
// Agent subprocess is NOT started - call StartAgentProcess() explicitly.
func (a *lifecycleAdapter) LaunchAgent(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	// The RepositoryURL field contains a local filesystem path for the workspace
	// If empty, the agent will run without a mounted workspace
	launchReq := &lifecycle.LaunchRequest{
		TaskID:          req.TaskID,
		SessionID:       req.SessionID,
		TaskTitle:       req.TaskTitle,
		AgentProfileID:  req.AgentProfileID,
		WorkspacePath:   req.RepositoryURL, // May be empty - lifecycle manager handles this
		TaskDescription: req.TaskDescription,
		Env:             req.Env,
		ACPSessionID:    req.ACPSessionID,
		Metadata:        req.Metadata,
		// Worktree configuration for concurrent agent execution
		UseWorktree:    req.UseWorktree,
		RepositoryID:   req.RepositoryID,
		RepositoryPath: req.RepositoryPath,
		BaseBranch:     req.BaseBranch,
	}

	// Create the agentctl execution (does NOT start agent process)
	execution, err := a.mgr.Launch(ctx, launchReq)
	if err != nil {
		return nil, err
	}

	// Extract worktree info from metadata if available
	var worktreeID, worktreePath, worktreeBranch string
	if execution.Metadata != nil {
		if id, ok := execution.Metadata["worktree_id"].(string); ok {
			worktreeID = id
		}
		if path, ok := execution.Metadata["worktree_path"].(string); ok {
			worktreePath = path
		}
		if branch, ok := execution.Metadata["worktree_branch"].(string); ok {
			worktreeBranch = branch
		}
	}

	return &executor.LaunchAgentResponse{
		AgentExecutionID: execution.ID,
		ContainerID:     execution.ContainerID,
		Status:          execution.Status,
		WorktreeID:      worktreeID,
		WorktreePath:    worktreePath,
		WorktreeBranch:  worktreeBranch,
	}, nil
}

// StartAgentProcess starts the agent subprocess for an instance.
// The command is built internally based on the instance's agent profile.
func (a *lifecycleAdapter) StartAgentProcess(ctx context.Context, agentInstanceID string) error {
	return a.mgr.StartAgentProcess(ctx, agentInstanceID)
}

// StopAgent stops a running agent
func (a *lifecycleAdapter) StopAgent(ctx context.Context, agentInstanceID string, force bool) error {
	return a.mgr.StopAgent(ctx, agentInstanceID, force)
}

// GetAgentStatus returns the status of an agent execution
func (a *lifecycleAdapter) GetAgentStatus(ctx context.Context, agentInstanceID string) (*v1.AgentExecution, error) {
	execution, found := a.mgr.GetExecution(agentInstanceID)
	if !found {
		return nil, fmt.Errorf("agent execution %q not found", agentInstanceID)
	}

	containerID := execution.ContainerID
	now := time.Now()
	result := &v1.AgentExecution{
		ID:             execution.ID,
		TaskID:         execution.TaskID,
		AgentProfileID: execution.AgentProfileID,
		ContainerID:    &containerID,
		Status:         execution.Status,
		StartedAt:      &execution.StartedAt,
		StoppedAt:      execution.FinishedAt,
		CreatedAt:      execution.StartedAt,
		UpdatedAt:      now,
	}

	if execution.ExitCode != nil {
		result.ExitCode = execution.ExitCode
	}
	if execution.ErrorMessage != "" {
		result.ErrorMessage = &execution.ErrorMessage
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

// GetRecoveredExecutions returns executions recovered from Docker during startup
func (a *lifecycleAdapter) GetRecoveredExecutions() []executor.RecoveredExecutionInfo {
	recovered := a.mgr.GetRecoveredExecutions()
	result := make([]executor.RecoveredExecutionInfo, len(recovered))
	for i, r := range recovered {
		result[i] = executor.RecoveredExecutionInfo{
			ExecutionID:    r.ExecutionID,
			TaskID:         r.TaskID,
			ContainerID:    r.ContainerID,
			AgentProfileID: r.AgentProfileID,
		}
	}
	return result
}

// IsAgentRunningForTask checks if an agent is actually running for a task
// This probes the actual agent (Docker container or standalone process)
func (a *lifecycleAdapter) IsAgentRunningForTask(ctx context.Context, taskID string) bool {
	return a.mgr.IsAgentRunningForTask(ctx, taskID)
}

// CleanupStaleExecutionByTaskID removes a stale agent execution from tracking without trying to stop it.
func (a *lifecycleAdapter) CleanupStaleExecutionByTaskID(ctx context.Context, taskID string) error {
	return a.mgr.CleanupStaleExecutionByTaskID(ctx, taskID)
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

func (a *orchestratorAdapter) ResumeTaskSession(ctx context.Context, taskID, taskSessionID string) error {
	_, err := a.svc.ResumeTaskSession(ctx, taskID, taskSessionID)
	return err
}

// shellSessionResumerAdapter adapts the orchestrator service to the shell handlers' SessionResumer interface
type shellSessionResumerAdapter struct {
	svc *orchestrator.Service
}

func (a *shellSessionResumerAdapter) ResumeTaskSession(ctx context.Context, taskID, taskSessionID string) error {
	_, err := a.svc.ResumeTaskSession(ctx, taskID, taskSessionID)
	return err
}

// messageCreatorAdapter adapts the task service to the orchestrator.MessageCreator interface
type messageCreatorAdapter struct {
	svc *taskservice.Service
}

// CreateAgentMessage creates a message with author_type="agent"
func (a *messageCreatorAdapter) CreateAgentMessage(ctx context.Context, taskID, content, agentSessionID string) error {
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		Content:       content,
		AuthorType:    "agent",
	})
	return err
}

// CreateUserMessage creates a message with author_type="user"
func (a *messageCreatorAdapter) CreateUserMessage(ctx context.Context, taskID, content, agentSessionID string) error {
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		Content:       content,
		AuthorType:    "user",
	})
	return err
}

// CreateToolCallMessage creates a message for a tool call with type="tool_call"
func (a *messageCreatorAdapter) CreateToolCallMessage(ctx context.Context, taskID, toolCallID, title, status, agentSessionID string, args map[string]interface{}) error {
	metadata := map[string]interface{}{
		"tool_call_id": toolCallID,
		"title":        title,
		"status":       status,
	}

	// Add args if provided (contains kind, path, locations, raw_input)
	if len(args) > 0 {
		metadata["args"] = args

		// Extract kind as tool_name for icon selection in the frontend
		if kind, ok := args["kind"].(string); ok && kind != "" {
			metadata["tool_name"] = kind
		}
	}

	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		Content:       title,
		AuthorType:    "agent",
		Type:          "tool_call",
		Metadata:      metadata,
	})
	return err
}

// UpdateToolCallMessage updates a tool call message's status
func (a *messageCreatorAdapter) UpdateToolCallMessage(ctx context.Context, taskID, toolCallID, status, result, agentSessionID string) error {
	return a.svc.UpdateToolCallMessage(ctx, agentSessionID, toolCallID, status, result)
}

// CreateSessionMessage creates a message for non-chat session updates (status/progress/error/etc).
func (a *messageCreatorAdapter) CreateSessionMessage(ctx context.Context, taskID, content, agentSessionID, messageType string, metadata map[string]interface{}, requestsInput bool) error {
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		Content:       content,
		AuthorType:    "agent",
		Type:          messageType,
		Metadata:      metadata,
		RequestsInput: requestsInput,
	})
	return err
}
