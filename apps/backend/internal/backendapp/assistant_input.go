package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/redaction"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestrator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"slices"
)

const inputStopFailed = "failed"

type assistantInputSessions interface {
	GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error)
}
type assistantSessionStopper interface {
	StopTaskSessionForCoordinator(context.Context, string, string) (bool, error)
}
type assistantInputResolver struct {
	attention      *assistantAttentionReader
	sessions       assistantInputSessions
	permissions    permissionResolver
	clarifications clarificationBundleResolver
	stopper        assistantSessionStopper
}

func inputRejected(status int, reason string) error {
	return &shared.InputRejection{Status: status, Reason: reason}
}
func (a *assistantInputResolver) ReadInput(ctx context.Context, b *shared.AssistantBinding, row shared.Attention) (*shared.AttentionInput, error) {
	if _, err := a.attention.authorizedTask(ctx, b, row.TaskID); err != nil {
		return nil, inputRejected(404, "native_input_unavailable")
	}
	request, err := a.attention.readCanonicalInput(ctx, row)
	if err != nil {
		return nil, err
	}
	if request == nil || request.TaskID != row.TaskID || request.SessionID != row.SessionID {
		return nil, inputRejected(404, "native_input_unavailable")
	}
	session, err := a.sessions.GetTaskSession(ctx, row.SessionID)
	if err != nil || session == nil || session.TaskID != row.TaskID {
		return nil, inputRejected(404, "native_session_unavailable")
	}
	live, err := a.attention.permissions.ListPendingAgentPermissions(ctx, row.TaskID, row.SessionID)
	if err != nil {
		return nil, err
	}
	return a.presentInput(row, request, session.AgentProfileID, live)
}
func (a *assistantInputResolver) presentInput(row shared.Attention, request *taskmodels.Interaction, profileID string, live []streams.PendingAgentPermission) (*shared.AttentionInput, error) {
	source := a.attention.interactionSource(request, live)
	input := &shared.AttentionInput{SourceID: source.SourceID, Kind: source.Kind, State: source.State, SourceRevision: source.SourceRevision, TaskID: row.TaskID, SessionID: row.SessionID, PendingID: request.ID, RequestID: request.RequestID, ProfileID: profileID, Summary: source.Summary}
	for _, question := range request.Questions {
		input.Questions = append(input.Questions, shared.InputQuestion{ID: question.ID, Title: redaction.NewRedactor().String(question.Title), Prompt: redaction.NewRedactor().String(question.Prompt), Options: assistantInputOptions(question.Options), AssistantDelegable: question.AssistantDelegable})
	}
	for _, permission := range live {
		if permission.PendingID != request.ID || permission.RequestID != request.RequestID || permission.SessionID != row.SessionID || permission.TaskID != row.TaskID {
			continue
		}
		input.Permission = &permission
		for _, option := range permission.Options {
			input.Options = append(input.Options, shared.InputOption{ID: option.OptionID, Label: redaction.NewRedactor().String(option.Name), Kind: string(option.Kind)})
		}
	}
	raw, err := json.Marshal(input)
	if err != nil || len(raw) > 32*1024 {
		return nil, inputRejected(422, "open_native_task_for_full_input")
	}
	return input, nil
}
func assistantInputOptions(options []taskmodels.InteractionOption) []shared.InputOption {
	result := make([]shared.InputOption, 0, len(options))
	for _, option := range options {
		result = append(result, shared.InputOption{ID: option.ID, Label: redaction.NewRedactor().String(option.Label), Description: redaction.NewRedactor().String(option.Description), Kind: option.Kind})
	}
	return result
}
func (a *assistantInputResolver) ResolveInput(ctx context.Context, b *shared.AssistantBinding, row shared.Attention, response shared.InputResponse) (any, error) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.UserID != b.OwnerUserID {
		return nil, inputRejected(403, "native_input_owner_required")
	}
	input, err := a.ReadInput(ctx, b, row)
	if err != nil {
		return nil, err
	}
	if input.State != shared.AttentionPending || input.SourceRevision != row.SourceRevision {
		return nil, inputRejected(409, "native_input_expired_or_superseded")
	}
	if err := shared.CheckWorkspaceEffect(ctx); err != nil {
		return nil, err
	}
	if input.Kind == attentionPermission {
		return a.resolvePermission(ctx, b, input, response)
	}
	if input.Kind != attentionQuestion {
		return nil, inputRejected(403, "native_input_not_answerable")
	}
	return a.resolveQuestion(ctx, b, input, response)
}
func (a *assistantInputResolver) resolvePermission(ctx context.Context, b *shared.AssistantBinding, input *shared.AttentionInput, response shared.InputResponse) (any, error) {
	if response.ActorType != string(taskmodels.MessageAuthorUser) || response.ActorID != b.OwnerUserID || a.permissions == nil {
		return nil, inputRejected(403, "permission_requires_human")
	}
	if !slices.ContainsFunc(input.Options, func(option shared.InputOption) bool { return option.ID == response.OptionID }) {
		return nil, inputRejected(422, "permission_option_not_offered")
	}
	result, err := a.permissions.ResolveAgentPermission(ctx, orchestrator.ResolveAgentPermissionRequest{TaskID: input.TaskID, SessionID: input.SessionID, RequestID: input.RequestID, PendingID: input.PendingID, OptionID: response.OptionID, Source: taskmodels.PermissionSourceWeb})
	if errors.Is(err, orchestrator.ErrPermissionOptionNotOffered) || errors.Is(err, orchestrator.ErrPermissionStale) || errors.Is(err, orchestrator.ErrPermissionNotFound) {
		return nil, inputRejected(409, "permission_expired_or_superseded")
	}
	return result, err
}
func (a *assistantInputResolver) resolveQuestion(ctx context.Context, b *shared.AssistantBinding, input *shared.AttentionInput, response shared.InputResponse) (any, error) {
	if a.clarifications == nil {
		return nil, fmt.Errorf("native clarification resolver unavailable")
	}
	var err error
	ctx, err = assistantQuestionActor(ctx, b, input, response)
	if err != nil {
		return nil, err
	}
	outcome := clarification.Outcome{Rejected: response.Rejected, RejectReason: response.RejectReason}
	for _, answer := range response.Answers {
		outcome.Answers = append(outcome.Answers, clarification.Answer{QuestionID: answer.QuestionID, SelectedOptions: answer.SelectedOptions, CustomText: answer.CustomText})
	}
	result, claimed, err := a.clarifications.ResolveBundle(ctx, input.PendingID, outcome)
	if clarification.IsValidationError(err) {
		return nil, inputRejected(422, "invalid_native_answer")
	}
	if clarification.IsNotActiveError(err) {
		return nil, inputRejected(409, "native_input_expired_or_superseded")
	}
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("native clarification receipt unavailable")
	}
	if !claimed {
		return nil, inputRejected(409, "native_input_already_resolved")
	}
	return result, nil
}
func (a *assistantInputResolver) StopSession(ctx context.Context, b *shared.AssistantBinding, task, session string) (string, error) {
	if _, err := a.attention.authorizedTask(ctx, b, task); err != nil {
		return inputStopFailed, err
	}
	if a.stopper == nil {
		return inputStopFailed, fmt.Errorf("native session control unavailable")
	}
	if err := shared.CheckWorkspaceEffect(ctx); err != nil {
		return inputStopFailed, err
	}
	changed, err := a.stopper.StopTaskSessionForCoordinator(ctx, task, session)
	if err != nil {
		return capabilityUnknown, err
	}
	if !changed {
		return "already_finished", nil
	}
	return "stopped", nil
}

func assistantQuestionActor(ctx context.Context, b *shared.AssistantBinding, input *shared.AttentionInput, response shared.InputResponse) (context.Context, error) {
	switch response.ActorType {
	case string(taskmodels.MessageAuthorAgent):
		if response.ActorID != b.OrchestratorID || len(response.SourceMemoryIDs) == 0 {
			return ctx, inputRejected(403, "known_answer_scope_required")
		}
		for _, question := range input.Questions {
			if !question.AssistantDelegable {
				return ctx, inputRejected(403, "question_requires_human")
			}
		}
		ctx = taskmodels.WithAssistantClarification(ctx, response.ActorID, response.SourceMemoryIDs)
	case string(taskmodels.MessageAuthorUser):
		if response.ActorID != b.OwnerUserID {
			return ctx, inputRejected(403, "native_input_owner_required")
		}
	default:
		return ctx, inputRejected(403, "native_input_actor_required")
	}
	return ctx, nil
}
