package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
)

// EditValidator is implemented by a kind whose edits need a content check
// against live state before the claim.
type EditValidator interface {
	ValidateEditsLive(ctx context.Context, p *Proposal, base, edited json.RawMessage) error
}

// EditCarrier is implemented by a kind that decides for itself whether an
// approve request carries edits.
type EditCarrier interface {
	CarriesEdits(edits ApproveProposalRequest) bool
}

// CarriesEdits reports whether the approve body names any member at all,
// null included.
func (k *answerKind) CarriesEdits(edits ApproveProposalRequest) bool { return len(edits) > 0 }

var answerReadOnlyEditMembers = []string{fieldTarget, fieldPendingID, fieldSources, fieldConfidence, fieldNeedsDecision, fieldSnapshot}

// ValidateEdits accepts only a whole replacement draft. An absent, null or
// empty edit, or one whose normalized draft equals the stored one, returns
// base unchanged.
func (k *answerKind) ValidateEdits(base, edits json.RawMessage) (json.RawMessage, error) {
	members, err := decodeEditMembers(edits)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return base, nil
	}
	for _, name := range answerReadOnlyEditMembers {
		if _, ok := members[name]; ok {
			return nil, fieldErr(name, "not_editable")
		}
	}
	if name, bad := firstUnknown(members, func(n string) bool { return n == fieldDraft }); bad {
		return nil, fieldErr(name, messageUnknownField)
	}
	var stored answerSpec
	if err := json.Unmarshal(base, &stored); err != nil {
		return nil, fieldErr(fieldDraft, messageStoredSpecUnreadble)
	}
	raw := members[fieldDraft]
	if isNull(raw, true) {
		return nil, fieldErr(fieldDraft, "draft is required")
	}
	edited, err := parseAnswerDraft(raw, stored.Target)
	if err != nil {
		return nil, err
	}
	if reflect.DeepEqual(normalizeDraft(stored.Draft), edited) {
		return base, nil
	}
	stored.Draft = edited
	return json.Marshal(stored)
}

func decodeEditMembers(edits json.RawMessage) (map[string]json.RawMessage, error) {
	if len(edits) == 0 || isNull(edits, true) {
		return nil, nil
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(edits, &members); err != nil {
		return nil, fieldErr(fieldBody, "body must be a JSON object")
	}
	return members, nil
}

// ValidateEditsLive validates an edited draft against the live target. A task
// that is unavailable, or a target that is absent or of another identity,
// passes: the claim proceeds and Execute refuses it. A read failure is
// returned as ErrAnswerTargetRead.
func (k *answerKind) ValidateEditsLive(ctx context.Context, p *Proposal, _, edited json.RawMessage) error {
	var spec answerSpec
	if err := json.Unmarshal(edited, &spec); err != nil {
		return fieldErr(fieldDraft, messageStoredSpecUnreadble)
	}
	c, err := k.svc.store.GetCoordinatorByID(ctx, p.CoordinatorID)
	if err != nil {
		return err
	}
	if _, err := k.svc.proposeTarget(ctx, c, spec.TaskID); err != nil {
		var fe *FieldError
		if errors.As(err, &fe) {
			return nil
		}
		return err
	}
	set, err := k.svc.targetReader().CurrentTargets(ctx, p.WorkspaceID, spec.TaskID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !targetIdentityMatches(set, spec.Target, spec.PendingID, spec.RequestID) {
		return nil
	}
	return validateDraftAgainst(spec.Target, normalizeDraft(spec.Draft), set)
}
