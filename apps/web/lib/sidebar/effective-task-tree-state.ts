import { classifyTask, type TaskBucket } from "@/components/task/task-classify";
import type { TaskSwitcherItem } from "@/components/task/task-switcher";
import type { TaskState } from "@/lib/types/http";
import { formatTaskStateLabel } from "@/lib/ui/state-labels";

export const STATE_BUCKET_ORDER: Record<TaskBucket, number> = {
  review: 0,
  in_progress: 1,
  backlog: 2,
};

export const NOT_STARTED_STATE_GROUP_KEY = "__not_started__";

export type EffectiveTaskTreeState = {
  groupKey: string;
  label: string;
  bucket: TaskBucket;
};

type StateAggregate = {
  hasActive: boolean;
  hasScheduling: boolean;
  allCompleted: boolean;
  firstIncompleteMember?: TaskSwitcherItem;
  bestIncompleteCandidate?: TaskSwitcherItem;
};

type TaskGraph = {
  tasksById: Map<string, TaskSwitcherItem>;
  childrenById: Map<string, string[]>;
};

export const STATE_GROUP_ORDER: Record<string, number> = {
  [NOT_STARTED_STATE_GROUP_KEY]: 0,
  CREATED: 1,
  SCHEDULING: 2,
  TODO: 3,
  IN_PROGRESS: 4,
  WAITING_FOR_INPUT: 5,
  REVIEW: 6,
  BLOCKED: 7,
  FAILED: 8,
  COMPLETED: 9,
  CANCELLED: 10,
};

export function getStateBucket(task: TaskSwitcherItem): TaskBucket {
  return classifyTask(task.sessionState, task.state);
}

export function getTaskStateGroup(task: TaskSwitcherItem): { key: string; label: string } {
  if (!task.state) {
    return { key: NOT_STARTED_STATE_GROUP_KEY, label: formatTaskStateLabel(undefined) };
  }
  return { key: task.state, label: formatTaskStateLabel(task.state) };
}

function compareStateCandidates(a: TaskSwitcherItem, b: TaskSwitcherItem): number {
  const bucket = STATE_BUCKET_ORDER[getStateBucket(a)] - STATE_BUCKET_ORDER[getStateBucket(b)];
  if (bucket !== 0) return bucket;
  return (
    (STATE_GROUP_ORDER[a.state ?? NOT_STARTED_STATE_GROUP_KEY] ?? 99) -
    (STATE_GROUP_ORDER[b.state ?? NOT_STARTED_STATE_GROUP_KEY] ?? 99)
  );
}

function createStateAggregate(task: TaskSwitcherItem): StateAggregate {
  return {
    hasActive: task.state === "IN_PROGRESS" || task.sessionState === "RUNNING",
    hasScheduling: task.state === "SCHEDULING",
    allCompleted: task.state === "COMPLETED",
    firstIncompleteMember: task.state === "COMPLETED" ? undefined : task,
    bestIncompleteCandidate:
      task.state !== undefined && task.state !== "COMPLETED" ? task : undefined,
  };
}

function mergeStateAggregate(target: StateAggregate, source: StateAggregate): void {
  target.hasActive ||= source.hasActive;
  target.hasScheduling ||= source.hasScheduling;
  target.allCompleted &&= source.allCompleted;
  target.firstIncompleteMember ??= source.firstIncompleteMember;
  if (
    source.bestIncompleteCandidate &&
    (!target.bestIncompleteCandidate ||
      compareStateCandidates(source.bestIncompleteCandidate, target.bestIncompleteCandidate) < 0)
  ) {
    target.bestIncompleteCandidate = source.bestIncompleteCandidate;
  }
}

function buildTaskGraph(
  tasks: TaskSwitcherItem[],
  subMap: Map<string, TaskSwitcherItem[]>,
): TaskGraph {
  const tasksById = new Map<string, TaskSwitcherItem>();
  const pending = [...tasks];
  for (let index = 0; index < pending.length; index += 1) {
    const task = pending[index];
    if (!task || tasksById.has(task.id)) continue;
    tasksById.set(task.id, task);
    pending.push(...(subMap.get(task.id) ?? []));
  }

  const childrenById = new Map<string, string[]>();
  for (const task of tasksById.values()) {
    childrenById.set(
      task.id,
      (subMap.get(task.id) ?? []).map((child) => child.id),
    );
  }
  return { tasksById, childrenById };
}

type AggregateFrame = { taskId: string; expanded: boolean };

function completeAggregateFrame(
  frame: AggregateFrame,
  tasksById: Map<string, TaskSwitcherItem>,
  childrenById: Map<string, string[]>,
  aggregates: Map<string, StateAggregate>,
): void {
  const task = tasksById.get(frame.taskId);
  if (!task) return;
  const aggregate = createStateAggregate(task);
  for (const childId of childrenById.get(frame.taskId) ?? []) {
    const childAggregate = aggregates.get(childId);
    if (childAggregate) mergeStateAggregate(aggregate, childAggregate);
  }
  aggregates.set(frame.taskId, aggregate);
}

function scheduleAggregateChildren(
  taskId: string,
  childrenById: Map<string, string[]>,
  aggregates: Map<string, StateAggregate>,
  visiting: Set<string>,
  stack: AggregateFrame[],
): void {
  const children = childrenById.get(taskId) ?? [];
  for (let index = children.length - 1; index >= 0; index -= 1) {
    const childId = children[index];
    if (childId && !aggregates.has(childId) && !visiting.has(childId)) {
      stack.push({ taskId: childId, expanded: false });
    }
  }
}

function resolveTaskAggregate(
  rootId: string,
  tasksById: Map<string, TaskSwitcherItem>,
  childrenById: Map<string, string[]>,
  aggregates: Map<string, StateAggregate>,
): void {
  const visiting = new Set<string>();
  const stack: AggregateFrame[] = [{ taskId: rootId, expanded: false }];
  while (stack.length > 0) {
    const frame = stack.pop();
    if (!frame || aggregates.has(frame.taskId)) continue;
    if (frame.expanded) {
      completeAggregateFrame(frame, tasksById, childrenById, aggregates);
      visiting.delete(frame.taskId);
      continue;
    }
    if (visiting.has(frame.taskId)) continue;
    visiting.add(frame.taskId);
    stack.push({ taskId: frame.taskId, expanded: true });
    scheduleAggregateChildren(frame.taskId, childrenById, aggregates, visiting, stack);
  }
}

function resolveEffectiveState(
  task: TaskSwitcherItem,
  aggregate: StateAggregate,
): EffectiveTaskTreeState {
  if (aggregate.hasActive) {
    return {
      groupKey: "IN_PROGRESS",
      label: formatTaskStateLabel("IN_PROGRESS"),
      bucket: "in_progress",
    };
  }
  if (aggregate.hasScheduling) {
    return {
      groupKey: "SCHEDULING",
      label: formatTaskStateLabel("SCHEDULING"),
      bucket: "in_progress",
    };
  }
  if (aggregate.allCompleted) {
    return { groupKey: "COMPLETED", label: formatTaskStateLabel("COMPLETED"), bucket: "review" };
  }

  const bestCandidate = aggregate.bestIncompleteCandidate;
  const rootCanCompete = task.state !== "COMPLETED";
  const bestTask =
    rootCanCompete && bestCandidate && compareStateCandidates(bestCandidate, task) >= 0
      ? task
      : (bestCandidate ?? aggregate.firstIncompleteMember ?? task);
  const group = getTaskStateGroup(bestTask);
  return { groupKey: group.key, label: group.label, bucket: getStateBucket(bestTask) };
}

export function resolveEffectiveStateMap(
  tasks: TaskSwitcherItem[],
  subMap: Map<string, TaskSwitcherItem[]>,
): Map<string, EffectiveTaskTreeState> {
  const graph = buildTaskGraph(tasks, subMap);
  const aggregates = new Map<string, StateAggregate>();
  for (const task of graph.tasksById.values()) {
    if (!aggregates.has(task.id)) {
      resolveTaskAggregate(task.id, graph.tasksById, graph.childrenById, aggregates);
    }
  }

  const resolved = new Map<string, EffectiveTaskTreeState>();
  for (const task of tasks) {
    const aggregate = aggregates.get(task.id);
    if (aggregate) resolved.set(task.id, resolveEffectiveState(task, aggregate));
  }
  return resolved;
}

export function getStateGroupForKey(key: TaskState): { key: string; label: string } {
  return { key, label: formatTaskStateLabel(key) };
}
