package debug

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// getAdapterBaseDir returns the absolute path to the adapter directory.
// It handles different CWD scenarios (running from git root or apps/backend).
func getAdapterBaseDir() (string, error) {
	// Try multiple possible paths
	candidates := []string{
		"apps/backend/internal/agentctl/server/adapter",   // From git root
		"internal/agentctl/server/adapter",                 // From apps/backend
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for _, candidate := range candidates {
		absPath := filepath.Join(cwd, candidate)
		if info, err := os.Stat(absPath); err == nil && info.IsDir() {
			return absPath, nil
		}
	}

	// Fallback: try the first candidate
	return filepath.Abs(candidates[0])
}

// RegisterRoutes registers debug API routes.
func RegisterRoutes(router *gin.Engine, log *logger.Logger) {
	api := router.Group("/api/v1/debug")
	api.GET("/fixture-files", handleListFixtureFiles(log))
	api.GET("/normalize-messages", handleNormalizeMessages(log))
	api.GET("/normalized-files", handleListNormalizedFiles(log))
	api.GET("/normalized-events", handleReadNormalizedEvents(log))
}

// handleListFixtureFiles handles GET /api/v1/debug/fixture-files
// Returns a list of discovered fixture files with their protocols.
func handleListFixtureFiles(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir, err := getAdapterBaseDir()
		if err != nil {
			log.Error("failed to resolve adapter directory", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to resolve adapter directory",
			})
			return
		}

		files, err := discoverFixtureFiles(baseDir)
		if err != nil {
			log.Error("failed to discover fixture files", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"files": files})
	}
}

// handleNormalizeMessages handles GET /api/v1/debug/normalize-messages
// Query params:
//   - file: relative path to a fixture file (e.g., "transport/acp/testdata/acp-messages.jsonl")
func handleNormalizeMessages(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		filePath := c.Query("file")
		if filePath == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "file parameter required",
			})
			return
		}

		// Resolve base directory
		baseDir, err := getAdapterBaseDir()
		if err != nil {
			log.Error("failed to resolve adapter directory", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to resolve adapter directory",
			})
			return
		}

		// Construct full path and validate it's within the allowed directory
		fullPath := filepath.Join(baseDir, filePath)
		fullPath = filepath.Clean(fullPath)

		if !isPathAllowed(fullPath, baseDir) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid file path",
			})
			return
		}

		fixtures, err := normalizeFixtureFile(fullPath)
		if err != nil {
			log.Error("failed to normalize fixtures",
				zap.String("file", filePath),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, fixtures)
	}
}

// isPathAllowed validates that the path is within the allowed base directory.
func isPathAllowed(fullPath, baseDir string) bool {
	// Ensure the path starts with the base directory (prevent path traversal)
	return strings.HasPrefix(fullPath, baseDir+string(filepath.Separator)) ||
		fullPath == baseDir
}

// handleListNormalizedFiles handles GET /api/v1/debug/normalized-files
// Returns a list of discovered normalized event files (normalized-*.jsonl).
func handleListNormalizedFiles(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir, err := getAdapterBaseDir()
		if err != nil {
			log.Error("failed to resolve adapter directory", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to resolve adapter directory",
			})
			return
		}

		files, err := discoverNormalizedFiles(baseDir)
		if err != nil {
			log.Error("failed to discover normalized files", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"files": files})
	}
}

// handleReadNormalizedEvents handles GET /api/v1/debug/normalized-events
// Query params:
//   - file: relative path to a normalized file (e.g., "../../../../normalized-acp-auggie.jsonl")
func handleReadNormalizedEvents(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		filePath := c.Query("file")
		if filePath == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "file parameter required",
			})
			return
		}

		// Resolve base directory
		baseDir, err := getAdapterBaseDir()
		if err != nil {
			log.Error("failed to resolve adapter directory", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to resolve adapter directory",
			})
			return
		}

		// Construct full path
		fullPath := filepath.Join(baseDir, filePath)
		fullPath = filepath.Clean(fullPath)

		// Validate the file is a normalized file (security check)
		filename := filepath.Base(fullPath)
		if !strings.HasPrefix(filename, "normalized-") || !strings.HasSuffix(filename, ".jsonl") {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid file: must be a normalized-*.jsonl file",
			})
			return
		}

		messages, err := readNormalizedEventsAsMessages(fullPath)
		if err != nil {
			log.Error("failed to read normalized events",
				zap.String("file", filePath),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"messages": messages})
	}
}
