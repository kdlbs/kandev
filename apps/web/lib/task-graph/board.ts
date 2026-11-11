import type { TaskGraphEdgeStatus, TaskGraphResponse } from "@/lib/api/domains/task-graph-api";

// Board Graph view: the board's own (filtered) tasks as one graph, with
// subtasks framed by parent. Built client-side from the board projection.

export type BoardGraphTask = {
  id: string;
  title: string;
  state?: string;
  parentTaskId?: string | null;
  blocked?: boolean;
  blockedReason?: string;
  dependsOn?: Array<{ id: string; status?: TaskGraphEdgeStatus }>;
  related?: Array<{ id: string }>;
};

export type BoardGraphGroup = {
  id: string;
  parentId: string;
  title: string;
  memberIds: string[];
};

export function boardGraphGroupId(parentId: string): string {
  return `group:${parentId}`;
}

export function buildBoardGraph(
  tasks: BoardGraphTask[],
  focusId: string | null,
  parentTitle: (parentId: string) => string | undefined = () => undefined,
): { response: TaskGraphResponse; groups: BoardGraphGroup[] } {
  const ids = new Set(tasks.map((t) => t.id));
  const titles = new Map(tasks.map((t) => [t.id, t.title]));
  const relatedPairs = new Set<string>();
  const relatedEdges: TaskGraphResponse["edges"] = [];
  for (const task of tasks) {
    for (const relation of task.related ?? []) {
      if (!ids.has(relation.id)) continue;
      const [from, to] = [task.id, relation.id].sort();
      const pairKey = JSON.stringify([from, to]);
      if (relatedPairs.has(pairKey)) continue;
      relatedPairs.add(pairKey);
      relatedEdges.push({ from, to, kind: "related" });
    }
  }
  const dependencyEdges = tasks.flatMap((t) =>
    (t.dependsOn ?? [])
      .filter((dep) => ids.has(dep.id))
      .map((dep) => ({
        from: dep.id,
        to: t.id,
        kind: "dependency" as const,
        status: dep.status ?? "pending",
      })),
  );
  const response: TaskGraphResponse = {
    mode: "chain",
    available_modes: ["chain"],
    focus_task_id: focusId ?? "",
    nodes: tasks.map((t) => ({
      id: t.id,
      title: t.title,
      state: t.state ?? "TODO",
      parent_id: t.parentTaskId ?? undefined,
      depth: 0,
      role: "member",
      archived: false,
      blocked: Boolean(t.blocked),
      blocked_reason: t.blockedReason,
    })),
    edges: [...dependencyEdges, ...relatedEdges],
    truncated: false,
  };
  const groups = new Map<string, BoardGraphGroup>();
  for (const t of tasks) {
    if (!t.parentTaskId) continue;
    const group = groups.get(t.parentTaskId) ?? {
      id: boardGraphGroupId(t.parentTaskId),
      parentId: t.parentTaskId,
      title: titles.get(t.parentTaskId) ?? parentTitle(t.parentTaskId) ?? "",
      memberIds: [],
    };
    group.memberIds.push(t.id);
    groups.set(t.parentTaskId, group);
  }
  return { response, groups: [...groups.values()] };
}
