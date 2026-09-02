package statussummary

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// SummaryStore is the small persistence boundary needed by the live
// projector. Keeping it local to this package avoids making the projector
// depend on the task repository package, which already imports this model.
type SummaryStore interface {
	LoadTaskStatusSummaries(ctx context.Context, taskIDs []string) (map[string]*TaskStatusSummary, error)
	CompareAndUpdateTaskStatusSummary(ctx context.Context, stored *StoredTaskStatusSummary) (bool, error)
}

// WorkspaceResolver fills in workspace ownership for source events that only
// carry task_id (session, message, and Git observations historically do not
// include workspace_id in their payloads).
type WorkspaceResolver func(context.Context, string) (string, error)

// GitObservation is the bounded source state for one repository. It is kept
// internal to the projector and never serialized into the task summary.
type GitObservation struct {
	Repository string
	Summary    GitSummary
}

// GitObservationLoader rehydrates the keyed repository observations that are
// needed when a projector is recreated after a process restart.
type GitObservationLoader func(context.Context, string) ([]GitObservation, error)

// PendingActionLoader returns the authoritative pending action for every
// input-capable session belonging to one task.
type PendingActionLoader func(context.Context, string) (map[string]string, error)

// SessionObservationSnapshot is the authoritative keyed session state needed
// to rebuild aggregates that cannot be recovered from their single summary
// representative after a restart or compare-and-set loss.
type SessionObservationSnapshot struct {
	Sessions         []RebuildSession
	ActivityObserved bool
	ErrorsObserved   bool
}

// SessionObservationLoader rehydrates session, activity, and error source
// observations for one task.
type SessionObservationLoader func(context.Context, string) (SessionObservationSnapshot, error)

// TaskLaunchErrorObservation reports whether the task-owned launch-error key
// was observed. A malformed key is not authoritative, so callers can retain
// the last valid summary while an absent key explicitly clears the projection.
type TaskLaunchErrorObservation struct {
	Error    *ActiveErrorSummary
	Observed bool
}

// TaskLaunchErrorLoader reads the bounded task-owned pre-session error source.
type TaskLaunchErrorLoader func(context.Context, string) (TaskLaunchErrorObservation, error)

// TaskActivityLoader rehydrates the durable maximum activity timestamp for a
// task when a projector starts with a legacy or incomplete summary row.
type TaskActivityLoader func(context.Context, string) (*time.Time, error)

// PullRequestLoader rehydrates the keyed PR observations needed to preserve
// sibling pull requests across projector restarts and CAS rebases.
type PullRequestLoader func(context.Context, string) ([]PullRequestInput, error)

// SummaryUpdated is the complete replacement payload sent to workspace
// subscribers. It intentionally contains no transcript, file list, or source
// event payload.
type SummaryUpdated struct {
	TaskID      string            `json:"task_id"`
	WorkspaceID string            `json:"workspace_id"`
	Summary     TaskStatusSummary `json:"status_summary"`
}

// GetWorkspaceID lets the WebSocket broadcaster route the event without
// reflecting into the replacement payload.
func (e SummaryUpdated) GetWorkspaceID() string { return e.WorkspaceID }

type ProjectorConfig struct {
	Store                   SummaryStore
	EventBus                bus.EventBus
	ResolveWorkspace        WorkspaceResolver
	LoadGitObservations     GitObservationLoader
	LoadPendingActions      PendingActionLoader
	LoadSessionObservations SessionObservationLoader
	LoadTaskLaunchError     TaskLaunchErrorLoader
	LoadTaskActivity        TaskActivityLoader
	LoadPullRequests        PullRequestLoader
	// CountQueuedPrompts returns the number of prompts currently en-queued for
	// a task across all of its sessions (pending semantics identical to
	// message.queue.get). Wired from the messagequeue service at the
	// composition root; nil disables queued-prompt projection.
	CountQueuedPrompts func(context.Context, string) (int, error)
	Logger             *logger.Logger
	Now                func() time.Time
}

// Projector converts authoritative, bounded occurrences into one complete
// task summary. It serializes source application and persistence per task so
// independent tasks do not block one another during a burst. Raw stream events
// are never subscribed to here.
type Projector struct {
	store                   SummaryStore
	eventBus                bus.EventBus
	resolveWorkspace        WorkspaceResolver
	loadGitObservations     GitObservationLoader
	loadPendingActions      PendingActionLoader
	loadSessionObservations SessionObservationLoader
	loadTaskLaunchError     TaskLaunchErrorLoader
	loadTaskActivity        TaskActivityLoader
	loadPullRequests        PullRequestLoader
	countQueuedPrompts      func(context.Context, string) (int, error)
	logger                  *logger.Logger
	now                     func() time.Time

	mu         sync.Mutex
	state      map[string]*projectionState
	taskLockMu sync.Mutex
	taskLocks  map[string]*taskProjectionLock
	subs       []bus.Subscription
}

type taskProjectionLock struct {
	mu   sync.Mutex
	refs int
}

type projectionState struct {
	workspaceID       string
	revision          uint64
	current           *TaskStatusSummary
	queuedCount       int
	lastActivityAt    *time.Time
	sessions          map[string]sessionObservation
	pending           map[string]string
	pendingRequests   map[string]pendingRequestIdentity
	taskPending       string
	pendingObserved   bool
	activityObserved  bool
	errors            map[string]*ActiveErrorSummary
	taskError         *ActiveErrorSummary
	taskErrorObserved bool
	// clearedErrorStamps records, per session, the stamp of the last error this
	// projection cleared, so a durable breadcrumb replayed on a later session
	// event cannot re-arm an error affordance the agent already recovered from.
	clearedErrorStamps map[string]string
	errorsObserved     bool
	git                map[string]GitSummary
	gitBaseline        *GitSummary
	gitObserved        bool
	prs                map[string]pullRequestObservation
	prBaseline         *PullRequestSummary
	prObserved         bool
}

type sessionObservation struct {
	id                  string
	state               string
	isPrimary           bool
	foregroundActivity  string
	activeSubagentCount int
}

type pendingRequestIdentity struct {
	messageType string
	pendingID   string
}

type pullRequestObservation struct {
	state                 string
	number                int
	url                   string
	reviewState           string
	checksState           string
	mergeableState        string
	mergeQueueState       string
	unresolvedReviewCount int
	pendingReviewCount    int
	requiredReviews       int
	checksTotal           int
	checksPassing         int
	autoFixEnabled        bool
	autoMergeEnabled      bool
}

func NewProjector(cfg ProjectorConfig) *Projector {
	log := cfg.Logger
	if log == nil {
		log = logger.Default()
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Projector{
		store:                   cfg.Store,
		eventBus:                cfg.EventBus,
		resolveWorkspace:        cfg.ResolveWorkspace,
		loadGitObservations:     cfg.LoadGitObservations,
		loadPendingActions:      cfg.LoadPendingActions,
		loadSessionObservations: cfg.LoadSessionObservations,
		loadTaskLaunchError:     cfg.LoadTaskLaunchError,
		loadTaskActivity:        cfg.LoadTaskActivity,
		loadPullRequests:        cfg.LoadPullRequests,
		countQueuedPrompts:      cfg.CountQueuedPrompts,
		logger:                  log.WithFields(zap.String("component", "task-status-summary-projector")),
		now:                     now,
		state:                   make(map[string]*projectionState),
		taskLocks:               make(map[string]*taskProjectionLock),
	}
}

// Start subscribes only to source occurrences that can affect a summary.
// In particular, it does not subscribe to agent.stream, shell, process,
// model, MCP, or per-message streaming subjects.
func (p *Projector) Start(ctx context.Context) error {
	if p == nil || p.eventBus == nil {
		return nil
	}
	patterns := []string{
		events.TaskCreated,
		events.TaskUpdated,
		events.TaskStateChanged,
		events.TaskSessionStateChanged,
		events.TaskSessionActivityChanged,
		events.TaskSessionErrorChanged,
		events.MessageAdded,
		events.MessageUpdated,
		events.MessageDeleted,
		events.TurnStarted,
		events.TurnCompleted,
		events.ClarificationAnswered,
		events.ClarificationPrimaryAnswered,
		events.ClarificationCancelled,
		events.ClarificationStaleDismissed,
		events.BuildPermissionRequestWildcardSubject(),
		events.BuildGitEventWildcardSubject(),
		events.GitHubTaskPRUpdated,
		events.GitHubTaskCIOptionsUpdated,
		events.MessageQueueStatusChanged,
	}
	for _, pattern := range patterns {
		sub, err := p.eventBus.Subscribe(pattern, p.handleEvent)
		if err != nil {
			p.Close()
			return fmt.Errorf("subscribe task status summary source %q: %w", pattern, err)
		}
		p.subs = append(p.subs, sub)
	}
	go func() {
		<-ctx.Done()
		p.Close()
	}()
	return nil
}

func (p *Projector) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	subs := p.subs
	p.subs = nil
	p.mu.Unlock()
	for _, sub := range subs {
		if sub != nil && sub.IsValid() {
			_ = sub.Unsubscribe()
		}
	}
}

// HandleEvent is exported for focused tests and for callers that already
// multiplex event-bus subscriptions. Start normally installs it directly.
func (p *Projector) HandleEvent(ctx context.Context, event *bus.Event) error {
	return p.handleEvent(ctx, event)
}

func (p *Projector) handleEvent(ctx context.Context, event *bus.Event) error {
	if event == nil {
		return nil
	}
	data, err := eventDataMap(event.Data)
	if err != nil {
		return err
	}
	taskID := stringField(data, "task_id")
	if taskID == "" {
		return nil
	}

	unlockTask := p.lockTask(taskID)
	defer unlockTask()

	state, err := p.ensureState(ctx, taskID)
	if err != nil {
		return err
	}
	if workspaceID := stringField(data, "workspace_id"); workspaceID != "" {
		state.workspaceID = workspaceID
	}
	if state.workspaceID == "" && p.resolveWorkspace != nil {
		state.workspaceID, err = p.resolveWorkspace(ctx, taskID)
		if err != nil {
			// Lifecycle purge publishes queue-status after DeleteTask commits.
			// Only skip verified not-found; transient resolve errors must surface.
			if event.Type == events.MessageQueueStatusChanged && isMissingTaskResolveErr(err) {
				p.dropProjectionState(taskID)
				p.logger.Debug("skipping queue status projection for missing task",
					zap.String("task_id", taskID),
					zap.Error(err))
				return nil
			}
			return fmt.Errorf("resolve task workspace %q: %w", taskID, err)
		}
	}
	if state.workspaceID == "" {
		if event.Type == events.MessageQueueStatusChanged {
			// ensureState may have inserted a placeholder; drop it so deleted
			// tasks do not retain projectionState for the process lifetime.
			p.dropProjectionState(taskID)
			p.logger.Debug("skipping queue status projection without workspace",
				zap.String("task_id", taskID))
			return nil
		}
		return fmt.Errorf("task status summary %q has no workspace", taskID)
	}
	pullRequestChanged := false
	if event.Type == events.GitHubTaskCIOptionsUpdated && p.loadPullRequests != nil {
		before := derivePullRequestSummary(state)
		if err := p.restorePullRequestObservations(ctx, taskID, state); err != nil {
			return err
		}
		pullRequestChanged = !equalPullRequestSummary(before, derivePullRequestSummary(state))
	}
	taskErrorChanged := false
	if p.loadTaskLaunchError != nil && isTaskErrorRefreshEvent(event.Type) {
		taskErrorChanged, err = p.refreshTaskLaunchError(ctx, taskID, state)
		if err != nil {
			return err
		}
	}

	if event.Type == events.MessageQueueStatusChanged {
		activityChanged := applyTaskActivityEventLocked(state, event.Type, data)
		pendingChanged := false
		if p.loadPendingActions != nil && !state.pendingObserved {
			var refreshErr error
			pendingChanged, refreshErr = p.refreshPendingLocked(ctx, taskID, state)
			if refreshErr != nil {
				return refreshErr
			}
		}
		return p.applyQueueStatusEvent(ctx, state, taskID, pendingChanged || activityChanged || taskErrorChanged, event.Type, data)
	}

	refreshPending := p.loadPendingActions != nil &&
		(!state.pendingObserved || isPendingSensitiveEvent(event.Type, data))
	if refreshPending {
		pendingChanged, refreshErr := p.refreshPendingLocked(ctx, taskID, state)
		if refreshErr != nil {
			return refreshErr
		}
		changed := p.applySourceEventLocked(state, event.Type, data) || pendingChanged || taskErrorChanged || pullRequestChanged
		return p.persistPendingRefreshLocked(ctx, taskID, state, changed, event.Type, data)
	}

	changed := p.applySourceEventLocked(state, event.Type, data) || taskErrorChanged || pullRequestChanged
	if !changed {
		return nil
	}
	return p.persistPendingRefreshLocked(ctx, taskID, state, true, event.Type, data)
}

const maxPendingPersistAttempts = 3

func (p *Projector) persistPendingRefreshLocked(
	ctx context.Context,
	taskID string,
	state *projectionState,
	changed bool,
	eventType string,
	eventData map[string]interface{},
) error {
	if !changed {
		return nil
	}
	for attempt := 0; attempt < maxPendingPersistAttempts; attempt++ {
		accepted, err := p.persistAndPublishLocked(ctx, taskID, state)
		if err != nil {
			return err
		}
		if accepted {
			return nil
		}
		// The rejected writer reloaded the winning summary into state.current.
		// Rebase every derived source before replaying this event; otherwise stale
		// observation maps can overwrite unrelated fields from the winner.
		if err := p.rebaseProjectionStateFromCurrent(ctx, taskID, state); err != nil {
			return err
		}
		if p.loadPendingActions != nil {
			if _, err := p.refreshPendingLocked(ctx, taskID, state); err != nil {
				return err
			}
		}
		if eventType != "" {
			p.applySourceEventLocked(state, eventType, eventData)
		}
	}
	p.logger.Warn("exhausted CAS retries refreshing pending task status",
		zap.String("task_id", taskID),
		zap.Int("attempts", maxPendingPersistAttempts))
	return fmt.Errorf(
		"exhausted CAS retries refreshing pending task status for task %s after %d attempts",
		taskID,
		maxPendingPersistAttempts,
	)
}

// maxQueueCountPersistAttempts bounds the retry loop when a competing writer
// keeps winning the compare-and-set. Each attempt re-reads the stored summary
// (which persistAndPublishLocked syncs into state on rejection) and re-queries
// the authoritative count, so a rejection can never suppress the update by
// leaving state.queuedCount ahead of the persisted value.
const maxQueueCountPersistAttempts = 3

// applyQueueStatusEvent refreshes the task's queued prompt count from the
// authoritative queue store. The event payload's per-session count is not
// reused: the badge is per-task across all sessions, and the queue may have
// changed between the status snapshot and this projection.
func (p *Projector) applyQueueStatusEvent(
	ctx context.Context,
	state *projectionState,
	taskID string,
	sourceChanged bool,
	eventType string,
	eventData map[string]interface{},
) error {
	for attempt := 0; attempt < maxQueueCountPersistAttempts; attempt++ {
		count, err := p.loadQueueCount(ctx, taskID, state)
		if err != nil {
			if persistErr := p.persistQueueSourceOnCountError(ctx, taskID, state, sourceChanged); persistErr != nil {
				return persistErr
			}
			return err
		}
		if !sourceChanged && count == state.queuedCount {
			return nil
		}
		accepted, err := p.persistQueueStatus(ctx, taskID, state, count)
		if err != nil {
			return err
		}
		if accepted {
			return nil
		}
		if err := p.rebaseQueueStatusEvent(ctx, taskID, state, eventType, eventData); err != nil {
			return err
		}
		sourceChanged = true
	}
	// The count self-corrects on the next queue event or list load, but a
	// sustained contention run is worth surfacing so a repeated rejector is not
	// silently starved.
	p.logger.Warn("exhausted CAS retries updating queued prompt count",
		zap.String("task_id", taskID),
		zap.Int("attempts", maxQueueCountPersistAttempts))
	return nil
}

func (p *Projector) loadQueueCount(ctx context.Context, taskID string, state *projectionState) (int, error) {
	if p.countQueuedPrompts == nil {
		return state.queuedCount, nil
	}
	count, err := p.countQueuedPrompts(ctx, taskID)
	if err != nil {
		return 0, fmt.Errorf("count queued prompts for task %q: %w", taskID, err)
	}
	return count, nil
}

func (p *Projector) persistQueueSourceOnCountError(
	ctx context.Context,
	taskID string,
	state *projectionState,
	sourceChanged bool,
) error {
	if !sourceChanged {
		return nil
	}
	_, err := p.persistAndPublishLocked(ctx, taskID, state)
	return err
}

func (p *Projector) persistQueueStatus(ctx context.Context, taskID string, state *projectionState, count int) (bool, error) {
	previousCount := state.queuedCount
	state.queuedCount = count
	accepted, err := p.persistAndPublishLocked(ctx, taskID, state)
	if err == nil {
		return accepted, nil
	}
	// Keep in-memory state aligned with the last persisted value so a later
	// zero-count event can retry instead of short-circuiting.
	state.queuedCount = previousCount
	// Delete cascades the summary row (FK) before the post-commit queue-status
	// event. With warm state the recount hits persist rather than resolveWorkspace.
	// Only suppress a verified gone-task failure; transient DB errors propagate.
	if count == 0 && isGoneTaskPersistErr(err) {
		p.logger.Debug("skipping queue status persist for gone task",
			zap.String("task_id", taskID),
			zap.Error(err))
		return true, nil
	}
	return false, err
}

func (p *Projector) rebaseQueueStatusEvent(
	ctx context.Context,
	taskID string,
	state *projectionState,
	eventType string,
	eventData map[string]interface{},
) error {
	// A competing writer may have changed any summary domain. Rebuild every
	// keyed source before recounting so the queue retry cannot overwrite the
	// winner with stale in-memory observations.
	if err := p.rebaseProjectionStateFromCurrent(ctx, taskID, state); err != nil {
		return err
	}
	if eventType != "" {
		applyTaskActivityEventLocked(state, eventType, eventData)
	}
	if p.loadPendingActions == nil {
		return nil
	}
	_, err := p.refreshPendingLocked(ctx, taskID, state)
	return err
}

func (p *Projector) lockTask(taskID string) func() {
	p.taskLockMu.Lock()
	lock := p.taskLocks[taskID]
	if lock == nil {
		lock = &taskProjectionLock{}
		p.taskLocks[taskID] = lock
	}
	lock.refs++
	p.taskLockMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		p.taskLockMu.Lock()
		lock.refs--
		if lock.refs == 0 && p.taskLocks[taskID] == lock {
			delete(p.taskLocks, taskID)
		}
		p.taskLockMu.Unlock()
	}
}

// dropProjectionState removes a task's in-memory projection entry. Call under
// the task lock after a queue-status event proves the task no longer exists.
func (p *Projector) dropProjectionState(taskID string) {
	p.mu.Lock()
	delete(p.state, taskID)
	p.mu.Unlock()
}

func (p *Projector) persistAndPublishLocked(ctx context.Context, taskID string, state *projectionState) (bool, error) {
	next := deriveSummary(state)
	if state.current != nil && state.current.SemanticEqual(next) {
		return true, nil
	}
	if state.revision == ^uint64(0) {
		return false, fmt.Errorf("task status summary %q revision overflow", taskID)
	}
	next.Revision = state.revision + 1
	next.UpdatedAt = p.now().UTC()
	if err := next.Validate(); err != nil {
		return false, fmt.Errorf("validate projected task status summary %q: %w", taskID, err)
	}
	if p.store == nil {
		return false, fmt.Errorf("task status summary store is unavailable")
	}
	accepted, err := p.store.CompareAndUpdateTaskStatusSummary(ctx, &StoredTaskStatusSummary{
		TaskID:      taskID,
		WorkspaceID: state.workspaceID,
		Summary:     next,
	})
	if err != nil {
		return false, fmt.Errorf("persist projected task status summary %q: %w", taskID, err)
	}
	if !accepted {
		rows, loadErr := p.store.LoadTaskStatusSummaries(ctx, []string{taskID})
		if loadErr != nil {
			return false, fmt.Errorf("reload projected task status summary %q: %w", taskID, loadErr)
		}
		if stored := rows[taskID]; stored != nil {
			state.current = cloneSummary(stored)
			state.revision = stored.Revision
			state.queuedCount = stored.QueuedPromptCount
			state.lastActivityAt = maxTimePtr(state.lastActivityAt, stored.LastActivityAt)
		}
		return false, nil
	}
	state.current = cloneSummary(&next)
	state.revision = next.Revision
	if p.eventBus != nil {
		payload := SummaryUpdated{TaskID: taskID, WorkspaceID: state.workspaceID, Summary: next}
		if err := p.eventBus.Publish(ctx, events.TaskStatusSummaryUpdated,
			bus.NewEvent(events.TaskStatusSummaryUpdated, "task-status-summary", payload)); err != nil {
			return false, fmt.Errorf("publish task status summary %q: %w", taskID, err)
		}
	}
	return true, nil
}
