package automation

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
)

// Components holds the automation subsystem components for lifecycle management.
type Components struct {
	Service   *Service
	Scheduler *CronScheduler
	Evaluator *GitHubEvaluator
}

// Start begins background processing (scheduler + GitHub polling).
func (c *Components) Start(ctx context.Context) {
	c.Scheduler.Start(ctx)
	c.Evaluator.Start(ctx)
}

// Stop gracefully shuts down background processing.
func (c *Components) Stop() {
	c.Scheduler.Stop()
	c.Evaluator.Stop()
}

// Provide creates the full automation stack: store, service, scheduler, evaluator.
func Provide(
	writer, reader *sqlx.DB,
	eventBus bus.EventBus,
	ghSvc *github.Service,
	log *logger.Logger,
) (*Components, error) {
	store, err := NewStore(writer, reader)
	if err != nil {
		return nil, err
	}

	svc := NewService(store, eventBus, log)
	scheduler := NewCronScheduler(svc, log)

	var ghClient github.Client
	if ghSvc != nil {
		ghClient = ghSvc.Client()
	}
	evaluator := NewGitHubEvaluator(svc, ghClient, log)

	return &Components{
		Service:   svc,
		Scheduler: scheduler,
		Evaluator: evaluator,
	}, nil
}
