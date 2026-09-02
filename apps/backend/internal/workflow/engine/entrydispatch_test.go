package engine

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/stepentry"
)

// TestSessionShapedAndSessionIndependentKindsPartitionCompiledOnEnterKinds
// asserts the two classification maps in this file are exhaustive and
// disjoint over exactly the ActionKind values CompileOnEnterAction (types.go)
// can emit for a single on_enter declaration — the per-kind switch
// compileOnEnter delegates to. It parses that function's source rather than
// hand-listing the kinds, so a kind added to the compiler later without a
// deliberate classification decision fails this test instead of silently
// defaulting into double execution (dispatched from both
// HandleTriggerSessionShapedOnly and DispatchStepEntry) or silence
// (dispatched from neither) — see
// docs/specs/office/system-design/step-entry-sequence-execution.md
// ("The two lists together are exhaustive").
func TestSessionShapedAndSessionIndependentKindsPartitionCompiledOnEnterKinds(t *testing.T) {
	compiled, err := actionKindsAssignedInFunc("types.go", "CompileOnEnterAction")
	require.NoError(t, err)
	require.NotEmpty(t, compiled, "CompileOnEnterAction must assign at least one ActionKind for this test to be meaningful")

	for _, kind := range compiled {
		shaped := isSessionShapedActionKind(kind)
		independent := isSessionIndependentActionKind(kind)
		assert.True(t, shaped || independent,
			"compiled on_enter kind %q is classified in neither sessionShapedActionKinds nor sessionIndependentActionKinds", kind)
		assert.False(t, shaped && independent,
			"compiled on_enter kind %q is classified in both sessionShapedActionKinds and sessionIndependentActionKinds", kind)
	}

	// configure_session is deliberately excluded: it is a marker-owned kind
	// written into the ownership table by hand (system design, "Ownership
	// declaration"), but CompileOnEnterAction has no case for it, so it can
	// never appear in `compiled`. Every other marker-owned kind must.
	for _, kind := range stepentry.KnownKinds(stepentry.DispatcherMarker) {
		if kind == string(wfmodels.OnEnterConfigureSession) {
			continue
		}
		assert.Contains(t, compiled, ActionKind(kind),
			"the ownership table lists %q as marker-owned, which CompileOnEnterAction no longer emits", kind)
	}
	for _, kind := range stepentry.KnownKinds(stepentry.DispatcherLedger) {
		assert.Contains(t, compiled, ActionKind(kind),
			"the ownership table lists %q as ledger-owned, which CompileOnEnterAction no longer emits", kind)
	}
}

// actionKindsAssignedInFunc parses file (relative to this package directory)
// and returns every ActionKind identifier assigned to a "Kind" struct field
// inside the named function's body, in source order with duplicates removed.
func actionKindsAssignedInFunc(file, funcName string) ([]ActionKind, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		return nil, err
	}
	var fn *ast.FuncDecl
	for _, decl := range f.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == funcName {
			fn = d
			break
		}
	}
	if fn == nil {
		return nil, errors.New("function " + funcName + " not found in " + file)
	}

	seen := map[ActionKind]bool{}
	var kinds []ActionKind
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Kind" {
			return true
		}
		ident, ok := kv.Value.(*ast.Ident)
		if !ok {
			return true
		}
		kind := ActionKind(ident.Name)
		// The Ident's textual name is the ActionKind constant's Go
		// identifier (e.g. "ActionEnablePlanMode"), not its string
		// value ("enable_plan_mode") — resolve it below.
		if resolved, ok := actionKindConstants[kind]; ok {
			kind = resolved
		}
		if !seen[kind] {
			seen[kind] = true
			kinds = append(kinds, kind)
		}
		return true
	})
	return kinds, nil
}

// actionKindConstants maps each ActionKind Go identifier (as it appears in
// source) to its string constant value, so actionKindsAssignedInFunc can
// resolve the identifiers it finds via AST inspection back to real
// ActionKind values without re-parsing types.go's const block.
var actionKindConstants = map[ActionKind]ActionKind{
	ActionKind("ActionEnablePlanMode"):             ActionEnablePlanMode,
	ActionKind("ActionAutoStartAgent"):             ActionAutoStartAgent,
	ActionKind("ActionResetAgentContext"):          ActionResetAgentContext,
	ActionKind("ActionSetSessionMode"):             ActionSetSessionMode,
	ActionKind("ActionRunCodeReview"):              ActionRunCodeReview,
	ActionKind("ActionClearDecisions"):             ActionClearDecisions,
	ActionKind("ActionQueueRunForEachParticipant"): ActionQueueRunForEachParticipant,
	ActionKind("ActionQueueRun"):                   ActionQueueRun,
	ActionKind("ActionEnsureParticipantSeat"):      ActionEnsureParticipantSeat,
}

// fakeEntryDispatchStore is a minimal TransitionStore fake for
// DispatchStepEntry tests: only LoadStep is exercised.
type fakeEntryDispatchStore struct {
	step StepSpec
	err  error
}

func (f *fakeEntryDispatchStore) LoadState(context.Context, string, string) (MachineState, error) {
	return MachineState{}, nil
}
func (f *fakeEntryDispatchStore) LoadStep(context.Context, string, string) (StepSpec, error) {
	return f.step, f.err
}
func (f *fakeEntryDispatchStore) LoadNextStep(context.Context, string, int) (StepSpec, error) {
	return StepSpec{}, nil
}
func (f *fakeEntryDispatchStore) LoadPreviousStep(context.Context, string, int) (StepSpec, error) {
	return StepSpec{}, nil
}
func (f *fakeEntryDispatchStore) ApplyTransition(context.Context, string, string, string, string, Trigger) error {
	return nil
}
func (f *fakeEntryDispatchStore) ApplyTransitionIfAtStep(context.Context, string, string, string, string, Trigger) (bool, error) {
	return false, nil
}
func (f *fakeEntryDispatchStore) PersistData(context.Context, string, map[string]any) error {
	return nil
}
func (f *fakeEntryDispatchStore) IsOperationApplied(context.Context, string) (bool, error) {
	return false, nil
}
func (f *fakeEntryDispatchStore) MarkOperationApplied(context.Context, string) error { return nil }

// entryRecordingCallback records every ActionInput it was invoked with and
// returns a canned error, letting tests assert both "did every declared
// action run" and "did a failure stop the rest".
type entryRecordingCallback struct {
	err    error
	inputs *[]ActionInput
}

func (c entryRecordingCallback) Execute(_ context.Context, in ActionInput) (ActionResult, error) {
	*c.inputs = append(*c.inputs, in)
	return ActionResult{}, c.err
}

func TestDispatchStepEntry_ExcludesSessionShapedKinds(t *testing.T) {
	var seen []ActionInput
	registry := MapRegistry{
		ActionEnablePlanMode: entryRecordingCallback{inputs: &seen},
		ActionClearDecisions: entryRecordingCallback{inputs: &seen},
	}
	step := StepSpec{
		ID: "step-1",
		Events: map[Trigger][]Action{
			TriggerOnEnter: {
				{Kind: ActionEnablePlanMode},
				{Kind: ActionClearDecisions},
			},
		},
	}
	e := New(&fakeEntryDispatchStore{step: step}, registry)

	results := e.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-1", "entry-1", 0)

	require.Len(t, results, 1, "only the session-independent action should be attempted")
	assert.Equal(t, ActionClearDecisions, results[0].Kind)
	assert.NoError(t, results[0].Err)
	require.Len(t, seen, 1, "only the session-independent callback should have run")
	assert.Equal(t, ActionClearDecisions, seen[0].Action.Kind)
	assert.Equal(t, "entry-1", seen[0].EntryID)
}

func TestDispatchStepEntry_ContinuesAfterOneActionFails(t *testing.T) {
	var seen []ActionInput
	failure := errors.New("boom")
	registry := MapRegistry{
		ActionClearDecisions:             entryRecordingCallback{err: failure, inputs: &seen},
		ActionQueueRunForEachParticipant: entryRecordingCallback{inputs: &seen},
	}
	step := StepSpec{
		ID: "step-1",
		Events: map[Trigger][]Action{
			TriggerOnEnter: {
				{Kind: ActionClearDecisions},
				{Kind: ActionQueueRunForEachParticipant},
			},
		},
	}
	e := New(&fakeEntryDispatchStore{step: step}, registry)

	results := e.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-1", "entry-1", 0)

	require.Len(t, results, 2, "both declared actions must be attempted despite the first failing")
	assert.Equal(t, ActionClearDecisions, results[0].Kind)
	assert.ErrorIs(t, results[0].Err, failure)
	assert.Equal(t, ActionQueueRunForEachParticipant, results[1].Kind)
	assert.NoError(t, results[1].Err)
	assert.Len(t, seen, 2)
}

func TestDispatchStepEntry_NoDeclaredActionsIsNoop(t *testing.T) {
	step := StepSpec{ID: "step-1"}
	e := New(&fakeEntryDispatchStore{step: step}, MapRegistry{})

	results := e.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-1", "entry-1", 0)

	assert.Empty(t, results)
}

// TestDispatchStepEntry_RepeatedParticipantSeatRoleDispatchesOnce covers
// AC-OFFICE-REVIEW-SEATS-001.12/004.7: a step that declares
// ensure_participant_seat twice for the same role writes at most one seat
// and emits at most one record for it — DispatchStepEntry must not invoke
// the callback a second time for a role it already dispatched in this call.
func TestDispatchStepEntry_RepeatedParticipantSeatRoleDispatchesOnce(t *testing.T) {
	var seen []ActionInput
	registry := MapRegistry{
		ActionEnsureParticipantSeat: entryRecordingCallback{inputs: &seen},
	}
	step := StepSpec{
		ID: "step-1",
		Events: map[Trigger][]Action{
			TriggerOnEnter: {
				{Kind: ActionEnsureParticipantSeat, EnsureParticipantSeat: &EnsureParticipantSeatAction{Role: "reviewer"}},
				{Kind: ActionEnsureParticipantSeat, EnsureParticipantSeat: &EnsureParticipantSeatAction{Role: "reviewer"}},
			},
		},
	}
	e := New(&fakeEntryDispatchStore{step: step}, registry)

	results := e.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-1", "entry-1", 0)

	require.Len(t, results, 1, "the repeated declaration for the same role must not be dispatched a second time")
	assert.Len(t, seen, 1)
}

// TestDispatchStepEntry_DistinctParticipantSeatRolesBothDispatch asserts the
// dedup in TestDispatchStepEntry_RepeatedParticipantSeatRoleDispatchesOnce
// is scoped to the repeated role only — two declarations for different
// roles are unrelated and both must run.
func TestDispatchStepEntry_DistinctParticipantSeatRolesBothDispatch(t *testing.T) {
	var seen []ActionInput
	registry := MapRegistry{
		ActionEnsureParticipantSeat: entryRecordingCallback{inputs: &seen},
	}
	step := StepSpec{
		ID: "step-1",
		Events: map[Trigger][]Action{
			TriggerOnEnter: {
				{Kind: ActionEnsureParticipantSeat, EnsureParticipantSeat: &EnsureParticipantSeatAction{Role: "reviewer"}},
				{Kind: ActionEnsureParticipantSeat, EnsureParticipantSeat: &EnsureParticipantSeatAction{Role: "approver"}},
			},
		},
	}
	e := New(&fakeEntryDispatchStore{step: step}, registry)

	results := e.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-1", "entry-1", 0)

	require.Len(t, results, 2)
	assert.Len(t, seen, 2)
}

// fakeMarkerExecutor is a MarkerBearingStepEntryExecutor test double that
// returns canned (abandon, err) pairs per ActionKind, and records every call
// it received so a test can assert both what ran and in what order.
type fakeMarkerExecutor struct {
	outcomes map[ActionKind]struct {
		abandon bool
		err     error
	}
	calls []struct {
		Kind          ActionKind
		Position      int
		MarkerEntryID int64
	}
}

func (f *fakeMarkerExecutor) ExecuteMarkerBearingStepEntryAction(
	_ context.Context, _ string, _ StepSpec, action Action, position int, markerEntryID int64,
) (bool, error) {
	f.calls = append(f.calls, struct {
		Kind          ActionKind
		Position      int
		MarkerEntryID int64
	}{action.Kind, position, markerEntryID})
	outcome := f.outcomes[action.Kind]
	return outcome.abandon, outcome.err
}

// TestDispatchStepEntry_MarkerBearingFailureAbandonsRemainingSequence covers
// AC-OFFICE-STEP-ENTRY-DISPATCH-002.10: when a marker-bearing action fails,
// no later position in the same entry's sequence — marker-bearing or not —
// may run, because it could depend on state the failed action never actually
// produced (e.g. a participant fan-out reading decisions clear_decisions
// never cleared).
func TestDispatchStepEntry_MarkerBearingFailureAbandonsRemainingSequence(t *testing.T) {
	var seen []ActionInput
	failure := errors.New("boom")
	executor := &fakeMarkerExecutor{outcomes: map[ActionKind]struct {
		abandon bool
		err     error
	}{
		ActionClearDecisions: {err: failure},
	}}
	registry := MapRegistry{
		ActionEnsureParticipantSeat: entryRecordingCallback{inputs: &seen},
	}
	step := StepSpec{
		ID: "step-1",
		Events: map[Trigger][]Action{
			TriggerOnEnter: {
				{Kind: ActionClearDecisions},
				{Kind: ActionEnsureParticipantSeat, EnsureParticipantSeat: &EnsureParticipantSeatAction{Role: "reviewer"}},
			},
		},
	}
	e := New(&fakeEntryDispatchStore{step: step}, registry, WithMarkerBearingStepEntryExecutor(executor))

	results := e.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-1", "entry-1", 42)

	require.Len(t, results, 1, "the position after the failed marker-bearing action must not run")
	assert.Equal(t, ActionClearDecisions, results[0].Kind)
	assert.ErrorIs(t, results[0].Err, failure)
	assert.Empty(t, seen, "ensure_participant_seat must not have been dispatched")
	require.Len(t, executor.calls, 1)
	assert.Equal(t, 0, executor.calls[0].Position)
	assert.Equal(t, int64(42), executor.calls[0].MarkerEntryID)
}

// TestDispatchStepEntry_MarkerPositionMatchesDeclaredPositionAcrossNonCompilingAction
// covers AC-OFFICE-STEP-ENTRY-DISPATCH-002.1: the position a marker-bearing
// action claims must match the position stepentry.BuildPendingAllocation
// allocated for it — both must count from the step's raw declared on_enter
// list, not from whatever survives compilation. configure_session has no
// CompileOnEnterAction case, so it is silently dropped from the compiled
// action list; a naive loop index over the compiled list would then claim
// position 0 for clear_decisions even though it was declared at raw
// position 1, disagreeing with the allocation built from the raw list.
func TestDispatchStepEntry_MarkerPositionMatchesDeclaredPositionAcrossNonCompilingAction(t *testing.T) {
	executor := &fakeMarkerExecutor{outcomes: map[ActionKind]struct {
		abandon bool
		err     error
	}{}}
	wfStep := &wfmodels.WorkflowStep{
		ID: "step-1",
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterConfigureSession}, // raw position 0, does not compile
				{Type: wfmodels.OnEnterClearDecisions},   // raw position 1
			},
		},
	}
	step := CompileStep(wfStep)
	require.Len(t, step.Events[TriggerOnEnter], 1, "configure_session must not survive compilation")

	e := New(&fakeEntryDispatchStore{step: step}, MapRegistry{}, WithMarkerBearingStepEntryExecutor(executor))

	e.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-1", "entry-1", 42)

	require.Len(t, executor.calls, 1)
	assert.Equal(t, ActionClearDecisions, executor.calls[0].Kind)
	assert.Equal(t, 1, executor.calls[0].Position,
		"claimed position must match the raw declared position (1), matching what stepentry.BuildPendingAllocation allocates from the same raw list")
}

// TestDispatchStepEntry_MarkerBearingAbandonStopsSequenceWithoutRecordingResult
// covers the abandon (claim-loss) branch of the same stop rule: a concurrent
// dispatch already owns the marker, which is a normal race outcome, not a
// failure, so no StepEntryActionResult is recorded for the abandoned
// position — but the sequence still stops.
func TestDispatchStepEntry_MarkerBearingAbandonStopsSequenceWithoutRecordingResult(t *testing.T) {
	executor := &fakeMarkerExecutor{outcomes: map[ActionKind]struct {
		abandon bool
		err     error
	}{
		ActionClearDecisions: {abandon: true},
	}}
	step := StepSpec{
		ID: "step-1",
		Events: map[Trigger][]Action{
			TriggerOnEnter: {
				{Kind: ActionClearDecisions},
				{Kind: ActionQueueRunForEachParticipant},
			},
		},
	}
	e := New(&fakeEntryDispatchStore{step: step}, MapRegistry{}, WithMarkerBearingStepEntryExecutor(executor))

	results := e.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-1", "entry-1", 42)

	assert.Empty(t, results, "an abandoned claim records no result and stops the sequence")
	require.Len(t, executor.calls, 1, "the position after the abandoned claim must not be attempted")
}
