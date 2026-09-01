import type { KanbanState } from "@/lib/state/slices";
import type { TaskSessionState, TaskState } from "@/lib/types/http";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import { statusSummaryActiveErrorPreview } from "@/lib/task-status-summary";
import { workflowStepTitle } from "./task-session-sidebar-aggregate";
import type { WipQueueStatus } from "@/lib/kanban/wip-queue";
import { resolveTaskRepositorySlugs } from "@/lib/sidebar/sidebar-task-repositories";
import { taskPRInfoFromSummary } from "./task-pr-info";

type SidebarItemContext = {
  repositorySlugById: Map<string, string | undefined>;
  titleById: Map<string, string>;
  workflowNameById: Map<string, string>;
  stepTitleById: Map<string, string>;
  wipQueueByTaskId?: Map<string, WipQueueStatus>;
  acknowledgedAgentErrors?: Record<string, string>;
  dismissedAgentErrors?: Record<string, string>;
};

function summaryDiffStats(
  summary: TaskStatusSummary | null | undefined,
): { additions: number; deletions: number } | undefined {
  const git = summary?.git;
  if (!git || git.comparison_unavailable) return undefined;
  const additions = git.additions ?? 0;
  const deletions = git.deletions ?? 0;
  return additions > 0 || deletions > 0 ? { additions, deletions } : undefined;
}

function repositoryPathFromSummary(
  summary: TaskStatusSummary | null | undefined,
): string | undefined {
  const url = summary?.pull_request?.url;
  if (!url) return undefined;
  try {
    const path = new URL(url).pathname.split("/").filter(Boolean);
    if (path.length >= 2) return `${path[0]}/${path[1]}`;
  } catch {
    // A malformed provider URL must not make the task switcher disappear.
  }
  return undefined;
}

function pendingFlags(summary: TaskStatusSummary | null | undefined, fallback?: string | null) {
  const action = summary != null ? summary.pending_action : fallback;
  return {
    clarification: action === "clarification",
    permission: action === "permission",
  };
}

function issueInfoForTask(task: KanbanState["tasks"][number]) {
  if (!task.issueUrl || !task.issueNumber) return undefined;
  return { url: task.issueUrl, number: task.issueNumber };
}

function sidebarLastActivityAt(
  summary: TaskStatusSummary | null | undefined,
  task: KanbanState["tasks"][number],
) {
  return summary?.last_activity_at ?? task.updatedAt ?? task.createdAt;
}

function sidebarSessionStatus(
  summary: TaskStatusSummary | null | undefined,
  task: KanbanState["tasks"][number],
) {
  const primarySession = summary?.primary_session;
  const hasSummary = summary != null;
  return {
    sessionState: (hasSummary ? primarySession?.state : task.primarySessionState) as
      | TaskSessionState
      | undefined,
    // The task-level MOST-ACTIVE-WINS activity aggregate (ADR-0049) is
    // authoritative for the sidebar row: when no status summary is available,
    // fall back to the task record's own aggregate so multi-session and
    // off-screen rows still agree with the board card.
    foregroundActivity: hasSummary ? summary?.foreground_activity : task.foregroundActivity,
    primarySessionId: hasSummary ? (primarySession?.id ?? null) : (task.primarySessionId ?? null),
    updatedAt: hasSummary ? summary?.updated_at : (task.updatedAt ?? task.createdAt),
    lastActivityAt: sidebarLastActivityAt(summary, task),
  };
}

function sidebarStatus(
  task: KanbanState["tasks"][number] & { _workflowId: string },
  context: SidebarItemContext,
) {
  const summary = task.statusSummary;
  const pending = pendingFlags(summary, task.taskPendingAction);
  const sessionStatus = sidebarSessionStatus(summary, task);
  return {
    ...sessionStatus,
    agentErrorMessage: statusSummaryActiveErrorPreview(
      summary,
      context.acknowledgedAgentErrors,
      context.dismissedAgentErrors,
    ),
    repositoryPath:
      repositoryPathFromSummary(summary) ??
      (task.repositoryId ? context.repositorySlugById.get(task.repositoryId) : undefined),
    diffStats: summaryDiffStats(summary),
    comparisonUnavailable: summary?.git?.comparison_unavailable === true,
    hasPendingClarification: pending.clarification,
    hasPendingPermission: pending.permission,
    prInfo: taskPRInfoFromSummary(summary),
    issueInfo: issueInfoForTask(task),
    queuedCount: summary?.queued_prompt_count,
    wipQueue: context.wipQueueByTaskId?.get(task.id),
  };
}

/** Map a task-level status projection to a sidebar item without session streams. */
export function buildSidebarItem(
  task: KanbanState["tasks"][number] & { _workflowId: string },
  context: SidebarItemContext,
) {
  const status = sidebarStatus(task, context);

  return {
    id: task.id,
    title: task.title,
    autopilot: task.autopilot,
    state: task.state as TaskState | undefined,
    interrupted: task.interrupted,
    ...status,
    description: task.description,
    workflowId: task._workflowId,
    workflowName: context.workflowNameById.get(task._workflowId),
    workflowStepId: task.workflowStepId as string | undefined,
    workflowStepTitle: workflowStepTitle(task, context.stepTitleById),
    isRemoteExecutor: task.isRemoteExecutor,
    remoteExecutorId: task.primaryExecutorId ?? undefined,
    remoteExecutorType: task.primaryExecutorType ?? undefined,
    remoteExecutorName: task.primaryExecutorName ?? undefined,
    createdAt: task.createdAt,
    isArchived: task.isArchived === true,
    parentTaskTitle: task.parentTaskId ? context.titleById.get(task.parentTaskId) : undefined,
    parentTaskId: task.parentTaskId ?? undefined,
    workspaceMode: task.workspaceMode,
    repositories: resolveTaskRepositorySlugs(task.repositories, context.repositorySlugById),
    repositoryLinks: task.repositories,
    isPRReview: task.isPRReview ?? false,
    isIssueWatch: task.isIssueWatch ?? false,
  };
}
