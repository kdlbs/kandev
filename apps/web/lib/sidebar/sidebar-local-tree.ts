import type { TaskSwitcherItem } from "@/components/task/task-switcher-types";
import { sqliteBinary } from "./sidebar-local-order";

export function sidebarChildren(tasks: TaskSwitcherItem[]): Map<string, TaskSwitcherItem[]> {
  const ids = new Set(tasks.map((task) => task.id));
  const children = new Map<string, TaskSwitcherItem[]>();
  for (const task of tasks) {
    if (!task.parentTaskId || !ids.has(task.parentTaskId)) continue;
    const siblings = children.get(task.parentTaskId) ?? [];
    siblings.push(task);
    children.set(task.parentTaskId, siblings);
  }
  return children;
}

/** Break each display cycle at its smallest ID without rewriting canonical relationships. */
export function breakSidebarCycles<T extends TaskSwitcherItem>(tasks: T[]): T[] {
  const byId = new Map(tasks.map((task) => [task.id, task]));
  const visited = new Set<string>();
  const roots = new Set<string>();
  for (const task of tasks) {
    const path: string[] = [];
    const index = new Map<string, number>();
    let id: string | undefined = task.id;
    while (id && byId.has(id) && !visited.has(id)) {
      if (index.has(id)) {
        roots.add(path.slice(index.get(id)).sort(sqliteBinary)[0]);
        break;
      }
      index.set(id, path.length);
      path.push(id);
      id = byId.get(id)?.parentTaskId;
    }
    path.forEach((value) => visited.add(value));
  }
  return tasks.map((task) =>
    roots.has(task.id) || !byId.has(task.parentTaskId ?? "")
      ? { ...task, parentTaskId: undefined }
      : task,
  );
}

export function collapsedSidebarDescendants(
  children: Map<string, TaskSwitcherItem[]>,
  collapsed: string[],
): Set<string> {
  const hidden = new Set<string>();
  const pending = collapsed.flatMap((id) => children.get(id) ?? []);
  while (pending.length) {
    const task = pending.pop()!;
    if (hidden.has(task.id)) continue;
    hidden.add(task.id);
    pending.push(...(children.get(task.id) ?? []));
  }
  return hidden;
}

export function sidebarDescendantCounts(
  roots: TaskSwitcherItem[],
  children: Map<string, TaskSwitcherItem[]>,
): Map<string, number> {
  const ordered: TaskSwitcherItem[] = [];
  const pending = [...roots];
  while (pending.length) {
    const task = pending.pop()!;
    ordered.push(task);
    pending.push(...(children.get(task.id) ?? []));
  }
  const counts = new Map<string, number>();
  for (let index = ordered.length - 1; index >= 0; index--) {
    const task = ordered[index];
    counts.set(
      task.id,
      (children.get(task.id) ?? []).reduce(
        (total, child) => total + 1 + (counts.get(child.id) ?? 0),
        0,
      ),
    );
  }
  return counts;
}
