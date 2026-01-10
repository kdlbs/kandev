// Package api provides HTTP middleware for the Orchestrator API.
package api

import (
	stderrors "errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/errors"
	"github.com/kandev/kandev/internal/common/logger"
)

// RequestLogger logs all incoming requests with detailed information.
func RequestLogger(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Generate request ID
		requestID := uuid.New().String()
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)

		// Process request
		c.Next()

		// Log request details
		duration := time.Since(start)
		log.Info("Request completed",
			zap.String("path", c.Request.URL.Path),
			zap.String("method", c.Request.Method),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("duration", duration),
			zap.String("request_id", requestID),
		)
	}
}

// ErrorHandler handles errors and returns appropriate responses.
func ErrorHandler(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Check for errors
		if len(c.Errors) > 0 {
			err := c.Errors.Last().Err

			// Check if it's an AppError
			var appErr *errors.AppError
			if stderrors.As(err, &appErr) {
				log.Error("Request error",
					zap.String("code", appErr.Code),
					zap.String("message", appErr.Message),
					zap.Int("status", appErr.HTTPStatus),
				)
				c.JSON(appErr.HTTPStatus, gin.H{
					"error": gin.H{
						"code":    appErr.Code,
						"message": appErr.Message,
					},
				})
				return
			}

			// Default to internal server error
			log.Error("Internal server error", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"code":    errors.ErrCodeInternalError,
					"message": "An internal server error occurred",
				},
			})
		}
	}
}

// Recovery recovers from panics and logs them.
func Recovery(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("Panic recovered",
					zap.Any("panic", r),
					zap.String("path", c.Request.URL.Path),
					zap.String("method", c.Request.Method),
				)

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": gin.H{
						"code":    errors.ErrCodeInternalError,
						"message": "An internal server error occurred",
					},
				})
			}
		}()

		c.Next()
	}
}

// CORS adds CORS headers for cross-origin requests.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Request-ID")
		c.Header("Access-Control-Expose-Headers", "X-Request-ID")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// RateLimit provides basic rate limiting using a token bucket algorithm.
// This is a placeholder implementation - for production, consider using
// a distributed rate limiter like redis-based solutions.
func RateLimit(requestsPerSecond int) gin.HandlerFunc {
	var (
		mu       sync.Mutex
		tokens   = float64(requestsPerSecond)
		lastTime = time.Now()
	)

	return func(c *gin.Context) {
		mu.Lock()

		now := time.Now()
		elapsed := now.Sub(lastTime).Seconds()
		lastTime = now

		// Refill tokens
		tokens += elapsed * float64(requestsPerSecond)
		if tokens > float64(requestsPerSecond) {
			tokens = float64(requestsPerSecond)
		}

		// Check if we have tokens available
		if tokens < 1 {
			mu.Unlock()
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"code":    "RATE_LIMIT_EXCEEDED",
					"message": "Too many requests, please try again later",
				},
			})
			return
		}

		tokens--
		mu.Unlock()

		c.Next()
	}
}

