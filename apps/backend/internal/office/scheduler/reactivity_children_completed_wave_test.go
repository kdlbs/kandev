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
