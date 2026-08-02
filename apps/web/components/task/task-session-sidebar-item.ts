import type { KanbanState } from "@/lib/state/slices";
import type { TaskSessionState, TaskState } from "@/lib/types/http";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import { statusSummaryActiveErrorPreview } from "@/lib/task-status-summary";
import { workflowStepTitle } from "./task-session-sidebar-aggregate";

type SidebarItemContext = {
  repositorySlugById: Map<string, string | undefined>;
  titleById: Map<string, string>;
  workflowNameById: Map<string, string>;
  stepTitleById: Map<string, string>;
  acknowledgedAgentErrors?: Record<string, string>;
  dismissedAgentErrors?: Record<string, string>;
};

function summaryDiffStats(
  summary: TaskStatusSummary | null | undefined,
): { additions: number; deletions: number } | undefined {
  const git = summary?.git;
  if (!git) return undefined;
  const additions = git.additions ?? 0;
  const deletions = git.deletions ?? 0;
  return additions > 0 || deletions > 0 ? { additions, deletions } : undefined;
}

function capitalize(value: string): string {
  return value.length > 0 ? value[0].toUpperCase() + value.slice(1) : value;
}

function summaryPRInfo(
  summary: TaskStatusSummary | null | undefined,
): { number: number; state: string; aggregateState?: string } | undefined {
  const pullRequest = summary?.pull_request;
  if (!pullRequest?.number) return undefined;
  return {
    number: pullRequest.number,
    state: capitalize(pullRequest.state ?? pullRequest.aggregate_state ?? "open"),
    aggregateState: pullRequest.aggregate_state,
  };
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
    foregroundActivity: hasSummary ? summary?.foreground_activity : task.foregroundActivity,
    primarySessionId: hasSummary ? (primarySession?.id ?? null) : (task.primarySessionId ?? null),
    updatedAt: hasSummary ? summary?.updated_at : (task.updatedAt ?? task.createdAt),
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
    hasPendingClarification: pending.clarification,
    hasPendingPermission: pending.permission,
    prInfo: summaryPRInfo(summary),
    issueInfo: issueInfoForTask(task),
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
    state: task.state as TaskState | undefined,
    ...status,
    description: task.description,
    workflowId: task._workflowId,
    workflowName: context.workflowNameById.get(task._workflowId),
    workflowStepId: task.workflowStepId as string | undefined,
    workflowStepTitle: workflowStepTitle(task, context.stepTitleById),
    isRemoteExecutor: task.isRemoteExecutor,
    remoteExecutorType: task.primaryExecutorType ?? undefined,
    remoteExecutorName: task.primaryExecutorName ?? undefined,
    createdAt: task.createdAt,
    isArchived: false as boolean,
    parentTaskTitle: task.parentTaskId ? context.titleById.get(task.parentTaskId) : undefined,
    parentTaskId: task.parentTaskId ?? undefined,
    workspaceMode: task.workspaceMode,
    isPRReview: task.isPRReview ?? false,
    isIssueWatch: task.isIssueWatch ?? false,
  };
}
