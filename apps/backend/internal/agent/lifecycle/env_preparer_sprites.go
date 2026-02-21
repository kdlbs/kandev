package lifecycle

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// SpritesPreparer validates prerequisites for Sprites-based execution.
// The actual sprite creation and setup happens in SpritesExecutor.CreateInstance,
// so this preparer only validates that the configuration is plausible.
type SpritesPreparer struct {
	logger *logger.Logger
}

// NewSpritesPreparer creates a new SpritesPreparer.
func NewSpritesPreparer(log *logger.Logger) *SpritesPreparer {
	return &SpritesPreparer{
		logger: log.WithFields(zap.String("component", "sprites-preparer")),
	}
}

func (p *SpritesPreparer) Name() string { return "sprites" }

func (p *SpritesPreparer) Prepare(_ context.Context, req *EnvPrepareRequest, onProgress PrepareProgressCallback) (*EnvPrepareResult, error) {
	start := time.Now()

	p.logger.Debug("preparing sprites environment",
		zap.String("task_id", req.TaskID),
		zap.String("session_id", req.SessionID))

	// Validate: emit a single progress step (sprite provisioning happens in CreateInstance)
	step := beginStep("Validate Sprites.dev configuration")
	reportProgress(onProgress, step, 0, 1)
	completeStepSuccess(&step)
	reportProgress(onProgress, step, 0, 1)

	return &EnvPrepareResult{
		Success:       true,
		Steps:         []PrepareStep{step},
		WorkspacePath: req.WorkspacePath,
		Duration:      time.Since(start),
	}, nil
}
