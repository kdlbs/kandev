package worktree

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// RecoveryProgressStart contains only durable authority and the selected
// inventory. Filesystem paths stay inside the worktree recovery journals.
type RecoveryProgressStart struct {
	TaskID                string
	SessionID             string
	TaskEnvironmentID     string
	OwnerTaskID           string
	OwnershipGeneration   int64
	OperationID           string
	ErrorStamp            string
	Kind                  string
	SelectedRepositoryIDs []string
	RepositoryTotal       int
	RunnerInstanceID      string
}

// RecoveryProgressBinding fences every callback to one admitted attempt.
type RecoveryProgressBinding struct {
	TaskEnvironmentID   string
	OwnerTaskID         string
	OwnershipGeneration int64
	SessionID           string
	OperationID         string
	AttemptID           string
	RunnerInstanceID    string
	Revision            int64
}

// RecoveryProgressUpdate is a complete path-free status projection.
type RecoveryProgressUpdate struct {
	State              string
	Phase              string
	RepositoryID       string
	RepositoryPosition int
	RepositoryTotal    int
	CompletedSlots     int
	WorkspaceComplete  bool
	AgentReady         bool
	ReasonCode         string
	EndedAt            *time.Time
}

// RecoveryProgressReporter persists and publishes admitted recovery progress.
type RecoveryProgressReporter interface {
	BeginWorkspaceRecovery(context.Context, RecoveryProgressStart) (RecoveryProgressBinding, error)
	UpdateWorkspaceRecovery(context.Context, RecoveryProgressBinding, RecoveryProgressUpdate) (RecoveryProgressBinding, error)
	HeartbeatWorkspaceRecovery(context.Context, RecoveryProgressBinding) (RecoveryProgressBinding, error)
	EndWorkspaceRecoveryRunner(context.Context, RecoveryProgressBinding)
}

// RecoveryProgressLiveReader is an optional early guard for unrelated
// workspace reconstruction while an admitted repair still owns its runner.
type RecoveryProgressLiveReader interface {
	WorkspaceRecoveryIsLive(context.Context, string) (bool, error)
}

// RecoveryProgressProjectionReader exposes the persisted operation and its
// exact in-process runner proof when a progress begin may have committed.
type RecoveryProgressProjectionReader interface {
	WorkspaceRecoveryProjection(context.Context, string) (*models.TaskEnvironmentRecoveryOperation, bool, error)
}

type recoveryProgressContextKey struct{}
type recoveryProgressLifecycleContextKey struct{}
type recoverySlotProgressContextKey struct{}

type recoverySlotProgress struct {
	repositoryID   string
	position       int
	total          int
	completedSlots int
}

type recoveryProgressTracker struct {
	mu       sync.Mutex
	reporter RecoveryProgressReporter
	binding  RecoveryProgressBinding
	update   RecoveryProgressUpdate
	err      error
	ended    bool
}

const recoveryProgressCleanupTimeout = 8 * time.Second

func recoveryProgressCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), recoveryProgressCleanupTimeout)
}

func withRecoveryProgress(ctx context.Context, tracker *recoveryProgressTracker) context.Context {
	return context.WithValue(ctx, recoveryProgressContextKey{}, tracker)
}

func recoveryProgressFromContext(ctx context.Context) *recoveryProgressTracker {
	if ctx == nil {
		return nil
	}
	tracker, _ := ctx.Value(recoveryProgressContextKey{}).(*recoveryProgressTracker)
	return tracker
}

// WithRecoveryLifecycleContext binds accepted recovery to backend shutdown.
// It is set by the orchestrator before admission and cannot be influenced by a
// browser request after the recovery claim has been acquired.
func WithRecoveryLifecycleContext(ctx, lifecycleCtx context.Context) context.Context {
	if ctx == nil || lifecycleCtx == nil {
		return ctx
	}
	return context.WithValue(ctx, recoveryProgressLifecycleContextKey{}, lifecycleCtx)
}

func acceptedRecoveryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.WithoutCancel(ctx)
	var operationCtx context.Context
	var cancel context.CancelFunc
	if deadline, ok := ctx.Deadline(); ok {
		operationCtx, cancel = context.WithDeadline(base, deadline)
	} else {
		operationCtx, cancel = context.WithCancel(base)
	}
	if lifecycleCtx, ok := ctx.Value(recoveryProgressLifecycleContextKey{}).(context.Context); ok && lifecycleCtx != nil {
		stop := context.AfterFunc(lifecycleCtx, cancel)
		return operationCtx, func() {
			stop()
			cancel()
		}
	}
	return operationCtx, cancel
}

func updateRecoveryProgress(ctx context.Context, update RecoveryProgressUpdate) error {
	tracker := recoveryProgressFromContext(ctx)
	if tracker == nil {
		return nil
	}
	return tracker.updateProgress(ctx, update)
}

func withRecoverySlotProgress(ctx context.Context, progress recoverySlotProgress) context.Context {
	return context.WithValue(ctx, recoverySlotProgressContextKey{}, progress)
}

func reportRecoveryPhase(ctx context.Context, phase string) error {
	if recoveryProgressFromContext(ctx) == nil {
		return nil
	}
	progress, _ := ctx.Value(recoverySlotProgressContextKey{}).(recoverySlotProgress)
	return updateRecoveryProgress(ctx, RecoveryProgressUpdate{
		State: "running", Phase: phase, RepositoryID: progress.repositoryID,
		RepositoryPosition: progress.position, RepositoryTotal: progress.total,
		CompletedSlots: progress.completedSlots,
	})
}

func (t *recoveryProgressTracker) currentUpdate() RecoveryProgressUpdate {
	if t == nil {
		return RecoveryProgressUpdate{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.update
}

func (t *recoveryProgressTracker) updateProgress(ctx context.Context, update RecoveryProgressUpdate) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.err != nil {
		return t.err
	}
	if t.ended {
		return errors.New("workspace recovery progress is already terminal")
	}
	update = mergeRecoveryProgressUpdate(t.update, update)
	nextBinding, err := t.reporter.UpdateWorkspaceRecovery(ctx, t.binding, update)
	if err != nil {
		t.err = err
		return err
	}
	t.binding = nextBinding
	t.update = update
	if update.State != "running" {
		t.ended = true
	}
	return nil
}

func (t *recoveryProgressTracker) heartbeat(ctx context.Context) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended || t.err != nil {
		return
	}
	nextBinding, err := t.reporter.HeartbeatWorkspaceRecovery(ctx, t.binding)
	if err != nil {
		t.err = err
		return
	}
	t.binding = nextBinding
}

func (t *recoveryProgressTracker) finish(ctx context.Context, state, phase, reason string, agentReady bool) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended {
		return nil
	}
	current := t.update
	current.State = state
	if phase != "" {
		current.Phase = phase
	}
	current.AgentReady = agentReady
	current.ReasonCode = reason
	current.EndedAt = timePointer(time.Now().UTC())
	current = mergeRecoveryProgressUpdate(t.update, current)
	nextBinding, err := t.reporter.UpdateWorkspaceRecovery(ctx, t.binding, current)
	if err != nil {
		if t.err == nil {
			t.err = err
		} else {
			t.err = errors.Join(t.err, err)
		}
		return err
	}
	t.binding = nextBinding
	t.update = current
	t.err = nil
	t.ended = true
	return nil
}

func mergeRecoveryProgressUpdate(current, update RecoveryProgressUpdate) RecoveryProgressUpdate {
	if update.RepositoryTotal == 0 {
		update.RepositoryTotal = current.RepositoryTotal
	}
	if update.RepositoryPosition == 0 {
		update.RepositoryPosition = current.RepositoryPosition
	}
	if update.RepositoryID == "" {
		update.RepositoryID = current.RepositoryID
	}
	if update.CompletedSlots == 0 {
		update.CompletedSlots = current.CompletedSlots
	}
	if current.WorkspaceComplete && !update.WorkspaceComplete {
		update.WorkspaceComplete = true
	}
	if update.State == "" {
		update.State = current.State
	}
	return update
}

func (t *recoveryProgressTracker) terminal() bool {
	if t == nil {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ended
}

func (t *recoveryProgressTracker) endRunner(ctx context.Context) {
	if t == nil || t.reporter == nil {
		return
	}
	t.reporter.EndWorkspaceRecoveryRunner(ctx, t.currentBinding())
}

func (t *recoveryProgressTracker) currentBinding() RecoveryProgressBinding {
	if t == nil {
		return RecoveryProgressBinding{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.binding
}

func timePointer(value time.Time) *time.Time { return &value }

type managedCloneRelocationErrorStampContextKey struct{}

// WithManagedCloneRelocationErrorStamp attaches the exact stamped error that
// authorized a user-initiated relocation.
func WithManagedCloneRelocationErrorStamp(ctx context.Context, stamp string) context.Context {
	if strings.TrimSpace(stamp) == "" {
		return ctx
	}
	return context.WithValue(ctx, managedCloneRelocationErrorStampContextKey{}, strings.TrimSpace(stamp))
}

func managedCloneRelocationErrorStampFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	stamp, _ := ctx.Value(managedCloneRelocationErrorStampContextKey{}).(string)
	return stamp
}
