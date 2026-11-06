import type { WorkspaceAggregateEntry } from "@/lib/api/domains/office-extended-api";
import type { ActivityEntry } from "@/lib/state/slices/office/types";

/**
 * Stalled: work is in progress and no agent in the workspace is working.
 * Blocked: nothing is stalled, but tasks wait on a blocker.
 * Active: an agent is working or work is in progress.
 * Idle: nothing in progress and no agent working.
 */
export type AggregateWorkspaceStatus = "stalled" | "blocked" | "active" | "idle";

export function aggregateStatus(entry: WorkspaceAggregateEntry): AggregateWorkspaceStatus {
  if (entry.in_progress_tasks > 0 && entry.running_agents === 0) return "stalled";
  if (entry.blocked_tasks > 0) return "blocked";
  if (entry.running_agents > 0 || entry.in_progress_tasks > 0) return "active";
  return "idle";
}

export type AggregateTotals = {
  workspaces: number;
  tasks: number;
  open: number;
  inProgress: number;
  blocked: number;
  done: number;
  approvals: number;
  agents: number;
  runningAgents: number;
  byStatus: Record<AggregateWorkspaceStatus, number>;
};

export function aggregateTotals(entries: WorkspaceAggregateEntry[]): AggregateTotals {
  const totals: AggregateTotals = {
    workspaces: entries.length,
    tasks: 0,
    open: 0,
    inProgress: 0,
    blocked: 0,
    done: 0,
    approvals: 0,
    agents: 0,
    runningAgents: 0,
    byStatus: { stalled: 0, blocked: 0, active: 0, idle: 0 },
  };
  for (const entry of entries) {
    totals.tasks += entry.task_count;
    totals.open += entry.open_tasks;
    totals.inProgress += entry.in_progress_tasks;
    totals.blocked += entry.blocked_tasks;
    totals.done += entry.done_tasks;
    totals.approvals += entry.pending_approvals;
    totals.agents += entry.agent_count;
    totals.runningAgents += entry.running_agents;
    totals.byStatus[aggregateStatus(entry)] += 1;
  }
  return totals;
}

export type AggregateAttentionKind = "stalled" | "blocked" | "approvals";

export type AggregateAttentionItem = {
  kind: AggregateAttentionKind;
  workspaceId: string;
  workspaceName: string;
  /** In-progress tasks for a stall, blocked tasks, or pending approvals. */
  count: number;
};

const ATTENTION_ORDER: AggregateAttentionKind[] = ["stalled", "blocked", "approvals"];

/**
 * The workspace-level conditions that need a person: a stall, blocked tasks,
 * and approvals waiting on a decision. One workspace can raise several rows;
 * rows sort by kind, then by count (largest first), then by name.
 */
export function buildAggregateAttention(
  entries: WorkspaceAggregateEntry[],
): AggregateAttentionItem[] {
  const items: AggregateAttentionItem[] = [];
  for (const entry of entries) {
    const base = { workspaceId: entry.workspace_id, workspaceName: entry.name };
    if (aggregateStatus(entry) === "stalled")
      items.push({ ...base, kind: "stalled", count: entry.in_progress_tasks });
    if (entry.blocked_tasks > 0)
      items.push({ ...base, kind: "blocked", count: entry.blocked_tasks });
    if (entry.pending_approvals > 0)
      items.push({ ...base, kind: "approvals", count: entry.pending_approvals });
  }
  return items.sort(
    (a, b) =>
      ATTENTION_ORDER.indexOf(a.kind) - ATTENTION_ORDER.indexOf(b.kind) ||
      b.count - a.count ||
      a.workspaceName.localeCompare(b.workspaceName) ||
      a.workspaceId.localeCompare(b.workspaceId),
  );
}

export type ActivityGroup = {
  key: string;
  /** The newest entry of the run; it carries the labels the row shows. */
  entry: ActivityEntry;
  count: number;
};

function groupKey(entry: ActivityEntry): string {
  return [
    entry.workspaceId,
    entry.action,
    entry.actorType,
    entry.actorId,
    entry.targetId ?? "",
  ].join("\u0000");
}

/**
 * Folds consecutive activity entries (newest first) that repeat the same
 * action by the same actor on the same target in the same workspace into one
 * row with a count. Order is preserved; non-adjacent repeats stay separate.
 */
export function groupActivity(entries: ActivityEntry[]): ActivityGroup[] {
  const groups: ActivityGroup[] = [];
  for (const entry of entries) {
    const key = groupKey(entry);
    const last = groups[groups.length - 1];
    if (last && groupKey(last.entry) === key) {
      last.count += 1;
      continue;
    }
    groups.push({ key: `${key}\u0000${entry.id}`, entry, count: 1 });
  }
  return groups;
}

/** Workspace ids the control navigation lists but the Office aggregate does not cover. */
export function uncoveredWorkspaceIds(
  controlWorkspaceIds: string[],
  entries: WorkspaceAggregateEntry[],
): string[] {
  const covered = new Set(entries.map((entry) => entry.workspace_id));
  return controlWorkspaceIds.filter((id) => !covered.has(id));
}
