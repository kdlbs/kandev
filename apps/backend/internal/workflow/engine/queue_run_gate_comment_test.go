package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newObservedFanOutLogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	return log, logs
}

func gateCommentInput(seats []ParticipantInfo, decisions DecisionStore, log *logger.Logger, authorID string) (
	QueueRunForEachParticipantCallback, ActionInput, *fakeRunQueue,
) {
	q := &fakeRunQueue{}
	parts := fakeParticipants{list: seats}
	cb := QueueRunForEachParticipantCallback{Adapter: q, Participants: parts, Decisions: decisions, Logger: log}
	in := ActionInput{
		Trigger:     TriggerOnComment,
		State:       MachineState{TaskID: "task-1"},
		Step:        StepSpec{ID: "review"},
		OperationID: "op-1",
		Payload:     OnCommentPayload{CommentID: "c-1", AuthorID: authorID, Body: "please decide"},
		Action: Action{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role:        "reviewer",
				Reason:      "task_comment",
				SkipDecided: true,
			},
		},
	}
	return cb, in, q
}

func TestQueueRunForEachParticipantCallback_SkipDecidedWakesOnlyUndecidedSeat(t *testing.T) {
	decisions := newFakeDecisionStore()
	decisions.byKey[dkey("task-1", "review")] = []DecisionInfo{
		{ParticipantID: "p-decided", TaskID: "task-1", StepID: "review", Decision: "approve"},
	}
	seats := []ParticipantInfo{
		{ID: "p-decided", Role: "reviewer", AgentProfileID: "rev-decided"},
		{ID: "p-undecided", Role: "reviewer", AgentProfileID: "rev-undecided"},
	}
	cb, in, q := gateCommentInput(seats, decisions, nil, "")

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 queued run, got %d: %#v", len(q.calls), q.calls)
	}
	got := q.calls[0]
	if got.AgentProfileID != "rev-undecided" {
		t.Fatalf("agent_profile_id = %q, want rev-undecided", got.AgentProfileID)
	}
	if got.Payload["comment_id"] != "c-1" {
		t.Fatalf("payload comment_id = %v, want c-1", got.Payload["comment_id"])
	}
}

func TestQueueRunForEachParticipantCallback_SkipDecidedSupersededCountsAsUndecided(t *testing.T) {
	supersededAt := time.Now()
	decisions := newFakeDecisionStore()
	decisions.byKey[dkey("task-1", "review")] = []DecisionInfo{
		{
			ParticipantID: "p-superseded", TaskID: "task-1", StepID: "review", Decision: "reject",
			SupersededAt: &supersededAt,
		},
	}
	seats := []ParticipantInfo{
		{ID: "p-superseded", Role: "reviewer", AgentProfileID: "rev-superseded"},
	}
	cb, in, q := gateCommentInput(seats, decisions, nil, "")

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected superseded-only seat to still be woken, got %d calls", len(q.calls))
	}
	if q.calls[0].AgentProfileID != "rev-superseded" {
		t.Fatalf("agent_profile_id = %q, want rev-superseded", q.calls[0].AgentProfileID)
	}
}

func TestQueueRunForEachParticipantCallback_AuthorExcludedWithSkipDecided(t *testing.T) {
	decisions := newFakeDecisionStore()
	seats := []ParticipantInfo{
		{ID: "p-author", Role: "reviewer", AgentProfileID: "rev-author"},
		{ID: "p-other", Role: "reviewer", AgentProfileID: "rev-other"},
	}
	cb, in, q := gateCommentInput(seats, decisions, nil, "rev-author")

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 || q.calls[0].AgentProfileID != "rev-other" {
		t.Fatalf("expected only rev-other queued, got %#v", q.calls)
	}
}

func TestQueueRunForEachParticipantCallback_AuthorExcludedWithoutSkipDecided(t *testing.T) {
	q := &fakeRunQueue{}
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p-author", Role: "reviewer", AgentProfileID: "rev-author"},
		{ID: "p-other", Role: "reviewer", AgentProfileID: "rev-other"},
	}}
	cb := QueueRunForEachParticipantCallback{Adapter: q, Participants: parts}
	in := ActionInput{
		Trigger:     TriggerOnComment,
		State:       MachineState{TaskID: "task-1"},
		Step:        StepSpec{ID: "review"},
		OperationID: "op-1",
		Payload:     OnCommentPayload{CommentID: "c-1", AuthorID: "rev-author"},
		Action: Action{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role:   "reviewer",
				Reason: "task_comment",
			},
		},
	}

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 || q.calls[0].AgentProfileID != "rev-other" {
		t.Fatalf("expected only rev-other queued, got %#v", q.calls)
	}
}

func TestQueueRunForEachParticipantCallback_HumanAuthorExcludesNobody(t *testing.T) {
	decisions := newFakeDecisionStore()
	seats := []ParticipantInfo{
		{ID: "p1", Role: "reviewer", AgentProfileID: "rev-A"},
		{ID: "p2", Role: "reviewer", AgentProfileID: "rev-B"},
	}
	cb, in, q := gateCommentInput(seats, decisions, nil, "user-1")

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 2 {
		t.Fatalf("expected both seats queued for a human author, got %d", len(q.calls))
	}
}

func TestQueueRunForEachParticipantCallback_EmptyAuthorExcludesNobody(t *testing.T) {
	decisions := newFakeDecisionStore()
	seats := []ParticipantInfo{
		{ID: "p1", Role: "reviewer", AgentProfileID: "rev-A"},
		{ID: "p2", Role: "reviewer", AgentProfileID: "rev-B"},
	}
	cb, in, q := gateCommentInput(seats, decisions, nil, "")

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 2 {
		t.Fatalf("expected both seats queued for an empty author, got %d", len(q.calls))
	}
}

// TestQueueRunForEachParticipantCallback_OnEnterExcludesNobodyAndSkipsNobody
// pins AC-OFFICE-GATE-COMMENT-005.2/.3: the existing step-entry fan-out
// (SkipDecided false, no comment payload) is unaffected by these filters.
func TestQueueRunForEachParticipantCallback_OnEnterExcludesNobodyAndSkipsNobody(t *testing.T) {
	decisions := newFakeDecisionStore()
	decisions.byKey[dkey("task-1", "review")] = []DecisionInfo{
		{ParticipantID: "p-decided", TaskID: "task-1", StepID: "review", Decision: "approve"},
	}
	q := &fakeRunQueue{}
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p-decided", Role: "reviewer", AgentProfileID: "rev-decided"},
		{ID: "p-undecided", Role: "reviewer", AgentProfileID: "rev-undecided"},
	}}
	cb := QueueRunForEachParticipantCallback{Adapter: q, Participants: parts, Decisions: decisions}
	in := ActionInput{
		Trigger: TriggerOnEnter,
		State:   MachineState{TaskID: "task-1"},
		Step:    StepSpec{ID: "review"},
		Action: Action{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role:   "reviewer",
				Reason: "review_started",
			},
		},
	}

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 2 {
		t.Fatalf("expected both seats queued at step-entry, got %d", len(q.calls))
	}
}

func TestQueueRunForEachParticipantCallback_DecisionReadErrorFailsOpen(t *testing.T) {
	decisions := newFakeDecisionStore()
	decisions.listErr = errors.New("db unavailable")
	log, logs := newObservedFanOutLogger(t)
	seats := []ParticipantInfo{
		{ID: "p1", Role: "reviewer", AgentProfileID: "rev-A"},
		{ID: "p2", Role: "reviewer", AgentProfileID: "rev-B"},
	}
	cb, in, q := gateCommentInput(seats, decisions, log, "")

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 2 {
		t.Fatalf("expected fail-open to wake every non-author seat, got %d", len(q.calls))
	}
	if got := logs.FilterMessageSnippet("decision read failed").Len(); got != 1 {
		t.Fatalf("expected exactly 1 warning log, got %d", got)
	}
}

func TestQueueRunForEachParticipantCallback_NilDecisionsWithSkipDecidedFailsOpen(t *testing.T) {
	log, logs := newObservedFanOutLogger(t)
	seats := []ParticipantInfo{
		{ID: "p1", Role: "reviewer", AgentProfileID: "rev-A"},
		{ID: "p2", Role: "reviewer", AgentProfileID: "rev-B"},
	}
	cb, in, q := gateCommentInput(seats, nil, log, "")

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 2 {
		t.Fatalf("expected fail-open to wake every non-author seat, got %d", len(q.calls))
	}
	if got := logs.FilterMessageSnippet("decision read failed").Len(); got != 1 {
		t.Fatalf("expected exactly 1 warning log, got %d", got)
	}
}

func TestQueueRunForEachParticipantCallback_NilDecisionsWithSkipDecidedNoLoggerNoPanic(t *testing.T) {
	seats := []ParticipantInfo{
		{ID: "p1", Role: "reviewer", AgentProfileID: "rev-A"},
	}
	cb, in, q := gateCommentInput(seats, nil, nil, "")

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected fail-open with nil Logger to still queue the seat, got %d", len(q.calls))
	}
}

func TestReadQueueRunForEachParticipantConfig_SkipDecidedNonBooleanLeavesItOff(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{"string", "true"},
		{"int", 1},
		{"missing", nil},
		{"false", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := map[string]any{"role": "reviewer"}
			if tc.name != "missing" {
				config["skip_decided"] = tc.value
			}
			got := readQueueRunForEachParticipantConfig(config)
			if got.SkipDecided {
				t.Fatalf("SkipDecided = true, want false for value %#v", tc.value)
			}
		})
	}
}

func TestReadQueueRunForEachParticipantConfig_SkipDecidedTrue(t *testing.T) {
	got := readQueueRunForEachParticipantConfig(map[string]any{"role": "reviewer", "skip_decided": true})
	if !got.SkipDecided {
		t.Fatalf("SkipDecided = false, want true")
	}
}

// --- Idempotency digest: SkipDecided must salt the key only when true ---

func fanOutDigestInput(skipDecided bool) ActionInput {
	return ActionInput{
		Trigger: TriggerOnComment,
		State:   MachineState{TaskID: "task-1"},
		Step:    StepSpec{ID: "review"},
		Action: Action{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role:        "reviewer",
				Reason:      "task_comment",
				SkipDecided: skipDecided,
			},
		},
	}
}

func TestQueueActionDigest_SkipDecidedTrueChangesDigest(t *testing.T) {
	withSkip := queueActionDigest(fanOutDigestInput(true))
	withoutSkip := queueActionDigest(fanOutDigestInput(false))
	if withSkip == withoutSkip {
		t.Fatalf("expected skip_decided:true to change the digest, both = %q", withSkip)
	}
}

// TestQueueActionDigest_SkipDecidedFalseMatchesPreExistingDigestShape pins
// that an action which never opts into skip_decided keeps the exact digest
// it had before this field existed: the "skip_decided" key is dropped by
// omitempty when false, so re-deriving the digest from the pre-WO-01 key
// shape (role/reason/payload only) must match today's digest byte-for-byte.
func TestQueueActionDigest_SkipDecidedFalseMatchesPreExistingDigestShape(t *testing.T) {
	in := fanOutDigestInput(false)
	got := queueActionDigest(in)

	preExistingKey := struct {
		Kind    ActionKind     `json:"kind"`
		Target  string         `json:"target,omitempty"`
		TaskID  string         `json:"task_id,omitempty"`
		Role    string         `json:"role,omitempty"`
		Reason  string         `json:"reason"`
		Payload map[string]any `json:"payload,omitempty"`
	}{
		Kind:   in.Action.Kind,
		Role:   "reviewer",
		Reason: "task_comment",
	}
	b, err := json.Marshal(preExistingKey)
	if err != nil {
		t.Fatalf("marshal pre-existing key: %v", err)
	}
	sum := sha256.Sum256(b)
	want := hex.EncodeToString(sum[:8])

	if got != want {
		t.Fatalf("digest changed for an action that never sets skip_decided: got %q, want %q", got, want)
	}
}

// --- Sentinel wrap: ErrCommentFanOutIncomplete ---

func TestQueueRunForEachParticipantCallback_OnCommentMissingRoleWrapsSentinel(t *testing.T) {
	cb := QueueRunForEachParticipantCallback{Adapter: &fakeRunQueue{}, Participants: fakeParticipants{}}
	in := ActionInput{
		Trigger: TriggerOnComment,
		State:   MachineState{TaskID: "task-1"},
		Step:    StepSpec{ID: "review"},
		Payload: OnCommentPayload{CommentID: "c-1"},
		Action: Action{
			Kind:                       ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: nil,
		},
	}
	_, err := cb.Execute(context.Background(), in)
	if err == nil || !errors.Is(err, ErrCommentFanOutIncomplete) {
		t.Fatalf("expected ErrCommentFanOutIncomplete, got %v", err)
	}
	if !strings.Contains(err.Error(), "task task-1") || !strings.Contains(err.Error(), "step review") {
		t.Fatalf("error missing task/step identity: %v", err)
	}
	if !strings.Contains(err.Error(), `role ""`) {
		t.Fatalf("error missing empty role marker: %v", err)
	}
	if !strings.Contains(err.Error(), "missing role") {
		t.Fatalf("error missing 'missing role' text: %v", err)
	}
}

func TestQueueRunForEachParticipantCallback_OnCommentNilAdapterWrapsBothSentinels(t *testing.T) {
	cb := QueueRunForEachParticipantCallback{}
	in := ActionInput{
		Trigger: TriggerOnComment,
		State:   MachineState{TaskID: "task-1"},
		Step:    StepSpec{ID: "review"},
		Payload: OnCommentPayload{CommentID: "c-1"},
		Action: Action{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role: "reviewer",
			},
		},
	}
	_, err := cb.Execute(context.Background(), in)
	if err == nil || !errors.Is(err, ErrCommentFanOutIncomplete) {
		t.Fatalf("expected ErrCommentFanOutIncomplete, got %v", err)
	}
	if !errors.Is(err, ErrActionNotYetWired) {
		t.Fatalf("expected ErrActionNotYetWired to also be wrapped, got %v", err)
	}
}

func TestQueueRunForEachParticipantCallback_OnCommentListParticipantsErrorWrapsSentinel(t *testing.T) {
	boom := errors.New("boom")
	cb := QueueRunForEachParticipantCallback{Adapter: &fakeRunQueue{}, Participants: fakeParticipants{err: boom}}
	in := ActionInput{
		Trigger: TriggerOnComment,
		State:   MachineState{TaskID: "task-1"},
		Step:    StepSpec{ID: "review"},
		Payload: OnCommentPayload{CommentID: "c-1"},
		Action: Action{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role: "reviewer",
			},
		},
	}
	_, err := cb.Execute(context.Background(), in)
	if err == nil || !errors.Is(err, ErrCommentFanOutIncomplete) {
		t.Fatalf("expected ErrCommentFanOutIncomplete, got %v", err)
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped participant-list error, got %v", err)
	}
	if !strings.Contains(err.Error(), `role "reviewer"`) {
		t.Fatalf("error missing configured role: %v", err)
	}
}

func TestQueueRunForEachParticipantCallback_OnCommentQueueRunFailureWrapsSentinel(t *testing.T) {
	boom := errors.New("boom")
	q := &selectiveFailRunQueue{failFor: map[string]error{"rev-A": boom}}
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", AgentProfileID: "rev-A"},
	}}
	cb := QueueRunForEachParticipantCallback{Adapter: q, Participants: parts}
	in := ActionInput{
		Trigger: TriggerOnComment,
		State:   MachineState{TaskID: "task-1"},
		Step:    StepSpec{ID: "review"},
		Payload: OnCommentPayload{CommentID: "c-1"},
		Action: Action{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role: "reviewer",
			},
		},
	}
	_, err := cb.Execute(context.Background(), in)
	if err == nil || !errors.Is(err, ErrCommentFanOutIncomplete) {
		t.Fatalf("expected ErrCommentFanOutIncomplete, got %v", err)
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped per-seat queue_run error, got %v", err)
	}
}

// TestQueueRunForEachParticipantCallback_OnEnterFailuresDoNotWrapSentinel pins
// that the same failure modes under TriggerOnEnter never satisfy
// errors.Is(err, ErrCommentFanOutIncomplete) — the sentinel is exclusive to
// TriggerOnComment (AC-OFFICE-GATE-COMMENT-001.13/.14).
func TestQueueRunForEachParticipantCallback_OnEnterFailuresDoNotWrapSentinel(t *testing.T) {
	cb := QueueRunForEachParticipantCallback{Adapter: &fakeRunQueue{}, Participants: fakeParticipants{}}
	in := ActionInput{
		Trigger: TriggerOnEnter,
		State:   MachineState{TaskID: "task-1"},
		Step:    StepSpec{ID: "review"},
		Action: Action{
			Kind:                       ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: nil,
		},
	}
	_, err := cb.Execute(context.Background(), in)
	if err == nil {
		t.Fatalf("expected missing-role error")
	}
	if errors.Is(err, ErrCommentFanOutIncomplete) {
		t.Fatalf("on_enter must never wrap ErrCommentFanOutIncomplete: %v", err)
	}
}

// --- Engine-level: a failure ahead of the fan-out in the same on_comment
// list must never reach it, so the sentinel is never wrapped and nothing
// is queued (AC-OFFICE-GATE-COMMENT-001.15). ---

func TestHandleTrigger_OnCommentFailingActionAheadOfFanOutNeverReachesFanOut(t *testing.T) {
	fanOutAdapter := &fakeRunQueue{}
	actions := []Action{
		{Kind: ActionQueueRun, QueueRun: &QueueRunAction{Target: "nonsense_target"}},
		{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role:        "reviewer",
				SkipDecided: true,
			},
		},
	}
	store := newTriggerStoreWithActions(TriggerOnComment, actions)
	eng := New(store, MapRegistry{
		ActionQueueRun: QueueRunCallback{Adapter: &fakeRunQueue{}},
		ActionQueueRunForEachParticipant: QueueRunForEachParticipantCallback{
			Adapter:      fanOutAdapter,
			Participants: fakeParticipants{list: []ParticipantInfo{{ID: "p1", Role: "reviewer", AgentProfileID: "rev-A"}}},
		},
	})

	_, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnComment,
		Payload: OnCommentPayload{CommentID: "c-1"}, OperationID: "op-1",
	})
	if err == nil {
		t.Fatalf("expected the leading queue_run's failure to propagate")
	}
	if errors.Is(err, ErrCommentFanOutIncomplete) {
		t.Fatalf("a failure ahead of the fan-out must not wrap ErrCommentFanOutIncomplete: %v", err)
	}
	if len(fanOutAdapter.calls) != 0 {
		t.Fatalf("fan-out must never run once an earlier action failed, got %d calls", len(fanOutAdapter.calls))
	}
	if store.applied["op-1"] {
		t.Fatalf("operation must not be marked applied when the trigger errored")
	}
}

// isOperationAppliedErrorStore wraps triggerStore to fail the leading
// IsOperationApplied idempotency check, so the trigger never reaches
// evaluateActions at all.
type isOperationAppliedErrorStore struct {
	*triggerStore
	err error
}

func (s *isOperationAppliedErrorStore) IsOperationApplied(_ context.Context, _ string) (bool, error) {
	return false, s.err
}

func TestHandleTrigger_OnCommentIsOperationAppliedErrorNeverReachesFanOut(t *testing.T) {
	fanOutAdapter := &fakeRunQueue{}
	actions := []Action{
		{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role:        "reviewer",
				SkipDecided: true,
			},
		},
	}
	store := &isOperationAppliedErrorStore{
		triggerStore: newTriggerStoreWithActions(TriggerOnComment, actions),
		err:          errors.New("store unavailable"),
	}
	eng := New(store, MapRegistry{
		ActionQueueRunForEachParticipant: QueueRunForEachParticipantCallback{
			Adapter:      fanOutAdapter,
			Participants: fakeParticipants{list: []ParticipantInfo{{ID: "p1", Role: "reviewer", AgentProfileID: "rev-A"}}},
		},
	})

	_, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnComment,
		Payload: OnCommentPayload{CommentID: "c-1"}, OperationID: "op-1",
	})
	if err == nil {
		t.Fatalf("expected the IsOperationApplied error to propagate")
	}
	if errors.Is(err, ErrCommentFanOutIncomplete) {
		t.Fatalf("an IsOperationApplied store error must not wrap ErrCommentFanOutIncomplete: %v", err)
	}
	if len(fanOutAdapter.calls) != 0 {
		t.Fatalf("fan-out must never run when the idempotency check itself failed, got %d calls", len(fanOutAdapter.calls))
	}
}
