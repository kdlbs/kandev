package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

// Task relationship graph (docs/specs/tasks/system-design/task-relationship-graph.md):
// a bounded, read-only view of a subtree or a dependency chain.

// Graph modes accepted by BuildTaskGraph.
const (
	TaskGraphModeSubtasks = "subtasks"
	TaskGraphModeChain    = "chain"
)

// Graph node roles.
const (
	TaskGraphRoleMember   = "member"
	TaskGraphRoleExternal = "external"
)

// maxTaskGraphNodes caps every graph read, external nodes included.
const maxTaskGraphNodes = 200

// ErrTaskGraphModeUnavailable is returned for an unknown mode or a subtasks
// request on a task with no subtasks and no parent.
var ErrTaskGraphModeUnavailable = errors.New("graph mode unavailable")

// TaskGraphNode is one displayed task.
type TaskGraphNode struct {
	Task        *models.Task
	Role        string
	Depth       int
	ParentTitle string
	View        DependencyView
}

// TaskGraphEdge points from predecessor to dependent.
type TaskGraphEdge struct {
	From   string
	To     string
	Kind   string
	Status string
}

const (
	TaskGraphEdgeDependency = "dependency"
	TaskGraphEdgeRelated    = "related"
)

// TaskGraph is the result of BuildTaskGraph.
type TaskGraph struct {
	Mode           string
	AvailableModes []string
	FocusTaskID    string
	Root           *models.Task
	Nodes          []TaskGraphNode
	Edges          []TaskGraphEdge
	Truncated      bool
}

type limitedChildRepository interface {
	ListChildrenLimited(ctx context.Context, parentID string, limit int) ([]*models.Task, error)
}

// BuildTaskGraph returns the subtree or chain graph around taskID. An empty
// mode selects subtasks when available and chain otherwise.
func (s *Service) BuildTaskGraph(ctx context.Context, taskID, mode string) (*TaskGraph, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("%w: %s", taskrepo.ErrTaskNotFound, taskID)
	}
	root, err := s.taskGraphSubtreeRoot(ctx, task)
	if err != nil {
		return nil, err
	}
	available := []string{TaskGraphModeChain}
	if root != nil {
		available = []string{TaskGraphModeSubtasks, TaskGraphModeChain}
	}
	if mode == "" {
		mode = available[0]
	}
	b := newTaskGraphBuilder(task.WorkspaceID)
	switch {
	case mode == TaskGraphModeSubtasks && root != nil:
		err = s.collectTaskGraphSubtree(ctx, b, root)
	case mode == TaskGraphModeChain:
		root = nil
		err = s.collectTaskGraphChain(ctx, b, task)
	default:
		return nil, fmt.Errorf("%w: %q", ErrTaskGraphModeUnavailable, mode)
	}
	if err != nil {
		return nil, err
	}
	edges, err := s.taskGraphEdges(ctx, b, mode == TaskGraphModeChain)
	if err != nil {
		return nil, err
	}
	nodes := b.list()
	s.applyTaskGraphViews(ctx, nodes)
	return &TaskGraph{
		Mode: mode, AvailableModes: available, FocusTaskID: task.ID, Root: root,
		Nodes: nodes, Edges: edges, Truncated: b.truncated,
	}, nil
}

// taskGraphSubtreeRoot is the task itself when it has children, else its
// parent, else nil (subtasks mode unavailable).
func (s *Service) taskGraphSubtreeRoot(ctx context.Context, task *models.Task) (*models.Task, error) {
	children, err := s.listTaskGraphChildren(ctx, task.ID, 1)
	if err != nil {
		return nil, err
	}
	if len(children) > 0 {
		return task, nil
	}
	if task.ParentID == "" {
		return nil, nil
	}
	parent, err := s.tasks.GetTask(ctx, task.ParentID)
	if err != nil {
		if errors.Is(err, taskrepo.ErrTaskNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if parent == nil || parent.WorkspaceID != task.WorkspaceID {
		return nil, nil
	}
	return parent, nil
}

func (s *Service) listTaskGraphChildren(ctx context.Context, parentID string, limit int) ([]*models.Task, error) {
	if repo, ok := s.tasks.(limitedChildRepository); ok {
		return repo.ListChildrenLimited(ctx, parentID, limit)
	}
	children, err := s.tasks.ListChildren(ctx, parentID)
	if err != nil || len(children) <= limit {
		return children, err
	}
	return children[:limit], nil
}

func (s *Service) collectTaskGraphSubtree(ctx context.Context, b *taskGraphBuilder, root *models.Task) error {
	b.visited[root.ID] = true
	type level struct {
		parent *models.Task
		depth  int
	}
	queue := []level{{parent: root, depth: 1}}
	for len(queue) > 0 && !b.truncated {
		current := queue[0]
		queue = queue[1:]
		// One extra row tells a full budget apart from an exact fit.
		children, err := s.listTaskGraphChildren(ctx, current.parent.ID, b.remaining()+1)
		if err != nil {
			return err
		}
		for _, child := range children {
			if child == nil || b.visited[child.ID] {
				continue
			}
			if !b.add(child, TaskGraphRoleMember, current.depth, current.parent.Title) {
				break
			}
			queue = append(queue, level{parent: child, depth: current.depth + 1})
		}
	}
	return s.addTaskGraphExternals(ctx, b)
}

// addTaskGraphExternals adds one-hop edge ends outside the subtree.
func (s *Service) addTaskGraphExternals(ctx context.Context, b *taskGraphBuilder) error {
	if s.blockers == nil || len(b.order) == 0 {
		return nil
	}
	members := append([]string(nil), b.order...)
	preds, err := s.blockers.ListBlockersForTasks(ctx, members)
	if err != nil {
		return err
	}
	deps, err := s.blockers.ListDependentsForTasks(ctx, members)
	if err != nil {
		return err
	}
	var external []string
	seen := map[string]bool{}
	for _, id := range members {
		for _, other := range append(append([]string(nil), preds[id]...), deps[id]...) {
			if b.nodes[other] == nil && !seen[other] {
				seen[other] = true
				external = append(external, other)
			}
		}
	}
	return s.addTaskGraphTasks(ctx, b, external, TaskGraphRoleExternal)
}

func (s *Service) collectTaskGraphChain(ctx context.Context, b *taskGraphBuilder, focus *models.Task) error {
	b.add(focus, TaskGraphRoleMember, 0, "")
	if s.blockers == nil {
		return nil
	}
	if err := s.walkTaskGraphDirection(ctx, b, focus.ID, s.blockers.ListBlockersForTasks); err != nil {
		return err
	}
	return s.walkTaskGraphDirection(ctx, b, focus.ID, s.blockers.ListDependentsForTasks)
}

type taskGraphEdgeReader func(ctx context.Context, ids []string) (map[string][]string, error)

func (s *Service) walkTaskGraphDirection(ctx context.Context, b *taskGraphBuilder, start string, next taskGraphEdgeReader) error {
	walked := map[string]bool{start: true}
	frontier := []string{start}
	for len(frontier) > 0 && !b.truncated {
		edges, err := next(ctx, frontier)
		if err != nil {
			return err
		}
		var ids []string
		for _, id := range frontier {
			for _, other := range edges[id] {
				if !walked[other] {
					walked[other] = true
					ids = append(ids, other)
				}
			}
		}
		before := len(b.order)
		if err := s.addTaskGraphTasks(ctx, b, ids, TaskGraphRoleMember); err != nil {
			return err
		}
		frontier = append([]string(nil), b.order[before:]...)
	}
	return nil
}

// addTaskGraphTasks resolves ids in one read and adds them in the given
// order. Missing rows (dangling edges) are skipped.
func (s *Service) addTaskGraphTasks(ctx context.Context, b *taskGraphBuilder, ids []string, role string) error {
	if len(ids) == 0 {
		return nil
	}
	found, err := s.tasks.GetTasksByIDs(ctx, ids)
	if err != nil {
		return err
	}
	byID := make(map[string]*models.Task, len(found))
	for _, t := range found {
		if t != nil {
			byID[t.ID] = t
		}
	}
	for _, id := range ids {
		if t := byID[id]; t != nil && !b.add(t, role, 0, "") {
			break
		}
	}
	return nil
}

// taskGraphEdges returns every edge between displayed nodes. Subtasks mode
// keeps only edges touching a member, so externals stay unexpanded.
func (s *Service) taskGraphEdges(ctx context.Context, b *taskGraphBuilder, includeAll bool) ([]TaskGraphEdge, error) {
	if len(b.order) == 0 {
		return nil, nil
	}
	seen := map[taskGraphEdgeKey]bool{}
	blockerEdges, err := s.appendBlockerEdges(ctx, b, includeAll, seen)
	if err != nil {
		return nil, err
	}
	relatedEdges, err := s.appendRelatedEdges(ctx, b, includeAll, seen)
	if err != nil {
		return nil, err
	}
	return append(blockerEdges, relatedEdges...), nil
}

func (s *Service) appendBlockerEdges(ctx context.Context, b *taskGraphBuilder, includeAll bool, seen map[taskGraphEdgeKey]bool) ([]TaskGraphEdge, error) {
	if s.blockers == nil {
		return nil, nil
	}
	preds, err := s.blockers.ListBlockersForTasks(ctx, b.order)
	if err != nil {
		return nil, err
	}
	var edges []TaskGraphEdge
	for _, to := range b.order {
		for _, from := range preds[to] {
			fromNode, toNode := b.nodes[from], b.nodes[to]
			key := taskGraphEdgeKey{from, to, TaskGraphEdgeDependency}
			if fromNode == nil || seen[key] {
				continue
			}
			if !includeAll && fromNode.Role != TaskGraphRoleMember && toNode.Role != TaskGraphRoleMember {
				continue
			}
			seen[key] = true
			edges = append(edges, TaskGraphEdge{From: from, To: to, Kind: TaskGraphEdgeDependency, Status: DependencyStatusForTask(fromNode.Task)})
		}
	}
	return edges, nil
}

func (s *Service) appendRelatedEdges(ctx context.Context, b *taskGraphBuilder, includeAll bool, seen map[taskGraphEdgeKey]bool) ([]TaskGraphEdge, error) {
	if s.relations == nil {
		return nil, nil
	}
	relations, err := s.relations.ListRelationsForTasks(ctx, b.order)
	if err != nil {
		return nil, err
	}
	var edges []TaskGraphEdge
	for _, taskID := range b.order {
		for _, relatedID := range relations[taskID] {
			a, z := taskID, relatedID
			if a > z {
				a, z = z, a
			}
			fromNode, toNode := b.nodes[a], b.nodes[z]
			if fromNode == nil || toNode == nil {
				continue
			}
			if !includeAll && (fromNode.Role != TaskGraphRoleMember || toNode.Role != TaskGraphRoleMember) {
				continue
			}
			key := taskGraphEdgeKey{a, z, TaskGraphEdgeRelated}
			if seen[key] {
				continue
			}
			seen[key] = true
			edges = append(edges, TaskGraphEdge{From: a, To: z, Kind: TaskGraphEdgeRelated})
		}
	}
	return edges, nil
}

func (s *Service) applyTaskGraphViews(ctx context.Context, nodes []TaskGraphNode) {
	tasks := make([]*models.Task, 0, len(nodes))
	for _, n := range nodes {
		tasks = append(tasks, n.Task)
	}
	views := s.BuildDependencyViews(ctx, tasks)
	for i := range nodes {
		nodes[i].View = views[nodes[i].Task.ID]
	}
}

type taskGraphEdgeKey struct{ from, to, kind string }

type taskGraphBuilder struct {
	workspaceID string
	order       []string
	nodes       map[string]*TaskGraphNode
	visited     map[string]bool
	truncated   bool
}

func newTaskGraphBuilder(workspaceID string) *taskGraphBuilder {
	return &taskGraphBuilder{workspaceID: workspaceID, nodes: map[string]*TaskGraphNode{}, visited: map[string]bool{}}
}

func (b *taskGraphBuilder) remaining() int { return maxTaskGraphNodes - len(b.order) }

// add returns false once the node budget is exhausted.
func (b *taskGraphBuilder) add(task *models.Task, role string, depth int, parentTitle string) bool {
	if b.nodes[task.ID] != nil {
		return true
	}
	if b.workspaceID != "" && task.WorkspaceID != "" && task.WorkspaceID != b.workspaceID {
		return true
	}
	if len(b.order) >= maxTaskGraphNodes {
		b.truncated = true
		return false
	}
	b.visited[task.ID] = true
	b.order = append(b.order, task.ID)
	b.nodes[task.ID] = &TaskGraphNode{Task: task, Role: role, Depth: depth, ParentTitle: parentTitle}
	return true
}

func (b *taskGraphBuilder) list() []TaskGraphNode {
	out := make([]TaskGraphNode, 0, len(b.order))
	for _, id := range b.order {
		out = append(out, *b.nodes[id])
	}
	return out
}
