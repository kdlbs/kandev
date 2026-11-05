package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"go.uber.org/zap"
)

const (
	// pluginExecutorCleanupSweepInterval spaces the retries of resources whose
	// cleanup could not finish: handoffs and retained cleanup_pending rows.
	pluginExecutorCleanupSweepInterval = 5 * time.Minute
	// pluginExecutorCleanupSweepBatch bounds one pass so a backlog is worked
	// down over several ticks instead of holding provider calls open.
	pluginExecutorCleanupSweepBatch = 16
)

// pluginExecutorCleanupSweep owns the sweep goroutine: Start and Stop are
// idempotent and restartable.
type pluginExecutorCleanupSweep struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	// handoffCursor and pendingCursor are where the next pass resumes in each
	// list, so work that keeps failing cannot hold the batch.
	// They hold the last visited stable id, not an index, because the lists
	// shrink and reorder between passes.
	cursorMu                     sync.Mutex
	handoffCursor, pendingCursor string
}

// StartCleanupSweep retries unfinished plugin resource cleanup until
// StopCleanupSweep or ctx ends. Calling it while running is a no-op.
func (r *PluginRemoteExecutor) StartCleanupSweep(ctx context.Context) {
	sweep := &r.cleanupSweep
	sweep.mu.Lock()
	defer sweep.mu.Unlock()
	if sweep.cancel != nil {
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	sweep.cancel, sweep.done = cancel, done
	go func() {
		defer close(done)
		r.runCleanupSweep(loopCtx)
	}()
}

// StopCleanupSweep stops the sweep and waits for an in-flight pass to return.
func (r *PluginRemoteExecutor) StopCleanupSweep() {
	sweep := &r.cleanupSweep
	sweep.mu.Lock()
	cancel, done := sweep.cancel, sweep.done
	sweep.cancel, sweep.done = nil, nil
	sweep.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

func (r *PluginRemoteExecutor) runCleanupSweep(ctx context.Context) {
	timer := time.NewTimer(r.cleanupSweepInterval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if err := r.SweepPendingCleanup(ctx); err != nil && ctx.Err() == nil {
			r.logger.Warn("plugin executor cleanup sweep left work pending", zap.Error(err))
		}
		timer.Reset(r.cleanupSweepInterval)
	}
}

// SweepPendingCleanup makes one bounded pass over the durable cleanup work: the
// handoffs of resources a launch could not record, and rows still
// cleanup_pending after a destroy was not confirmed. Each target is destroyed
// under the cleanup claim by the same path the environment cleanup uses, so a
// busy claim or an unconfirmed destroy leaves the record for the next pass.
func (r *PluginRemoteExecutor) SweepPendingCleanup(ctx context.Context) error {
	if r.inventoryStore == nil {
		return nil
	}
	handoffs, err := r.inventoryStore.ListAllUnrecordedPluginExecutors(ctx)
	if err != nil {
		return fmt.Errorf("read unrecorded plugin executors: %w", err)
	}
	records, recordsErr := r.inventoryStore.ListExecutorsRunningPluginRemote(ctx)
	pending := cleanupPendingRecords(records)
	// Rows that fail every pass must not hold the batch: cleanup_pending rows
	// keep a reserved share, and each list resumes where the last pass stopped.
	handoffBudget := pluginExecutorCleanupSweepBatch - min(len(pending), pluginExecutorCleanupSweepBatch/2)
	var sweepErrors []error
	// A handoff adopted into a cleanup_pending row was just destroyed or failed
	// this pass, so its row is not retried in the same pass.
	triedAsHandoff := map[string]bool{}
	attempted := sweepRotated(ctx, &r.cleanupSweep, &r.cleanupSweep.handoffCursor, handoffs, handoffBudget, func(h *models.UnrecordedPluginExecutor) string { return h.ExecutionID }, func(handoff *models.UnrecordedPluginExecutor) {
		triedAsHandoff[handoff.ExecutionID] = true
		if err := r.sweepHandoff(ctx, handoff); r.sweepFailure(err, handoff.ExecutionID) {
			sweepErrors = append(sweepErrors, err)
		}
	})
	if recordsErr != nil {
		return errors.Join(append(sweepErrors, fmt.Errorf("read plugin executor cleanup inventory: %w", recordsErr))...)
	}
	sweepRotated(ctx, &r.cleanupSweep, &r.cleanupSweep.pendingCursor, pending, pluginExecutorCleanupSweepBatch-attempted, func(rec *models.ExecutorRunning) string { return rec.AgentExecutionID }, func(listed *models.ExecutorRunning) {
		// The listing predates the handoffs settled above, so re-read the row and
		// skip one that is gone or no longer owes cleanup.
		if triedAsHandoff[listed.AgentExecutionID] {
			return
		}
		record, err := r.currentPluginExecutorRecord(ctx, listed.SessionID, listed.AgentExecutionID)
		if errors.Is(err, models.ErrExecutionRotated) {
			return
		}
		if err != nil {
			sweepErrors = append(sweepErrors, err)
			return
		}
		environment, owed := cleanupPendingEnvironment(record)
		if !owed {
			return
		}
		if err := r.destroyPluginExecutorEnvironmentRecord(ctx, environment, record, nil); r.sweepFailure(err, record.AgentExecutionID) {
			sweepErrors = append(sweepErrors, err)
		}
	})
	if err := ctx.Err(); err != nil {
		sweepErrors = append(sweepErrors, err)
	}
	return errors.Join(sweepErrors...)
}

func cleanupPendingRecords(records []*models.ExecutorRunning) []*models.ExecutorRunning {
	pending := make([]*models.ExecutorRunning, 0, len(records))
	for _, record := range records {
		if _, owed := cleanupPendingEnvironment(record); owed {
			pending = append(pending, record)
		}
	}
	return pending
}

// rotate visits at most budget items in stable key order, starting after the
// key this list's previous pass ended on, and returns how many it visited. A key
// that has since disappeared is not needed: the pass resumes at the first key
// that sorts after it, wrapping to the start.
func sweepRotated[T any](ctx context.Context, sweep *pluginExecutorCleanupSweep, cursor *string, items []T, budget int, key func(T) string, visit func(T)) int {
	if len(items) == 0 || budget <= 0 {
		return 0
	}
	sorted := append([]T(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return key(sorted[i]) < key(sorted[j]) })
	sweep.cursorMu.Lock()
	last := *cursor
	sweep.cursorMu.Unlock()
	start := sort.Search(len(sorted), func(i int) bool { return key(sorted[i]) > last })
	if start == len(sorted) {
		start = 0
	}
	visited := 0
	for visited < min(budget, len(sorted)) && ctx.Err() == nil {
		item := sorted[(start+visited)%len(sorted)]
		visit(item)
		last = key(item)
		visited++
	}
	sweep.cursorMu.Lock()
	*cursor = last
	sweep.cursorMu.Unlock()
	return visited
}

// sweepFailure reports whether err is a failure of the pass. A task cleanup
// job in an active state owns the task's resources and runs with its own
// identity in the claim; the sweep has none, so it leaves the target to that
// job, which retries until it settles or fails terminally.
func (r *PluginRemoteExecutor) sweepFailure(err error, executionID string) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, repoerrors.ErrTaskCleanupInProgress) {
		r.logger.Debug("plugin executor cleanup sweep left a target to its task cleanup job", zap.String("execution_id", executionID))
		return false
	}
	return true
}

func (r *PluginRemoteExecutor) sweepHandoff(ctx context.Context, handoff *models.UnrecordedPluginExecutor) error {
	inventory, err := decodePluginExecutorInventory(handoff.Running.Metadata)
	if err != nil {
		return err
	}
	environment := &models.TaskEnvironment{ID: handoff.EnvironmentID, TaskID: handoff.TaskID, OwnershipGeneration: inventory.EnvironmentGeneration}
	if environment.OwnershipGeneration <= 0 {
		return fmt.Errorf("unrecorded plugin executor %s has no environment generation", handoff.ExecutionID)
	}
	if err := r.adoptUnrecordedPluginExecutor(ctx, environment, handoff); err != nil {
		return fmt.Errorf("adopt unrecorded plugin executor %s: %w", handoff.ExecutionID, err)
	}
	record, err := r.currentPluginExecutorRecord(ctx, handoff.SessionID, handoff.ExecutionID)
	if err != nil {
		return fmt.Errorf("unrecorded plugin executor %s has no inventory record: %w", handoff.ExecutionID, err)
	}
	return r.destroyPluginExecutorEnvironmentRecord(ctx, environment, record, handoff)
}

func pluginExecutorRecordMatchesEnvironment(environment *models.TaskEnvironment, record *models.ExecutorRunning) bool {
	return record != nil && record.TaskID == environment.TaskID && record.Runtime == agentruntime.RuntimePluginRemote
}

// cleanupPendingEnvironment returns the environment a retained cleanup_pending
// row belongs to.
func cleanupPendingEnvironment(record *models.ExecutorRunning) (*models.TaskEnvironment, bool) {
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil || !pluginExecutorCleanupOwed(inventory.Phase) || inventory.EnvironmentID == "" || inventory.EnvironmentGeneration <= 0 {
		return nil, false
	}
	return &models.TaskEnvironment{ID: inventory.EnvironmentID, TaskID: record.TaskID, OwnershipGeneration: inventory.EnvironmentGeneration}, true
}
