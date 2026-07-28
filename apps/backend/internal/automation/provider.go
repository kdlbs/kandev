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
	Service           *Service
	Scheduler         *CronScheduler
	Evaluator         *GitHubEvaluator
	WebhookSubscriber *GitHubWebhookSubscriber
}

// Start begins background processing (scheduler + GitHub polling + webhook subscriber).
func (c *Components) Start(ctx context.Context) {
	c.Scheduler.Start(ctx)
	c.Evaluator.Start(ctx)
	c.WebhookSubscriber.Start(ctx)
}

// Stop gracefully shuts down background processing.
func (c *Components) Stop() {
	c.Scheduler.Stop()
	c.Evaluator.Stop()
	c.WebhookSubscriber.Stop()
}

// Provide creates the full automation stack: store, service, scheduler, evaluator,
// webhook subscriber.
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

	evaluator := NewGitHubEvaluator(svc, ghSvc, log)
	webhookSubscriber := NewGitHubWebhookSubscriber(svc, eventBus, log)

	return &Components{
		Service:           svc,
		Scheduler:         scheduler,
		Evaluator:         evaluator,
		WebhookSubscriber: webhookSubscriber,
	}, nil
}
