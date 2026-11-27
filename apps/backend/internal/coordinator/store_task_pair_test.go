package coordinator

import (
	"context"
	"testing"
)

func TestCreateCoordinator_RoundtripsTaskPair(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := &Coordinator{
		WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e",
		TaskAgentProfileID: "ta", TaskExecutorProfileID: "te",
	}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	got, err := store.GetCoordinator(ctx, "ws-1", c.ID)
	if err != nil {
		t.Fatalf("GetCoordinator: %v", err)
	}
	if got.TaskAgentProfileID != "ta" || got.TaskExecutorProfileID != "te" {
		t.Fatalf("task pair = (%q, %q), want (ta, te)", got.TaskAgentProfileID, got.TaskExecutorProfileID)
	}
	byID, err := store.GetCoordinatorByID(ctx, c.ID)
	if err != nil || byID.TaskAgentProfileID != "ta" {
		t.Fatalf("GetCoordinatorByID = %+v, %v", byID, err)
	}
	listed, err := store.ListCoordinators(ctx, "ws-1")
	if err != nil || len(listed) != 1 || listed[0].TaskExecutorProfileID != "te" {
		t.Fatalf("ListCoordinators = %+v, %v", listed, err)
	}
}

func TestPatchCoordinator_TaskPairKeepsConversationAndRevision(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	conv := "conv-task"
	c := &Coordinator{
		WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e",
		TaskAgentProfileID: "ta", TaskExecutorProfileID: "te", ConversationTaskID: &conv,
	}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	newAgent, newExec := "ta2", "te2"
	res, err := store.PatchCoordinatorResult(ctx, "ws-1", c.ID,
		CoordinatorPatch{TaskAgentProfileID: &newAgent, TaskExecutorProfileID: &newExec}, nil)
	if err != nil {
		t.Fatalf("PatchCoordinatorResult: %v", err)
	}
	if res.ClearedConversationTaskID != nil {
		t.Fatalf("task pair PATCH cleared the conversation %v", *res.ClearedConversationTaskID)
	}
	if !res.TaskPairChanged {
		t.Fatal("TaskPairChanged = false, want true")
	}
	got, _ := store.GetCoordinator(ctx, "ws-1", c.ID)
	if got.TaskAgentProfileID != "ta2" || got.TaskExecutorProfileID != "te2" ||
		got.ConfigRevision != 0 || got.ConversationTaskID == nil || *got.ConversationTaskID != conv {
		t.Fatalf("after PATCH: %+v", got)
	}
	same, err := store.PatchCoordinatorResult(ctx, "ws-1", c.ID, CoordinatorPatch{TaskAgentProfileID: &newAgent}, nil)
	if err != nil {
		t.Fatalf("PatchCoordinatorResult same value: %v", err)
	}
	if same.TaskPairChanged {
		t.Fatal("TaskPairChanged = true for an unchanged value")
	}
}

func TestPatchCoordinator_ValidatorSeesStoredRow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e", TaskAgentProfileID: "ta", TaskExecutorProfileID: "te"}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	newAgent := "ta2"
	var storedAgent, mergedAgent string
	_, err := store.PatchCoordinatorResult(ctx, "ws-1", c.ID, CoordinatorPatch{TaskAgentProfileID: &newAgent},
		func(_ context.Context, merged, stored *Coordinator) error {
			storedAgent, mergedAgent = stored.TaskAgentProfileID, merged.TaskAgentProfileID
			return nil
		})
	if err != nil {
		t.Fatalf("PatchCoordinatorResult: %v", err)
	}
	if storedAgent != "ta" || mergedAgent != "ta2" {
		t.Fatalf("validator saw stored=%q merged=%q", storedAgent, mergedAgent)
	}
}

func TestInitSchema_BackfillsEmptyTaskPairOnly(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	set := &Coordinator{WorkspaceID: "ws-1", Name: "Set", AgentProfileID: "a", ExecutorProfileID: "e", TaskAgentProfileID: "ta", TaskExecutorProfileID: "te"}
	empty := &Coordinator{WorkspaceID: "ws-1", Name: "Empty", AgentProfileID: "a2", ExecutorProfileID: "e2"}
	for _, c := range []*Coordinator{set, empty} {
		if err := store.CreateCoordinator(ctx, c); err != nil {
			t.Fatalf("CreateCoordinator: %v", err)
		}
	}
	before, _ := store.GetCoordinator(ctx, "ws-1", empty.ID)
	for i := 0; i < 2; i++ {
		if err := store.initSchema(); err != nil {
			t.Fatalf("initSchema #%d: %v", i, err)
		}
	}
	gotEmpty, _ := store.GetCoordinator(ctx, "ws-1", empty.ID)
	if gotEmpty.TaskAgentProfileID != "a2" || gotEmpty.TaskExecutorProfileID != "e2" {
		t.Fatalf("backfilled pair = (%q, %q), want (a2, e2)", gotEmpty.TaskAgentProfileID, gotEmpty.TaskExecutorProfileID)
	}
	if !gotEmpty.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatal("backfill wrote updated_at")
	}
	gotSet, _ := store.GetCoordinator(ctx, "ws-1", set.ID)
	if gotSet.TaskAgentProfileID != "ta" || gotSet.TaskExecutorProfileID != "te" {
		t.Fatalf("set pair changed: %+v", gotSet)
	}
}
