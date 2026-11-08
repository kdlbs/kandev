package coordinator

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/clarification"
)

// Closed failure codes of an answer proposal and the refusal codes that
// identify a refused one.
const (
	failInvalidDraft         = "invalid_draft"
	failOptionNotOffered     = "option_not_offered"
	failResolutionInProgress = "resolution_in_progress"
	failNotDelivered         = "not_delivered"
	refuseStale              = "stale"
	refuseTaskArchived       = failTaskArchived
)

// Delivery errors an AnswerDelivery adapter maps the permission operation's
// errors onto.
var (
	ErrDeliveryStale             = errors.New("coordinator: permission request is no longer current")
	ErrDeliveryTaskOrSessionGone = errors.New("coordinator: task or session not found")
	ErrDeliveryOptionNotOffered  = errors.New("coordinator: option is not offered")
	ErrDeliveryInProgress        = errors.New("coordinator: resolution is in progress")
	ErrDeliveryNotDelivered      = errors.New("coordinator: nothing was delivered")
)

// AnswerDelivery is the pair of resolution operations an approved answer uses,
// the same ones the in-place answer calls.
type AnswerDelivery interface {
	ResolveBundle(ctx context.Context, pendingID string, outcome clarification.Outcome) (*clarification.Resolution, bool, error)
	RespondToPermission(ctx context.Context, taskID, sessionID, requestID, pendingID, optionID string) error
}

// execRefusal is a definite refusal found while executing: the proposal
// settles as refused with the code rather than failed.
type execRefusal struct{ code string }

func (e *execRefusal) Error() string { return "refused: " + e.code }

func refusal(code string) error { return &execRefusal{code: code} }

// answerRetryable reports whether a failed answer proposal can be approved
// again.
func answerRetryable(code string) bool {
	switch code {
	case failInvalidDraft, failOptionNotOffered, failResolutionInProgress, failNotDelivered:
		return true
	}
	return false
}

// Execute delivers the frozen draft to the live target.
func (k *answerKind) Execute(ctx context.Context, claim Claim) (Outcome, error) {
	var spec answerSpec
	if err := json.Unmarshal(claim.Spec, &spec); err != nil {
		k.svc.logger.Warn("answer proposal spec is unreadable")
		return Outcome{}, failWith(failInvalidDraft)
	}
	deps := k.svc.kindDeps
	if deps.Answer == nil {
		return Outcome{}, failWith(failNotDelivered)
	}
	if spec.TaskID != claim.TargetTaskID {
		return Outcome{}, refusal(refuseTaskArchived)
	}
	if err := k.checkTaskAvailable(ctx, claim, spec.TaskID); err != nil {
		return Outcome{}, err
	}
	set, err := k.svc.targetReader().CurrentTargets(ctx, claim.WorkspaceID, spec.TaskID)
	switch {
	case errors.Is(err, ErrNotFound):
		return Outcome{}, refusal(refuseTaskArchived)
	case err != nil:
		return Outcome{}, failWith(failNotDelivered)
	case !targetIdentityMatches(set, spec.Target, spec.PendingID, spec.RequestID):
		return Outcome{}, refusal(refuseStale)
	}
	draft := normalizeDraft(spec.Draft)
	if err := validateDraftAgainst(spec.Target, draft, set); err != nil {
		return Outcome{}, failWith(failInvalidDraft)
	}
	if spec.Target == answerTargetPermission {
		return k.deliverPermission(ctx, claim, spec, draft, set)
	}
	return k.deliverClarification(ctx, claim, spec, draft, set)
}

// checkTaskAvailable refuses a task that is deleted, archived, in another
// workspace, unwatched or a coordinator conversation.
func (k *answerKind) checkTaskAvailable(ctx context.Context, claim Claim, taskID string) error {
	if _, err := k.svc.proposeTarget(ctx, claim.Coordinator, taskID); err != nil {
		var fe *FieldError
		if errors.As(err, &fe) {
			return refusal(refuseTaskArchived)
		}
		return failWith(failNotDelivered)
	}
	return nil
}

// refusalAfterGone decides, for a delivery error that says the session or
// bundle is gone, between an unavailable task and a stale target.
func (k *answerKind) refusalAfterGone(ctx context.Context, claim Claim, taskID string) error {
	if err := k.checkTaskAvailable(ctx, claim, taskID); err != nil {
		return err
	}
	return refusal(refuseStale)
}

func (k *answerKind) deliverClarification(ctx context.Context, claim Claim, spec answerSpec, draft answerDraft, set TargetSet) (Outcome, error) {
	answers := make([]clarification.Answer, len(draft.Answers))
	for i, e := range draft.Answers {
		answers[i] = clarification.Answer{QuestionID: e.QuestionID, SelectedOptions: e.OptionIDs, CustomText: e.CustomText}
	}
	_, won, err := k.svc.kindDeps.Answer.ResolveBundle(ctx, spec.PendingID, clarification.Outcome{Answers: answers})
	if err != nil {
		return Outcome{}, k.clarificationFailure(ctx, claim, spec.TaskID, won, err)
	}
	if !won {
		return Outcome{}, refusal(refuseStale)
	}
	type delivered struct {
		QuestionID string   `json:"question_id"`
		OptionIDs  []string `json:"option_ids"`
	}
	out := make([]delivered, len(draft.Answers))
	for i, e := range draft.Answers {
		out[i] = delivered{QuestionID: e.QuestionID, OptionIDs: append([]string{}, e.OptionIDs...)}
	}
	raw, _ := json.Marshal(map[string]any{sessionIDKey: set.Clarification.SessionID, "pending_id": spec.PendingID, "answers": out})
	return Outcome{TaskID: spec.TaskID, OutcomeJSON: string(raw), Detail: clarificationDetail(set.Clarification, draft)}, nil
}

func (k *answerKind) clarificationFailure(ctx context.Context, claim Claim, taskID string, won bool, err error) error {
	switch {
	case clarification.IsNotActiveError(err):
		return refusal(refuseStale)
	case errors.Is(err, clarification.ErrBundleNotFound):
		return k.refusalAfterGone(ctx, claim, taskID)
	case clarification.IsValidationError(err):
		return failWith(failInvalidDraft)
	case won:
		return failWith(outcomeUnknownError)
	}
	return failWith(failNotDelivered)
}

func (k *answerKind) deliverPermission(ctx context.Context, claim Claim, spec answerSpec, draft answerDraft, set TargetSet) (Outcome, error) {
	p := set.Permission
	err := k.svc.kindDeps.Answer.RespondToPermission(ctx, spec.TaskID, p.SessionID, p.RequestID, p.PendingID, draft.OptionID)
	switch {
	case err == nil:
	case errors.Is(err, ErrDeliveryStale):
		return Outcome{}, refusal(refuseStale)
	case errors.Is(err, ErrDeliveryTaskOrSessionGone):
		return Outcome{}, k.refusalAfterGone(ctx, claim, spec.TaskID)
	case errors.Is(err, ErrDeliveryOptionNotOffered):
		return Outcome{}, failWith(failOptionNotOffered)
	case errors.Is(err, ErrDeliveryInProgress):
		return Outcome{}, failWith(failResolutionInProgress)
	case errors.Is(err, ErrDeliveryNotDelivered):
		return Outcome{}, failWith(failNotDelivered)
	default:
		return Outcome{}, failWith(outcomeUnknownError)
	}
	raw, _ := json.Marshal(map[string]any{sessionIDKey: p.SessionID, "pending_id": p.PendingID, "option_id": draft.OptionID})
	return Outcome{TaskID: spec.TaskID, OutcomeJSON: string(raw), Detail: permissionDetail(p, draft)}, nil
}

// sessionIDKey names the session in a delivery payload.
const sessionIDKey = "session_id"
