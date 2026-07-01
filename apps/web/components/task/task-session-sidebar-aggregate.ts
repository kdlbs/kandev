import type { KanbanState } from "@/lib/state/slices";
import type { Message, TaskPendingAction } from "@/lib/types/http";
import {
  hasPendingClarification,
  hasPendingPermissionRequest,
} from "@/lib/utils/pending-clarification";

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

export type PendingActionFallback = {
  primarySessionState?: string | null;
  primarySessionPendingAction?: TaskPendingAction | null;
};

function fallbackPendingFlags(fallback?: PendingActionFallback): {
  clarification: boolean;
  permission: boolean;
} {
  if (fallback?.primarySessionState !== "WAITING_FOR_INPUT") {
    return { clarification: false, permission: false };
  }
  return {
    clarification: fallback.primarySessionPendingAction === "clarification",
    permission: fallback.primarySessionPendingAction === "permission",
  };
}

export function readPendingFlags(
  pendingFlags: Record<string, boolean>,
  sessionId?: string | null,
  fallback?: PendingActionFallback,
): { clarification: boolean; permission: boolean } {
  if (!sessionId) return { clarification: false, permission: false };
  const clarKey = pendingClarKey(sessionId);
  const permKey = pendingPermKey(sessionId);
  const hasMessageFlags =
    Object.prototype.hasOwnProperty.call(pendingFlags, clarKey) ||
    Object.prototype.hasOwnProperty.call(pendingFlags, permKey);
  if (!hasMessageFlags) return fallbackPendingFlags(fallback);
  return {
    clarification: pendingFlags[clarKey] ?? false,
    permission: pendingFlags[permKey] ?? false,
  };
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

/**
 * Aggregate the sidebar's task/step view across all loaded workflow snapshot
 * query caches.
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
