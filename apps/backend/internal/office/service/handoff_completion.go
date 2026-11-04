package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const handoffCompletionCommentSource = "handoff"

// HandoffCompletionSource is the stored handoff relationship, not an actor
// supplied by a state-change event or a new permission-bearing request.
type HandoffCompletionSource struct {
	DeliveryTaskID, DeliveryWorkspaceID string
	DeliveryTitle, DeliveryState        string
	SourceTaskID, SourceWorkspaceID     string
	SourceSessionID                     string
	SourceAgentProfileID                string
	SourceAssigneeAgentProfileID        string
}

// HandoffCompletionAuthorization keeps report and prompt rights independent.
// A collaboration grant can authorize a stored report, a source-task wake, or
// both; report permission alone never starts an agent run.
type HandoffCompletionAuthorization struct {
	Report bool
	Prompt bool
}

// HandoffCompletionPolicy is wired once before subscriptions. Nil is the
// default-off policy; composition checks current directional collaboration
// grants.
type HandoffCompletionPolicy func(context.Context, HandoffCompletionSource) (HandoffCompletionAuthorization, error)

// SetHandoffCompletionPolicy opts the service into reporting handed-off task
// completions to the originating office task. A nil policy leaves the feature
// off: no comment, wake, or side effect.
func (s *Service) SetHandoffCompletionPolicy(policy HandoffCompletionPolicy) {
	s.handoffCompletionPolicy = policy
}

type handoffTaskStateChangedData struct {
	TaskID   string `json:"task_id"`
	NewState string `json:"new_state"`
	OldState string `json:"old_state"`
}

func (s *Service) handleHandoffTaskStateChanged(ctx context.Context, event *bus.Event) error {
	if s.handoffCompletionPolicy == nil {
		return nil
	}
	data, valid := handoffCompletionTransition(event)
	if !valid {
		return nil
	}
	source, err := s.handoffCompletionSource(ctx, data.TaskID, data.NewState)
	if err != nil || source == nil {
		return err
	}
	return s.completeHandoffTaskStateChange(ctx, data, source)
}

func handoffCompletionTransition(event *bus.Event) (handoffTaskStateChangedData, bool) {
	data, err := decodeEventData[handoffTaskStateChangedData](event)
	if err != nil {
		return handoffTaskStateChangedData{}, false
	}
	// Report a completion, not a re-observation of an already-terminal task:
	// a state-change event with no transition (or no previous state) did not
	// reach the terminal state and must not produce a report or a wake.
	valid := data.TaskID != "" && taskmodels.IsTerminalTaskState(v1.TaskState(data.NewState)) &&
		data.OldState != "" && data.OldState != data.NewState
	return *data, valid
}

func (s *Service) completeHandoffTaskStateChange(
	ctx context.Context, data handoffTaskStateChangedData, source *HandoffCompletionSource,
) error {
	authorization, err := s.handoffCompletionPolicy(ctx, *source)
	if err != nil {
		return err
	}
	if !authorization.Report && !authorization.Prompt {
		return nil
	}
	delivery, err := s.repo.GetTaskBasicInfo(ctx, source.DeliveryTaskID)
	if err != nil {
		return fmt.Errorf("read handoff completion task title: %w", err)
	}
	if delivery == nil {
		return fmt.Errorf("handoff delivery task disappeared before completion handling")
	}
	source.DeliveryTitle = delivery.Title

	key := handoffCompletionKey(data.TaskID, data.NewState)
	var comment *models.TaskComment
	if authorization.Report {
		comment = handoffCompletionComment(*source, key)
	}
	wakePending := authorization.Prompt && source.SourceAssigneeAgentProfileID != ""
	if comment == nil && !wakePending {
		return nil
	}
	item := handoffCompletionOutbox(*source, key, comment, wakePending)
	if err := s.repo.EnsureHandoffCompletionOutbox(ctx, item, comment); err != nil {
		return err
	}
	return s.processHandoffCompletionOutbox(ctx, key)
}

// handoffCompletionSource resolves write-once handoff provenance plus the
// current delivery state and source-task assignee. A stale transition, a
// missing provenance, or a non-office origin produces nil.
func (s *Service) handoffCompletionSource(ctx context.Context, taskID, state string) (*HandoffCompletionSource, error) {
	delivery, err := s.repo.GetTaskExecutionFields(ctx, taskID)
	if err != nil {
		if errors.Is(err, sqlite.ErrTaskNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("read handoff delivery task: %w", err)
	}
	if delivery == nil || delivery.State != state {
		return nil, nil // A stale transition cannot report the current task.
	}
	metadata, err := s.repo.GetTaskMetadata(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("read handoff source metadata: %w", err)
	}
	source := parseHandoffCompletionSource(metadata)
	if source == nil {
		return nil, nil
	}
	source.DeliveryTaskID, source.DeliveryWorkspaceID = taskID, delivery.WorkspaceID
	source.DeliveryState = state
	origin, err := s.repo.GetTaskExecutionFields(ctx, source.SourceTaskID)
	if errors.Is(err, sqlite.ErrTaskNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read originating handoff task: %w", err)
	}
	if origin == nil || !origin.IsFromOffice || origin.WorkspaceID != source.SourceWorkspaceID {
		return nil, nil
	}
	source.SourceAssigneeAgentProfileID = origin.AssigneeAgentProfileID
	return source, nil
}

func parseHandoffCompletionSource(metadata map[string]interface{}) *HandoffCompletionSource {
	raw, ok := metadata[taskmodels.MetaKeyHandoffSource].(map[string]interface{})
	if !ok {
		return nil
	}
	value := func(key string) string {
		text, _ := raw[key].(string)
		return strings.TrimSpace(text)
	}
	source := &HandoffCompletionSource{
		SourceTaskID: value("source_task_id"), SourceWorkspaceID: value("source_workspace_id"),
		SourceSessionID: value("source_session_id"), SourceAgentProfileID: value("source_agent_profile_id"),
	}
	if source.SourceTaskID == "" || source.SourceWorkspaceID == "" || source.SourceSessionID == "" || source.SourceAgentProfileID == "" {
		return nil
	}
	return source
}

// handoffCompletionKey is stable across redeliveries of one delivery-task
// transition. The durable outbox receipt preserves that identity after run
// history is pruned.
func handoffCompletionKey(taskID, state string) string {
	return RunReasonHandoffCompleted + ":" + taskID + ":" + state
}

func handoffCompletionComment(source HandoffCompletionSource, key string) *models.TaskComment {
	return &models.TaskComment{
		ID:         key,
		TaskID:     source.SourceTaskID,
		AuthorType: participantTypeAgent,
		AuthorID:   source.SourceAgentProfileID,
		Body: fmt.Sprintf("Handed-off task %q (%s) reached state %s.",
			source.DeliveryTitle, source.DeliveryTaskID, source.DeliveryState),
		Source: handoffCompletionCommentSource,
	}
}

func handoffCompletionOutbox(
	source HandoffCompletionSource, key string, comment *models.TaskComment, wakePending bool,
) *sqlite.HandoffCompletionOutbox {
	commentID := ""
	commentEventPending := false
	if comment != nil {
		commentID = comment.ID
		commentEventPending = true
	}
	return &sqlite.HandoffCompletionOutbox{
		CompletionKey: key, CommentID: commentID,
		SourceTaskID: source.SourceTaskID, SourceWorkspaceID: source.SourceWorkspaceID,
		SourceSessionID: source.SourceSessionID, SourceAgentProfileID: source.SourceAgentProfileID,
		SourceAssigneeAgentProfileID: source.SourceAssigneeAgentProfileID,
		DeliveryTaskID:               source.DeliveryTaskID, DeliveryWorkspaceID: source.DeliveryWorkspaceID,
		DeliveryTitle: source.DeliveryTitle, DeliveryState: source.DeliveryState,
		CommentEventPending: commentEventPending, WakePending: wakePending,
	}
}

func (s *Service) processHandoffCompletionOutbox(ctx context.Context, completionKey string) error {
	item, err := s.repo.GetHandoffCompletionOutbox(ctx, completionKey)
	if err != nil || item == nil {
		return err
	}
	if err := s.repo.TouchHandoffCompletionOutboxAttempt(ctx, completionKey); err != nil {
		return err
	}
	var deliveryErrors []error
	if item.CommentEventPending {
		comment, err := s.repo.GetTaskComment(ctx, item.CommentID)
		if err != nil {
			deliveryErrors = append(deliveryErrors, fmt.Errorf("read handoff completion report: %w", err))
		} else if comment == nil {
			deliveryErrors = append(deliveryErrors, fmt.Errorf("handoff completion outbox has no report comment %q", item.CommentID))
		} else if err := s.publishCommentCreated(ctx, comment); err != nil {
			deliveryErrors = append(deliveryErrors, fmt.Errorf("publish handoff completion report: %w", err))
		} else if err := s.repo.MarkHandoffCompletionCommentEventPublished(ctx, completionKey); err != nil {
			deliveryErrors = append(deliveryErrors, err)
		}
	}
	if item.WakePending {
		if err := s.replayHandoffCompletionWake(ctx, item); err != nil {
			deliveryErrors = append(deliveryErrors, err)
		}
	}
	return errors.Join(deliveryErrors...)
}

func (s *Service) replayHandoffCompletionWake(ctx context.Context, item *sqlite.HandoffCompletionOutbox) error {
	if s.handoffCompletionPolicy == nil {
		return s.repo.MarkHandoffCompletionWakeResolved(ctx, item.CompletionKey)
	}
	source, err := s.handoffCompletionSource(ctx, item.DeliveryTaskID, item.DeliveryState)
	if err != nil {
		return err
	}
	if source == nil || source.SourceTaskID != item.SourceTaskID ||
		source.SourceWorkspaceID != item.SourceWorkspaceID || source.SourceSessionID != item.SourceSessionID ||
		source.SourceAgentProfileID != item.SourceAgentProfileID ||
		source.SourceAssigneeAgentProfileID != item.SourceAssigneeAgentProfileID {
		return s.repo.MarkHandoffCompletionWakeResolved(ctx, item.CompletionKey)
	}
	authorization, err := s.handoffCompletionPolicy(ctx, *source)
	if err != nil {
		return err
	}
	if !authorization.Prompt {
		return s.repo.MarkHandoffCompletionWakeResolved(ctx, item.CompletionKey)
	}
	// Preserve the title and report captured at completion even if the delivery
	// task's editable title changes before a paused wake can be replayed.
	source.DeliveryTitle = item.DeliveryTitle
	commentID := item.CommentID
	if err := s.queueHandoffCompletedRun(ctx, *source, commentID, item.CompletionKey); err != nil {
		return err
	}
	return s.repo.MarkHandoffCompletionWakeResolved(ctx, item.CompletionKey)
}

// ReplayPendingHandoffCompletionOutbox retries reports and source-task wakes
// after a transient failure or a process restart. Completed rows remain as
// durable idempotency receipts after run retention.
func (s *Service) ReplayPendingHandoffCompletionOutbox(ctx context.Context) error {
	const batchSize = 5
	items, err := s.repo.ListPendingHandoffCompletionOutbox(ctx, batchSize)
	if err != nil {
		return err
	}
	var replayErrors []error
	for _, item := range items {
		if err := s.processHandoffCompletionOutbox(ctx, item.CompletionKey); err != nil {
			replayErrors = append(replayErrors, fmt.Errorf("replay handoff completion %s: %w", item.CompletionKey, err))
		}
	}
	return errors.Join(replayErrors...)
}

func (s *Service) queueHandoffCompletedRun(
	ctx context.Context, source HandoffCompletionSource, commentID, key string,
) error {
	if source.SourceAssigneeAgentProfileID == "" {
		return nil
	}
	payload := map[string]string{
		"task_id":          source.SourceTaskID,
		"delivery_task_id": source.DeliveryTaskID,
		"delivery_title":   source.DeliveryTitle,
		"delivery_state":   source.DeliveryState,
	}
	if commentID != "" {
		payload["comment_id"] = commentID
	}
	if err := s.QueueRunFromTaskBoundary(ctx, source.SourceAssigneeAgentProfileID,
		RunReasonHandoffCompleted, mustJSON(payload), key, source.SourceTaskID); err != nil {
		return fmt.Errorf("queue handoff completion wake: %w", err)
	}
	return nil
}
