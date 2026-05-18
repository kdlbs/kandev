import type { KanbanState } from "@/lib/state/slices";

export type SidebarStepInfo = {
  id: string;
  title: string;
  color: string;
  position: number;
  events?: { on_enter?: Array<{ type: string; config?: Record<string, unknown> }> };
};

export type WorkflowSnapshotMap = Record<
  string,
  { steps: SidebarStepInfo[]; tasks: KanbanState["tasks"] }
>;

export type AggregatedSidebarTasks = {
  allTasks: Array<KanbanState["tasks"][number] & { _workflowId: string }>;
  allSteps: SidebarStepInfo[];
  stepsByWorkflowId: Record<string, SidebarStepInfo[]>;
};

type Acc = {
  tasks: AggregatedSidebarTasks["allTasks"];
  seen: Set<string>;
  stepMap: Map<string, SidebarStepInfo>;
  wfSteps: Record<string, SidebarStepInfo[]>;
};

function collectSnapshotTasks(snapshots: WorkflowSnapshotMap, acc: Acc): void {
  for (const [wfId, snapshot] of Object.entries(snapshots)) {
    for (const step of snapshot.steps)
      if (!acc.stepMap.has(step.id)) acc.stepMap.set(step.id, step);
    acc.wfSteps[wfId] = [...snapshot.steps].sort((a, b) => a.position - b.position);
    for (const t of snapshot.tasks) {
      acc.tasks.push({ ...t, _workflowId: wfId });
      acc.seen.add(t.id);
    }
  }
}

function applyActiveKanbanFallback(
  activeWorkflowId: string,
  activeTasks: KanbanState["tasks"],
  activeSteps: KanbanState["steps"],
  acc: Acc,
): void {
  if (!acc.wfSteps[activeWorkflowId] && activeSteps.length > 0) {
    for (const step of activeSteps) if (!acc.stepMap.has(step.id)) acc.stepMap.set(step.id, step);
    acc.wfSteps[activeWorkflowId] = [...activeSteps].sort((a, b) => a.position - b.position);
  }
  // Hydrator's mergeKanbanTasks accumulates tasks across workflow switches, so
  // activeTasks can carry stale entries whose workflowStepId references a step
  // from another workspace's workflow. Filter to current step membership so
  // those leaks don't get re-tagged with the active workflow id.
  const activeStepIds = new Set(activeSteps.map((s) => s.id));
  for (const t of activeTasks) {
    if (acc.seen.has(t.id)) continue;
    if (activeStepIds.size > 0 && !activeStepIds.has(t.workflowStepId)) continue;
    acc.tasks.push({ ...t, _workflowId: activeWorkflowId });
    acc.seen.add(t.id);
  }
}

/**
 * Aggregate the sidebar's task/step view across all loaded workflow snapshots,
 * with a fallback to the active `kanban` slice. The fallback is essential
 * because `task.created` WS events arriving before `fetchWorkflowSnapshot`
 * completes are dropped from `kanbanMulti.snapshots` and would otherwise be
 * invisible until the next snapshot refresh.
 */
export function aggregateSidebarTasks(
  snapshots: WorkflowSnapshotMap,
  activeWorkflowId: string | null,
  activeTasks: KanbanState["tasks"],
  activeSteps: KanbanState["steps"],
): AggregatedSidebarTasks {
  const acc: Acc = {
    tasks: [],
    seen: new Set<string>(),
    stepMap: new Map<string, SidebarStepInfo>(),
    wfSteps: {},
  };
  collectSnapshotTasks(snapshots, acc);
  if (activeWorkflowId) {
    applyActiveKanbanFallback(activeWorkflowId, activeTasks, activeSteps, acc);
  }
  const allSteps = [...acc.stepMap.values()].sort((a, b) => a.position - b.position);
  return { allTasks: acc.tasks, allSteps, stepsByWorkflowId: acc.wfSteps };
}
