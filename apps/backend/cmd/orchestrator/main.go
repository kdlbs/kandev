// Package main is the entry point for the Orchestrator service.
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

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/database"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/api"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/streaming"
	"github.com/kandev/kandev/pkg/acp/protocol"
)

func main() {
	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize logger
	logCfg := logger.LoggingConfig{
		Level:      cfg.Logging.Level,
		Format:     cfg.Logging.Format,
		OutputPath: cfg.Logging.OutputPath,
	}
	log, err := logger.NewLogger(logCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()
	logger.SetDefault(log)

	log.Info("Starting Orchestrator service...")

	// 3. Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 4. Connect to PostgreSQL
	db, err := database.NewDB(ctx, cfg.Database)
	if err != nil {
		log.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()
	log.Info("Connected to PostgreSQL")

	// 5. Connect to NATS event bus
	eventBus, err := bus.NewNATSEventBus(cfg.NATS, log)
	if err != nil {
		log.Fatal("Failed to connect to NATS", zap.Error(err))
	}
	defer eventBus.Close()
	log.Info("Connected to NATS event bus")

	// 6. Initialize components
	// For now, use mock implementations
	agentManager := executor.NewMockAgentManagerClient(log)
	taskRepo := scheduler.NewMockTaskRepository(log)

	// 7. Create orchestrator service
	serviceCfg := orchestrator.DefaultServiceConfig()
	service := orchestrator.NewService(serviceCfg, eventBus, db, agentManager, taskRepo, log)

	// 8. Create WebSocket hub
	wsHub := streaming.NewHub(log)
	go wsHub.Run(ctx)

	// 9. Wire ACP handler to broadcast to WebSocket clients
	service.RegisterACPHandler(func(taskID string, msg *protocol.Message) {
		wsHub.Broadcast(taskID, msg)
	})

	// 10. Start orchestrator service
	if err := service.Start(ctx); err != nil {
		log.Fatal("Failed to start orchestrator service", zap.Error(err))
	}
	log.Info("Orchestrator service started")

	// 11. Setup HTTP server with Gin
	if cfg.Logging.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(api.RequestLogger(log))
	router.Use(api.Recovery(log))
	router.Use(api.CORS())

	// 12. Register API routes
	v1 := router.Group("/api/v1/orchestrator")
	api.SetupRoutes(v1, service, log)

	// 13. Register WebSocket routes
	wsHandler := streaming.NewWSHandler(wsHub, service, log)
	streaming.SetupWebSocketRoutes(v1, wsHandler)

	// 14. Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 15. Create HTTP server
	port := cfg.Server.Port
	if port == 0 {
		port = 8082 // Default orchestrator port
	}
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeoutDuration(),
		WriteTimeout: cfg.Server.WriteTimeoutDuration(),
	}

	// 16. Start server in goroutine
	go func() {
		log.Info("HTTP server listening", zap.Int("port", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Failed to start HTTP server", zap.Error(err))
		}
	}()

	// 17. Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down Orchestrator service...")

	// 18. Graceful shutdown
	cancel() // Cancel context to stop background goroutines

	// Shutdown HTTP server with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("HTTP server shutdown error", zap.Error(err))
	}

	// Stop orchestrator service
	if err := service.Stop(); err != nil {
		log.Error("Orchestrator service stop error", zap.Error(err))
	}

	log.Info("Orchestrator service stopped")
}

