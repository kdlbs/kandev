package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/waveidentity"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// TestResolveWaveIdentity_AllTerminalWaveMembers_ReturnsWaveIdentity is the
// happy path for the terminality-confirming last read
// (AC-OFFICE-WAKE-WAVE-IDENTITY-002.15): every wave member terminal in the
// read itself yields the wave identity waveidentity derives from the same
// ordered id set.
func TestResolveWaveIdentity_AllTerminalWaveMembers_ReturnsWaveIdentity(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	ctx := context.Background()

	if _, err := ss.repo.ExecRaw(ctx, `INSERT INTO tasks (id, workspace_id) VALUES ('parent-1', 'ws-1')`); err != nil {
		t.Fatalf("insert parent: %v", err)
	}
	insertChildTask(t, ss, "child-2", "parent-1", "COMPLETED")
	insertChildTask(t, ss, "child-1", "parent-1", "CANCELLED")

	waveKey, waveString, ok := ss.resolveWaveIdentity(ctx, "parent-1")
	if !ok {
		t.Fatalf("resolveWaveIdentity ok = false, want true")
	}
	wantIDs := []string{"child-1", "child-2"} // ascending by id
	wantKey := waveidentity.WaveKey("parent-1", wantIDs)
	wantString := waveidentity.WaveString("parent-1", wantIDs)
	if waveKey != wantKey {
		t.Fatalf("waveKey = %q, want %q", waveKey, wantKey)
	}
	if waveString != wantString {
		t.Fatalf("waveString = %q, want %q", waveString, wantString)
	}
}

// TestResolveWaveIdentity_NonTerminalMember_ReturnsNotOK covers a wave
// member the read itself observes as not terminal — the case
// AC-...-002.15 exists to guard: recording an identity for a set never
// observed simultaneously terminal would permanently suppress the real
// wave, because dedup is unbounded.
func TestResolveWaveIdentity_NonTerminalMember_ReturnsNotOK(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	ctx := context.Background()

	if _, err := ss.repo.ExecRaw(ctx, `INSERT INTO tasks (id, workspace_id) VALUES ('parent-1', 'ws-1')`); err != nil {
		t.Fatalf("insert parent: %v", err)
	}
	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")
	insertChildTask(t, ss, "child-2", "parent-1", "IN_PROGRESS")

	if _, _, ok := ss.resolveWaveIdentity(ctx, "parent-1"); ok {
		t.Fatalf("resolveWaveIdentity ok = true, want false when a wave member is not terminal")
	}
}

// TestResolveWaveIdentity_NonTerminalMember_ThenTerminal_RecoversWaveIdentity
// is AC-...-002.15's recovery half: skipping a wave for a member the read
// observed non-terminal must not permanently suppress that parent's real
// wave once every member genuinely is terminal. resolveWaveIdentity itself
// is stateless (a read confirming false today says nothing about a later
// read), so this pins that guarantee end to end — including through the
// full cascadeChildrenCompleted path, so a run is actually queued, not
// just a non-empty identity returned.
func TestResolveWaveIdentity_NonTerminalMember_ThenTerminal_RecoversWaveIdentity(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	ctx := context.Background()

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")
	insertChildTask(t, ss, "child-2", "parent-1", "IN_PROGRESS")

	if _, _, ok := ss.resolveWaveIdentity(ctx, "parent-1"); ok {
		t.Fatalf("resolveWaveIdentity ok = true, want false while child-2 is non-terminal")
	}

	var queuedTo string
	var captured RunContext
	queued := false
	queue := func(agentID string, c RunContext) {
		queuedTo = agentID
		captured = c
		queued = true
	}
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)
	if queued {
		t.Fatalf("cascade queued a run while child-2 was still non-terminal")
	}

	// The wave genuinely completes: child-2 reaches a terminal state too.
	if _, err := ss.repo.ExecRaw(ctx, `UPDATE tasks SET state = 'COMPLETED' WHERE id = 'child-2'`); err != nil {
		t.Fatalf("complete child-2: %v", err)
	}

	waveKey, waveString, ok := ss.resolveWaveIdentity(ctx, "parent-1")
	if !ok {
		t.Fatalf("resolveWaveIdentity ok = false, want true once every member is terminal")
	}
	wantIDs := []string{"child-1", "child-2"}
	if wantKey := waveidentity.WaveKey("parent-1", wantIDs); waveKey != wantKey {
		t.Fatalf("waveKey = %q, want %q", waveKey, wantKey)
	}
	if wantString := waveidentity.WaveString("parent-1", wantIDs); waveString != wantString {
		t.Fatalf("waveString = %q, want %q", waveString, wantString)
	}

	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-2", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)
	if !queued {
		t.Fatalf("cascade queued nothing once the wave genuinely completed")
	}
	if queuedTo != "agent-1" {
		t.Fatalf("cascade queued to %q, want agent-1", queuedTo)
	}
	if captured.WaveKey != waveKey {
		t.Fatalf("queued WaveKey = %q, want %q", captured.WaveKey, waveKey)
	}
}

// TestResolveWaveIdentity_NoWaveMembers_ReturnsNotOK is AC-...-001.7: a
// parent with no wave members has no wave.
func TestResolveWaveIdentity_NoWaveMembers_ReturnsNotOK(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	ctx := context.Background()

	if _, err := ss.repo.ExecRaw(ctx, `INSERT INTO tasks (id, workspace_id) VALUES ('parent-1', 'ws-1')`); err != nil {
		t.Fatalf("insert parent: %v", err)
	}

	if _, _, ok := ss.resolveWaveIdentity(ctx, "parent-1"); ok {
		t.Fatalf("resolveWaveIdentity ok = true, want false when the parent has no wave members")
	}
}

// TestCascadeChildrenCompleted_QueuesWaveIdentityOnRunContext proves the
// wave identity resolveWaveIdentity derives actually reaches the queued
// RunContext.
func TestCascadeChildrenCompleted_QueuesWaveIdentityOnRunContext(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	ctx := context.Background()

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")

	var captured RunContext
	queue := func(_ string, c RunContext) { captured = c }
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	wantKey := waveidentity.WaveKey("parent-1", []string{"child-1"})
	wantString := waveidentity.WaveString("parent-1", []string{"child-1"})
	if captured.WaveKey != wantKey {
		t.Fatalf("WaveKey = %q, want %q", captured.WaveKey, wantKey)
	}
	if captured.WaveString != wantString {
		t.Fatalf("WaveString = %q, want %q", captured.WaveString, wantString)
	}
}

// TestCascadeChildrenCompleted_OnlyArchivedChild_QueuesNothing is
// AC-...-001.7 exercised through the full cascade path: the parent has a
// terminal child (so the pre-existing ListChildStates loop does not
// short-circuit), but that child is archived and therefore not a wave
// member, so the parent has no wave and no run is queued.
func TestCascadeChildrenCompleted_OnlyArchivedChild_QueuesNothing(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	ctx := context.Background()

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")
	if _, err := ss.repo.ExecRaw(ctx,
		`UPDATE tasks SET archived_at = ? WHERE id = 'child-1'`, time.Now().UTC()); err != nil {
		t.Fatalf("archive child-1: %v", err)
	}

	var queued int
	queue := func(_ string, _ RunContext) { queued++ }
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	if queued != 0 {
		t.Fatalf("queued = %d, want 0 (parent has no wave members)", queued)
	}
}

// fakeWorkflowStepGetter is a minimal WorkflowStepGetter test double: a
// static step map, or a fixed error.
type fakeWorkflowStepGetter struct {
	steps map[string]*wfmodels.WorkflowStep
	err   error
}

func (f *fakeWorkflowStepGetter) GetStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.steps[stepID], nil
}

// TestCascadeChildrenCompleted_PayloadParity_MergesWorkflowAuthoredPayload
// is AC-OFFICE-WAKE-WAVE-IDENTITY-002.16: a cascade wake that never
// consults the engine still attaches the same queue_run payload the
// engine would attach for the parent's current step.
func TestCascadeChildrenCompleted_PayloadParity_MergesWorkflowAuthoredPayload(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	ctx := context.Background()

	if _, err := ss.repo.ExecRaw(ctx,
		`UPDATE tasks SET workflow_step_id = 'step-x' WHERE id = 'parent-1'`); err != nil {
		t.Fatalf("bind parent step: %v", err)
	}
	ss.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-x": {
			ID: "step-x",
			Events: wfmodels.StepEvents{
				OnChildrenCompleted: []wfmodels.GenericAction{
					{
						Type: wfmodels.GenericActionQueueRun,
						Config: map[string]any{
							"reason":  RunReasonTaskChildrenCompleted,
							"payload": map[string]any{"escalate_to": "lead-agent"},
						},
					},
				},
			},
		},
	}})

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")

	var payload string
	queue := func(_ string, c RunContext) {
		encoded, err := encodeRunContext(c)
		if err != nil {
			t.Fatalf("encode run context: %v", err)
		}
		payload = encoded
	}
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	if !strings.Contains(payload, `"escalate_to":"lead-agent"`) {
		t.Fatalf("payload = %s, want it to contain the workflow-authored escalate_to key", payload)
	}
}

// TestCascadeChildrenCompleted_PayloadParity_IgnoresNonPrimaryTargetedAction
// is AC-OFFICE-WAKE-WAVE-IDENTITY-002.10's target-matching half: a step
// authoring two queue_run actions on one on_children_completed trigger —
// one targeted elsewhere, one implicit-primary — must only ever carry the
// primary-targeted action's payload into the cascade wake, which always
// wakes the parent's assignee. Picking the first non-empty payload
// regardless of target would leak a foreign recipient's content onto this
// wake.
func TestCascadeChildrenCompleted_PayloadParity_IgnoresNonPrimaryTargetedAction(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	ctx := context.Background()

	if _, err := ss.repo.ExecRaw(ctx,
		`UPDATE tasks SET workflow_step_id = 'step-x' WHERE id = 'parent-1'`); err != nil {
		t.Fatalf("bind parent step: %v", err)
	}
	ss.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-x": {
			ID: "step-x",
			Events: wfmodels.StepEvents{
				OnChildrenCompleted: []wfmodels.GenericAction{
					{
						Type: wfmodels.GenericActionQueueRun,
						Config: map[string]any{
							"target":  "workspace.ceo_agent",
							"reason":  RunReasonTaskChildrenCompleted,
							"payload": map[string]any{"escalate_to": "ceo-agent"},
						},
					},
					{
						Type: wfmodels.GenericActionQueueRun,
						Config: map[string]any{
							"reason":  RunReasonTaskChildrenCompleted,
							"payload": map[string]any{"escalate_to": "lead-agent"},
						},
					},
				},
			},
		},
	}})

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")

	var payload string
	queue := func(_ string, c RunContext) {
		encoded, err := encodeRunContext(c)
		if err != nil {
			t.Fatalf("encode run context: %v", err)
		}
		payload = encoded
	}
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	if !strings.Contains(payload, `"escalate_to":"lead-agent"`) {
		t.Fatalf("payload = %s, want it to contain the primary-targeted action's escalate_to key", payload)
	}
	if strings.Contains(payload, "ceo-agent") {
		t.Fatalf("payload = %s, must not contain the non-primary-targeted action's payload", payload)
	}
}

// TestCascadeChildrenCompleted_PayloadParity_EmptyPayloadActionStillMatches
// proves a collision is decided by reason and target alone, never by
// whether the matched action happens to author a payload. A step with two
// implicit-primary, task_children_completed-reasoned queue_run actions —
// the first with no payload, the second with one — is exactly the
// ordering the engine's own step.Events[trigger] loop would dispatch
// first: the first action wins that race (whichever insert lands first
// under the wave-key unique index), so cascade selecting the *second*
// action's payload here would attach content the engine path would never
// actually have delivered for this wave.
func TestCascadeChildrenCompleted_PayloadParity_EmptyPayloadActionStillMatches(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	ctx := context.Background()

	if _, err := ss.repo.ExecRaw(ctx,
		`UPDATE tasks SET workflow_step_id = 'step-x' WHERE id = 'parent-1'`); err != nil {
		t.Fatalf("bind parent step: %v", err)
	}
	ss.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-x": {
			ID: "step-x",
			Events: wfmodels.StepEvents{
				OnChildrenCompleted: []wfmodels.GenericAction{
					{
						Type: wfmodels.GenericActionQueueRun,
						Config: map[string]any{
							"reason": RunReasonTaskChildrenCompleted,
						},
					},
					{
						Type: wfmodels.GenericActionQueueRun,
						Config: map[string]any{
							"reason":  RunReasonTaskChildrenCompleted,
							"payload": map[string]any{"escalate_to": "lead-agent"},
						},
					},
				},
			},
		},
	}})

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")

	var payload string
	queue := func(_ string, c RunContext) {
		encoded, err := encodeRunContext(c)
		if err != nil {
			t.Fatalf("encode run context: %v", err)
		}
		payload = encoded
	}
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	if strings.Contains(payload, "lead-agent") {
		t.Fatalf("payload = %s, must not contain the second action's payload: the first, empty-payload action already matched and the engine would dispatch it first", payload)
	}
}

// TestCascadeChildrenCompleted_PayloadParity_StepLookupFails_StillQueues
// covers the omission path: a step-lookup failure must not block the wake
// itself (AC-...-004.2 — the wake is unconditional), it just queues
// without the merged payload.
func TestCascadeChildrenCompleted_PayloadParity_StepLookupFails_StillQueues(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	ctx := context.Background()

	if _, err := ss.repo.ExecRaw(ctx,
		`UPDATE tasks SET workflow_step_id = 'step-x' WHERE id = 'parent-1'`); err != nil {
		t.Fatalf("bind parent step: %v", err)
	}
	ss.SetWorkflowStepGetter(&fakeWorkflowStepGetter{err: errors.New("step lookup boom")})

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")

	var queued int
	queue := func(_ string, _ RunContext) { queued++ }
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	if queued != 1 {
		t.Fatalf("queued = %d, want 1 (a step-lookup failure must not block the wake)", queued)
	}
}

// TestCascadeChildrenCompleted_PersistsWorkflowStepID is AC-002.10's
// staleness-guard half: an engine-routed producer's persisted payload
// carries workflow_step_id, which evaluateRunStaleness uses to cancel a
// queued run whose parent has since moved to a different step. A cascade
// wake for the same wave must carry the same field, or the two producers'
// wakes are not equivalent — whichever wins the unique-index race decides
// whether the staleness guard applies at all.
func TestCascadeChildrenCompleted_PersistsWorkflowStepID(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	ctx := context.Background()

	if _, err := ss.repo.ExecRaw(ctx,
		`UPDATE tasks SET workflow_step_id = 'step-x' WHERE id = 'parent-1'`); err != nil {
		t.Fatalf("bind parent step: %v", err)
	}

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")

	var payload string
	queue := func(_ string, c RunContext) {
		encoded, err := encodeRunContext(c)
		if err != nil {
			t.Fatalf("encode run context: %v", err)
		}
		payload = encoded
	}
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	if !strings.Contains(payload, `"workflow_step_id":"step-x"`) {
		t.Fatalf("payload = %s, want it to carry workflow_step_id like an engine-routed producer would", payload)
	}
}

// TestCascadeChildrenCompleted_ExtraPayloadWorkflowStepID_CannotOverride
// mirrors the existing task_id/workspace_id/child_task_id re-assertion
// tests: a workflow-authored payload key must not be able to override the
// wake's own workflow_step_id, the same envelope-field guarantee
// encodeRunContext already provides for the other identity fields.
func TestCascadeChildrenCompleted_ExtraPayloadWorkflowStepID_CannotOverride(t *testing.T) {
	rc := RunContext{
		Reason:         RunReasonTaskChildrenCompleted,
		TaskID:         "parent-1",
		WorkflowStepID: "step-x",
		ExtraPayload:   map[string]any{"workflow_step_id": "step-foreign"},
	}
	payload, err := encodeRunContext(rc)
	if err != nil {
		t.Fatalf("encode run context: %v", err)
	}
	if !strings.Contains(payload, `"workflow_step_id":"step-x"`) {
		t.Fatalf("payload = %s, want workflow_step_id = step-x (envelope field), not step-foreign", payload)
	}
}

// TestCascadeChildrenCompleted_PayloadParity_IgnoresDifferentReasonAction
// is AC-002.16's reason-matching half, mirroring the engine's own gate
// (QueueRunCallback.Execute only attaches wave identity when
// queueRunReason(in) == reasonTaskChildrenCompleted): a step authoring a
// second primary-targeted queue_run action on this trigger with a
// different (or unset) reason must not have its payload merged onto the
// real task_children_completed wake. Without the reason check, cascade
// would attach an unrelated action's content to this wave.
func TestCascadeChildrenCompleted_PayloadParity_IgnoresDifferentReasonAction(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	ctx := context.Background()

	if _, err := ss.repo.ExecRaw(ctx,
		`UPDATE tasks SET workflow_step_id = 'step-x' WHERE id = 'parent-1'`); err != nil {
		t.Fatalf("bind parent step: %v", err)
	}
	ss.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-x": {
			ID: "step-x",
			Events: wfmodels.StepEvents{
				OnChildrenCompleted: []wfmodels.GenericAction{
					{
						Type: wfmodels.GenericActionQueueRun,
						Config: map[string]any{
							"reason":  "follow_up",
							"payload": map[string]any{"escalate_to": "follow-up-agent"},
						},
					},
					{
						Type: wfmodels.GenericActionQueueRun,
						Config: map[string]any{
							"reason":  RunReasonTaskChildrenCompleted,
							"payload": map[string]any{"escalate_to": "lead-agent"},
						},
					},
				},
			},
		},
	}})

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")

	var payload string
	queue := func(_ string, c RunContext) {
		encoded, err := encodeRunContext(c)
		if err != nil {
			t.Fatalf("encode run context: %v", err)
		}
		payload = encoded
	}
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	if !strings.Contains(payload, `"escalate_to":"lead-agent"`) {
		t.Fatalf("payload = %s, want it to contain the task_children_completed-reasoned action's escalate_to key", payload)
	}
	if strings.Contains(payload, "follow-up-agent") {
		t.Fatalf("payload = %s, must not contain the differently-reasoned action's payload", payload)
	}
}
