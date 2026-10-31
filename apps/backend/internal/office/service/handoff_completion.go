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
	DeliveryTaskID, DeliveryWorkspaceID                string
	DeliveryState                                      string
	SourceTaskID, SourceWorkspaceID, SourceSessionID   string
	SourceAgentProfileID, SourceAssigneeAgentProfileID string
}

// HandoffCompletionPolicy is wired once before subscriptions. Nil is the
// default-off policy; composition checks current directional report grants.
type HandoffCompletionPolicy func(context.Context, HandoffCompletionSource) (bool, error)

// SetHandoffCompletionPolicy opts the service into reporting handed-off task
// completions back to the originating office task. A nil policy leaves the
// feature off: no comment, no wake, no side effect.
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
	data, err := decodeEventData[handoffTaskStateChangedData](event)
	if err != nil || data.TaskID == "" || !taskmodels.IsTerminalTaskState(v1.TaskState(data.NewState)) {
		return nil
	}
	source, err := s.handoffCompletionSource(ctx, data.TaskID, data.NewState)
	if err != nil || source == nil {
		return err
	}
	allowed, err := s.handoffCompletionPolicy(ctx, *source)
	if err != nil || !allowed {
		return err
	}
	comment, created, err := s.handoffCompletionComment(ctx, *source, data.NewState)
	if err != nil {
		return err
	}
	if created {
		s.publishCommentCreated(ctx, comment)
	}
	return s.queueHandoffCompletedRun(ctx, *source, comment.ID, handoffCompletionKey(data.TaskID, data.NewState))
}

// handoffCompletionSource resolves the stored provenance plus the current
// delivery state and the originating task's current assignee. A stale
// transition, a missing provenance, or a non-office origin produces nil.
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
	if origin == nil || !origin.IsFromOffice || origin.WorkspaceID != source.SourceWorkspaceID || origin.AssigneeAgentProfileID == "" {
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
// transition, so the report comment and the wake run dedupe onto the same
// identity instead of posting and waking twice.
func handoffCompletionKey(taskID, state string) string {
	return RunReasonHandoffCompleted + ":" + taskID + ":" + state
}

// handoffCompletionComment stores the report once. The deterministic id is
// the dedup identity: a redelivered event reuses the existing row, and no
// second comment is posted.
func (s *Service) handoffCompletionComment(
	ctx context.Context, source HandoffCompletionSource, state string,
) (*models.TaskComment, bool, error) {
	delivery, err := s.repo.GetTaskBasicInfo(ctx, source.DeliveryTaskID)
	if err != nil {
		return nil, false, fmt.Errorf("read handoff completion title: %w", err)
	}
	if delivery == nil {
		return nil, false, fmt.Errorf("handoff delivery task disappeared before completion report")
	}
	comment := &models.TaskComment{
		ID:         handoffCompletionKey(source.DeliveryTaskID, state),
		TaskID:     source.SourceTaskID,
		AuthorType: participantTypeAgent,
		Body: fmt.Sprintf("Handed-off task %q (%s) reached state %s.",
			delivery.Title, source.DeliveryTaskID, state),
		Source: handoffCompletionCommentSource,
	}
	return s.repo.CreateHandoffCompletionComment(ctx, comment)
}

func (s *Service) queueHandoffCompletedRun(
	ctx context.Context, source HandoffCompletionSource, commentID, key string,
) error {
	payload := mustJSON(map[string]string{"task_id": source.SourceTaskID, "comment_id": commentID})
	if err := s.QueueRunFromTaskBoundary(ctx, source.SourceAssigneeAgentProfileID, RunReasonHandoffCompleted, payload, key, source.SourceTaskID); err != nil {
		var paused *pausedQueueError
		if errors.As(err, &paused) && paused.pause != nil {
			s.RecordDeferredAssignment(ctx, source.SourceTaskID, paused.pause.ID)
			return nil
		}
		return fmt.Errorf("queue handoff completion wake: %w", err)
	}
	return nil
}
