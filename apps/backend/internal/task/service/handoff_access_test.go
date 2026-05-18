package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// fakeTaskLookup is a tiny in-memory implementation of taskLookup keyed
// by task id. Setting parent="" represents a root task.
type fakeTaskLookup struct {
	tasks map[string]*models.Task
}

func (f *fakeTaskLookup) GetTask(ctx context.Context, id string) (*models.Task, error) {
	if t, ok := f.tasks[id]; ok {
		return t, nil
	}
	return nil, nil
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
