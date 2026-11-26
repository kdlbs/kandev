package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/clarification"
)

// targetReader is the AnswerTargets the answer kind reads: the service's own
// relay-backed read unless a test or wiring replaces it.
func (s *Service) targetReader() AnswerTargets {
	if s.answerTargets != nil {
		return s.answerTargets
	}
	return s
}

// ValidateShape is stage 1: it decodes every member, returns the task id and
// stores the validated standing order ids in the scope.
func (k *answerKind) ValidateShape(args json.RawMessage, scope *proposeScope) (string, error) {
	shape, err := parseAnswerShape(args)
	if err != nil {
		return "", err
	}
	scope.OrderIDs = shape.OrderIDs
	return shape.TaskID, nil
}

// ValidatePropose is stage 2: the task and target checks, one read of the
// task's targets, then the draft's content against the live target.
func (k *answerKind) ValidatePropose(ctx context.Context, c *Coordinator, args json.RawMessage) (json.RawMessage, error) {
	shape, err := parseAnswerShape(args)
	if err != nil {
		return nil, err
	}
	if _, err := k.svc.proposeTarget(ctx, c, shape.TaskID); err != nil {
		return nil, err
	}
	startedAt := time.Now()
	set, err := k.svc.targetReader().CurrentTargets(ctx, c.WorkspaceID, shape.TaskID)
	if errors.Is(err, ErrNotFound) {
		return nil, notFoundTarget()
	}
	if err != nil {
		return nil, err
	}
	if scope := proposeScopeFrom(ctx); scope != nil {
		scope.Targets, scope.TargetsRead, scope.ReadStartedAt = set, true, startedAt
	}
	if !targetIdentityMatches(set, shape.Target, shape.PendingID, "") {
		return nil, &ProposeRefusal{Code: refusalNothingToAnswer, Field: fieldPendingID}
	}
	shape.Draft = normalizeDraft(shape.Draft)
	if err := validateDraftAgainst(shape.Target, shape.Draft, set); err != nil {
		return nil, err
	}
	spec := answerSpec{
		TaskID: shape.TaskID, Target: shape.Target, PendingID: shape.PendingID, Draft: shape.Draft,
		Snapshot: buildSnapshot(shape.Target, set), Sources: shape.Sources, Confidence: shape.Confidence,
		NeedsDecision: shape.NeedsDecision, Rationale: shape.Rationale,
	}
	if shape.Target == answerTargetPermission {
		spec.RequestID = set.Permission.RequestID
	}
	return json.Marshal(spec)
}

// targetIdentityMatches reports whether the set holds a target of the kind
// with the pending id and, when requestID is set, the permission request id.
func targetIdentityMatches(set TargetSet, target, pendingID, requestID string) bool {
	switch target {
	case answerTargetClarification:
		return set.Clarification != nil && set.Clarification.PendingID == pendingID
	case answerTargetPermission:
		return set.Permission != nil && set.Permission.PendingID == pendingID &&
			(requestID == "" || set.Permission.RequestID == requestID)
	}
	return false
}

// validateDraftAgainst runs the content rules against a live target set. The
// draft must already be normalized.
func validateDraftAgainst(target string, draft answerDraft, set TargetSet) error {
	if target == answerTargetPermission {
		return validatePermissionDraft(draft, set.Permission)
	}
	return validateClarificationDraft(draft, set.Clarification)
}

func validateClarificationDraft(draft answerDraft, live *ClarificationTarget) error {
	if live == nil {
		return fieldErr(fieldDraft, "no clarification to answer")
	}
	if len(draft.Answers) != len(live.Questions) {
		return fieldErr(fieldDraft, "draft must answer each question once, in the bundle's order")
	}
	answers := make([]clarification.Answer, len(draft.Answers))
	for i, e := range draft.Answers {
		if e.QuestionID != live.Questions[i].ID {
			return fieldErr(fieldDraft, "draft must answer each question once, in the bundle's order")
		}
		answers[i] = clarification.Answer{QuestionID: e.QuestionID, SelectedOptions: e.OptionIDs, CustomText: e.CustomText}
	}
	if err := clarification.ValidateOutcome(live.Questions, clarification.Outcome{Answers: answers}); err != nil {
		if clarification.IsValidationError(err) {
			return fieldErr(fieldDraft, err.Error())
		}
		return err
	}
	return nil
}

func validatePermissionDraft(draft answerDraft, live *PermissionTarget) error {
	if live == nil {
		return fieldErr(fieldDraft, "no permission request to answer")
	}
	for _, o := range live.Options {
		if o.ID != draft.OptionID {
			continue
		}
		if o.Kind != "allow_once" && o.Kind != "reject_once" {
			return fieldErr(fieldDraft, "option_kind_not_allowed")
		}
		return nil
	}
	return fieldErr(fieldDraft, "option_not_offered")
}

func buildSnapshot(target string, set TargetSet) answerSnapshot {
	if target == answerTargetPermission {
		p := set.Permission
		snap := &snapshotPermission{Title: truncateRunes(p.Title, answerSnapshotTextRunes), Summary: truncateRunes(p.Summary, answerSnapshotTextRunes),
			Options: make([]snapshotPermissionOption, 0, len(p.Options))}
		for _, o := range p.Options {
			snap.Options = append(snap.Options, snapshotPermissionOption{ID: o.ID, Name: truncateRunes(o.Name, answerSnapshotTextRunes), Kind: o.Kind})
		}
		return answerSnapshot{Permission: snap}
	}
	questions := make([]snapshotQuestion, 0, len(set.Clarification.Questions))
	for _, q := range set.Clarification.Questions {
		text := q.Prompt
		if text == "" {
			text = q.Title
		}
		sq := snapshotQuestion{ID: q.ID, Text: truncateRunes(text, answerSnapshotTextRunes), AllowCustomText: q.AllowCustomText,
			Options: make([]snapshotOption, 0, len(q.Options))}
		for _, o := range q.Options {
			sq.Options = append(sq.Options, snapshotOption{ID: o.ID, Label: truncateRunes(o.Label, answerSnapshotTextRunes)})
		}
		questions = append(questions, sq)
	}
	return answerSnapshot{Questions: questions}
}
