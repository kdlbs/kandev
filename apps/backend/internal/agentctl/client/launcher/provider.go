package launcher

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

// Provide starts the agentctl launcher and returns a cleanup to stop it.
func Provide(ctx context.Context, cfg Config, log *logger.Logger) (*Launcher, func() error, error) {
	launcher := New(cfg, log)
	if err := launcher.Start(ctx); err != nil {
		return nil, nil, err
	}

	var stopOnce sync.Once
	cleanup := func() error {
		var stopErr error
		stopOnce.Do(func() {
			stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			stopErr = launcher.Stop(stopCtx)
		})
		return stopErr
	}

	return launcher, cleanup, nil
}
