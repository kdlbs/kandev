// Package deliveryretention runs bounded cleanup against the task-owned
// durable delivery journal.
package deliveryretention

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/maintenance"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"go.uber.org/zap"
)

// BackgroundBatchRows bounds event-retention writer leases under the configured candidate ceiling.
const BackgroundBatchRows = 50

const (
	defaultInterval = time.Minute
	passBudget      = 5 * time.Second
	batchBudget     = 250 * time.Millisecond
	defaultRows     = 200
	defaultBytes    = 4 << 20
	maxRetryBackoff = 5 * time.Minute
	failureDeadline = "deadline"
)

var (
	rowsTotal        = expvar.NewMap("agent_delivery_retention_rows_total")
	bytesTotal       = expvar.NewMap("agent_delivery_retention_bytes_total")
	batchesTotal     = expvar.NewMap("agent_delivery_retention_batches_total")
	skippedTotal     = expvar.NewMap("agent_delivery_retention_skipped_total")
	errorsTotal      = expvar.NewMap("agent_delivery_retention_errors_total")
	backlogRows      = expvar.NewInt("agent_delivery_retention_backlog_rows")
	backlogLimited   = expvar.NewInt("agent_delivery_retention_backlog_sample_limited")
	oldestAgeSeconds = expvar.NewInt("agent_delivery_retention_oldest_eligible_age_seconds")
	lastSuccess      = expvar.NewInt("agent_delivery_retention_last_success_unixtime")
	durationBuckets  = expvar.NewMap("agent_delivery_retention_batch_duration_seconds")
)

// Repository runs one atomic and independently bounded transaction per method.
type Repository interface {
	RunAgentDeliveryRetentionBatch(context.Context, models.AgentDeliveryRetentionBatchOptions) (models.AgentDeliveryRetentionBatchResult, error)
	RunAgentDeliveryOrphanBatch(context.Context, models.AgentDeliveryOrphanBatchOptions) (models.AgentDeliveryOrphanBatchResult, error)
	RunAgentDeliveryRetirementBatch(context.Context, models.AgentDeliveryRetirementBatchOptions) (models.AgentDeliveryRetirementBatchResult, error)
}

type Options struct {
	Interval     time.Duration
	BatchRows    int
	BatchBytes   int64
	PayloadGrace time.Duration
	ReplayAge    time.Duration
	ReplayEvents int
	Readiness    func(context.Context) error
	RetryDelay   func(int) time.Duration
	After        func(time.Duration) <-chan time.Time
	Now          func() time.Time
	Logger       *logger.Logger
	OnSnapshot   func(Snapshot)
}

// Snapshot contains only aggregate values. It never carries session or stream identity.
type Snapshot struct {
	ContinuationPending      bool
	Duration                 time.Duration
	Batches                  int
	CommittedBatches         int
	DiscoveryBatches         int
	BusyBatches              int
	FailedBatches            int
	Candidates               int
	ProtectedCandidates      int
	EventPayloadsPruned      int
	PromptPayloadsPruned     int
	EventRowsRetired         int
	RetirementRowsPurged     int
	RetirementJobsCompleted  int
	OrphanJobsScheduled      int
	UnboundWorkflowRows      int
	PayloadBytesPruned       int64
	PayloadBytesRead         int64
	SampleLimitReached       bool
	BacklogSampleAvailable   bool
	BacklogSampleRows        int
	OldestEligibleAgeSeconds int64
	FailureReason            string
	StructuralFailure        bool
	ReadinessRecovered       bool
}

type Worker struct {
	repo Repository
	pool *db.Pool
	opts Options

	lifecycleMu  sync.Mutex
	mu           sync.Mutex
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	suspended    bool
	retryAttempt int
	discovery    discoveryProgress
	batchRows    map[string]int
}

func New(repo Repository, pool *db.Pool, options Options) *Worker {
	if options.Interval <= 0 {
		options.Interval = defaultInterval
	}
	if options.Interval < 10*time.Second {
		options.Interval = 10 * time.Second
	}
	if options.BatchRows <= 0 || options.BatchRows > defaultRows {
		options.BatchRows = defaultRows
	}
	if options.BatchBytes <= 0 || options.BatchBytes > defaultBytes {
		options.BatchBytes = defaultBytes
	}
	if options.PayloadGrace <= 0 {
		options.PayloadGrace = time.Hour
	}
	if options.ReplayAge <= 0 {
		options.ReplayAge = 24 * time.Hour
	}
	if options.ReplayEvents <= 0 {
		options.ReplayEvents = 10000
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.After == nil {
		options.After = time.After
	}
	if options.RetryDelay == nil {
		options.RetryDelay = jitteredRetryDelay
	}
	return &Worker{repo: repo, pool: pool, opts: options}
}

func (w *Worker) Start(ctx context.Context) error {
	if w == nil || w.repo == nil || w.pool == nil {
		return errors.New("delivery retention worker requires a repository and database pool")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	w.mu.Lock()
	if w.cancel != nil {
		w.mu.Unlock()
		return nil
	}
	workerCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.wg.Add(1)
	w.mu.Unlock()
	go w.loop(workerCtx)
	return nil
}

func (w *Worker) Stop() {
	if w == nil {
		return
	}
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	w.mu.Lock()
	cancel := w.cancel
	w.cancel = nil
	w.mu.Unlock()
	if cancel != nil {
		cancel()
		w.wg.Wait()
	}
}

func (w *Worker) loop(ctx context.Context) {
	defer w.wg.Done()
	defer func() { w.discovery = discoveryProgress{} }()
	for ctx.Err() == nil {
		if w.suspended {
			readiness := w.checkReadiness(ctx)
			if !readiness.ReadinessRecovered {
				w.report(readiness)
				w.retryAttempt++
				if !wait(ctx, w.opts.After, w.opts.RetryDelay(w.retryAttempt)) {
					return
				}
				continue
			}
		}
		snapshot := w.runPass(ctx)
		if w.suspended && snapshot.FailedBatches == 0 && snapshot.CommittedBatches > 0 {
			w.suspended = false
			snapshot.ReadinessRecovered = true
		}
		w.report(snapshot)
		delay := w.opts.Interval
		switch {
		case snapshot.StructuralFailure:
			w.suspended = true
			w.retryAttempt++
			delay = w.opts.RetryDelay(w.retryAttempt)
		case snapshot.FailedBatches > 0 || w.suspended:
			w.retryAttempt++
			delay = w.opts.RetryDelay(w.retryAttempt)
		default:
			w.retryAttempt = 0
			if snapshot.ContinuationPending {
				continue
			}
		}
		w.discovery = discoveryProgress{}
		if !wait(ctx, w.opts.After, delay) {
			return
		}
	}
}

func (w *Worker) checkReadiness(ctx context.Context) Snapshot {
	snapshot := Snapshot{OldestEligibleAgeSeconds: -1, StructuralFailure: true}
	if w.opts.Readiness == nil {
		snapshot.FailedBatches = 1
		snapshot.FailureReason = "schema"
		errorsTotal.Add("schema", 1)
		return snapshot
	}
	release, acquired := maintenance.ForPool(w.pool).TryAcquire()
	if !acquired {
		snapshot.BusyBatches = 1
		batchesTotal.Add("busy", 1)
		return snapshot
	}
	defer release()
	checkCtx, cancel := context.WithTimeout(ctx, batchBudget)
	defer cancel()
	if err := w.opts.Readiness(checkCtx); err != nil {
		snapshot.FailedBatches = 1
		snapshot.FailureReason = classifyError(err)
		if snapshot.FailureReason != failureDeadline {
			errorsTotal.Add(snapshot.FailureReason, 1)
		}
		return snapshot
	}
	snapshot.StructuralFailure = false
	snapshot.ReadinessRecovered = true
	return snapshot
}

func (w *Worker) runPass(ctx context.Context) Snapshot {
	started := w.opts.Now()
	deadline := started.Add(passBudget)
	snapshot := Snapshot{OldestEligibleAgeSeconds: -1}
	phases := []string{"retention", "orphans", "retirements"}
	for len(phases) > 0 {
		if ctx.Err() != nil {
			break
		}
		remaining := deadline.Sub(w.opts.Now())
		if remaining < batchBudget {
			snapshot.ContinuationPending = committedRetentionWork(snapshot) > 0
			break
		}
		phase := phases[0]
		phases = phases[1:]
		before := committedRetentionWork(snapshot)
		if !w.runPassBatch(ctx, phase, remaining, &snapshot) {
			break
		}
		if committedRetentionWork(snapshot) > before {
			phases = append(phases, phase)
		}
	}
	snapshot.Duration = w.opts.Now().Sub(started)
	if !snapshot.BacklogSampleAvailable {
		backlogRows.Set(-1)
		backlogLimited.Set(-1)
	} else {
		backlogRows.Set(int64(snapshot.BacklogSampleRows))
		if snapshot.SampleLimitReached {
			backlogLimited.Set(1)
		} else {
			backlogLimited.Set(0)
		}
	}
	oldestAgeSeconds.Set(snapshot.OldestEligibleAgeSeconds)
	recordDuration(snapshot.Duration)
	return snapshot
}

func (w *Worker) runPassBatch(ctx context.Context, phase string, remaining time.Duration, snapshot *Snapshot) bool {

	snapshot.Batches++
	batchCtx, cancel := context.WithTimeout(ctx, minDuration(batchBudget, remaining))
	release, acquired := maintenance.ForPool(w.pool).TryAcquire()
	if !acquired {
		cancel()
		snapshot.BusyBatches++
		batchesTotal.Add("busy", 1)
		return true
	}
	committed, err := w.runBatch(batchCtx, phase, snapshot)
	release()
	cancel()
	if err != nil {
		snapshot.FailedBatches++
		reason := classifyError(err)
		snapshot.FailureReason = reason
		w.recordBatchFailure(ctx, phase, reason, err)
		if reason != failureDeadline {
			errorsTotal.Add(reason, 1)
		}
		if reason == failureDeadline {
			batchesTotal.Add("deadline", 1)
		} else {
			batchesTotal.Add("error", 1)
		}
		if isStructuralError(err) {
			snapshot.StructuralFailure = true
			return false
		}
		return true
	}
	if committed {
		snapshot.CommittedBatches++
		batchesTotal.Add("committed", 1)
		lastSuccess.Set(w.opts.Now().Unix())
	}
	return true
}

func (w *Worker) runBatch(ctx context.Context, phase string, snapshot *Snapshot) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	workTimeout := batchBudget
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < workTimeout {
			workTimeout = remaining
		}
	}
	batchRows := w.batchRowLimit(phase)
	switch phase {
	case "retention":
		result, err := w.repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{
			MaxCandidates: batchRows, MaxPayloadBytes: w.opts.BatchBytes, WorkTimeout: workTimeout,
			PayloadGrace: w.opts.PayloadGrace, ReplayAge: w.opts.ReplayAge, ReplayEvents: w.opts.ReplayEvents,
		})
		if err != nil {
			return false, err
		}
		if w.discovery.advance(result.DiscoveryBefore, result.DiscoveryAfter) {
			snapshot.DiscoveryBatches++
		}
		snapshot.Candidates += result.Candidates
		snapshot.BacklogSampleAvailable = true
		snapshot.BacklogSampleRows += result.EligibleCandidates
		snapshot.SampleLimitReached = snapshot.SampleLimitReached || result.Candidates >= batchRows
		if result.OldestEligibleAgeSeconds > snapshot.OldestEligibleAgeSeconds {
			snapshot.OldestEligibleAgeSeconds = result.OldestEligibleAgeSeconds
		}
		snapshot.EventPayloadsPruned += result.EventPayloadsPruned
		snapshot.PromptPayloadsPruned += result.PromptPayloadsPruned
		snapshot.EventRowsRetired += result.EventRowsRetired
		snapshot.PayloadBytesPruned += result.PayloadBytesPruned
		snapshot.PayloadBytesRead += result.PayloadBytesRead
		snapshot.ProtectedCandidates += protectedCandidateCount(result.Candidates,
			result.EventPayloadsPruned+result.PromptPayloadsPruned+result.EventRowsRetired)
		rowsTotal.Add("inbox_payload", int64(result.EventPayloadsPruned))
		rowsTotal.Add("prompt_payload", int64(result.PromptPayloadsPruned))
		rowsTotal.Add("event_receipt", int64(result.EventRowsRetired))
		bytesTotal.Add("inbox", result.PayloadBytesPruned-result.PromptPayloadBytesPruned)
		bytesTotal.Add("prompt", result.PromptPayloadBytesPruned)
	case "orphans":
		result, err := w.repo.RunAgentDeliveryOrphanBatch(ctx, models.AgentDeliveryOrphanBatchOptions{
			MaxCandidates: batchRows, WorkTimeout: workTimeout,
		})
		if err != nil {
			return false, err
		}
		snapshot.Candidates += result.Candidates
		snapshot.OrphanJobsScheduled += result.JobsScheduled
		snapshot.UnboundWorkflowRows += result.UnboundWorkflowRows
		rowsTotal.Add("orphan", int64(result.JobsScheduled))
		if result.UnboundWorkflowRows > 0 {
			skippedTotal.Add("unknown", int64(result.UnboundWorkflowRows))
		}
	case "retirements":
		result, err := w.repo.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{
			MaxCandidates: batchRows, WorkTimeout: workTimeout,
		})
		if err != nil {
			return false, err
		}
		snapshot.RetirementRowsPurged += result.RowsPurged
		snapshot.RetirementJobsCompleted += result.Completed
		rowsTotal.Add("deleted_session", int64(result.RowsPurged))
	default:
		return false, fmt.Errorf("unknown delivery retention batch %q", phase)
	}
	return true, nil
}

func committedRetentionWork(snapshot Snapshot) int {
	return snapshot.EventPayloadsPruned + snapshot.PromptPayloadsPruned + snapshot.EventRowsRetired +
		snapshot.RetirementRowsPurged + snapshot.OrphanJobsScheduled + snapshot.RetirementJobsCompleted + snapshot.DiscoveryBatches
}

func (w *Worker) report(snapshot Snapshot) {
	if w.opts.OnSnapshot != nil {
		w.opts.OnSnapshot(snapshot)
	}
	if w.opts.Logger == nil {
		return
	}
	fields := []zap.Field{
		zap.Duration("duration", snapshot.Duration),
		zap.Bool("continuation_pending", snapshot.ContinuationPending),
		zap.Int("batches", snapshot.Batches),
		zap.Int("committed_batches", snapshot.CommittedBatches),
		zap.Int("discovery_batches", snapshot.DiscoveryBatches),
		zap.Int("busy_batches", snapshot.BusyBatches),
		zap.Int("failed_batches", snapshot.FailedBatches),
		zap.Int("candidate_sample_rows", snapshot.Candidates),
		zap.Bool("sample_limit_reached", snapshot.SampleLimitReached),
		zap.Bool("backlog_sample_available", snapshot.BacklogSampleAvailable),
		zap.Int("eligible_backlog_sample_rows", snapshot.BacklogSampleRows),
		zap.Bool("readiness_recovered", snapshot.ReadinessRecovered),
		zap.Int("protected_candidates", snapshot.ProtectedCandidates),
		zap.Int("event_payloads_pruned", snapshot.EventPayloadsPruned),
		zap.Int("prompt_payloads_pruned", snapshot.PromptPayloadsPruned),
		zap.Int("event_rows_retired", snapshot.EventRowsRetired),
		zap.Int("retirement_rows_purged", snapshot.RetirementRowsPurged),
		zap.Int("retirement_jobs_completed", snapshot.RetirementJobsCompleted),
		zap.Int("orphan_jobs_scheduled", snapshot.OrphanJobsScheduled),
		zap.Int("unbound_workflow_rows", snapshot.UnboundWorkflowRows),
		zap.Int64("payload_bytes_read", snapshot.PayloadBytesRead),
		zap.Int64("payload_bytes_pruned", snapshot.PayloadBytesPruned),
		zap.Int64("oldest_eligible_age_seconds", snapshot.OldestEligibleAgeSeconds),
	}
	if snapshot.FailureReason != "" {
		fields = append(fields, zap.String("reason", snapshot.FailureReason))
		w.opts.Logger.Warn("agent_delivery.retention.snapshot", fields...)
		return
	}
	w.opts.Logger.Info("agent_delivery.retention.snapshot", fields...)
}

func wait(ctx context.Context, after func(time.Duration) <-chan time.Time, delay time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-after(delay):
		return ctx.Err() == nil
	}
}

func classifyError(err error) string {
	if errors.Is(err, repoerrors.ErrAgentDeliveryReceiptCorrupt) || errors.Is(err, repoerrors.ErrAgentDeliveryHashVersion) {
		return "hash"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return failureDeadline
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "format"):
		return "format"
	case strings.Contains(message, "schema") || strings.Contains(message, "no such table") || strings.Contains(message, "does not exist"):
		return "schema"
	case strings.Contains(message, "hash"):
		return "hash"
	case strings.Contains(message, "lock") || strings.Contains(message, "busy") || strings.Contains(message, "serializ"):
		return "lock"
	default:
		return "storage"
	}
}

func isStructuralError(err error) bool {
	if errors.Is(err, repoerrors.ErrAgentDeliveryReceiptCorrupt) || errors.Is(err, repoerrors.ErrAgentDeliveryHashVersion) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "format") || strings.Contains(message, "schema") ||
		strings.Contains(message, "no such table") || strings.Contains(message, "does not exist") ||
		strings.Contains(message, "missing index") || strings.Contains(message, "disk full") ||
		strings.Contains(message, "no space left")
}

func jitteredRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	bound := time.Second
	for i := 1; i < attempt && bound < maxRetryBackoff; i++ {
		bound *= 2
	}
	if bound > maxRetryBackoff {
		bound = maxRetryBackoff
	}
	minimum := bound / 2
	return minimum + time.Duration(rand.Int64N(int64(bound-minimum)+1))
}

func protectedCandidateCount(candidates, changed int) int {
	if candidates <= changed {
		return 0
	}
	return candidates - changed
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func recordDuration(duration time.Duration) {
	seconds := duration.Seconds()
	switch {
	case seconds <= 0.05:
		durationBuckets.Add("le_0_05", 1)
	case seconds <= 0.1:
		durationBuckets.Add("le_0_1", 1)
	case seconds <= 0.25:
		durationBuckets.Add("le_0_25", 1)
	default:
		durationBuckets.Add("overflow", 1)
	}
}
