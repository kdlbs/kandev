package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

// fakeTaskLookup is a tiny in-memory implementation of taskLookup keyed
// by task id. Setting parent="" represents a root task.
type fakeTaskLookup struct {
	tasks map[string]*models.Task
}

// GetTask mirrors the production repository shape (task/repository/sqlite
// GetTask): a missing id returns repository.ErrTaskNotFound wrapped with
// the id, never a nil/nil result. Access-guard tests rely on this to prove
// loadAccessPair normalizes not-found into a plain deny.
func (f *fakeTaskLookup) GetTask(ctx context.Context, id string) (*models.Task, error) {
	if t, ok := f.tasks[id]; ok {
		return t, nil
	}
	return nil, fmt.Errorf("%w: %s", repository.ErrTaskNotFound, id)
}
func (f *fakeTaskLookup) GetTasksByIDs(ctx context.Context, ids []string) ([]*models.Task, error) {
	var out []*models.Task
	for _, id := range ids {
		if t, ok := f.tasks[id]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

func newGraph(tasks ...*models.Task) *fakeTaskLookup {
	m := make(map[string]*models.Task, len(tasks))
	for _, t := range tasks {
		m[t.ID] = t
	}
	return &fakeTaskLookup{tasks: m}
}

func newTask(id, parent, ws string) *models.Task {
	return &models.Task{ID: id, ParentID: parent, WorkspaceID: ws}
}

func TestCanReadDocuments_Self(t *testing.T) {
	g := newGraph(newTask("A", "", "ws-1"))
	ok, err := canReadDocuments(context.Background(), g, nil, "A", "A")
	if err != nil || !ok {
		t.Fatalf("self read should be allowed: ok=%v err=%v", ok, err)
	}
}

func TestCanReadDocuments_AncestorAndDescendant(t *testing.T) {
	g := newGraph(
		newTask("root", "", "ws-1"),
		newTask("child", "root", "ws-1"),
		newTask("grand", "child", "ws-1"),
	)
	ctx := context.Background()
	cases := []struct {
		name        string
		caller, tgt string
		want        bool
	}{
		{"child reads root (ancestor)", "child", "root", true},
		{"grand reads root (ancestor 2 hops)", "grand", "root", true},
		{"root reads child (descendant)", "root", "child", true},
		{"root reads grand (descendant 2 hops)", "root", "grand", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := canReadDocuments(ctx, g, nil, tc.caller, tc.tgt)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCanReadDocuments_Sibling(t *testing.T) {
	g := newGraph(
		newTask("parent", "", "ws-1"),
		newTask("a", "parent", "ws-1"),
		newTask("b", "parent", "ws-1"),
	)
	ok, err := canReadDocuments(context.Background(), g, nil, "a", "b")
	if err != nil || !ok {
		t.Fatalf("siblings should read each other: ok=%v err=%v", ok, err)
	}
}

// REGRESSION: two ROOT tasks both have empty parent_id. The naive
// "shared parent_id" rule would have made them siblings and leaked
// document access. The fix requires a non-empty common parent.
func TestCanReadDocuments_RootsAreNotSiblings(t *testing.T) {
	g := newGraph(
		newTask("root-a", "", "ws-1"),
		newTask("root-b", "", "ws-1"),
	)
	ok, err := canReadDocuments(context.Background(), g, nil, "root-a", "root-b")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Error("two roots in the same workspace must NOT see each other as siblings")
	}
}

func TestCanReadDocuments_WorkspaceMismatch(t *testing.T) {
	g := newGraph(
		newTask("p", "", "ws-1"),
		newTask("a", "p", "ws-1"),
		newTask("b", "p", "ws-2"), // somehow same parent but different workspace
	)
	ok, err := canReadDocuments(context.Background(), g, nil, "a", "b")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Error("workspace mismatch must always deny access")
	}
}

func TestCanReadDocuments_UnrelatedTasks(t *testing.T) {
	g := newGraph(
		newTask("p1", "", "ws-1"),
		newTask("a1", "p1", "ws-1"),
		newTask("p2", "", "ws-1"),
		newTask("a2", "p2", "ws-1"),
	)
	ok, _ := canReadDocuments(context.Background(), g, nil, "a1", "a2")
	if ok {
		t.Error("unrelated tasks (different parents) must not see each other")
	}
}

// fakeBlockerLookup implements blockerLookup for the access tests.
// Returns the canned blocker IDs for the queried task; defaults to
// empty so unrelated tasks stay denied.
type fakeBlockerLookup struct {
	byTask map[string][]string
}

func (f *fakeBlockerLookup) BlockerTaskIDs(_ context.Context, taskID string) ([]string, error) {
	return f.byTask[taskID], nil
}

// REGRESSION (post-review #6): the simplified Phase 3 model uses
// blocker edges as the document handoff readiness gate. A consumer
// task MUST be able to read its blocker's documents — without this
// branch the simplified handoff model is unreachable end-to-end.
func TestCanReadDocuments_BlockerEdgeGrantsRead(t *testing.T) {
	g := newGraph(
		newTask("planner", "", "ws-1"),
		newTask("implementer", "", "ws-1"),
	)
	blockers := &fakeBlockerLookup{
		byTask: map[string][]string{"implementer": {"planner"}},
	}
	ok, err := canReadDocuments(context.Background(), g, blockers, "implementer", "planner")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok {
		t.Error("implementer (blocked-by planner) must be allowed to read planner's docs")
	}
	// Reverse direction is NOT granted by a blocker edge — the producer
	// can't read the consumer's docs via the same relationship.
	if got, _ := canReadDocuments(context.Background(), g, blockers, "planner", "implementer"); got {
		t.Error("planner (blocking implementer) must NOT auto-read implementer's docs via blocker edge")
	}
}

func TestCanReadDocuments_BlockerCrossWorkspaceDenied(t *testing.T) {
	g := newGraph(
		newTask("planner", "", "ws-1"),
		newTask("implementer", "", "ws-2"),
	)
	blockers := &fakeBlockerLookup{
		byTask: map[string][]string{"implementer": {"planner"}},
	}
	if got, _ := canReadDocuments(context.Background(), g, blockers, "implementer", "planner"); got {
		t.Error("workspace mismatch must deny even when a blocker edge exists")
	}
}

func TestCanReadDocuments_MissingTasksDeny(t *testing.T) {
	g := newGraph(newTask("known", "", "ws-1"))
	ok, _ := canReadDocuments(context.Background(), g, nil, "known", "missing")
	if ok {
		t.Error("missing target must deny")
	}
	ok, _ = canReadDocuments(context.Background(), g, nil, "missing", "known")
	if ok {
		t.Error("missing caller must deny")
	}
}

// REGRESSION (AC-001.11): a not-found task must normalize to a plain deny,
// not leak the wrapped repository.ErrTaskNotFound (which embeds the raw
// task id) up through the access guard.
func TestLoadAccessPair_NotFoundNormalizesToPlainDeny(t *testing.T) {
	g := newGraph(newTask("known", "", "ws-1"))
	ctx := context.Background()

	current, target, ok, err := loadAccessPair(ctx, g, "known", "missing")
	if err != nil {
		t.Fatalf("loadAccessPair err = %v, want nil (plain deny)", err)
	}
	if ok || current != nil || target != nil {
		t.Fatalf("loadAccessPair(known, missing) = (%v, %v, %v), want (nil, nil, false)", current, target, ok)
	}

	current, target, ok, err = loadAccessPair(ctx, g, "missing", "known")
	if err != nil {
		t.Fatalf("loadAccessPair err = %v, want nil (plain deny)", err)
	}
	if ok || current != nil || target != nil {
		t.Fatalf("loadAccessPair(missing, known) = (%v, %v, %v), want (nil, nil, false)", current, target, ok)
	}
}

func TestCanWriteDocuments_SelfAndAncestorOnly(t *testing.T) {
	g := newGraph(
		newTask("root", "", "ws-1"),
		newTask("child", "root", "ws-1"),
		newTask("sib", "root", "ws-1"),
		newTask("grand", "child", "ws-1"),
	)
	ctx := context.Background()
	cases := []struct {
		name        string
		caller, tgt string
		want        bool
	}{
		{"self write", "child", "child", true},
		{"child writes parent", "child", "root", true},
		{"grand writes 2-hop ancestor", "grand", "root", true},
		{"sibling write denied", "child", "sib", false},
		{"descendant write denied", "root", "child", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := canWriteDocuments(ctx, g, tc.caller, tc.tgt)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAncestorIDs_HopCap: corrupt data with a parent cycle must not
// loop forever. The walk caps at ancestorWalkHopCap and bails cleanly.
func TestAncestorIDs_HopCap(t *testing.T) {
	// Build a long chain longer than the hop cap. The walk should
	// terminate at the cap without erroring.
	const n = ancestorWalkHopCap + 10
	tasks := make([]*models.Task, n)
	for i := 0; i < n; i++ {
		parent := ""
		if i > 0 {
			parent = "t" + itoa(i-1)
		}
		tasks[i] = newTask("t"+itoa(i), parent, "ws-1")
	}
	g := newGraph(tasks...)
	got, err := ancestorIDs(context.Background(), g, "t"+itoa(n-1))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != ancestorWalkHopCap {
		t.Errorf("ancestor walk len = %d, want %d (hop cap)", len(got), ancestorWalkHopCap)
	}
}

// TestAncestorIDs_CycleHandled: an explicit parent cycle a → b → a must
// not loop forever.
func TestAncestorIDs_CycleHandled(t *testing.T) {
	g := newGraph(
		&models.Task{ID: "a", ParentID: "b", WorkspaceID: "ws-1"},
		&models.Task{ID: "b", ParentID: "a", WorkspaceID: "ws-1"},
	)
	got, err := ancestorIDs(context.Background(), g, "a")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Either ["b"] (cycle detected after one step) or the full chain
	// truncated by the cap is acceptable; the important property is no
	// hang and bounded length.
	if len(got) == 0 || len(got) > ancestorWalkHopCap {
		t.Errorf("unexpected walk length: %v", got)
	}
}

// REGRESSION (SEC-001): a dangling parent_id is reachable via ordinary
// delete_task_kandev usage — it deletes a task via a bare
// `DELETE FROM tasks WHERE id = ?` with no reparenting or cascade, so a
// surviving child's parent_id can point at a row that no longer exists.
// ancestorIDs must treat a not-found parent the same as reaching the end
// of the chain (return what it has accumulated so far), not propagate the
// wrapped repository.ErrTaskNotFound the way loadAccessPair already
// refuses to for its own two direct GetTask calls.
func TestAncestorIDs_DanglingParentNormalizesToAccumulatedChain(t *testing.T) {
	// "child" points at "deleted-parent", which does not exist in the
	// graph — simulating the state left behind by delete_task_kandev
	// deleting a task that still had children. The dangling id itself is
	// still recorded (added to the chain before the walk discovers, on
	// the next hop, that it doesn't exist) — the same
	// append-before-verify shape TestAncestorIDs_CycleHandled already
	// tolerates. What matters is the walk stops cleanly instead of
	// propagating an error: neither loadAccessPair's `current` nor
	// `target` can ever equal a dangling id (both are validated to exist
	// before ancestorIDs runs), so its mere presence in this slice is
	// harmless for the access decision.
	g := newGraph(newTask("child", "deleted-parent", "ws-1"))
	got, err := ancestorIDs(context.Background(), g, "child")
	if err != nil {
		t.Fatalf("ancestorIDs err = %v, want nil (dangling parent must normalize, not error)", err)
	}
	if len(got) != 1 || got[0] != "deleted-parent" {
		t.Fatalf("ancestorIDs = %v, want [\"deleted-parent\"] (walk stops cleanly at the dangling hop)", got)
	}
}

// REGRESSION (SEC-001): the same dangling-parent scenario must not leak a
// raw internal error out of canReadDocuments either — the guard must reach
// a normal allow/deny decision instead of propagating the wrapped
// repository.ErrTaskNotFound (which embeds the deleted task's id) up to
// mapHandoffError's uncaught default branch.
func TestCanReadDocuments_DanglingParentDoesNotLeakRawError(t *testing.T) {
	g := newGraph(
		newTask("child", "deleted-parent", "ws-1"),
		newTask("unrelated", "", "ws-1"),
	)
	ok, err := canReadDocuments(context.Background(), g, nil, "child", "unrelated")
	if err != nil {
		t.Fatalf("canReadDocuments err = %v, want nil (dangling ancestor must not leak)", err)
	}
	if ok {
		t.Fatal("canReadDocuments(child, unrelated) = true, want false (no real relation)")
	}
}

// itoa avoids pulling in strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
