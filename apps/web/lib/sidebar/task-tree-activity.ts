import type { TaskSwitcherItem } from "@/components/task/task-switcher";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

type IncludedTaskGraph = {
  tasksById: Map<string, TaskSwitcherItem>;
  taskIds: Set<string>;
  childrenById: Map<string, string[]>;
  parentsById: Map<string, string[]>;
};

type ComponentGraph = {
  children: Set<number>[];
  parents: Set<number>[];
};

export function taskActivitySortValue(task: TaskSwitcherItem): string {
  return task.lastActivityAt ?? task.updatedAt ?? task.createdAt ?? "";
}

function parseActivityTimestamp(value: string): bigint | null {
  return parseStrictRfc3339Timestamp(value);
}

/** Compare valid timestamps by instant, preserving lexical fallback for missing or invalid values. */
export function compareActivityTimestamps(a: string, b: string): number {
  if (a === b) return 0;
  const parsedA = parseActivityTimestamp(a);
  const parsedB = parseActivityTimestamp(b);
  if (parsedA !== null && parsedB !== null) {
    if (parsedA === parsedB) return 0;
    return parsedA < parsedB ? -1 : 1;
  }
  return a.localeCompare(b);
}

function laterActivity(a: string, b: string): string {
  return compareActivityTimestamps(a, b) >= 0 ? a : b;
}

function buildIncludedTaskGraph(
  tasks: TaskSwitcherItem[],
  subTasksByParentId: Map<string, TaskSwitcherItem[]>,
): IncludedTaskGraph {
  const tasksById = new Map(tasks.map((task) => [task.id, task]));
  const taskIds = new Set(tasksById.keys());
  const childrenById = new Map<string, string[]>();
  const parentsById = new Map<string, string[]>();
  for (const taskId of taskIds) {
    childrenById.set(taskId, []);
    parentsById.set(taskId, []);
  }

  for (const [parentId, children] of subTasksByParentId) {
    if (!taskIds.has(parentId)) continue;
    const includedChildren = childrenById.get(parentId)!;
    const seenChildIds = new Set<string>();
    for (const child of children) {
      if (!taskIds.has(child.id) || seenChildIds.has(child.id)) continue;
      seenChildIds.add(child.id);
      includedChildren.push(child.id);
      parentsById.get(child.id)!.push(parentId);
    }
  }

  return { tasksById, taskIds, childrenById, parentsById };
}

function buildFinishOrder(taskIds: Set<string>, childrenById: Map<string, string[]>): string[] {
  const visited = new Set<string>();
  const finishOrder: string[] = [];
  for (const startId of taskIds) {
    if (visited.has(startId)) continue;
    visited.add(startId);
    const stack: Array<{ taskId: string; nextChildIndex: number }> = [
      { taskId: startId, nextChildIndex: 0 },
    ];
    while (stack.length > 0) {
      const frame = stack[stack.length - 1];
      const children = childrenById.get(frame.taskId)!;
      if (frame.nextChildIndex === children.length) {
        finishOrder.push(frame.taskId);
        stack.pop();
        continue;
      }

      const childId = children[frame.nextChildIndex++];
      if (visited.has(childId)) continue;
      visited.add(childId);
      stack.push({ taskId: childId, nextChildIndex: 0 });
    }
  }
  return finishOrder;
}

function buildStrongComponents(
  finishOrder: string[],
  parentsById: Map<string, string[]>,
): { components: string[][]; componentByTaskId: Map<string, number> } {
  const componentByTaskId = new Map<string, number>();
  const components: string[][] = [];
  for (let index = finishOrder.length - 1; index >= 0; index -= 1) {
    const startId = finishOrder[index];
    if (componentByTaskId.has(startId)) continue;

    const componentId = components.length;
    const members: string[] = [];
    const stack = [startId];
    componentByTaskId.set(startId, componentId);
    while (stack.length > 0) {
      const taskId = stack.pop()!;
      members.push(taskId);
      for (const parentId of parentsById.get(taskId)!) {
        if (componentByTaskId.has(parentId)) continue;
        componentByTaskId.set(parentId, componentId);
        stack.push(parentId);
      }
    }
    components.push(members);
  }
  return { components, componentByTaskId };
}

function buildComponentGraph(
  components: string[][],
  componentByTaskId: Map<string, number>,
  childrenById: Map<string, string[]>,
): ComponentGraph {
  const children = components.map(() => new Set<number>());
  const parents = components.map(() => new Set<number>());
  for (const [parentId, childIds] of childrenById) {
    const parentComponent = componentByTaskId.get(parentId)!;
    for (const childId of childIds) {
      const childComponent = componentByTaskId.get(childId)!;
      if (parentComponent === childComponent) continue;
      children[parentComponent].add(childComponent);
      parents[childComponent].add(parentComponent);
    }
  }
  return { children, parents };
}

function resolveComponentActivity(
  components: string[][],
  graph: ComponentGraph,
  tasksById: Map<string, TaskSwitcherItem>,
): string[] {
  const latestByComponent = components.map((members) =>
    members.reduce(
      (latest, taskId) => laterActivity(latest, taskActivitySortValue(tasksById.get(taskId)!)),
      "",
    ),
  );
  const remainingChildren = graph.children.map((children) => children.size);
  const ready: number[] = [];
  for (let componentId = 0; componentId < remainingChildren.length; componentId += 1) {
    if (remainingChildren[componentId] === 0) ready.push(componentId);
  }

  for (let head = 0; head < ready.length; head += 1) {
    const childComponent = ready[head];
    for (const parentComponent of graph.parents[childComponent]) {
      latestByComponent[parentComponent] = laterActivity(
        latestByComponent[parentComponent],
        latestByComponent[childComponent],
      );
      remainingChildren[parentComponent] -= 1;
      if (remainingChildren[parentComponent] === 0) ready.push(parentComponent);
    }
  }
  return latestByComponent;
}

/** Resolve each included task to the newest activity in its reachable subtree. */
export function resolveTaskTreeActivity(
  tasks: TaskSwitcherItem[],
  subTasksByParentId: Map<string, TaskSwitcherItem[]>,
): Map<string, string> {
  const taskGraph = buildIncludedTaskGraph(tasks, subTasksByParentId);
  const finishOrder = buildFinishOrder(taskGraph.taskIds, taskGraph.childrenById);
  const { components, componentByTaskId } = buildStrongComponents(
    finishOrder,
    taskGraph.parentsById,
  );
  const componentGraph = buildComponentGraph(components, componentByTaskId, taskGraph.childrenById);
  const latestByComponent = resolveComponentActivity(
    components,
    componentGraph,
    taskGraph.tasksById,
  );
  return new Map(
    Array.from(componentByTaskId, ([taskId, componentId]) => [
      taskId,
      latestByComponent[componentId],
    ]),
  );
}
