import type { TaskSwitcherItem } from "@/components/task/task-switcher-types";
import type {
  SidebarTaskPageEntry,
  SidebarTaskPageResponse,
  SidebarTaskQuery,
} from "@/lib/types/http";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import { applyGroup, type SidebarGroup, type SidebarTaskPrefs } from "./apply-view";
import { resolveEffectiveStateMap, STATE_GROUP_ORDER } from "./effective-task-tree-state";
import { resolveTaskTreeActivity } from "./task-tree-activity";
import { resolveTaskTreeRunning } from "./task-tree-running";
import { sidebarSortFromWire, sidebarSortHasKey } from "./sidebar-sort-chain";
import {
  idOrder,
  localTaskComparator,
  sqliteActivityKey,
  sqliteBinary,
} from "./sidebar-local-order";
import { sqliteLower } from "./sidebar-local-filter";
import { matchesLocalSidebarTask, type LocalSidebarTask } from "./sidebar-local-projection";
import {
  breakSidebarCycles,
  collapsedSidebarDescendants,
  sidebarChildren,
  sidebarDescendantCounts,
} from "./sidebar-local-tree";
import { localSidebarQueues } from "./sidebar-local-wip";
import { effectiveSidebarColorToken, type SidebarColorRankingSettings } from "./sidebar-color-rank";

function repositoryRank(key: string): number {
  if (key === "__multi__") return 0;
  if (key.startsWith("__repo_combination__:")) return 1;
  return key === "__unassigned__" ? 3 : 2;
}

function orderGroups(
  groups: SidebarGroup[],
  groupKey: SidebarView["group"],
  rootOrder: Map<string, number>,
) {
  const first = (group: SidebarGroup) =>
    group.tasks.reduce((first, task) => Math.min(first, rootOrder.get(task.id)!), Infinity);
  groups.sort((a, b) => {
    if (groupKey === "state")
      return (
        (STATE_GROUP_ORDER[a.key] ?? 99) - (STATE_GROUP_ORDER[b.key] ?? 99) || first(a) - first(b)
      );
    if (groupKey === "repository")
      return (
        repositoryRank(a.key) - repositoryRank(b.key) ||
        sqliteBinary(sqliteLower(a.label), sqliteLower(b.label)) ||
        sqliteBinary(a.label, b.label) ||
        first(a) - first(b)
      );
    return first(a) - first(b) || sqliteBinary(a.key, b.key);
  });
}

function orderedLocalTree(
  tasks: LocalSidebarTask[],
  query: SidebarTaskQuery,
  prefs: SidebarTaskPrefs,
  colorSettings?: SidebarColorRankingSettings,
) {
  const children = sidebarChildren(tasks);
  const states = resolveEffectiveStateMap(tasks, children);
  const activityTasks = tasks.map((task) => ({
    ...task,
    lastActivityAt: sqliteActivityKey(task.lastActivityAt ?? ""),
  }));
  const activities = resolveTaskTreeActivity(
    activityTasks,
    sidebarChildren(activityTasks),
    sqliteBinary,
  );
  const sort = sidebarSortFromWire(query.sort);
  const running = sidebarSortHasKey(sort, "running")
    ? resolveTaskTreeRunning(tasks, children)
    : new Map(tasks.map((task) => [task.id, task.sessionState === "RUNNING"]));
  const colors =
    sidebarSortHasKey(sort, "color") && colorSettings
      ? new Map(tasks.map((task) => [task.id, effectiveSidebarColorToken(task, colorSettings)]))
      : new Map<string, string | null>();
  const compare = localTaskComparator({
    sort,
    orderedIds: prefs.orderedTaskIds,
    states,
    activities,
    running,
    colors,
  });
  const sorted = breakSidebarCycles(tasks).sort(compare);
  const rootOrder = new Map(sorted.map((task, index) => [task.id, index]));
  const grouped = applyGroup(sorted, query.group as SidebarView["group"], children, states);
  grouped.groups = grouped.groups.filter((group) => group.tasks.length > 0);
  orderGroups(grouped.groups, query.group as SidebarView["group"], rootOrder);
  const pin = idOrder(prefs.pinnedTaskIds);
  for (const group of grouped.groups)
    group.tasks.sort((a, b) => pin(a.id) - pin(b.id) || compare(a, b));
  for (const [parent, children] of grouped.subTasksByParentId) {
    const manual = idOrder(prefs.subtaskOrderByParentId?.[parent] ?? []);
    children.sort((a, b) => manual(a.id) - manual(b.id) || compare(a, b));
  }
  return { ...grouped, hidden: collapsedSidebarDescendants(children, query.collapsed_task_ids) };
}

type TreeRow = { task: TaskSwitcherItem; group: SidebarGroup; depth: number; position: number };

function flattenTree(tree: ReturnType<typeof orderedLocalTree>, query: SidebarTaskQuery) {
  const rows: TreeRow[] = [];
  const counts = new Map<string, number>();
  const collapsed = new Set(query.collapsed_group_keys);
  for (const group of tree.groups) {
    let position = 0;
    const pending = group.tasks.map((task) => ({ task, depth: 0 })).reverse();
    while (pending.length) {
      const { task, depth } = pending.pop()!;
      const children = tree.subTasksByParentId.get(task.id) ?? [];
      for (let index = children.length - 1; index >= 0; index--)
        pending.push({ task: children[index], depth: depth + 1 });
      if (tree.hidden.has(task.id)) continue;
      position++;
      if (!collapsed.has(group.key)) rows.push({ task, depth, group, position });
    }
    counts.set(group.key, position);
  }
  return { rows, counts };
}

function pageEntries(
  tree: ReturnType<typeof orderedLocalTree>,
  selected: TreeRow[],
  counts: Map<string, number>,
  query: SidebarTaskQuery,
) {
  const entries: SidebarTaskPageEntry[] = [];
  const ids = new Set(selected.map((row) => row.task.id));
  const continuations = new Set<string>();
  const titles = new Map<string, string>();
  for (const group of tree.groups) for (const task of group.tasks) titles.set(task.id, task.title);
  for (const children of tree.subTasksByParentId.values())
    for (const task of children) titles.set(task.id, task.title);
  const descendants = sidebarDescendantCounts(
    tree.groups.flatMap((group) => group.tasks),
    tree.subTasksByParentId,
  );
  for (const group of tree.groups) {
    const rows = selected.filter((row) => row.group.key === group.key);
    if (!rows.length && !query.collapsed_group_keys.includes(group.key)) continue;
    entries.push({
      kind: "group",
      group_key: group.key,
      group_label: group.label,
      matching_count: counts.get(group.key),
      continuation: (rows[0]?.position ?? 1) > 1,
    });
    for (const row of rows) {
      const parent = row.task.parentTaskId;
      if (parent && !ids.has(parent) && !continuations.has(parent)) {
        entries.push({
          kind: "continuation",
          parent_id: parent,
          parent_title: titles.get(parent),
          depth: Math.max(0, row.depth - 1),
        });
        continuations.add(parent);
      }
      entries.push({
        kind: "task",
        task_id: row.task.id,
        group_key: group.key,
        group_label: group.label,
        depth: row.depth,
        parent_id: parent,
        subtask_count: descendants.get(row.task.id),
        workflow_id: row.task.workflowId,
        workflow_step_id: row.task.workflowStepId,
        workflow_name: row.task.workflowName,
        workflow_step_name: row.task.workflowStepTitle,
        workflow_step_color: row.task.workflowStepColor,
      });
    }
  }
  return entries;
}

/** Evaluate a verified complete scope, then apply the same bounded page contract as the server. */
export function localSidebarPage(
  tasks: LocalSidebarTask[],
  query: SidebarTaskQuery,
  prefs: SidebarTaskPrefs,
  colorSettings?: SidebarColorRankingSettings,
): SidebarTaskPageResponse {
  const filtered = tasks.filter((task) => matchesLocalSidebarTask(task, query.filters));
  const tree = orderedLocalTree(filtered, query, prefs, colorSettings);
  const { rows, counts } = flattenTree(tree, query);
  const size = Math.min(100, Math.max(1, query.page_size));
  const pageCount = Math.max(1, Math.ceil(rows.length / size));
  const page = Math.min(pageCount, Math.max(1, query.page));
  const selected = rows.slice((page - 1) * size, page * size);
  const entries = pageEntries(tree, selected, counts, query);
  const queues = localSidebarQueues(tasks);
  for (const entry of entries) {
    const queue = entry.task_id ? queues.get(entry.task_id) : undefined;
    if (queue)
      Object.assign(entry, { wip_queue_position: queue.position, wip_queue_total: queue.total });
  }
  return {
    query_key: JSON.stringify([query, prefs]),
    page,
    page_size: size,
    total_entries: rows.length + tree.groups.length,
    total_tasks: filtered.length,
    total_visible_tasks: rows.length,
    has_previous: page > 1,
    has_next: page < pageCount,
    entries,
  };
}
