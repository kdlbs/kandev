package changes

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"go.uber.org/zap"
)

const (
	turnChangeExportProcessorInterval    = 2 * time.Second
	turnChangeExportProcessorBatch       = 20
	turnChangeExportProcessorConcurrency = 2
	turnChangeReconcileEveryPasses       = 30
)

// TurnChangeExportClientProvider resolves the retained executor client for a
// session without creating or relaunching one.
type TurnChangeExportClientProvider interface {
	AcquireTurnChangeExporter(ctx context.Context, sessionID string) (CheckpointClient, func(), bool)
}

// TurnChangeRestoreStateProvider optionally reports a concrete in-flight
// executor restore. Resume defers only while that evidence exists.
type TurnChangeRestoreStateProvider interface {
	TurnChangeRestoreInProgress(sessionID string) bool
}

// TurnChangeExportSummaryPublisher publishes the affected summary after a
// content page settles.
type TurnChangeExportSummaryPublisher interface {
	PublishTurnChangeSummary(ctx context.Context, taskID, sessionID, changeSetID string) error
}

// ExportProcessor is the single task-owned bounded content processor. It does
// not schedule runs, relaunch executors, or extend writer intervals.
type ExportProcessor struct {
	repository  repository.TurnChangesRepository
	coordinator *Coordinator
	provider    TurnChangeExportClientProvider
	publisher   TurnChangeExportSummaryPublisher
	logger      *zap.Logger
	now         func() time.Time
	mu          sync.Mutex
	started     bool
	cancel      context.CancelFunc
	done        chan struct{}
}

func NewExportProcessor(repository repository.TurnChangesRepository, coordinator *Coordinator) *ExportProcessor {
	return &ExportProcessor{
		repository: repository, coordinator: coordinator,
		logger: zap.NewNop(), now: time.Now,
	}
}

func (p *ExportProcessor) SetClientProvider(provider TurnChangeExportClientProvider) {
	if p != nil {
		p.provider = provider
	}
}

func (p *ExportProcessor) SetSummaryPublisher(publisher TurnChangeExportSummaryPublisher) {
	if p != nil {
		p.publisher = publisher
	}
}

func (p *ExportProcessor) SetLogger(logger *zap.Logger) {
	if p == nil {
		return
	}
	if logger == nil {
		p.logger = zap.NewNop()
		return
	}
	p.logger = logger
}

// Start runs one processor for the lifetime of ctx. Repeated calls are no-ops.
func (p *ExportProcessor) Start(ctx context.Context) {
	if p == nil || p.repository == nil || p.coordinator == nil {
		return
	}
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	p.started, p.cancel, p.done = true, cancel, done
	p.mu.Unlock()
	go func() {
		defer close(done)
		p.loop(runCtx)
	}()
}

// Stop cancels the processor and waits for its in-flight pass to finish.
// Repeated calls are safe and all callers wait for the same run to drain.
func (p *ExportProcessor) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return
	}
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	done := p.done
	p.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (p *ExportProcessor) loop(ctx context.Context) {
	ticker := time.NewTicker(turnChangeExportProcessorInterval)
	defer ticker.Stop()
	for pass := 0; ; pass++ {
		if err := p.ProcessOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			p.logger.Warn("turn-change content processor pass failed", zap.Error(err))
		}
		if pass%turnChangeReconcileEveryPasses == 0 && p.coordinator != nil {
			if err := p.coordinator.ReconcileUnfinished(ctx); err != nil && !errors.Is(err, context.Canceled) {
				p.logger.Warn("turn-change reconciliation pass failed", zap.Error(err))
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ProcessOnce runs one bounded pass over eligible content rows.
func (p *ExportProcessor) ProcessOnce(ctx context.Context) error {
	if p == nil || p.repository == nil || p.provider == nil {
		return nil
	}
	if p.coordinator != nil {
		if err := p.coordinator.ResumeUnfinished(ctx, p.provider); err != nil && !errors.Is(err, context.Canceled) {
			p.logger.Warn("turn-change resume pass failed", zap.Error(err))
		}
	}
	candidates, err := p.repository.ListTurnChangeExportCandidates(ctx, p.now().UTC(), turnChangeExportProcessorBatch)
	if err != nil {
		return err
	}
	semaphore := make(chan struct{}, turnChangeExportProcessorConcurrency)
	var wait sync.WaitGroup
	for _, candidate := range candidates {
		if candidate == nil || ctx.Err() != nil {
			break
		}
		wait.Add(1)
		go func(row *models.TurnRepositoryChangeSet) {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-semaphore }()
			p.processCandidate(ctx, row)
		}(candidate)
	}
	wait.Wait()
	return ctx.Err()
}

func (p *ExportProcessor) processCandidate(ctx context.Context, row *models.TurnRepositoryChangeSet) {
	changeSet, err := p.repository.GetTurnChangeSetByID(ctx, row.TurnChangeSetID)
	if err != nil {
		p.logger.Warn("turn-change content row lost its change set",
			zap.String("change_set_id", row.TurnChangeSetID), zap.Error(err))
		return
	}
	client, release, ok := p.provider.AcquireTurnChangeExporter(ctx, changeSet.TaskSessionID)
	if !ok {
		if turnChangeRestoreInProgress(p.provider, changeSet.TaskSessionID) {
			return
		}
		if _, err := p.coordinator.failTurnRepositoryExport(ctx, *row, row.ExportClaimToken, models.TurnChangeReasonExportMissingObject); err != nil {
			p.logger.Warn("turn-change content source is unavailable",
				zap.String("change_set_id", changeSet.ID), zap.Error(err))
			return
		}
		_ = p.coordinator.settleContentProgress(ctx, changeSet.ID)
		if p.publisher != nil {
			_ = p.publisher.PublishTurnChangeSummary(ctx, changeSet.TaskID, changeSet.TaskSessionID, changeSet.ID)
		}
		return
	}
	defer release()
	if _, err := p.coordinator.preserveTerminalRepositoryContent(ctx, changeSet, *row, client); err != nil {
		p.logger.Warn("turn-change content preservation failed",
			zap.String("change_set_id", changeSet.ID), zap.String("checkout_id", row.CheckoutID), zap.Error(err))
		return
	}
	if err := p.coordinator.settleContentProgress(ctx, changeSet.ID); err != nil {
		p.logger.Warn("turn-change content settlement failed", zap.String("change_set_id", changeSet.ID), zap.Error(err))
		return
	}
	_ = p.coordinator.cleanupSettledCheckpointRefs(ctx, changeSet.ID, client)
	if p.publisher != nil {
		_ = p.publisher.PublishTurnChangeSummary(ctx, changeSet.TaskID, changeSet.TaskSessionID, changeSet.ID)
	}
}
