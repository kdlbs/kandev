package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type graphFixture struct {
	t         *testing.T
	svc       *Service
	repo      *sqliterepo.Repository
	blockers  *mockBlockerRepo
	relations *mockRelationRepo
}

func newGraphFixture(t *testing.T) *graphFixture {
	t.Helper()
	svc, _, repo := createTestService(t)
	setupTestTask(t, repo)
	blockers := &mockBlockerRepo{}
	relations := newMockRelationRepo()
	svc.SetBlockerRepository(blockers)
	svc.SetRelationRepository(relations)
	return &graphFixture{t: t, svc: svc, repo: repo, blockers: blockers, relations: relations}
}

func (f *graphFixture) task(id, parentID string, state v1.TaskState) {
	f.t.Helper()
	if err := f.repo.CreateTask(context.Background(), &models.Task{
		ID: id, WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123",
		Title: "Title " + id, Priority: "medium", ParentID: parentID, State: state,
	}); err != nil {
		f.t.Fatalf("create task %s: %v", id, err)
	}
}

func (f *graphFixture) archive(id string) {
	f.t.Helper()
	if err := f.repo.ArchiveTask(context.Background(), id); err != nil {
		f.t.Fatalf("archive %s: %v", id, err)
	}
}

// edge records "dependent depends on predecessor".
func (f *graphFixture) edge(predecessor, dependent string) {
	f.blockers.blockers = append(f.blockers.blockers, &orchmodels.TaskBlocker{TaskID: dependent, BlockerTaskID: predecessor})
}

func (f *graphFixture) related(a, b string) {
	f.t.Helper()
	if err := f.relations.AddRelation(context.Background(), a, b); err != nil {
		f.t.Fatalf("add relation %s/%s: %v", a, b, err)
	}
}

func (f *graphFixture) build(taskID, mode string) *TaskGraph {
	f.t.Helper()
	graph, err := f.svc.BuildTaskGraph(context.Background(), taskID, mode)
	if err != nil {
		f.t.Fatalf("BuildTaskGraph(%s, %q): %v", taskID, mode, err)
	}
	return graph
}

func graphNodeIDs(g *TaskGraph) []string {
	ids := make([]string, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		ids = append(ids, n.Task.ID)
	}
	sort.Strings(ids)
	return ids
}

func graphEdgeKeys(g *TaskGraph) []string {
	keys := make([]string, 0, len(g.Edges))
	for _, e := range g.Edges {
		keys = append(keys, e.From+">"+e.To+":"+e.Status)
	}
	sort.Strings(keys)
	return keys
}

func graphNode(g *TaskGraph, id string) *TaskGraphNode {
	for i := range g.Nodes {
		if g.Nodes[i].Task.ID == id {
			return &g.Nodes[i]
		}
	}
	return nil
}

func assertStrings(t *testing.T, label string, got, want []string) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

func TestBuildTaskGraphModeAvailability(t *testing.T) {
	f := newGraphFixture(t)
	f.task("lonely", "", v1.TaskStateTODO)
	f.task("parent", "", v1.TaskStateTODO)
	f.task("child", "parent", v1.TaskStateTODO)

	lonely := f.build("lonely", "")
	if lonely.Mode != TaskGraphModeChain {
		t.Fatalf("default mode without subtree = %q, want chain", lonely.Mode)
	}
	assertStrings(t, "lonely modes", lonely.AvailableModes, []string{TaskGraphModeChain})
	if _, err := f.svc.BuildTaskGraph(context.Background(), "lonely", TaskGraphModeSubtasks); !errors.Is(err, ErrTaskGraphModeUnavailable) {
		t.Fatalf("subtasks on lonely task err = %v, want ErrTaskGraphModeUnavailable", err)
	}

	for _, id := range []string{"parent", "child"} {
		g := f.build(id, "")
		if g.Mode != TaskGraphModeSubtasks {
			t.Fatalf("default mode for %s = %q, want subtasks", id, g.Mode)
		}
		assertStrings(t, id+" modes", g.AvailableModes, []string{TaskGraphModeSubtasks, TaskGraphModeChain})
	}
	if _, err := f.svc.BuildTaskGraph(context.Background(), "parent", "bogus"); !errors.Is(err, ErrTaskGraphModeUnavailable) {
		t.Fatalf("unknown mode err = %v, want ErrTaskGraphModeUnavailable", err)
	}
}

func TestBuildTaskGraphSubtasksWalksSubtreeWithExternalStubs(t *testing.T) {
	f := newGraphFixture(t)
	f.task("parent", "", v1.TaskStateInProgress)
	f.task("a", "parent", v1.TaskStateCompleted)
	f.task("b", "parent", v1.TaskStateInProgress)
	f.task("c", "parent", v1.TaskStateTODO)
	f.task("b1", "b", v1.TaskStateTODO)
	f.task("gone", "parent", v1.TaskStateTODO)
	f.archive("gone")
	f.task("outside", "", v1.TaskStateTODO)
	f.task("far", "", v1.TaskStateTODO)
	f.edge("a", "b")
	f.edge("b", "c")
	f.edge("outside", "a")
	f.edge("far", "outside") // external's own edge: not expanded

	g := f.build("parent", TaskGraphModeSubtasks)
	if g.Root == nil || g.Root.ID != "parent" {
		t.Fatalf("root = %+v, want parent", g.Root)
	}
	if g.FocusTaskID != "parent" {
		t.Fatalf("focus = %q, want parent", g.FocusTaskID)
	}
	assertStrings(t, "nodes", graphNodeIDs(g), []string{"a", "b", "b1", "c", "outside"})
	assertStrings(t, "edges", graphEdgeKeys(g), []string{"a>b:resolved", "b>c:pending", "outside>a:pending"})

	b1 := graphNode(g, "b1")
	if b1.Role != TaskGraphRoleMember || b1.Depth != 2 || b1.ParentTitle != "Title b" {
		t.Fatalf("b1 = role %q depth %d parent %q", b1.Role, b1.Depth, b1.ParentTitle)
	}
	if a := graphNode(g, "a"); a.Depth != 1 || a.ParentTitle != "Title parent" {
		t.Fatalf("a depth/parent = %d/%q", a.Depth, a.ParentTitle)
	}
	outside := graphNode(g, "outside")
	if outside.Role != TaskGraphRoleExternal || outside.Depth != 0 {
		t.Fatalf("outside = role %q depth %d", outside.Role, outside.Depth)
	}
	if c := graphNode(g, "c"); !c.View.Blocked || c.View.BlockedReason != BlockedReasonPending {
		t.Fatalf("c view = %+v, want blocked pending", c.View)
	}
	if g.Truncated {
		t.Fatal("unexpected truncation")
	}
}

func TestBuildTaskGraphSubtasksFromChildUsesParentSubtree(t *testing.T) {
	f := newGraphFixture(t)
	f.task("parent", "", v1.TaskStateTODO)
	f.task("a", "parent", v1.TaskStateTODO)
	f.task("b", "parent", v1.TaskStateTODO)

	g := f.build("b", TaskGraphModeSubtasks)
	if g.Root == nil || g.Root.ID != "parent" || g.FocusTaskID != "b" {
		t.Fatalf("root/focus = %v/%q, want parent/b", g.Root, g.FocusTaskID)
	}
	assertStrings(t, "nodes", graphNodeIDs(g), []string{"a", "b"})
	if len(g.Edges) != 0 {
		t.Fatalf("edges = %v, want none", graphEdgeKeys(g))
	}
}

func TestBuildTaskGraphChainFollowsOnlyTransitiveDirections(t *testing.T) {
	f := newGraphFixture(t)
	for _, id := range []string{"up2", "up1", "focus", "down1", "down2", "side", "unrelated"} {
		f.task(id, "", v1.TaskStateTODO)
	}
	f.task("old", "", v1.TaskStateCompleted)
	f.archive("old")
	f.edge("up2", "up1")
	f.edge("up1", "focus")
	f.edge("old", "focus")
	f.edge("focus", "down1")
	f.edge("down1", "down2")
	f.edge("side", "down1") // sibling predecessor of a dependent: excluded
	f.edge("up2", "unrelated")

	g := f.build("focus", TaskGraphModeChain)
	if g.Root != nil {
		t.Fatalf("chain root = %+v, want nil", g.Root)
	}
	assertStrings(t, "nodes", graphNodeIDs(g), []string{"down1", "down2", "focus", "old", "up1", "up2"})
	assertStrings(t, "edges", graphEdgeKeys(g), []string{
		"down1>down2:pending", "focus>down1:pending", "old>focus:resolved", "up1>focus:pending", "up2>up1:pending",
	})
	if old := graphNode(g, "old"); old.Task.ArchivedAt == nil {
		t.Fatal("archived predecessor must be kept and flagged")
	}
	if n := graphNode(g, "focus"); n.Depth != 0 || n.Role != TaskGraphRoleMember {
		t.Fatalf("focus node = role %q depth %d", n.Role, n.Depth)
	}
}

func TestBuildTaskGraphTruncatesAtNodeBudget(t *testing.T) {
	f := newGraphFixture(t)
	f.task("parent", "", v1.TaskStateTODO)
	for i := 0; i < maxTaskGraphNodes+5; i++ {
		f.task(fmt.Sprintf("kid-%03d", i), "parent", v1.TaskStateTODO)
	}
	g := f.build("parent", TaskGraphModeSubtasks)
	if !g.Truncated || len(g.Nodes) != maxTaskGraphNodes {
		t.Fatalf("truncated=%t nodes=%d, want true/%d", g.Truncated, len(g.Nodes), maxTaskGraphNodes)
	}
}

func TestBuildTaskGraphDeduplicatesEdgesAndDropsDanglingEnds(t *testing.T) {
	f := newGraphFixture(t)
	f.task("parent", "", v1.TaskStateTODO)
	f.task("a", "parent", v1.TaskStateTODO)
	f.task("b", "parent", v1.TaskStateTODO)
	f.edge("a", "b")
	f.edge("a", "b")
	f.edge("deleted-task", "b")

	g := f.build("parent", TaskGraphModeSubtasks)
	assertStrings(t, "nodes", graphNodeIDs(g), []string{"a", "b"})
	assertStrings(t, "edges", graphEdgeKeys(g), []string{"a>b:pending"})
}

func TestBuildTaskGraphSubtasksAddsOnlyRelatedEdgesBetweenMembers(t *testing.T) {
	f := newGraphFixture(t)
	f.task("parent-related", "", v1.TaskStateTODO)
	f.task("member-a", "parent-related", v1.TaskStateTODO)
	f.task("member-b", "parent-related", v1.TaskStateTODO)
	f.task("outside-related", "", v1.TaskStateTODO)
	f.related("member-a", "member-b")
	f.related("member-a", "outside-related")
	g := f.build("parent-related", TaskGraphModeSubtasks)
	if len(g.Edges) != 1 || g.Edges[0].Kind != TaskGraphEdgeRelated || g.Edges[0].From != "member-a" || g.Edges[0].To != "member-b" {
		t.Fatalf("edges = %+v", g.Edges)
	}
	if graphNode(g, "outside-related") != nil {
		t.Fatal("related-to edge pulled an external node into the subtree")
	}
}

func TestBuildTaskGraphChainAddsRelatedEdgesAndKeepsKindsDistinct(t *testing.T) {
	f := newGraphFixture(t)
	for _, id := range []string{"focus-related", "upstream-a", "upstream-b", "dependent-related"} {
		f.task(id, "", v1.TaskStateTODO)
	}
	f.edge("upstream-a", "focus-related")
	f.edge("upstream-b", "focus-related")
	f.edge("focus-related", "dependent-related")
	f.related("upstream-a", "upstream-b")
	f.related("focus-related", "dependent-related")
	g := f.build("focus-related", TaskGraphModeChain)
	relatedPairs := map[string]bool{}
	pairKinds := 0
	for _, edge := range g.Edges {
		if edge.Kind == TaskGraphEdgeRelated {
			relatedPairs[edge.From+">"+edge.To] = true
		}
		if (edge.From == "focus-related" && edge.To == "dependent-related") ||
			(edge.From == "dependent-related" && edge.To == "focus-related") {
			pairKinds++
		}
	}
	if !relatedPairs["upstream-a>upstream-b"] || !relatedPairs["dependent-related>focus-related"] {
		t.Fatalf("related edges = %v", relatedPairs)
	}
	if pairKinds != 2 {
		t.Fatalf("parallel dependency/related edges = %d, want 2: %+v", pairKinds, g.Edges)
	}
}

func TestBuildTaskGraphUnknownTaskIsNotFound(t *testing.T) {
	f := newGraphFixture(t)
	if _, err := f.svc.BuildTaskGraph(context.Background(), "missing", ""); err == nil {
		t.Fatal("expected an error for a missing task")
	}
}
