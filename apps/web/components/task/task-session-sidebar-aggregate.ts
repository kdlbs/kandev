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

/**
 * Aggregate the sidebar's task/step view across all loaded workflow snapshots
 * from the TanStack Query `qk.kanban.multi()` cache. The active workflow's
 * snapshot is always present in that cache (SSR seed + `useAllWorkflowSnapshots`
 * + the kanban bridge writing `task.created` / `kanban.update` directly), so no
 * separate single-workflow fallback is needed.
 */
export function aggregateSidebarTasks(snapshots: WorkflowSnapshotMap): AggregatedSidebarTasks {
  const acc: Acc = {
    tasks: [],
    seen: new Set<string>(),
    stepMap: new Map<string, SidebarStepInfo>(),
    wfSteps: {},
  };
  collectSnapshotTasks(snapshots, acc);
  const allSteps = [...acc.stepMap.values()].sort((a, b) => a.position - b.position);
  return { allTasks: acc.tasks, allSteps, stepsByWorkflowId: acc.wfSteps };
}
