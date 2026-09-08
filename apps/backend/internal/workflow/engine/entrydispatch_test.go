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

	for kind := range sessionShapedActionKinds {
		assert.Contains(t, compiled, kind,
			"sessionShapedActionKinds lists %q, which CompileOnEnterAction no longer emits", kind)
	}
	for kind := range sessionIndependentActionKinds {
		assert.Contains(t, compiled, kind,
			"sessionIndependentActionKinds lists %q, which CompileOnEnterAction no longer emits", kind)
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

	results := e.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-1", "entry-1")

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

	results := e.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-1", "entry-1")

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

	results := e.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-1", "entry-1")

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

	results := e.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-1", "entry-1")

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

	results := e.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-1", "entry-1")

	require.Len(t, results, 2)
	assert.Len(t, seen, 2)
}
