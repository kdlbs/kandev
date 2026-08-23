package statussummary

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/events"
)

func (p *Projector) applySourceEventLocked(state *projectionState, eventType string, data map[string]interface{}) bool {
	activityChanged := applyTaskActivityEventLocked(state, eventType, data)
	var sourceChanged bool
	switch eventType {
	case events.TaskCreated, events.TaskStateChanged:
		// These lifecycle events have no additional bounded status fields. Their
		// persisted timestamps are handled by applyTaskActivityEventLocked.
		sourceChanged = false
	case events.TaskUpdated:
		sourceChanged = p.applyTaskUpdatedEventLocked(state, data)
	case events.TaskSessionStateChanged:
		sourceChanged = p.applySessionEventLocked(state, data)
	case events.TaskSessionActivityChanged:
		sourceChanged = p.applyActivityEventLocked(state, data)
	case events.TaskSessionErrorChanged:
		sourceChanged = p.applyErrorEventLocked(state, data)
	case events.MessageAdded, events.MessageUpdated, events.MessageDeleted:
		sourceChanged = p.applyMessageEventLocked(state, eventType, data)
	case events.TurnStarted, events.TurnCompleted:
		// Turn timestamps are the bounded activity source. The turn payload does
		// not contribute another task-row status field.
		sourceChanged = false
	case events.ClarificationAnswered, events.ClarificationPrimaryAnswered,
		events.ClarificationCancelled:
		if p.loadPendingActions != nil {
			sourceChanged = false
			break
		}
		sourceChanged = p.clearPendingLocked(state, stringField(data, "session_id"))
	case events.ClarificationStaleDismissed:
		if p.loadPendingActions != nil {
			sourceChanged = false
			break
		}
		sessionID := stringField(data, "session_id")
		dismissedID := stringField(data, "pending_id")
		if dismissedID != "" {
			if current, ok := state.pendingRequests[sessionID]; ok && current.pendingID != dismissedID {
				sourceChanged = false
				break
			}
		}
		sourceChanged = p.clearPendingLocked(state, sessionID)
	case events.PermissionRequestReceived:
		if p.loadPendingActions != nil {
			sourceChanged = false
			break
		}
		sourceChanged = applyPermissionEventLocked(state, data)
	case events.GitEvent:
		sourceChanged = p.applyGitEventLocked(state, data)
	case events.GitHubTaskPRUpdated:
		sourceChanged = p.applyPREventLocked(state, data)
	}
	return activityChanged || sourceChanged
}

func applyTaskActivityEventLocked(state *projectionState, eventType string, data map[string]interface{}) bool {
	var candidate time.Time
	switch eventType {
	case events.TaskCreated, events.TaskUpdated, events.TaskStateChanged:
		candidate = timeValue(data["updated_at"])
		if candidate.IsZero() {
			candidate = timeValue(data["created_at"])
		}
	case events.MessageAdded:
		if stringField(data, "author_type") != messageTypeUser {
			return false
		}
		candidate = timeValue(data["created_at"])
	case events.MessageQueueStatusChanged:
		if !isUserQueuedPrompt(data) {
			return false
		}
		candidate = timeValue(data["queued_at"])
	case events.TurnStarted:
		candidate = timeValue(data["started_at"])
	case events.TurnCompleted:
		candidate = timeValue(data["completed_at"])
	default:
		return false
	}
	return advanceTaskActivity(state, candidate)
}

func isUserQueuedPrompt(data map[string]interface{}) bool {
	queuedBy := stringField(data, "queued_by")
	return queuedBy != "" && queuedBy != "agent" && queuedBy != "workflow" && queuedBy != "server"
}

func advanceTaskActivity(state *projectionState, candidate time.Time) bool {
	if candidate.IsZero() {
		return false
	}
	candidate = candidate.UTC()
	if state.lastActivityAt != nil && !candidate.After(*state.lastActivityAt) {
		return false
	}
	state.lastActivityAt = &candidate
	return true
}

func applyPermissionEventLocked(state *projectionState, data map[string]interface{}) bool {
	sessionID := stringField(data, "session_id")
	if sessionID == "" {
		return false
	}
	identity := pendingRequestIdentity{
		messageType: messageTypePermissionRequest,
		pendingID:   stringField(data, "pending_id"),
	}
	if state.pending[sessionID] == pendingPermission {
		if _, exists := state.pendingRequests[sessionID]; !exists {
			state.pendingRequests[sessionID] = identity
		}
		return false
	}
	state.pendingObserved = true
	state.pending[sessionID] = pendingPermission
	state.pendingRequests[sessionID] = identity
	return true
}

func (p *Projector) applyTaskUpdatedEventLocked(state *projectionState, data map[string]interface{}) bool {
	primarySessionID, primaryChanged := p.applyTaskPrimaryUpdateLocked(state, data)
	if p.loadPendingActions != nil {
		return primaryChanged
	}
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
			state.pendingRequests = make(map[string]pendingRequestIdentity)
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
		_, existed := state.pending[sessionID]
		delete(state.pending, sessionID)
		delete(state.pendingRequests, sessionID)
		return existed
	}
	if state.pending[sessionID] == action {
		return false
	}
	state.pending[sessionID] = action
	delete(state.pendingRequests, sessionID)
	return true
}

func newProjectionState() *projectionState {
	return &projectionState{
		sessions:           make(map[string]sessionObservation),
		pending:            make(map[string]string),
		pendingRequests:    make(map[string]pendingRequestIdentity),
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
	if p.store != nil {
		rows, err := p.store.LoadTaskStatusSummaries(ctx, []string{taskID})
		if err != nil {
			return fmt.Errorf("load task status summary %q: %w", taskID, err)
		}
		summary := rows[taskID]
		if summary != nil {
			state.current = cloneSummary(summary)
			state.revision = summary.Revision
			applySummaryBaseline(state, summary)
		}
	}
	if err := p.restoreTaskActivity(ctx, taskID, state); err != nil {
		return err
	}
	if err := p.restoreSessionObservations(ctx, taskID, state); err != nil {
		return err
	}
	if err := p.restoreTaskLaunchError(ctx, taskID, state); err != nil {
		return err
	}
	if err := p.restoreGitObservations(ctx, taskID, state); err != nil {
		return err
	}
	if err := p.restorePullRequestObservations(ctx, taskID, state); err != nil {
		return err
	}
	return nil
}

func applySummaryBaseline(state *projectionState, summary *TaskStatusSummary) {
	state.queuedCount = summary.QueuedPromptCount
	state.taskPending = summary.PendingAction
	state.lastActivityAt = maxTimePtr(state.lastActivityAt, summary.LastActivityAt)
	if summary.PrimarySession != nil && summary.PrimarySession.ID != "" {
		state.sessions[summary.PrimarySession.ID] = sessionObservation{
			id:        summary.PrimarySession.ID,
			state:     summary.PrimarySession.State,
			isPrimary: true,
		}
	}
	if summary.ActiveError != nil {
		copy := *summary.ActiveError
		copy.RecoveryActions = normalizeRecoveryActions(copy.RecoveryActions)
		if copy.SessionID != "" {
			state.errors[copy.SessionID] = &copy
		} else {
			state.taskError = &copy
		}
	}
	if summary.Git != nil {
		copy := *summary.Git
		state.gitBaseline = &copy
	}
	if summary.PullRequest != nil {
		copy := *summary.PullRequest
		state.prBaseline = &copy
	}
}

func (p *Projector) restoreTaskActivity(ctx context.Context, taskID string, state *projectionState) error {
	if p.loadTaskActivity == nil || state.lastActivityAt != nil {
		return nil
	}
	activityAt, err := p.loadTaskActivity(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load task activity for task status summary %q: %w", taskID, err)
	}
	state.lastActivityAt = maxTimePtr(state.lastActivityAt, activityAt)
	return nil
}

// rebaseProjectionStateFromCurrent rebuilds all derived source state from the
// summary that won a rejected compare-and-set. Pending authority and the
// triggering source event are refreshed by the caller before retrying.
func (p *Projector) rebaseProjectionStateFromCurrent(
	ctx context.Context,
	taskID string,
	state *projectionState,
) error {
	if state.current == nil {
		return nil
	}
	current := state.current
	previousLastActivityAt := cloneTimePtr(state.lastActivityAt)
	state.sessions = make(map[string]sessionObservation)
	state.activityObserved = false
	state.lastActivityAt = previousLastActivityAt
	state.pending = make(map[string]string)
	state.pendingRequests = make(map[string]pendingRequestIdentity)
	state.pendingObserved = false
	state.errors = make(map[string]*ActiveErrorSummary)
	state.errorsObserved = false
	state.taskError = nil
	state.taskErrorObserved = false
	state.git = make(map[string]GitSummary)
	state.gitBaseline = nil
	state.gitObserved = false
	state.prs = make(map[string]pullRequestObservation)
	state.prBaseline = nil
	state.prObserved = false
	applySummaryBaseline(state, current)
	if err := p.restoreTaskActivity(ctx, taskID, state); err != nil {
		return err
	}
	if err := p.restoreSessionObservations(ctx, taskID, state); err != nil {
		return err
	}
	if err := p.restoreTaskLaunchError(ctx, taskID, state); err != nil {
		return err
	}
	if err := p.restoreGitObservations(ctx, taskID, state); err != nil {
		return err
	}
	if err := p.restorePullRequestObservations(ctx, taskID, state); err != nil {
		return err
	}
	return nil
}

func (p *Projector) restoreSessionObservations(
	ctx context.Context,
	taskID string,
	state *projectionState,
) error {
	if p.loadSessionObservations == nil {
		return nil
	}
	snapshot, err := p.loadSessionObservations(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load session observations for task status summary %q: %w", taskID, err)
	}
	sessions := make(map[string]sessionObservation, len(snapshot.Sessions))
	errorsBySession := make(map[string]*ActiveErrorSummary, len(snapshot.Sessions))
	for _, input := range snapshot.Sessions {
		sessionID := strings.TrimSpace(input.ID)
		if sessionID == "" {
			continue
		}
		sessions[sessionID] = sessionObservation{
			id:                  sessionID,
			state:               input.State,
			isPrimary:           input.IsPrimary,
			foregroundActivity:  input.ForegroundActivity,
			activeSubagentCount: maxInt(input.ActiveSubagentCount, 0),
		}
		activeError := normalizeRebuildError(input.ActiveError, p.now().UTC())
		if activeError == nil || state.clearedErrorStamps[sessionID] == activeError.Stamp {
			continue
		}
		activeError.SessionID = sessionID
		errorsBySession[sessionID] = activeError
	}
	state.sessions = sessions
	state.activityObserved = snapshot.ActivityObserved
	if snapshot.ErrorsObserved {
		state.errors = errorsBySession
		state.errorsObserved = true
	}
	return nil
}

func (p *Projector) restoreTaskLaunchError(
	ctx context.Context,
	taskID string,
	state *projectionState,
) error {
	if p.loadTaskLaunchError == nil {
		return nil
	}
	observation, err := p.loadTaskLaunchError(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load task launch error for status summary %q: %w", taskID, err)
	}
	if !observation.Observed {
		return nil
	}
	state.taskErrorObserved = true
	state.taskError = normalizeRebuildError(observation.Error, p.now().UTC())
	return nil
}

func (p *Projector) refreshTaskLaunchError(
	ctx context.Context,
	taskID string,
	state *projectionState,
) (bool, error) {
	observation, err := p.loadTaskLaunchError(ctx, taskID)
	if err != nil {
		return false, fmt.Errorf("refresh task launch error for status summary %q: %w", taskID, err)
	}
	if !observation.Observed {
		return false, nil
	}
	next := normalizeRebuildError(observation.Error, p.now().UTC())
	if errorEqual(state.taskError, next) && state.taskErrorObserved {
		if state.current == nil && next != nil {
			return true, nil
		}
		return false, nil
	}
	state.taskErrorObserved = true
	state.taskError = next
	return true, nil
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

func (p *Projector) restorePullRequestObservations(
	ctx context.Context,
	taskID string,
	state *projectionState,
) error {
	if p.loadPullRequests == nil {
		return nil
	}
	pullRequests, err := p.loadPullRequests(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load PR observations for task status summary %q: %w", taskID, err)
	}
	state.prs = make(map[string]pullRequestObservation, len(pullRequests))
	state.prBaseline = nil
	state.prObserved = true
	applyPullRequestInputs(state, pullRequests)
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
	if eventType != events.MessageAdded || stringField(data, "author_type") != messageTypeAgent {
		return false
	}
	messageType := strings.ToLower(stringField(data, "type"))
	metadata, _ := data["metadata"].(map[string]interface{})
	if messageType == messageTypeError || messageType == messageTypeStatus || stringField(metadata, "variant") == "error" {
		return false
	}
	return p.clearErrorLocked(state, sessionID)
}

func (p *Projector) applyPendingMessageLocked(state *projectionState, eventType string, data map[string]interface{}, sessionID string) bool {
	if p.loadPendingActions != nil {
		return false
	}
	messageType := strings.ToLower(stringField(data, "type"))
	metadata, _ := data["metadata"].(map[string]interface{})
	status := strings.ToLower(stringField(metadata, "status"))
	requestIdentity := pendingRequestIdentity{
		messageType: messageType,
		pendingID:   stringField(metadata, "pending_id"),
	}
	action := pendingActionForMessage(messageType, boolValue(data["requests_input"]))
	if action == "" {
		return false
	}
	state.pendingObserved = true
	// A terminal status on the request row means the prompt is no longer awaiting
	// the user. Resolutions land as a message.updated on that row (approved/
	// rejected/expired for permissions, answered/rejected/cancelled/expired for
	// clarifications). Only the request that armed the affordance may clear it;
	// a detached-but-answerable clarification stays pending.
	if eventType == events.MessageDeleted || (status != "" && status != statusPending) {
		if state.pendingRequests[sessionID] == requestIdentity {
			return p.clearPendingLocked(state, sessionID)
		}
		return false
	}
	if state.pending[sessionID] == action {
		if _, exists := state.pendingRequests[sessionID]; !exists {
			state.pendingRequests[sessionID] = requestIdentity
		}
		return false
	}
	state.pending[sessionID] = action
	state.pendingRequests[sessionID] = requestIdentity
	return true
}

// pendingActionForMessage maps a message to the task-row affordance it drives,
// or "" when the message asks the user for nothing. A message that asks for
// nothing is not evidence about the pending state in either direction, so the
// caller ignores it rather than letting it arm or clear the affordance.
//
// The distinction matters because a status of "pending" is not exclusive to
// prompts: every announced tool call is persisted with the raw ACP tool status,
// which starts at "pending" and only becomes in_progress/complete on the first
// tool_update. Treating those rows as prompts made the amber "waiting on you"
// icon flash onto the task row for every tool call the agent made (and stick
// whenever a turn ended before its last tool_update landed), while their
// terminal updates tore down a genuinely pending prompt — a background tool
// completing would clear the icon while the foreground turn was still blocked.
// The exact request types are matched before the generic requests_input flag, so
// a permission row that ever starts carrying the flag stays a permission rather
// than silently degrading to a clarification.
func pendingActionForMessage(messageType string, requestsInput bool) string {
	if messageType == messageTypePermissionRequest {
		return pendingPermission
	}
	if messageType == messageTypeClarificationRequest || requestsInput {
		return pendingClarification
	}
	return ""
}

func (p *Projector) applyGitEventLocked(state *projectionState, data map[string]interface{}) bool {
	if stringField(data, "type") != messageTypeStatusUpdate {
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
		Additions:             nonNegativeInt(status, "branch_additions", "additions"),
		Deletions:             nonNegativeInt(status, "branch_deletions", "deletions"),
		ChangedFiles:          changedFileCount(status),
		Ahead:                 nonNegativeInt(status, "ahead"),
		Behind:                nonNegativeInt(status, "behind"),
		ComparisonUnavailable: stringField(status, "comparison_status") == "unavailable",
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
		mergeQueueState:       stringField(data, "merge_queue_state"),
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
