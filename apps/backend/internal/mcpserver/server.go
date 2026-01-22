// Package mcpserver provides a reusable MCP server for Kandev task management.
// It exposes tools for listing, creating, and updating tasks via SSE and Streamable HTTP transports.
package mcpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"
)

// Config holds the MCP server configuration.
type Config struct {
	Port      int    // Port to listen on
	KandevURL string // Kandev API URL (e.g., http://localhost:8080)
}

// Server wraps the SSE and Streamable HTTP servers with lifecycle management.
// It supports both transports for compatibility with different MCP clients:
// - SSE transport (/sse) for Claude Desktop, Cursor, etc.
// - Streamable HTTP transport (/mcp) for Codex
type Server struct {
	cfg                  Config
	sseServer            *server.SSEServer
	streamableHTTPServer *server.StreamableHTTPServer
	httpServer           *http.Server
	mu                   sync.Mutex
	running              bool
	logger               *logger.Logger
}

// New creates a new MCP server with the given configuration.
func New(cfg Config) *Server {
	return &Server{
		cfg:    cfg,
		logger: logger.Default().WithFields(),
	}
}

// Start starts the MCP server in a goroutine and returns when it's listening.
// It starts both SSE and Streamable HTTP transports on the same port.
// It returns an error if the server fails to start.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.mu.Unlock()

	// Create MCP server (shared between both transports)
	mcpServer := server.NewMCPServer(
		"kandev-mcp",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// Register tools (using internal logger)
	registerTools(mcpServer, s.cfg, s.logger)

	// Create SSE server (for Claude Desktop, Cursor, etc.)
	s.sseServer = server.NewSSEServer(mcpServer)

	// Create Streamable HTTP server (for Codex)
	// Use WithEndpointPath to ensure it handles /mcp path
	s.streamableHTTPServer = server.NewStreamableHTTPServer(mcpServer,
		server.WithEndpointPath("/mcp"),
	)

	// Create HTTP mux to route requests to appropriate transport
	mux := http.NewServeMux()

	// SSE transport routes - handle /sse and /message paths
	mux.Handle("/sse", s.sseServer.SSEHandler())
	mux.Handle("/message", s.sseServer.MessageHandler())

	// Streamable HTTP transport route - handle /mcp path
	mux.Handle("/mcp", s.streamableHTTPServer)

	// Verify the port is available by creating a test listener
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	if tcpAddr, ok := listener.Addr().(*net.TCPAddr); ok {
		s.cfg.Port = tcpAddr.Port
	}

	// Create HTTP server with the mux
	s.httpServer = &http.Server{
		Handler: mux,
	}

	// Ready channel to signal when server goroutine has started
	ready := make(chan struct{})

	// Start server in a goroutine
	go func() {
		s.mu.Lock()
		s.running = true
		s.mu.Unlock()

		// Signal that we're starting
		close(ready)

		s.logger.Info("MCP server listening",
			zap.Int("port", s.cfg.Port),
			zap.String("sse_endpoint", "/sse"),
			zap.String("streamable_http_endpoint", "/mcp"))

		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.logger.Error("MCP server error", zap.Error(err))
		}

		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	// Wait for the goroutine to start or context cancellation
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()

	if !running {
		return nil
	}

	// Shutdown the HTTP server (this stops both transports)
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown HTTP server: %w", err)
		}
	}

	// Also shutdown the SSE server to clean up any active sessions
	if s.sseServer != nil {
		if err := s.sseServer.Shutdown(ctx); err != nil {
			s.logger.Warn("failed to shutdown SSE server", zap.Error(err))
		}
	}

	// Shutdown the Streamable HTTP server to clean up any active sessions
	if s.streamableHTTPServer != nil {
		if err := s.streamableHTTPServer.Shutdown(ctx); err != nil {
			s.logger.Warn("failed to shutdown Streamable HTTP server", zap.Error(err))
		}
	}

	return nil
}

// SSEEndpoint returns the full SSE URL for clients that use SSE transport
// (e.g., Claude Desktop, Cursor).
func (s *Server) SSEEndpoint() string {
	return fmt.Sprintf("http://localhost:%d/sse", s.cfg.Port)
}

// StreamableHTTPEndpoint returns the full Streamable HTTP URL for clients that use
// streamable HTTP transport (e.g., Codex).
func (s *Server) StreamableHTTPEndpoint() string {
	return fmt.Sprintf("http://localhost:%d/mcp", s.cfg.Port)
}
