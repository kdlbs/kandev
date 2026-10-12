package automation

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
)

// Components holds the automation subsystem components for lifecycle management.
type Components struct {
	webhookCancel      context.CancelFunc
	webhookDone        chan struct{}
	managedCancel      context.CancelFunc
	managedDone        chan struct{}
	Service            *Service
	Scheduler          *CronScheduler
	Evaluator          *GitHubEvaluator
	WebhookSubscriber  *GitHubWebhookSubscriber
	PRMergedSubscriber *GitHubPRMergedSubscriber
}

// Start begins background processing (scheduler + GitHub polling + webhook subscriber + merged-PR subscriber).
func (c *Components) Start(ctx context.Context) {
	if err := c.Service.recoverWebhookClaims(ctx); err != nil {
		c.Service.logger.Warn("webhook claim recovery failed", zap.Error(err))
	}
	if err := c.Service.ReconcileOpenRuns(ctx); err != nil {
		c.Service.logger.Warn("automation open-run reconciliation failed", zap.Error(err))
	}
	if err := c.Service.ReconcileManagedConversationDeliveries(ctx); err != nil {
		c.Service.logger.Warn("managed automation delivery recovery failed", zap.Error(err))
	}
	if err := c.Service.ReconcileCleanupJobs(ctx); err != nil {
		c.Service.logger.Warn("automation cleanup-job reconciliation failed", zap.Error(err))
	}
	workerCtx, cancel := context.WithCancel(ctx)
	c.webhookCancel = cancel
	c.webhookDone = make(chan struct{})
	go func() { defer close(c.webhookDone); c.Service.runWebhookWorker(workerCtx) }()
	managedCtx, managedCancel := context.WithCancel(ctx)
	c.managedCancel = managedCancel
	c.managedDone = make(chan struct{})
	go func() { defer close(c.managedDone); c.Service.runManagedAutomationDeliveryWorker(managedCtx) }()
	c.Scheduler.Start(ctx)
	c.Evaluator.Start(ctx)
	c.WebhookSubscriber.Start(ctx)
	c.PRMergedSubscriber.Start(ctx)
}

// Stop gracefully shuts down background processing.
func (c *Components) Stop() {
	if c.webhookCancel != nil {
		c.webhookCancel()
		<-c.webhookDone
	}
	if c.managedCancel != nil {
		c.managedCancel()
		<-c.managedDone
	}
	c.Scheduler.Stop()
	c.Evaluator.Stop()
	c.WebhookSubscriber.Stop()
	c.PRMergedSubscriber.Stop()
}

func (s *Service) runManagedAutomationDeliveryWorker(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.ReconcileManagedConversationDeliveries(ctx); err != nil {
				s.logger.Warn("managed automation delivery reconciliation failed", zap.Error(err))
			}
		}
	}
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
	prMergedSubscriber := NewGitHubPRMergedSubscriber(svc, eventBus, log)

	return &Components{
		Service:            svc,
		Scheduler:          scheduler,
		Evaluator:          evaluator,
		WebhookSubscriber:  webhookSubscriber,
		PRMergedSubscriber: prMergedSubscriber,
	}, nil
}
