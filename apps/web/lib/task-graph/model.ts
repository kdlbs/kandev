import type {
  TaskGraphEdgeResponse,
  TaskGraphMode,
  TaskGraphNodeResponse,
  TaskGraphResponse,
} from "@/lib/api/domains/task-graph-api";

// Renderer-agnostic graph semantics. See
// docs/specs/tasks/system-design/task-relationship-graph.md#graph-model.

export type TaskGraphNode = TaskGraphNodeResponse & { step: number; focused: boolean };
export type TaskGraphEdge = TaskGraphEdgeResponse & { id: string };

export type TaskGraphModel = {
  mode: TaskGraphMode;
  availableModes: TaskGraphMode[];
  focusId: string;
  root?: { id: string; title: string };
  nodes: TaskGraphNode[];
  edges: TaskGraphEdge[];
  truncated: boolean;
  noOrderSet: boolean;
  predecessors: Map<string, string[]>;
  dependents: Map<string, string[]>;
};

export type TaskGraphChain = { nodeIds: Set<string>; edgeIds: Set<string> };

export function taskGraphEdgeId(from: string, to: string): string {
  return `${from}->${to}`;
}

export function taskGraphEdgeKey(from: string, to: string, kind: string): string {
  return `${kind}:${from}->${to}`;
}

export function buildTaskGraphModel(response: TaskGraphResponse): TaskGraphModel {
  const ids = new Set(response.nodes.map((n) => n.id));
  const edges = response.edges
    .filter((e) => ids.has(e.from) && ids.has(e.to) && e.from !== e.to)
    .map((e) => ({
      ...e,
      kind: e.kind ?? "dependency",
      id: taskGraphEdgeKey(e.from, e.to, e.kind ?? "dependency"),
    }));
  const dependencyEdges = edges.filter((edge) => edge.kind === "dependency");
  const predecessors = adjacency(ids, dependencyEdges, "to", "from");
  const dependents = adjacency(ids, dependencyEdges, "from", "to");
  const stepById = computeSteps(response.nodes, predecessors, dependents);
  const discovery = new Map(response.nodes.map((n, i) => [n.id, i]));
  const nodes = response.nodes
    .map((n) => ({ ...n, step: stepById.get(n.id) ?? 1, focused: n.id === response.focus_task_id }))
    .sort((a, b) => a.step - b.step || (discovery.get(a.id) ?? 0) - (discovery.get(b.id) ?? 0));
  return {
    mode: response.mode,
    availableModes: response.available_modes,
    focusId: response.focus_task_id,
    root: response.root,
    nodes,
    edges,
    truncated: response.truncated,
    noOrderSet: response.mode === "subtasks" && !hasMemberOrder(response.nodes, edges),
    predecessors,
    dependents,
  };
}

function adjacency(
  ids: Set<string>,
  edges: TaskGraphEdge[],
  key: "from" | "to",
  value: "from" | "to",
): Map<string, string[]> {
  const out = new Map<string, string[]>();
  for (const id of ids) out.set(id, []);
  for (const e of edges) out.get(e[key])?.push(e[value]);
  return out;
}

// Kahn pass over the longest-path rank; leftovers from a corrupt cycle are
// placed after every ranked node so the walk always terminates.
function computeSteps(
  nodes: TaskGraphNodeResponse[],
  predecessors: Map<string, string[]>,
  dependents: Map<string, string[]>,
): Map<string, number> {
  const remaining = new Map(nodes.map((n) => [n.id, predecessors.get(n.id)?.length ?? 0]));
  const steps = new Map<string, number>();
  const queue = nodes.filter((n) => remaining.get(n.id) === 0).map((n) => n.id);
  for (const id of queue) steps.set(id, 1);
  while (queue.length > 0) {
    const id = queue.shift() as string;
    for (const next of dependents.get(id) ?? []) {
      steps.set(next, Math.max(steps.get(next) ?? 0, (steps.get(id) ?? 1) + 1));
      const left = (remaining.get(next) ?? 0) - 1;
      remaining.set(next, left);
      if (left === 0) queue.push(next);
    }
  }
  const cycleStep = Math.max(0, ...steps.values()) + 1;
  for (const n of nodes) {
    if ((remaining.get(n.id) ?? 0) > 0) steps.set(n.id, cycleStep);
  }
  return steps;
}

function hasMemberOrder(nodes: TaskGraphNodeResponse[], edges: TaskGraphEdge[]): boolean {
  const members = new Set(nodes.filter((n) => n.role === "member").map((n) => n.id));
  return edges.some((e) => e.kind === "dependency" && members.has(e.from) && members.has(e.to));
}

/** Transitive predecessors and dependents of nodeId, plus the edges between them. */
export function traceChain(model: TaskGraphModel, nodeId: string): TaskGraphChain {
  const up = walk(nodeId, model.predecessors);
  const down = walk(nodeId, model.dependents);
  const nodeIds = new Set([...up, ...down]);
  const edgeIds = new Set(
    model.edges
      .filter((e) => (up.has(e.from) && up.has(e.to)) || (down.has(e.from) && down.has(e.to)))
      .map((e) => e.id),
  );
  return { nodeIds, edgeIds };
}

function walk(start: string, next: Map<string, string[]>): Set<string> {
  const seen = new Set([start]);
  const stack = [start];
  while (stack.length > 0) {
    for (const id of next.get(stack.pop() as string) ?? []) {
      if (!seen.has(id)) {
        seen.add(id);
        stack.push(id);
      }
    }
  }
  return seen;
}
