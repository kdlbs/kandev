import type { KanbanState } from "@/lib/state/slices";
import type { Message, TaskPendingAction } from "@/lib/types/http";
import {
  hasPendingClarification,
  hasPendingPermissionRequest,
} from "@/lib/utils/pending-clarification";
import { aggregateTaskPendingInput } from "@/lib/utils/task-pending-input";
import { pickFreshestStatusSummary } from "@/lib/task-status-summary";

/** Flat per-session pending flags keyed for shallow comparison so the sidebar
 *  only re-renders when a clarification/permission flag actually flips, not on
 *  every streaming token that churns the messages map. */
const pendingClarKey = (sessionId: string) => `${sessionId}#clar`;
const pendingPermKey = (sessionId: string) => `${sessionId}#perm`;

export function buildPendingFlags(
  bySession: Record<string, Message[] | undefined>,
  sessionIds: string[],
): Record<string, boolean> {
  const flags: Record<string, boolean> = {};
  for (const id of sessionIds) {
    const msgs = bySession[id];
    if (msgs === undefined) continue;
    flags[pendingClarKey(id)] = hasPendingClarification(msgs);
    flags[pendingPermKey(id)] = hasPendingPermissionRequest(msgs);
  }
  return flags;
}

export function workflowStepTitle(
  task: KanbanState["tasks"][number],
  stepTitleById: Map<string, string>,
): string | undefined {
  if (!task.workflowStepId) return undefined;
  return stepTitleById.get(task.workflowStepId as string);
}

export function readTaskPendingFlags(
  pendingFlags: Record<string, boolean>,
  sessions: Array<{ id: string; state: string }>,
  taskPendingAction?: TaskPendingAction | null,
): { clarification: boolean; permission: boolean } {
  const { clarification, permission } = aggregateTaskPendingInput(
    sessions,
    (session) => {
      const clarKey = pendingClarKey(session.id);
      const permKey = pendingPermKey(session.id);
      if (!(clarKey in pendingFlags) && !(permKey in pendingFlags)) return undefined;
      return {
        clarification: pendingFlags[clarKey] ?? false,
        permission: pendingFlags[permKey] ?? false,
      };
    },
    taskPendingAction,
  );
  return { clarification, permission };
}

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

function activeTaskIsNewer(
  active: KanbanState["tasks"][number],
  projected: KanbanState["tasks"][number],
): boolean {
  const activeTime = active.updatedAt ? Date.parse(active.updatedAt) : 0;
  const projectedTime = projected.updatedAt ? Date.parse(projected.updatedAt) : 0;
  return activeTime >= projectedTime;
}

function reconcileTaskProjection(
  active: KanbanState["tasks"][number],
  projected: AggregatedSidebarTasks["allTasks"][number],
  activeWorkflowId: string,
): AggregatedSidebarTasks["allTasks"][number] {
  const activeTask = {
    ...active,
    // Autopilot is immutable after creation. The active slice can be fed by
    // a partial WS event, so do not let it erase the value from the full
    // workflow projection.
    autopilot: active.autopilot ?? projected.autopilot,
    _workflowId: activeWorkflowId,
  };
  const task = activeTaskIsNewer(active, projected) ? activeTask : projected;
  const interrupted = task.interrupted ?? projected.interrupted;
  return {
    ...task,
    ...(interrupted === undefined ? {} : { interrupted }),
    // Workflow snapshots can re-stamp queued_prompt_count at an equal
    // revision, so treat the snapshot as the incoming reading and the live
    // projection as the cached fallback.
    statusSummary: pickFreshestStatusSummary(projected.statusSummary, active.statusSummary),
  };
}

function applyActiveKanbanFallback(
  activeWorkflowId: string,
  activeTasks: KanbanState["tasks"],
  activeSteps: KanbanState["steps"],
  acc: Acc,
): void {
  // The active kanban slice receives live workflow-step WS updates. Prefer it
  // over an existing snapshot so completion icons reflect creates, deletes,
  // and reorders without waiting for the next full snapshot refresh.
  if (activeSteps.length > 0) {
    for (const step of activeSteps) acc.stepMap.set(step.id, step);
    acc.wfSteps[activeWorkflowId] = [...activeSteps].sort((a, b) => a.position - b.position);
  }
  // Hydrator's mergeKanbanTasks accumulates tasks across workflow switches, so
  // activeTasks can carry stale entries whose workflowStepId references a step
  // from another workspace's workflow. Filter to current step membership so
  // those leaks don't get re-tagged with the active workflow id.
  const activeStepIds = new Set(activeSteps.map((s) => s.id));
  for (const t of activeTasks) {
    if (activeStepIds.size > 0 && !activeStepIds.has(t.workflowStepId)) continue;
    const existingIndex = acc.tasks.findIndex((task) => task.id === t.id);
    if (existingIndex >= 0) {
      acc.tasks[existingIndex] = reconcileTaskProjection(
        t,
        acc.tasks[existingIndex],
        activeWorkflowId,
      );
    } else {
      acc.tasks.push({ ...t, _workflowId: activeWorkflowId });
    }
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
