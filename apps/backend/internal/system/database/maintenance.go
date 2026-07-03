package database

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Vacuum runs VACUUM via a tracked job and returns the job ID. The job
// result includes "size_before" and "size_after" bytes; "reclaimed_bytes"
// is the difference (>= 0).
func (s *Service) Vacuum(ctx context.Context) string {
	return s.jobs.Start(ctx, "vacuum", func(jobCtx context.Context) (map[string]interface{}, error) {
		return s.runVacuum(jobCtx)
	})
}

func (s *Service) runVacuum(_ context.Context) (map[string]interface{}, error) {
	if s.pool == nil {
		return nil, fmt.Errorf("vacuum: no database pool")
	}
	before, _ := readDatabaseSize(s.pool.Reader())
	if _, err := s.pool.Writer().Exec("VACUUM"); err != nil {
		return nil, fmt.Errorf("vacuum: %w", err)
	}
	after, _ := readDatabaseSize(s.pool.Reader())
	reclaimed := before - after
	if reclaimed < 0 {
		reclaimed = 0
	}
	return map[string]interface{}{
		"size_before":     before,
		"size_after":      after,
		"reclaimed_bytes": reclaimed,
	}, nil
}

// Optimize runs PRAGMA optimize via a tracked job. PRAGMA optimize is
// cheap and idempotent; we still track it so the UI can show progress
// consistently with VACUUM.
func (s *Service) Optimize(ctx context.Context) string {
	return s.jobs.Start(ctx, "optimize", func(jobCtx context.Context) (map[string]interface{}, error) {
		return s.runOptimize(jobCtx)
	})
}

func (s *Service) runOptimize(_ context.Context) (map[string]interface{}, error) {
	if s.pool == nil {
		return nil, fmt.Errorf("optimize: no database pool")
	}
	if _, err := s.pool.Writer().Exec("PRAGMA optimize"); err != nil {
		return nil, fmt.Errorf("pragma optimize: %w", err)
	}
	return map[string]interface{}{"status": "ok"}, nil
}

// HandleVacuum returns a gin handler for POST /api/v1/system/database/vacuum.
func HandleVacuum(s *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		respondAccepted(c, s.Vacuum(ctxOrBackground(c)))
	}
}

// HandleOptimize returns a gin handler for POST /api/v1/system/database/optimize.
func HandleOptimize(s *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		respondAccepted(c, s.Optimize(ctxOrBackground(c)))
	}
}

// resetRequest is the JSON body for POST /api/v1/system/database/reset.
type resetRequest struct {
	Confirm string `json:"confirm"`
}

// HandleReset returns a gin handler for POST /api/v1/system/database/reset.
// Validates the body's confirm == "RESET" before spawning the reset job.
func HandleReset(s *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req resetRequest
		_ = c.ShouldBindJSON(&req)
		jobID, err := s.FactoryReset(ctxOrBackground(c), req.Confirm)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		respondAccepted(c, jobID)
	}
}
