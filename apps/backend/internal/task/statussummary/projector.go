package statussummary

import (
	"context"
	"fmt"
	"strings"
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
	Store               SummaryStore
	EventBus            bus.EventBus
	ResolveWorkspace    WorkspaceResolver
	LoadGitObservations GitObservationLoader
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
	store               SummaryStore
	eventBus            bus.EventBus
	resolveWorkspace    WorkspaceResolver
	loadGitObservations GitObservationLoader
	countQueuedPrompts  func(context.Context, string) (int, error)
	logger              *logger.Logger
	now                 func() time.Time

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
	workspaceID      string
	revision         uint64
	current          *TaskStatusSummary
	queuedCount      int
	sessions         map[string]sessionObservation
	pending          map[string]string
	taskPending      string
	pendingObserved  bool
	activityObserved bool
	errors           map[string]*ActiveErrorSummary
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

type pullRequestObservation struct {
	state                 string
	number                int
	url                   string
	reviewState           string
	checksState           string
	mergeableState        string
	unresolvedReviewCount int
	pendingReviewCount    int
	requiredReviews       int
	checksTotal           int
	checksPassing         int
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
		store:               cfg.Store,
		eventBus:            cfg.EventBus,
		resolveWorkspace:    cfg.ResolveWorkspace,
		loadGitObservations: cfg.LoadGitObservations,
		countQueuedPrompts:  cfg.CountQueuedPrompts,
		logger:              log.WithFields(zap.String("component", "task-status-summary-projector")),
		now:                 now,
		state:               make(map[string]*projectionState),
		taskLocks:           make(map[string]*taskProjectionLock),
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
		events.TaskUpdated,
		events.TaskSessionStateChanged,
		events.TaskSessionActivityChanged,
		events.TaskSessionErrorChanged,
		events.MessageAdded,
		events.MessageUpdated,
		events.MessageDeleted,
		events.ClarificationAnswered,
		events.ClarificationPrimaryAnswered,
		events.ClarificationCancelled,
		events.ClarificationStaleDismissed,
		events.BuildPermissionRequestWildcardSubject(),
		events.BuildGitEventWildcardSubject(),
		events.GitHubTaskPRUpdated,
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
			return fmt.Errorf("resolve task workspace %q: %w", taskID, err)
		}
	}
	if state.workspaceID == "" {
		return fmt.Errorf("task status summary %q has no workspace", taskID)
	}

	if event.Type == events.MessageQueueStatusChanged {
		return p.applyQueueStatusEvent(ctx, state, taskID)
	}

	changed := p.applySourceEventLocked(state, event.Type, data)
	if !changed {
		return nil
	}
	_, err = p.persistAndPublishLocked(ctx, taskID, state)
	return err
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
func (p *Projector) applyQueueStatusEvent(ctx context.Context, state *projectionState, taskID string) error {
	if p.countQueuedPrompts == nil {
		return nil
	}
	for attempt := 0; attempt < maxQueueCountPersistAttempts; attempt++ {
		count, err := p.countQueuedPrompts(ctx, taskID)
		if err != nil {
			return fmt.Errorf("count queued prompts for task %q: %w", taskID, err)
		}
		if count == state.queuedCount {
			return nil
		}
		state.queuedCount = count
		accepted, err := p.persistAndPublishLocked(ctx, taskID, state)
		if err != nil {
			return err
		}
		if accepted {
			return nil
		}
	}
	// The count self-corrects on the next queue event or list load, but a
	// sustained contention run is worth surfacing so a repeated rejector is not
	// silently starved.
	p.logger.Warn("exhausted CAS retries updating queued prompt count",
		zap.String("task_id", taskID),
		zap.Int("attempts", maxQueueCountPersistAttempts))
	return nil
}

func (p *Projector) applySourceEventLocked(state *projectionState, eventType string, data map[string]interface{}) bool {
	switch eventType {
	case events.TaskUpdated:
		return p.applyTaskUpdatedEventLocked(state, data)
	case events.TaskSessionStateChanged:
		return p.applySessionEventLocked(state, data)
	case events.TaskSessionActivityChanged:
		return p.applyActivityEventLocked(state, data)
	case events.TaskSessionErrorChanged:
		return p.applyErrorEventLocked(state, data)
	case events.MessageAdded, events.MessageUpdated, events.MessageDeleted:
		return p.applyMessageEventLocked(state, eventType, data)
	case events.ClarificationAnswered, events.ClarificationPrimaryAnswered,
		events.ClarificationCancelled, events.ClarificationStaleDismissed:
		return p.clearPendingLocked(state, stringField(data, "session_id"))
	case events.PermissionRequestReceived:
		return applyPermissionEventLocked(state, data)
	case events.GitEvent:
		return p.applyGitEventLocked(state, data)
	case events.GitHubTaskPRUpdated:
		return p.applyPREventLocked(state, data)
	default:
		return false
	}
}

func applyPermissionEventLocked(state *projectionState, data map[string]interface{}) bool {
	sessionID := stringField(data, "session_id")
	if sessionID == "" || state.pending[sessionID] == pendingPermission {
		return false
	}
	state.pendingObserved = true
	state.pending[sessionID] = pendingPermission
	return true
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

func (p *Projector) applyTaskUpdatedEventLocked(state *projectionState, data map[string]interface{}) bool {
	primarySessionID, primaryChanged := p.applyTaskPrimaryUpdateLocked(state, data)
	pendingChanged := applyTaskPendingUpdateLocked(state, data, primarySessionID)
	return primaryChanged || pendingChanged
}

func (p *Projector) applyTaskPrimaryUpdateLocked(state *projectionState, data map[string]interface{}) (string, bool) {
	if _, ok := data["foreground_activity"]; ok {
		state.activityObserved = true
	}
	if _, ok := data["active_subagent_count"]; ok {
		state.activityObserved = true
	}
	primaryValue, primaryPresent := data["primary_session_id"]
	if !primaryPresent {
		return "", false
	}
	primarySessionID := stringFromNullable(primaryValue)
	changed := updatePrimaryFlagsLocked(state, primarySessionID)
	if primarySessionID == "" {
		return primarySessionID, changed
	}
	observation := state.sessions[primarySessionID]
	observation.id = primarySessionID
	observation.isPrimary = true
	changed = updatePrimaryObservation(&observation, data) || changed
	state.sessions[primarySessionID] = observation
	return primarySessionID, changed
}

func updatePrimaryFlagsLocked(state *projectionState, primarySessionID string) bool {
	changed := false
	for sessionID, observation := range state.sessions {
		wantPrimary := primarySessionID != "" && sessionID == primarySessionID
		if observation.isPrimary == wantPrimary {
			continue
		}
		observation.isPrimary = wantPrimary
		state.sessions[sessionID] = observation
		changed = true
	}
	return changed
}

func updatePrimaryObservation(observation *sessionObservation, data map[string]interface{}) bool {
	changed := false
	if stateValue := stringFromNullable(data["primary_session_state"]); stateValue != "" && observation.state != stateValue {
		observation.state = stateValue
		changed = true
	}
	if activity, ok := data["foreground_activity"]; ok {
		value := stringFromNullable(activity)
		if observation.foregroundActivity != value {
			observation.foregroundActivity = value
			changed = true
		}
	}
	if count, ok := intValue(data["active_subagent_count"]); ok {
		count = maxInt(count, 0)
		if observation.activeSubagentCount != count {
			observation.activeSubagentCount = count
			changed = true
		}
	}
	return changed
}

func applyTaskPendingUpdateLocked(state *projectionState, data map[string]interface{}, primarySessionID string) bool {
	changed := false
	if pending, ok := data["task_pending_action"]; ok {
		state.pendingObserved = true
		value := stringFromNullable(pending)
		if len(state.pending) > 0 {
			state.pending = make(map[string]string)
			changed = true
		}
		if state.taskPending != value {
			state.taskPending = value
			changed = true
		}
	}
	if pending, ok := data["primary_session_pending_action"]; ok && primarySessionID != "" {
		state.pendingObserved = true
		changed = updateSessionPendingLocked(state, primarySessionID, stringFromNullable(pending)) || changed
	}
	return changed
}

func updateSessionPendingLocked(state *projectionState, sessionID, action string) bool {
	if action == "" {
		if _, exists := state.pending[sessionID]; exists {
			delete(state.pending, sessionID)
			return true
		}
		return false
	}
	if state.pending[sessionID] == action {
		return false
	}
	state.pending[sessionID] = action
	return true
}

func newProjectionState() *projectionState {
	return &projectionState{
		sessions:           make(map[string]sessionObservation),
		pending:            make(map[string]string),
		errors:             make(map[string]*ActiveErrorSummary),
		clearedErrorStamps: make(map[string]string),
		git:                make(map[string]GitSummary),
		prs:                make(map[string]pullRequestObservation),
	}
}

// ensureState performs persistence and source rehydration outside the global
// projector mutex. The per-task lock held by handleEvent still serializes all
// updates for this task, while unrelated tasks remain responsive during I/O.
func (p *Projector) ensureState(ctx context.Context, taskID string) (*projectionState, error) {
	p.mu.Lock()
	if state := p.state[taskID]; state != nil {
		p.mu.Unlock()
		return state, nil
	}
	p.mu.Unlock()

	state := newProjectionState()
	if err := p.restorePersistedState(ctx, taskID, state); err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if existing := p.state[taskID]; existing != nil {
		return existing, nil
	}
	p.state[taskID] = state
	return state, nil
}

func (p *Projector) restorePersistedState(ctx context.Context, taskID string, state *projectionState) error {
	if p.store == nil {
		return nil
	}
	rows, err := p.store.LoadTaskStatusSummaries(ctx, []string{taskID})
	if err != nil {
		return fmt.Errorf("load task status summary %q: %w", taskID, err)
	}
	summary := rows[taskID]
	if summary == nil {
		return nil
	}
	state.current = cloneSummary(summary)
	state.revision = summary.Revision
	state.queuedCount = summary.QueuedPromptCount
	state.taskPending = summary.PendingAction
	if summary.PrimarySession != nil && summary.PrimarySession.ID != "" {
		state.sessions[summary.PrimarySession.ID] = sessionObservation{
			id:        summary.PrimarySession.ID,
			state:     summary.PrimarySession.State,
			isPrimary: true,
		}
	}
	if summary.ActiveError != nil && summary.ActiveError.SessionID != "" {
		copy := *summary.ActiveError
		state.errors[summary.ActiveError.SessionID] = &copy
	}
	if summary.Git != nil {
		copy := *summary.Git
		state.gitBaseline = &copy
		if err := p.restoreGitObservations(ctx, taskID, state); err != nil {
			return err
		}
	}
	if summary.PullRequest != nil {
		copy := *summary.PullRequest
		state.prBaseline = &copy
	}
	return nil
}

func (p *Projector) restoreGitObservations(ctx context.Context, taskID string, state *projectionState) error {
	if p.loadGitObservations == nil {
		return nil
	}
	observations, err := p.loadGitObservations(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load Git observations for task status summary %q: %w", taskID, err)
	}
	for _, observation := range observations {
		if strings.TrimSpace(observation.Repository) == "" {
			continue
		}
		state.git[observation.Repository] = observation.Summary
	}
	state.gitObserved = true
	return nil
}

func (p *Projector) applySessionEventLocked(state *projectionState, data map[string]interface{}) bool {
	sessionID := firstString(data, "session_id", "primary_session_id")
	if sessionID == "" {
		return false
	}
	observation := state.sessions[sessionID]
	observation.id = sessionID
	if value := firstString(data, "new_state", "state", "primary_session_state"); value != "" {
		observation.state = value
	}
	if value, ok := data["is_primary"].(bool); ok {
		observation.isPrimary = value
		if value {
			for otherID, other := range state.sessions {
				if otherID == sessionID || !other.isPrimary {
					continue
				}
				other.isPrimary = false
				state.sessions[otherID] = other
			}
		}
	} else if stringField(data, "primary_session_id") == sessionID {
		observation.isPrimary = true
	}
	if value, ok := data["foreground_activity"]; ok {
		state.activityObserved = true
		observation.foregroundActivity = stringFromNullable(value)
	}
	if value, ok := intValue(data["active_subagent_count"]); ok {
		state.activityObserved = true
		observation.activeSubagentCount = maxInt(value, 0)
	}
	state.sessions[sessionID] = observation

	changed := true
	if metadata, ok := data["session_metadata"].(map[string]interface{}); ok {
		p.applySessionMetadataErrorLocked(state, sessionID, metadata)
	}
	return changed
}

// applySessionMetadataErrorLocked folds the session's durable error record into
// the projection. A dismissed or superseded record clears the affordance, and a
// record this projection already cleared is not re-applied — session metadata
// keeps failures as a breadcrumb, so replaying it verbatim would re-arm errors
// the agent has since recovered from.
func (p *Projector) applySessionMetadataErrorLocked(
	state *projectionState,
	sessionID string,
	metadata map[string]interface{},
) {
	errSummary, observed := errorFromMetadata(p.now().UTC(), sessionID, metadata)
	if !observed {
		return
	}
	state.errorsObserved = true
	if errSummary == nil {
		p.clearErrorLocked(state, sessionID)
		return
	}
	if state.clearedErrorStamps[sessionID] == errSummary.Stamp {
		return
	}
	state.errors[sessionID] = errSummary
}

func (p *Projector) applyActivityEventLocked(state *projectionState, data map[string]interface{}) bool {
	sessionID := stringField(data, "session_id")
	if sessionID == "" {
		return false
	}
	observation := state.sessions[sessionID]
	observation.id = sessionID
	if value, ok := data["foreground_activity"]; ok {
		state.activityObserved = true
		observation.foregroundActivity = stringFromNullable(value)
	}
	if value, ok := intValue(data["active_subagent_count"]); ok {
		state.activityObserved = true
		observation.activeSubagentCount = maxInt(value, 0)
	}
	state.sessions[sessionID] = observation
	return true
}

func (p *Projector) applyErrorEventLocked(state *projectionState, data map[string]interface{}) bool {
	sessionID := stringField(data, "session_id")
	if sessionID == "" {
		return false
	}
	state.errorsObserved = true
	if active, ok := data["active"].(bool); ok && !active {
		return p.clearErrorLocked(state, sessionID)
	}
	errSummary, ok := errorFromMap(p.now().UTC(), sessionID, data)
	if !ok {
		return false
	}
	if errorEqual(state.errors[sessionID], errSummary) {
		return false
	}
	// An explicit error event is authoritative: it re-arms even a stamp this
	// projection cleared earlier, which is what makes an identical failure
	// recurring after a recovery visible again.
	delete(state.clearedErrorStamps, sessionID)
	state.errors[sessionID] = errSummary
	return true
}

func (p *Projector) applyMessageEventLocked(state *projectionState, eventType string, data map[string]interface{}) bool {
	sessionID := stringField(data, "session_id")
	if sessionID == "" {
		return false
	}
	changed := p.clearSupersededErrorFromMessageLocked(state, eventType, sessionID, data)
	pendingChanged := p.applyPendingMessageLocked(state, eventType, data, sessionID)
	return changed || pendingChanged
}

func (p *Projector) clearSupersededErrorFromMessageLocked(
	state *projectionState,
	eventType, sessionID string,
	data map[string]interface{},
) bool {
	if eventType != events.MessageAdded || stringField(data, "author_type") != "agent" {
		return false
	}
	messageType := strings.ToLower(stringField(data, "type"))
	metadata, _ := data["metadata"].(map[string]interface{})
	if messageType == "error" || messageType == "status" || stringField(metadata, "variant") == "error" {
		return false
	}
	return p.clearErrorLocked(state, sessionID)
}

func (p *Projector) applyPendingMessageLocked(state *projectionState, eventType string, data map[string]interface{}, sessionID string) bool {
	messageType := strings.ToLower(stringField(data, "type"))
	metadata, _ := data["metadata"].(map[string]interface{})
	status := strings.ToLower(stringField(metadata, "status"))
	isPendingMessage := boolValue(data["requests_input"]) ||
		messageType == "clarification_request" || messageType == "permission_request"
	isTrackedMessage := isPendingMessage || stringField(metadata, "pending_id") != "" || status != ""
	if !isTrackedMessage {
		return false
	}
	state.pendingObserved = true
	// A terminal status means the prompt is no longer awaiting the user, whatever
	// the message type. Resolutions land as a message.updated on the request row
	// itself (approved/rejected/expired for permissions, answered/rejected/
	// cancelled/expired for clarifications), so gating the clear on "not a
	// request message" left the task-list affordance armed after the user had
	// already answered. Only the empty and "pending" statuses keep it armed —
	// that covers a detached-but-answerable clarification, which stays pending.
	if eventType == events.MessageDeleted || (status != "" && status != statusPending) {
		return p.clearPendingLocked(state, sessionID)
	}
	action := pendingPermission
	if messageType == "clarification_request" || boolValue(data["requests_input"]) {
		action = pendingClarification
	}
	if state.pending[sessionID] == action {
		return false
	}
	state.pending[sessionID] = action
	return true
}

func (p *Projector) applyGitEventLocked(state *projectionState, data map[string]interface{}) bool {
	if stringField(data, "type") != "status_update" {
		return false
	}
	status, _ := data["status"].(map[string]interface{})
	if status == nil {
		return false
	}
	repository := firstString(status, "repository_name", "repository")
	if repository == "" {
		repository = stringField(data, "session_id")
	}
	if repository == "" {
		return false
	}
	if !state.gitObserved {
		state.git = make(map[string]GitSummary)
		state.gitBaseline = nil
		state.gitObserved = true
	}
	observation := GitSummary{
		Additions:    nonNegativeInt(status, "branch_additions", "additions"),
		Deletions:    nonNegativeInt(status, "branch_deletions", "deletions"),
		ChangedFiles: changedFileCount(status),
		Ahead:        nonNegativeInt(status, "ahead"),
		Behind:       nonNegativeInt(status, "behind"),
	}
	if equalGitSummary(state.git[repository], observation) {
		return false
	}
	state.git[repository] = observation
	return true
}

func (p *Projector) applyPREventLocked(state *projectionState, data map[string]interface{}) bool {
	key := pullRequestObservationKey(data)
	if key == "" {
		return false
	}
	if !state.prObserved {
		state.prs = make(map[string]pullRequestObservation)
		state.prBaseline = nil
		state.prObserved = true
	}
	observation := pullRequestObservation{
		state:                 stringField(data, "state"),
		number:                intValueOrZero(data["pr_number"]),
		url:                   stringField(data, "pr_url"),
		reviewState:           stringField(data, "review_state"),
		checksState:           stringField(data, "checks_state"),
		mergeableState:        stringField(data, "mergeable_state"),
		unresolvedReviewCount: intValueOrZero(data["unresolved_review_threads"]),
		pendingReviewCount:    intValueOrZero(data["pending_review_count"]),
		checksTotal:           intValueOrZero(data["checks_total"]),
		checksPassing:         intValueOrZero(data["checks_passing"]),
	}
	if value, ok := intValue(data["required_reviews"]); ok {
		observation.requiredReviews = maxInt(value, 0)
	}
	if existing, ok := state.prs[key]; ok && existing == observation {
		return false
	}
	state.prs[key] = observation
	return true
}

func pullRequestObservationKey(data map[string]interface{}) string {
	repository := firstString(data, "repository_id", "repository")
	number := intValueOrZero(data["pr_number"])
	url := firstString(data, "pr_url", "url")
	if repository != "" && number > 0 {
		return fmt.Sprintf("%s#%d", repository, number)
	}
	if url != "" {
		return url
	}
	if repository != "" {
		return repository
	}
	if number > 0 {
		return fmt.Sprintf("#%d", number)
	}
	return ""
}

// persistAndPublishLocked persists the projected summary and publishes the
// replacement event. The boolean reports whether the write was accepted (or a
// no-op because the derived summary is already current); false means a
// competing writer won and the stored summary was reloaded into state,
// including queuedCount, so callers can retry against the refreshed revision.
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

func deriveSummary(state *projectionState) TaskStatusSummary {
	primary, foregroundActivity, activeSubagentCount := deriveSessionFields(state)
	return TaskStatusSummary{
		PrimarySession:      primary,
		ForegroundActivity:  foregroundActivity,
		ActiveSubagentCount: activeSubagentCount,
		PendingAction:       derivePendingAction(state),
		ActiveError:         deriveActiveError(state),
		Git:                 deriveGitSummary(state),
		PullRequest:         derivePullRequestSummary(state),
		QueuedPromptCount:   state.queuedCount,
	}
}

func deriveSessionFields(state *projectionState) (*PrimarySessionSummary, string, int) {
	var primary *sessionObservation
	activities := make([]string, 0, len(state.sessions))
	activeSubagentCount := 0
	for _, session := range state.sessions {
		if session.isPrimary && (primary == nil || session.id < primary.id) {
			copy := session
			primary = &copy
		}
		if state.activityObserved {
			if session.state == "RUNNING" || session.foregroundActivity == activityBackground {
				activities = append(activities, session.foregroundActivity)
			}
			if isActiveSessionState(session.state) {
				activeSubagentCount += maxInt(session.activeSubagentCount, 0)
			}
		}
	}
	var primarySummary *PrimarySessionSummary
	if primary != nil {
		primarySummary = &PrimarySessionSummary{ID: primary.id, State: primary.state}
	}
	foregroundActivity := ""
	if state.activityObserved {
		foregroundActivity = deriveForegroundActivity(activities)
	} else if state.current != nil {
		foregroundActivity = state.current.ForegroundActivity
		activeSubagentCount = state.current.ActiveSubagentCount
	}
	return primarySummary, foregroundActivity, activeSubagentCount
}

func isActiveSessionState(state string) bool {
	return state == "STARTING" || state == "RUNNING" || state == "WAITING_FOR_INPUT"
}

func deriveForegroundActivity(activities []string) string {
	hasBackground := false
	for _, activity := range activities {
		if activity == activityGenerating {
			return activityGenerating
		}
		if activity == activityBackground {
			hasBackground = true
		}
	}
	if hasBackground {
		return activityBackground
	}
	return ""
}

func derivePendingAction(state *projectionState) string {
	if !state.pendingObserved && state.current != nil {
		return state.current.PendingAction
	}
	action := state.taskPending
	for _, candidate := range state.pending {
		if candidate == pendingPermission {
			return candidate
		}
		if candidate == pendingClarification && action == "" {
			action = candidate
		}
	}
	return action
}

func deriveActiveError(state *projectionState) *ActiveErrorSummary {
	if !state.errorsObserved && state.current != nil && state.current.ActiveError != nil {
		copy := *state.current.ActiveError
		return &copy
	}
	var active *ActiveErrorSummary
	for _, candidate := range state.errors {
		if candidate == nil || (active != nil && !candidate.OccurredAt.After(active.OccurredAt)) {
			continue
		}
		copy := *candidate
		active = &copy
	}
	return active
}

func deriveGitSummary(state *projectionState) *GitSummary {
	if !state.gitObserved && state.gitBaseline != nil {
		copy := *state.gitBaseline
		return &copy
	}
	var git GitSummary
	for _, observation := range state.git {
		git.Additions += observation.Additions
		git.Deletions += observation.Deletions
		git.ChangedFiles += observation.ChangedFiles
		git.Ahead += observation.Ahead
		git.Behind += observation.Behind
	}
	if git == (GitSummary{}) {
		return nil
	}
	return &git
}
