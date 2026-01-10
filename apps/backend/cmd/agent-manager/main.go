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

	"github.com/kandev/kandev/internal/agent/api"
	"github.com/kandev/kandev/internal/agent/credentials"
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/streaming"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
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

	log.Info("Starting Agent Manager service...")

	// 3. Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 4. Connect to NATS event bus
	eventBus, err := bus.NewNATSEventBus(cfg.NATS, log)
	if err != nil {
		log.Fatal("Failed to connect to NATS", zap.Error(err))
	}
	defer eventBus.Close()
	log.Info("Connected to NATS event bus")

	// 5. Initialize Docker client
	dockerClient, err := docker.NewClient(cfg.Docker, log)
	if err != nil {
		log.Fatal("Failed to initialize Docker client", zap.Error(err))
	}
	defer dockerClient.Close()

	// Ping Docker to verify connection
	if err := dockerClient.Ping(ctx); err != nil {
		log.Fatal("Failed to connect to Docker daemon", zap.Error(err))
	}
	log.Info("Connected to Docker daemon")

	// 6. Initialize Agent Registry
	reg := registry.NewRegistry(log)
	reg.LoadDefaults()
	log.Info("Loaded agent registry", zap.Int("agent_types", len(reg.List())))

	// 7. Initialize Credentials Manager
	credsMgr := credentials.NewManager(log)
	credsMgr.AddProvider(credentials.NewEnvProvider("KANDEV_"))

	// Optionally load from file
	if credsFile := os.Getenv("KANDEV_CREDENTIALS_FILE"); credsFile != "" {
		credsMgr.AddProvider(credentials.NewFileProvider(credsFile))
	}
	log.Info("Initialized credentials manager")

	// 8. Initialize Streaming Manager
	streamMgr := streaming.NewManager(eventBus, log)
	log.Info("Initialized streaming manager")

	// 9. Initialize Lifecycle Manager
	lifecycleMgr := lifecycle.NewManager(dockerClient, reg, eventBus, log)

	if err := lifecycleMgr.Start(ctx); err != nil {
		log.Fatal("Failed to start lifecycle manager", zap.Error(err))
	}
	log.Info("Started lifecycle manager")

	// 10. Setup HTTP server with Gin
	if cfg.Logging.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())

	// 11. Register API routes
	v1 := router.Group("/api/v1/agents")
	api.SetupRoutes(v1, lifecycleMgr, reg, dockerClient, nil, log)

	// Health check endpoint at root level
	handler := api.NewHandler(lifecycleMgr, reg, dockerClient, nil, log)
	router.GET("/health", handler.HealthCheck)

	// 12. Create HTTP server
	port := cfg.Server.Port
	if port == 0 {
		port = 8083 // Default agent manager port
	}
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeoutDuration(),
		WriteTimeout: cfg.Server.WriteTimeoutDuration(),
	}

	// 13. Start server in goroutine
	go func() {
		log.Info("HTTP server listening", zap.Int("port", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Failed to start HTTP server", zap.Error(err))
		}
	}()

	// 14. Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down Agent Manager service...")

	// 15. Graceful shutdown
	cancel()

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("HTTP server shutdown error", zap.Error(err))
	}

	// Stop streaming manager
	streamMgr.StopAll()

	// Stop lifecycle manager
	if err := lifecycleMgr.Stop(); err != nil {
		log.Error("Lifecycle manager stop error", zap.Error(err))
	}

	// Log that credentials manager is available (for future use)
	_ = credsMgr // Used for building env vars when launching agents

	log.Info("Agent Manager service stopped")
}

