package coordinator

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// Answer targets and confidence levels.
const (
	answerTargetClarification = "clarification"
	answerTargetPermission    = "permission"
	answerSnapshotTextRunes   = 2000
)

// Field names the answer validators report in a *FieldError.
const (
	fieldTarget        = "target"
	fieldPendingID     = "pending_id"
	fieldDraft         = "draft"
	fieldSources       = "sources"
	fieldConfidence    = "confidence"
	fieldNeedsDecision = "needs_decision"
	fieldSnapshot      = "snapshot"
)

// Typed refusals of a propose call (the code is the message and the details
// code).
const (
	refusalNothingToAnswer     = "nothing_to_answer"
	refusalAnswerPending       = "answer_pending"
	refusalApprovalInProgress  = "approval_in_progress"
	messageUnknownField        = "unknown_field"
	messageEmptyAnswer         = "empty_answer"
	messageStoredSpecUnreadble = "stored_spec_unreadable"
)

// ProposeRefusal is a definite refusal of a propose call that is not a field
// error: the call was valid but the current state does not allow it.
type ProposeRefusal struct {
	Code  string
	Field string
}

func (e *ProposeRefusal) Error() string { return e.Code }

// answerDraftEntry is one question's answer in a clarification draft.
type answerDraftEntry struct {
	QuestionID string   `json:"question_id"`
	OptionIDs  []string `json:"option_ids,omitempty"`
	CustomText string   `json:"custom_text,omitempty"`
}

// answerDraft is the proposed answer: entries for a clarification, one option
// id for a permission.
type answerDraft struct {
	Answers  []answerDraftEntry `json:"answers,omitempty"`
	OptionID string             `json:"option_id,omitempty"`
}

type answerSource struct {
	Ref   string `json:"ref"`
	Label string `json:"label,omitempty"`
}

type snapshotOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type snapshotQuestion struct {
	ID              string           `json:"id"`
	Text            string           `json:"text"`
	AllowCustomText *bool            `json:"allow_custom_text,omitempty"`
	Options         []snapshotOption `json:"options"`
}

type snapshotPermissionOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type snapshotPermission struct {
	Title   string                     `json:"title"`
	Summary string                     `json:"summary"`
	Options []snapshotPermissionOption `json:"options"`
}

type answerSnapshot struct {
	Questions  []snapshotQuestion  `json:"questions,omitempty"`
	Permission *snapshotPermission `json:"permission,omitempty"`
}

// answerSpec is the stored spec of an answer proposal.
type answerSpec struct {
	TaskID        string         `json:"task_id"`
	Target        string         `json:"target"`
	PendingID     string         `json:"pending_id"`
	RequestID     string         `json:"request_id,omitempty"`
	Draft         answerDraft    `json:"draft"`
	Snapshot      answerSnapshot `json:"snapshot"`
	Sources       []answerSource `json:"sources"`
	Confidence    string         `json:"confidence"`
	NeedsDecision bool           `json:"needs_decision"`
	Rationale     string         `json:"rationale"`
}

type answerKind struct{ svc *Service }

func (k *answerKind) Kind() string             { return ProposalKindAnswer }
func (k *answerKind) Action() Action           { return ActionAnswer }
func (k *answerKind) ReRunsOnStaleClaim() bool { return false }

// proposeScope carries one propose call's validated standing-order ids and its
// single target read from the validator to the pre-insert callback.
type proposeScope struct {
	OrderIDs      []string
	Targets       TargetSet
	TargetsRead   bool
	ReadStartedAt time.Time
}

type proposeScopeKey struct{}

func withProposeScope(ctx context.Context, scope *proposeScope) context.Context {
	return context.WithValue(ctx, proposeScopeKey{}, scope)
}

func proposeScopeFrom(ctx context.Context) *proposeScope {
	scope, _ := ctx.Value(proposeScopeKey{}).(*proposeScope)
	return scope
}

// argsOwner is implemented by a kind that validates the whole argument object
// itself: its stage 1 replaces ProposeKind's task id and order id checks.
type argsOwner interface {
	ValidateShape(args json.RawMessage, scope *proposeScope) (taskID string, err error)
}

// SupersedeVerdict is a SupersedeOnPropose decision.
type SupersedeVerdict int

const (
	// SupersedeProceed lets the new proposal be inserted.
	SupersedeProceed SupersedeVerdict = iota
	// SupersedeDedupe returns the open proposal as the result.
	SupersedeDedupe
)

// OpenMatcher is implemented by a kind whose open proposal answers a repeat
// call only when the call names the same thing.
type OpenMatcher interface {
	OpenMatches(open *Proposal, args json.RawMessage) bool
}

// SupersedeOnPropose is implemented by a kind that decides, inside the propose
// transaction, what an open proposal means for a new one.
type SupersedeOnPropose interface {
	Supersede(ctx context.Context, tx coordinatorExec, open *Proposal, args json.RawMessage) (SupersedeVerdict, error)
}

// OpenMatches reports whether open is a proposal for the same target and
// pending id as the call. A stored spec that cannot be read is not a match.
func (k *answerKind) OpenMatches(open *Proposal, args json.RawMessage) bool {
	var stored answerSpec
	if json.Unmarshal([]byte(open.RawSpec), &stored) != nil {
		return false
	}
	var call struct {
		Target    string `json:"target"`
		PendingID string `json:"pending_id"`
	}
	if json.Unmarshal(args, &call) != nil {
		return false
	}
	return stored.Target == call.Target && stored.PendingID == strings.TrimSpace(call.PendingID)
}
