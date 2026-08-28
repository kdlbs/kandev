import {
  repositoryId as toRepositoryId,
  type TaskSessionState,
  type TaskState,
} from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";
import { statusSummaryActiveErrorPreview } from "@/lib/task-status-summary";
import type { WipQueueStatus } from "@/lib/kanban/wip-queue";
import { resolveTaskRepositorySlugs } from "@/lib/sidebar/sidebar-task-repositories";
import { effectiveTaskPendingAction } from "../task-select-helpers";
import { taskPRInfoFromSummary } from "../task-pr-info";

export type SheetItemCtx = {
  repositoryPathsById: Map<string, string | undefined>;
  workflowNameById: Map<string, string>;
  stepTitleById: Map<string, string>;
  wipQueueByTaskId?: Map<string, WipQueueStatus>;
  acknowledgedAgentErrors?: Record<string, string>;
  dismissedAgentErrors?: Record<string, string>;
};

function sheetDiffStats(summary: KanbanState["tasks"][number]["statusSummary"]) {
  const git = summary?.git;
  if (!git || ((git.additions ?? 0) <= 0 && (git.deletions ?? 0) <= 0)) return undefined;
  return { additions: git.additions ?? 0, deletions: git.deletions ?? 0 };
}

function sheetRepositoryPath(
  task: KanbanState["tasks"][number],
  ctx: SheetItemCtx,
): string | undefined {
  return task.repositoryId
    ? ctx.repositoryPathsById.get(toRepositoryId(task.repositoryId))
    : undefined;
}

function sheetPendingFlags(task: KanbanState["tasks"][number]) {
  const action = effectiveTaskPendingAction(task);
  return {
    clarification: action === "clarification",
    permission: action === "permission",
  };
}

function sheetStatus(task: KanbanState["tasks"][number], ctx: SheetItemCtx) {
  const summary = task.statusSummary;
  const hasSummary = summary != null;
  const pending = sheetPendingFlags(task);
  return {
    sessionState: hasSummary
      ? summary?.primary_session?.state
      : (task.primarySessionState as TaskSessionState | undefined),
    foregroundActivity: hasSummary ? summary?.foreground_activity : task.foregroundActivity,
    repositoryPath: sheetRepositoryPath(task, ctx),
    repositories: resolveTaskRepositorySlugs(task.repositories, ctx.repositoryPathsById),
    diffStats: sheetDiffStats(summary),
    comparisonUnavailable: summary?.git?.comparison_unavailable === true,
    updatedAt: hasSummary ? summary?.updated_at : task.updatedAt,
    primarySessionId: hasSummary
      ? (summary?.primary_session?.id ?? null)
      : (task.primarySessionId ?? null),
    hasPendingClarification: pending.clarification,
    hasPendingPermission: pending.permission,
    prInfo: taskPRInfoFromSummary(summary),
    agentErrorMessage: statusSummaryActiveErrorPreview(
      summary,
      ctx.acknowledgedAgentErrors,
      ctx.dismissedAgentErrors,
    ),
  };
}

export function toSheetItem(
  task: KanbanState["tasks"][number] & { _workflowId: string },
  ctx: SheetItemCtx,
) {
  const status = sheetStatus(task, ctx);
  return {
    id: task.id,
    title: task.title,
    autopilot: task.autopilot,
    parentTaskId: task.parentTaskId ?? undefined,
    workspaceMode: task.workspaceMode,
    state: task.state as TaskState | undefined,
    ...status,
    interrupted: task.interrupted,
    description: task.description,
    workflowId: task._workflowId,
    workflowName: ctx.workflowNameById.get(task._workflowId),
    workflowStepId: task.workflowStepId,
    workflowStepTitle: ctx.stepTitleById.get(task.workflowStepId),
    isArchived: task.isArchived === true,
    isRemoteExecutor: task.isRemoteExecutor,
    remoteExecutorType: task.primaryExecutorType ?? undefined,
    remoteExecutorName: task.primaryExecutorName ?? undefined,
    repositoryLinks: task.repositories,
    queuedCount: task.statusSummary?.queued_prompt_count,
    wipQueue: ctx.wipQueueByTaskId?.get(task.id),
  };
}
