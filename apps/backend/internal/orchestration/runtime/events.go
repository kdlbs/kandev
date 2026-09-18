package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"time"
)

func decode(event *bus.Event) (map[string]any, error) {
	raw, err := json.Marshal(event.Data)
	if err != nil {
		return nil, err
	}
	var data map[string]any
	err = json.Unmarshal(raw, &data)
	return data, err
}
func (s *Service) Subscribe(eb bus.EventBus) (func(), error) {
	s.AttentionUpdated = func(ctx context.Context, id string, now time.Time) {
		b, err := s.Repo.AssistantBindingByID(ctx, id)
		if err != nil {
			return
		}
		_ = eb.Publish(ctx, events.AssistantUpdated, bus.NewEvent(events.AssistantUpdated, "orchestration", map[string]any{"user_id": b.OwnerUserID, "binding_id": id, revisionResponseKey: now.Format(time.RFC3339Nano)}))
	}
	ids := []bus.Subscription{}
	cleanup := func() {
		for _, id := range ids {
			_ = id.Unsubscribe()
		}
	}
	for _, subject := range []string{events.TaskStateChanged, events.TaskMoved, events.AgentTurnMessageSaved, events.AgentCompleted, events.AgentStopped, events.AgentFailed, events.TaskSessionStateChanged, events.TaskSessionErrorChanged, events.TaskStatusSummaryUpdated, events.MessageAdded, events.MessageUpdated, events.ClarificationAnswered, events.ClarificationPrimaryAnswered, events.ClarificationCancelled, events.ClarificationStaleDismissed, events.BuildPermissionRequestWildcardSubject()} {
		id, err := eb.Subscribe(subject, s.onEvent)
		if err != nil {
			cleanup()
			return nil, err
		}
		ids = append(ids, id)
	}
	return cleanup, nil
}
func (s *Service) onEvent(ctx context.Context, event *bus.Event) error {
	data, err := decode(event)
	if err != nil {
		return err
	}
	taskID := s.attentionEventTask(ctx, data)
	if taskID == "" {
		return nil
	}
	if err := s.ReconcileAttentionTask(ctx, taskID); err != nil {
		return err
	}
	if event.Type == events.TaskStateChanged || event.Type == events.TaskMoved {
		return s.taskCallback(ctx, taskID)
	}
	owner, _, err := s.Repo.ConversationOwner(ctx, taskID)
	if err != nil || owner == "" {
		return nil
	}
	if event.Type == events.AgentTurnMessageSaved {
		return s.bridgeReply(ctx, event, data, taskID, owner)
	}

	return s.finishTurn(ctx, event, data, taskID, owner)
}

func (s *Service) finishTurn(ctx context.Context, event *bus.Event, data map[string]any, taskID, owner string) error {
	switch event.Type {
	case events.AgentCompleted, events.AgentStopped, events.AgentFailed:
	default:
		return nil
	}
	run, err := s.Runs.GetClaimedRunByTaskID(ctx, taskID)
	if err != nil || run == nil {
		return nil
	}
	sessionID, _ := data["session_id"].(string)
	eventRun, _ := data["run_id"].(string)
	if eventRun != run.ID {
		return nil
	}
	if run.SessionID == "" || sessionID != run.SessionID {
		return nil
	}
	if run.ClaimedAt != nil && !event.Timestamp.IsZero() && event.Timestamp.Before(*run.ClaimedAt) {
		return nil
	}
	status := "finished"
	if event.Type == events.AgentFailed {
		status = statusFailed
		message, _ := data["error_message"].(string)
		if err := s.Runs.RecordFailure(ctx, run.ID, message); err != nil {
			return err
		}
	}
	if err := s.Runs.FinishRun(ctx, run.ID, status, nil); err != nil {
		return err
	}
	return s.Repo.SetRuntimeWorking(ctx, owner, false)
}
func (s *Service) taskCallback(ctx context.Context, taskID string) error {
	task, err := s.Tasks.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	id, _ := task.Metadata["orchestration_chief_id"].(string)
	if id == "" {
		return nil
	}
	switch string(task.State) {
	case "REVIEW", "COMPLETED", "FAILED", "WAITING_FOR_INPUT", "BLOCKED":
	default:
		return nil
	}
	a, err := s.Personas.GetAgentInstance(ctx, id)
	if err != nil || a.WorkspaceID != task.WorkspaceID {
		return nil
	}
	conversation, err := s.Repo.EnsureAgentConversation(ctx, a)
	if err != nil {
		return err
	}
	if conversation.TaskID == taskID {
		return nil
	}
	if owner, err := s.Repo.ConversationUserOwner(ctx, conversation.TaskID); err != nil || owner != "" {
		return err
	}
	payload := map[string]any{"callback": map[string]string{taskIDKey: taskID, "title": clip(task.Title, 300), "state": string(task.State)}}
	key := fmt.Sprintf("workspace-task-callback:%s:%s:%s:%s", id, taskID, task.State, task.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"))
	return s.QueueTurn(ctx, id, conversation.TaskID, "workspace_task_callback", key, payload)
}

func (s *Service) bridgeReply(ctx context.Context, event *bus.Event, data map[string]any, taskID, owner string) error {
	var err error

	body, _ := data["agent_text"].(string)
	turnID, _ := data["turn_id"].(string)
	sessionID, _ := data["session_id"].(string)
	if body == "" && turnID != "" {
		body, err = s.Tasks.GetLastAgentMessageForTurn(ctx, turnID)
	}
	if body == "" && turnID == "" && sessionID != "" {
		body, err = s.Tasks.GetLastAgentMessage(ctx, sessionID)
	}
	if err != nil {
		return err
	}
	if body == "" {
		return nil
	}
	session, _ := data["session_id"].(string)
	identity := event.ID + session
	if turnID != "" {
		identity = turnID + session
	}
	if run, e := s.Runs.GetClaimedRunByTaskID(ctx, taskID); turnID == "" && e == nil && run != nil {
		identity = run.ID + session
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s:%x", identity, sha256.Sum256([]byte(body))))).String()
	return s.Repo.PutComment(ctx, &models.TaskComment{ID: id, TaskID: taskID, AuthorID: owner, AuthorType: authorTypeAgent, Body: body, Source: "session"})

}

func (s *Service) attentionEventTask(ctx context.Context, data map[string]any) string {
	if task, _ := data[taskIDKey].(string); task != "" {
		return task
	}
	session, _ := data["session_id"].(string)
	reader, ok := s.Tasks.(interface {
		GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error)
	})
	if !ok || session == "" {
		return ""
	}
	row, err := reader.GetTaskSession(ctx, session)
	if err != nil || row == nil {
		return ""
	}
	return row.TaskID
}

func (s *Service) notifyAssistantUpdated(ctx context.Context, bindingID string) {
	if s.AttentionUpdated != nil {
		s.AttentionUpdated(ctx, bindingID, time.Now().UTC())
	}
}
