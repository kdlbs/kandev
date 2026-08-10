package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const (
	parentQuestionStatusPending  = "pending"
	parentQuestionStatusAnswered = "answered"
)

type parentQuestionRequest struct {
	SessionID string                   `json:"session_id"`
	TaskID    string                   `json:"task_id"`
	Questions []clarification.Question `json:"questions"`
	Context   string                   `json:"context"`
}

type parentQuestionError struct {
	code    string
	message string
}

func (e *parentQuestionError) Error() string {
	return e.message
}

type parentQuestionTarget struct {
	childSession  *models.TaskSession
	childTask     *models.Task
	parentTask    *models.Task
	parentSession *models.TaskSession
}

func parentQuestionRequestFromMessage(msg *ws.Message) (parentQuestionRequest, *parentQuestionError) {
	var req parentQuestionRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return req, &parentQuestionError{ws.ErrorCodeBadRequest, "Invalid payload: " + err.Error()}
	}
	if req.SessionID == "" {
		return req, &parentQuestionError{ws.ErrorCodeValidation, "session_id is required"}
	}
	if errMsg := clarification.NormalizeAndValidateQuestions(req.Questions); errMsg != "" {
		return req, &parentQuestionError{ws.ErrorCodeValidation, errMsg}
	}
	return req, nil
}

func (h *Handlers) loadParentQuestionTarget(ctx context.Context, req parentQuestionRequest) (*parentQuestionTarget, *parentQuestionError) {
	if h.taskSvc == nil || h.sessionRepo == nil || h.taskRepo == nil {
		return nil, &parentQuestionError{ws.ErrorCodeInternalError, "task services are not available"}
	}
	childSession, err := h.taskSvc.GetTaskSession(ctx, req.SessionID)
	if err != nil || childSession == nil {
		return nil, &parentQuestionError{ws.ErrorCodeNotFound, "child session not found"}
	}
	taskID := req.TaskID
	if taskID == "" {
		taskID = childSession.TaskID
	}
	if childSession.TaskID != taskID {
		return nil, &parentQuestionError{ws.ErrorCodeValidation, "session_id does not belong to task_id"}
	}
	childTask, err := h.taskSvc.GetTask(ctx, taskID)
	if err != nil || childTask == nil {
		return nil, &parentQuestionError{ws.ErrorCodeNotFound, "child task not found"}
	}
	parentTask, parentSession, parentErr := h.loadParentQuestionParent(ctx, childTask)
	if parentErr != nil {
		return nil, parentErr
	}
	return &parentQuestionTarget{
		childSession:  childSession,
		childTask:     childTask,
		parentTask:    parentTask,
		parentSession: parentSession,
	}, nil
}

func (h *Handlers) loadParentQuestionParent(ctx context.Context, childTask *models.Task) (*models.Task, *models.TaskSession, *parentQuestionError) {
	if !childTask.Autopilot {
		return nil, nil, &parentQuestionError{ws.ErrorCodeValidation, "ask_parent_question_kandev is only available to autopilot tasks"}
	}
	if childTask.ParentID == "" {
		return nil, nil, &parentQuestionError{ws.ErrorCodeValidation, "an autopilot root task has no parent to ask"}
	}
	parentTask, err := h.taskSvc.GetTask(ctx, childTask.ParentID)
	if err != nil || parentTask == nil {
		return nil, nil, &parentQuestionError{ws.ErrorCodeNotFound, "direct parent task not found"}
	}
	parentSession, err := h.taskSvc.GetPrimarySession(ctx, parentTask.ID)
	if err != nil || parentSession == nil {
		return nil, nil, &parentQuestionError{ws.ErrorCodeNotFound, "direct parent task has no primary session"}
	}
	return parentTask, parentSession, nil
}

// handleAskParentQuestion records a durable question for an autopilot child,
// sends the question to its direct parent, and pauses the child turn. It does
// not wait for the answer: the parent answers with message_task_kandev and the
// child starts a new turn through the normal message dispatch path.
func (h *Handlers) handleAskParentQuestion(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, requestErr := parentQuestionRequestFromMessage(msg)
	if requestErr != nil {
		return ws.NewError(msg.ID, msg.Action, requestErr.code, requestErr.message, nil)
	}
	target, targetErr := h.loadParentQuestionTarget(ctx, req)
	if targetErr != nil {
		return ws.NewError(msg.ID, msg.Action, targetErr.code, targetErr.message, nil)
	}

	questionID := uuid.NewString()
	questionMetadata := parentQuestionMetadata(questionID, target.parentTask, target.childTask, req.Questions, req.Context)
	question, err := h.taskSvc.CreateMessageWithID(ctx, questionID, &service.CreateMessageRequest{
		TaskSessionID: target.childSession.ID,
		TaskID:        target.childTask.ID,
		Content:       req.Questions[0].Prompt,
		AuthorType:    "agent",
		Type:          string(models.MessageTypeClarificationRequest),
		Metadata:      questionMetadata,
		RequestsInput: true,
	})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to persist parent question: "+err.Error(), nil)
	}

	parentPrompt := formatParentQuestionPrompt(questionID, target.parentTask, target.childTask, req.Questions, req.Context)
	parentMetadata := map[string]interface{}{
		models.MetaKeyParentQuestionID:       questionID,
		models.MetaKeyParentQuestion:         true,
		models.MetaKeyParentQuestionParentID: target.parentTask.ID,
		models.MetaKeyParentQuestionChildID:  target.childTask.ID,
		"sender_task_id":                     target.childTask.ID,
		"sender_task_title":                  target.childTask.Title,
		"sender_session_id":                  target.childSession.ID,
	}
	if _, dispatchErr := h.dispatchTaskMessage(ctx, target.parentTask.ID, target.parentSession, parentPrompt, parentMetadata, false, false); dispatchErr != nil {
		_ = h.taskSvc.DeleteMessage(ctx, question.ID)
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to send question to direct parent: "+dispatchErr.Error(), nil)
	}

	// Mark the child as waiting before asking the orchestrator to cancel the
	// active turn. The pause is best effort because the durable message and
	// parent delivery are already committed; a later lifecycle watchdog can
	// still reconcile the session if the process exits during the race.
	h.setSessionWaitingForInput(ctx, target.childTask.ID, target.childSession.ID)
	if h.inputPauser != nil {
		if _, pauseErr := h.inputPauser.PauseForClarificationInput(context.WithoutCancel(ctx), target.childSession.ID); pauseErr != nil {
			h.logger.Warn("failed to pause autopilot child after parent question",
				zap.String("question_id", questionID), zap.String("session_id", target.childSession.ID), zap.Error(pauseErr))
		}
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"question_id":    questionID,
		"status":         "waiting_for_parent",
		"parent_task_id": target.parentTask.ID,
	})
}

// parentQuestionReply is the result of validating a reply_to_question_id.
// A terminal answer is returned as alreadyAnswered so retries do not dispatch
// the same answer twice.
type parentQuestionReply struct {
	message         *models.Message
	alreadyAnswered bool
}

func (h *Handlers) validateParentQuestionReply(ctx context.Context, questionID, targetTaskID string, senderTask, targetTask *models.Task) (*parentQuestionReply, error) {
	if strings.TrimSpace(questionID) == "" {
		return nil, nil
	}
	if h.taskSvc == nil {
		return nil, errors.New("task service is not available")
	}
	question, err := h.taskSvc.GetMessage(ctx, questionID)
	if err != nil || question == nil {
		return nil, fmt.Errorf("parent question %q not found", questionID)
	}
	if err := validateParentQuestionOwnership(question, targetTaskID, senderTask, targetTask); err != nil {
		return nil, err
	}
	metadata := question.Metadata
	status, _ := metadata[models.MetaKeyParentQuestionStatus].(string)
	if status == parentQuestionStatusAnswered {
		return &parentQuestionReply{message: question, alreadyAnswered: true}, nil
	}
	if status != parentQuestionStatusPending {
		return nil, fmt.Errorf("parent question %q is no longer pending", questionID)
	}
	return &parentQuestionReply{message: question}, nil
}

func validateParentQuestionOwnership(question *models.Message, targetTaskID string, senderTask, targetTask *models.Task) error {
	if question.Type != models.MessageTypeClarificationRequest || question.TaskID != targetTaskID || targetTask == nil || senderTask == nil {
		return errors.New("reply_to_question_id does not belong to the target child task")
	}
	metadata := question.Metadata
	parentMarker, _ := metadata[models.MetaKeyParentQuestion].(bool)
	parentID, _ := metadata[models.MetaKeyParentQuestionParentID].(string)
	childID, _ := metadata[models.MetaKeyParentQuestionChildID].(string)
	if !parentMarker || parentID == "" || childID == "" || parentID != senderTask.ID || targetTask.ParentID != senderTask.ID || childID != targetTask.ID {
		return errors.New("only the direct parent task can answer this question")
	}
	return nil
}

func (h *Handlers) markParentQuestionAnswered(ctx context.Context, question *models.Message, answer string) error {
	if question == nil || h.taskSvc == nil {
		return errors.New("parent question is not available")
	}
	metadata := cloneTaskMessageMetadataMap(question.Metadata)
	metadata["status"] = parentQuestionStatusAnswered
	metadata[models.MetaKeyParentQuestionStatus] = parentQuestionStatusAnswered
	metadata[models.MetaKeyParentQuestionResponse] = answer
	metadata["parent_question_answered_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	question.Metadata = metadata
	question.RequestsInput = false
	return h.taskSvc.UpdateMessage(ctx, question)
}

func (h *Handlers) restoreParentQuestionPending(ctx context.Context, question *models.Message) error {
	if question == nil || h.taskSvc == nil {
		return errors.New("parent question is not available")
	}
	metadata := cloneTaskMessageMetadataMap(question.Metadata)
	metadata["status"] = parentQuestionStatusPending
	metadata[models.MetaKeyParentQuestionStatus] = parentQuestionStatusPending
	delete(metadata, models.MetaKeyParentQuestionResponse)
	delete(metadata, "parent_question_answered_at")
	question.Metadata = metadata
	question.RequestsInput = true
	return h.taskSvc.UpdateMessage(ctx, question)
}

func parentQuestionMetadata(questionID string, parentTask, childTask *models.Task, questions []clarification.Question, questionContext string) map[string]interface{} {
	questionData := make([]clarification.Question, len(questions))
	copy(questionData, questions)
	return map[string]interface{}{
		"status":                             parentQuestionStatusPending,
		"pending_id":                         questionID,
		"question_id":                        questionID,
		"questions":                          questionData,
		"question":                           questions[0],
		"context":                            questionContext,
		models.MetaKeyParentQuestionID:       questionID,
		models.MetaKeyParentQuestion:         true,
		models.MetaKeyParentQuestionParentID: parentTask.ID,
		models.MetaKeyParentQuestionChildID:  childTask.ID,
		models.MetaKeyParentQuestionStatus:   parentQuestionStatusPending,
	}
}

func formatParentQuestionPrompt(questionID string, parentTask, childTask *models.Task, questions []clarification.Question, questionContext string) string {
	data, _ := json.MarshalIndent(questions, "", "  ")
	contextLine := strings.TrimSpace(questionContext)
	if contextLine == "" {
		contextLine = "(no additional context)"
	}
	return fmt.Sprintf("AUTOPILOT CHILD QUESTION\n\nChild task: %s (%s)\nParent task: %s (%s)\nQuestion ID: %s\nContext: %s\n\nQuestions:\n%s\n\nReply to this child with message_task_kandev using task_id=%s, reply_to_question_id=%s, and your answer as the prompt. Do not use ask_user_question_kandev for this child question. The child turn is paused until you answer.", childTask.Title, childTask.ID, parentTask.Title, parentTask.ID, questionID, contextLine, data, childTask.ID, questionID)
}
