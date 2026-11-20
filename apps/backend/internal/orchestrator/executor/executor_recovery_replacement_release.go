package executor

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type taskEnvironmentReplacementReleaser interface {
	ReleaseTaskEnvironmentReplacement(context.Context, *models.TaskEnvironment, string) (bool, error)
}

func (e *Executor) recoveryReplacementRelease(environment *models.TaskEnvironment, sessionID string) func(context.Context) error {
	releaser, ok := e.repo.(taskEnvironmentReplacementReleaser)
	if !ok || environment == nil || environment.Status != models.TaskEnvironmentStatusCreating || environment.MaterializationSessionID != sessionID {
		return nil
	}
	expected := *environment
	var once sync.Once
	var releaseErr error
	return func(ctx context.Context) error {
		once.Do(func() {
			cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			_, releaseErr = releaser.ReleaseTaskEnvironmentReplacement(cleanupCtx, &expected, sessionID)
		})
		return releaseErr
	}
}
