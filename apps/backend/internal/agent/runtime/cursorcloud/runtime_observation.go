package cursorcloud

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	provider "github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

// ProbeRemoteLiveness classifies the provider's latest run. Confirmed terminal
// evidence is committed through the same operation CAS as the stream observer.
func (r *Runtime) ProbeRemoteLiveness(ctx context.Context, executionID string) (RemoteLivenessState, error) {
	binding, err := r.repository.GetManagedAgentBindingByExecution(ctx, executionID)
	if errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return RemoteLivenessAbsent, nil
	}
	if err != nil {
		return RemoteLivenessUnknown, err
	}
	operation, err := r.repository.GetManagedAgentLatestOperation(ctx, binding.ID)
	if err != nil {
		return RemoteLivenessUnknown, err
	}
	if !models.ManagedAgentOperationActive(operation.State) {
		r.publishPendingCompletion(ctx, binding, operation)
		return RemoteLivenessTerminal, nil
	}
	if operation.RemoteRunID == "" {
		return RemoteLivenessUnknown, nil
	}
	client, err := r.clientFactory(ctx, binding)
	if err != nil {
		return RemoteLivenessUnknown, err
	}
	run, err := client.GetRun(ctx, binding.RemoteAgentID, operation.RemoteRunID)
	if err != nil {
		return RemoteLivenessUnknown, err
	}
	return r.classifyLivenessRun(ctx, binding, operation, client, run)
}

func (r *Runtime) classifyLivenessRun(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	client Provider,
	run provider.Run,
) (RemoteLivenessState, error) {
	if run.ID != operation.RemoteRunID || run.AgentID != binding.RemoteAgentID {
		return RemoteLivenessUnknown, errors.New("cursor cloud liveness response identity is invalid")
	}
	if _, terminal := terminalSubmissionState(run.Status); !terminal {
		return classifyActiveLiveness(run.Status)
	}
	return r.settleLivenessRun(ctx, binding, operation, client, run)
}

func classifyActiveLiveness(status string) (RemoteLivenessState, error) {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "CREATING", runStatusRunning:
		return RemoteLivenessLive, nil
	default:
		return RemoteLivenessUnknown, nil
	}
}

func (r *Runtime) settleLivenessRun(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	client Provider,
	run provider.Run,
) (RemoteLivenessState, error) {
	result := resultSnapshotForRun(binding, run)
	agent, err := client.GetAgent(ctx, binding.RemoteAgentID)
	if err == nil {
		result.AgentURL = safeCursorAgentURL(agent.URL)
	}
	settled, err := r.settleObservedRun(ctx, binding, operation, run, result)
	if err != nil {
		return RemoteLivenessUnknown, err
	}
	if settled {
		if err := r.publishSettledLivenessCompletion(ctx, binding, operation); err != nil {
			return RemoteLivenessUnknown, err
		}
	}
	return RemoteLivenessTerminal, nil
}

func (r *Runtime) publishSettledLivenessCompletion(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
) error {
	latest, err := r.repository.GetManagedAgentOperation(ctx, operation.ID)
	if err != nil {
		return err
	}
	r.publishPendingCompletion(ctx, binding, latest)
	return nil
}

// ObserveOnce replays one managed run from its durable SSE checkpoint and
// settles it only when Cursor confirms a terminal provider status.
func (r *Runtime) ObserveOnce(ctx context.Context, executionID, reconnectReason string) error {
	binding, operation, done, err := r.loadObservation(ctx, executionID)
	if err != nil || done {
		return err
	}
	lastEventID, assistantMessageStarted, err := r.loadObservationCursor(ctx, binding, operation)
	if err != nil {
		return err
	}
	client, err := r.clientFactory(ctx, binding)
	if err != nil {
		return err
	}
	streamErr := r.observeRunStream(ctx, client, binding, operation, lastEventID, assistantMessageStarted)
	if err := r.handleObservationStreamError(ctx, binding, operation, reconnectReason, streamErr); err != nil {
		return err
	}
	return r.settleReadbackRun(ctx, binding, operation, client, streamErr)
}

func (r *Runtime) loadObservation(
	ctx context.Context,
	executionID string,
) (*models.ManagedAgentBinding, *models.ManagedAgentOperation, bool, error) {
	binding, err := r.repository.GetManagedAgentBindingByExecution(ctx, executionID)
	if err != nil {
		return nil, nil, false, err
	}
	operation, err := r.repository.GetManagedAgentLatestOperation(ctx, binding.ID)
	if err != nil {
		return nil, nil, false, err
	}
	if !models.ManagedAgentOperationActive(operation.State) {
		r.publishPendingCompletion(ctx, binding, operation)
		return binding, operation, true, nil
	}
	if operation.RemoteRunID == "" {
		return nil, nil, false, provider.ErrOutcomeUnknown
	}
	if r.projectStream == nil {
		return nil, nil, false, runtime.ErrUnsupported
	}
	return binding, operation, false, nil
}

func (r *Runtime) loadObservationCursor(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
) (string, bool, error) {
	checkpoint, err := r.repository.GetManagedAgentStreamCheckpoint(ctx, binding.ID, operation.RemoteRunID)
	if errors.Is(err, repository.ErrManagedAgentStreamNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load Cursor Cloud stream checkpoint: %w", err)
	}
	if checkpoint.HistoryGap {
		return "", false, nil
	}
	return checkpoint.LastEventID, checkpoint.AssistantMessageStarted, nil
}

func (r *Runtime) observeRunStream(
	ctx context.Context,
	client Provider,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	lastEventID string,
	assistantMessageStarted bool,
) error {
	_, err := client.StreamRun(ctx, binding.RemoteAgentID, operation.RemoteRunID, lastEventID,
		func(event provider.StreamEvent) error {
			return r.handleObservationEvent(ctx, binding, operation, event, &assistantMessageStarted)
		})
	return err
}

func (r *Runtime) handleObservationEvent(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	event provider.StreamEvent,
	assistantMessageStarted *bool,
) error {
	projection, err := r.projectStream(ctx, binding, operation, event)
	if err != nil {
		return err
	}
	setObservationPayloadIdentity(projection, *assistantMessageStarted)
	managedEvent := managedStreamEvent(binding, operation, event, projection, *assistantMessageStarted)
	recorded, err := r.repository.CommitManagedAgentStreamEvent(ctx, managedEvent)
	if err != nil {
		return fmt.Errorf("commit Cursor Cloud stream event: %w", err)
	}
	if recorded && publishableStreamProjection(projection) && r.publishStream != nil {
		r.publishStream(ctx, projection.Payload)
	}
	if projection != nil && projection.AppendMessage {
		*assistantMessageStarted = true
	}
	return nil
}

func setObservationPayloadIdentity(projection *StreamProjection, assistantMessageStarted bool) {
	if projection == nil || projection.Payload == nil || projection.Payload.Data == nil || projection.Message == nil {
		return
	}
	projection.Payload.Data.MessageID = projection.Message.ID
	projection.Payload.Data.MessageType = string(projection.Message.Type)
	projection.Payload.Data.IsAppend = projection.AppendMessage && assistantMessageStarted
}

func managedStreamEvent(
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	event provider.StreamEvent,
	projection *StreamProjection,
	assistantMessageStarted bool,
) models.ManagedAgentStreamEvent {
	managedEvent := models.ManagedAgentStreamEvent{
		BindingID: binding.ID, OperationID: operation.ID, RemoteRunID: operation.RemoteRunID,
		EventID: event.ID, EventType: event.Type, Cursor: event.ID,
		DispatchGeneration: operation.DispatchGeneration, Message: projectionMessage(projection),
		AppendMessage:           projection != nil && projection.AppendMessage,
		AssistantMessageStarted: assistantMessageStarted || projection != nil && projection.AppendMessage,
	}
	if projection != nil && projection.TerminalStatus != "" {
		managedEvent.TerminalEventType = terminalEventType(projection.TerminalStatus)
	}
	return managedEvent
}

func publishableStreamProjection(projection *StreamProjection) bool {
	return projection != nil && projection.Payload != nil && projection.TerminalStatus == ""
}

func (r *Runtime) handleObservationStreamError(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	reconnectReason string,
	streamErr error,
) error {
	if streamErr == nil {
		recordReconnect(reconnectReason, "connected")
		return nil
	}
	var apiErr *provider.APIError
	if isExpiredStream(streamErr, &apiErr) {
		recordReconnect(reconnectReason, "history_expired")
		return r.markStreamHistoryGap(ctx, binding, operation)
	}
	if isStreamAuthenticationError(streamErr, &apiErr) {
		recordReconnect(reconnectReason, "auth_error")
		return streamErr
	}
	recordReconnect(reconnectReason, "retryable_error")
	if !r.allowStatusFallback(binding.ID) {
		return streamErr
	}
	return nil
}

func isExpiredStream(err error, apiErr **provider.APIError) bool {
	return errors.As(err, apiErr) && (*apiErr).StatusCode == 410 && (*apiErr).Code == "stream_expired"
}

func isStreamAuthenticationError(err error, apiErr **provider.APIError) bool {
	return errors.As(err, apiErr) && ((*apiErr).StatusCode == 401 || (*apiErr).StatusCode == 403)
}

func (r *Runtime) settleReadbackRun(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	client Provider,
	streamErr error,
) error {
	run, err := client.GetRun(ctx, binding.RemoteAgentID, operation.RemoteRunID)
	if err != nil {
		if streamErr != nil {
			return errors.Join(streamErr, fmt.Errorf("read Cursor Cloud run after observation: %w", err))
		}
		return fmt.Errorf("read Cursor Cloud run after observation: %w", err)
	}
	result := resultSnapshotForRun(binding, run)
	if _, terminal := terminalSubmissionState(run.Status); terminal {
		if agent, agentErr := client.GetAgent(ctx, binding.RemoteAgentID); agentErr == nil {
			result.AgentURL = safeCursorAgentURL(agent.URL)
		}
	}
	settled, err := r.settleObservedRun(ctx, binding, operation, run, result)
	if err != nil {
		return err
	}
	if settled {
		latest, loadErr := r.repository.GetManagedAgentOperation(ctx, operation.ID)
		if loadErr != nil {
			return loadErr
		}
		r.publishPendingCompletion(ctx, binding, latest)
	}
	return nil
}

func (r *Runtime) allowStatusFallback(bindingID string) bool {
	now := r.now()
	r.fallbackMu.Lock()
	defer r.fallbackMu.Unlock()
	if last, ok := r.fallbackAt[bindingID]; ok && now.Sub(last) < 15*time.Second {
		return false
	}
	r.fallbackAt[bindingID] = now
	return true
}

// ObserveContinuously reconnects with capped exponential delay. A failed
// stream is never treated as terminal evidence.
func (r *Runtime) ObserveContinuously(ctx context.Context, executionID, reason string) {
	if reason != "backend_restart" {
		reason = "disconnect"
	}
	for attempt := 0; ctx.Err() == nil; attempt++ {
		err := r.ObserveOnce(ctx, executionID, reason)
		if err == nil {
			binding, loadErr := r.repository.GetManagedAgentBindingByExecution(ctx, executionID)
			if loadErr != nil {
				return
			}
			operation, opErr := r.repository.GetManagedAgentLatestOperation(ctx, binding.ID)
			if opErr != nil || (!models.ManagedAgentOperationActive(operation.State) && !operation.CompletionPending) {
				return
			}
		} else {
			var apiErr *provider.APIError
			if errors.As(err, &apiErr) && (apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
				return
			}
		}
		if !waitForObservationRetry(ctx, attempt, err) {
			return
		}
		reason = "disconnect"
	}
}

func waitForObservationRetry(ctx context.Context, attempt int, cause error) bool {
	delay := observationRetryDelay(attempt, cause)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func observationRetryDelay(attempt int, cause error) time.Duration {
	delay := time.Second * time.Duration(1<<min(attempt, 5))
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	delay += time.Duration(rand.Int63n(int64(delay / 5)))
	var apiErr *provider.APIError
	if errors.As(cause, &apiErr) && apiErr.RetryAfter > delay {
		delay = apiErr.RetryAfter
	}
	return delay
}

func projectionMessage(projection *StreamProjection) *models.Message {
	if projection == nil {
		return nil
	}
	return projection.Message
}

func (r *Runtime) markStreamHistoryGap(ctx context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation) error {
	_, err := r.repository.CommitManagedAgentStreamEvent(ctx, models.ManagedAgentStreamEvent{
		BindingID: binding.ID, OperationID: operation.ID, RemoteRunID: operation.RemoteRunID,
		EventType: "history_gap", DispatchGeneration: operation.DispatchGeneration, HistoryGap: true,
	})
	if err != nil {
		return fmt.Errorf("record Cursor Cloud stream history gap: %w", err)
	}
	return nil
}

func (r *Runtime) settleObservedRun(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	run provider.Run,
	result models.ManagedAgentResultSnapshot,
) (bool, error) {
	state, terminal := terminalSubmissionState(run.Status)
	if !terminal {
		return false, nil
	}
	if operation.State == state {
		return false, nil
	}
	if err := r.persistRunResult(ctx, binding, operation, run); err != nil {
		return false, err
	}
	leaseBinding, owner, err := r.ensureLease(ctx, binding, operation)
	if err != nil {
		return false, err
	}
	pending := true
	if _, err := r.repository.CompareAndSwapManagedAgentOperation(ctx, models.ManagedAgentOperationUpdate{
		OperationID: operation.ID, ExpectedRevision: operation.Revision,
		ExpectedBindingRevision: leaseBinding.Revision, LeaseOwner: owner,
		State: state, ResultSnapshot: &result, CompletionPending: &pending,
	}); err != nil {
		latest, readErr := r.repository.GetManagedAgentLatestOperation(ctx, binding.ID)
		if readErr == nil && latest.State == state {
			return false, nil
		}
		return false, fmt.Errorf("settle observed Cursor Cloud run: %w", err)
	}
	return true, nil
}

func terminalStreamPayload(
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	status string,
	result models.ManagedAgentResultSnapshot,
) *lifecycle.AgentStreamEventPayload {
	state, terminal := terminalSubmissionState(status)
	if !terminal {
		return nil
	}
	data := map[string]interface{}{"stop_reason": "end_turn", "is_error": false}
	data["remote_result"] = map[string]interface{}{
		"repository_id":    result.RepositoryID,
		"branch":           result.Branch,
		"pull_request_url": result.PullRequestURL,
		"agent_url":        result.AgentURL,
	}
	data["remote_terminal"] = true
	errorText := ""
	switch state {
	case models.ManagedAgentSubmissionFailed:
		data["stop_reason"] = "error"
		data["is_error"] = true
		errorText = "Cursor Cloud run failed"
	case models.ManagedAgentSubmissionCancelled:
		data["stop_reason"] = "cancelled"
	}
	return &lifecycle.AgentStreamEventPayload{
		Type: "agent/event", Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		AgentID: binding.ExecutionID, ExecutionID: binding.ExecutionID,
		TaskID: binding.TaskID, SessionID: binding.SessionID, ManagedAgentOperationID: operation.ID,
		Data: &lifecycle.AgentStreamEventData{
			Type: "complete", TurnID: operation.RequestSnapshot.TurnID, Data: data, Error: errorText,
		},
	}
}

func resultSnapshotForRun(binding *models.ManagedAgentBinding, run provider.Run) models.ManagedAgentResultSnapshot {
	result := models.ManagedAgentResultSnapshot{}
	if binding == nil {
		return result
	}
	result.RepositoryID = binding.Launch.RepositoryID
	result.AssistantResult = run.Result
	if run.Git == nil {
		return result
	}
	expected, ok := githubRepositoryIdentity(binding.Launch.RepositoryURL)
	if !ok {
		return result
	}
	for _, branch := range run.Git.Branches {
		identity, valid := githubRepositoryIdentity(branch.RepositoryURL)
		if !valid || identity != expected {
			continue
		}
		result.Branch = strings.TrimSpace(branch.Branch)
		result.PullRequestURL = validatedPullRequestURL(branch.PRURL, expected)
		break
	}
	return result
}

func (r *Runtime) persistRunResult(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	run provider.Run,
) error {
	if binding == nil || operation == nil || run.Result == "" {
		return nil
	}
	assistantMessageStarted := false
	checkpoint, checkpointErr := r.repository.GetManagedAgentStreamCheckpoint(ctx, binding.ID, operation.RemoteRunID)
	if checkpointErr != nil && !errors.Is(checkpointErr, repository.ErrManagedAgentStreamNotFound) {
		return fmt.Errorf("load Cursor Cloud stream checkpoint for final result: %w", checkpointErr)
	}
	if checkpointErr == nil {
		assistantMessageStarted = checkpoint.AssistantMessageStarted
	}
	messageID := "cursor-cloud-assistant-" + operation.ID
	message := &models.Message{
		ID: messageID, TaskSessionID: binding.SessionID, TaskID: binding.TaskID,
		TurnID: operation.RequestSnapshot.TurnID, AuthorType: models.MessageAuthorAgent,
		AuthorID: binding.ExecutionID, Type: models.MessageTypeMessage, Content: run.Result,
	}
	payload := &lifecycle.AgentStreamEventPayload{
		Type: "agent/event", Timestamp: r.now().UTC().Format(time.RFC3339Nano),
		AgentID: binding.ExecutionID, ExecutionID: binding.ExecutionID,
		TaskID: binding.TaskID, SessionID: binding.SessionID, ManagedAgentOperationID: operation.ID,
		Data: &lifecycle.AgentStreamEventData{
			Type: "message_streaming", TurnID: operation.RequestSnapshot.TurnID,
			Text: run.Result, MessageID: messageID, MessageType: string(models.MessageTypeMessage),
			MessageUpdated: assistantMessageStarted,
		},
	}
	recorded, err := r.repository.CommitManagedAgentStreamEvent(ctx, models.ManagedAgentStreamEvent{
		BindingID: binding.ID, OperationID: operation.ID, RemoteRunID: operation.RemoteRunID,
		EventID:            "cursor-cloud-result-" + operation.ID,
		EventType:          "terminal_result_readback",
		DispatchGeneration: operation.DispatchGeneration, Message: message,
	})
	if err != nil {
		return fmt.Errorf("persist Cursor Cloud run result: %w", err)
	}
	if recorded && r.publishStream != nil {
		r.publishStream(ctx, payload)
	}
	return nil
}

func (r *Runtime) publishPendingCompletion(ctx context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation) {
	if r.publishStream == nil || binding == nil || operation == nil || !operation.CompletionPending {
		return
	}
	status := "FINISHED"
	switch operation.State {
	case models.ManagedAgentSubmissionFailed:
		status = runStatusError
	case models.ManagedAgentSubmissionCancelled:
		status = runStatusCancelled
	default:
		if !models.ManagedAgentOperationTerminal(operation.State) {
			return
		}
	}
	r.publishStream(ctx, terminalStreamPayload(binding, operation, status, operation.ResultSnapshot))
}
