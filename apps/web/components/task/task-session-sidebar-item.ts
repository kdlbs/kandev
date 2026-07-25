import type { TaskPR } from "@/lib/types/github";
import type { KanbanState } from "@/lib/state/slices";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import type { TaskSession, TaskSessionState, TaskState } from "@/lib/types/http";
import { getSessionInfoForTask } from "@/lib/utils/session-info";
import { type AgentErrorOptions, agentErrorMessageForTask } from "@/lib/task-agent-error";
import { readTaskPendingFlags, workflowStepTitle } from "./task-session-sidebar-aggregate";

type SidebarItemContext = AgentErrorOptions & {
  sessionsById: Record<string, TaskSession>;
  sessionsByTaskId: Record<string, TaskSession[]>;
  gitStatusByEnvId: Record<string, GitStatusEntry>;
  envIdBySessionId: Record<string, string>;
  repositorySlugById: Map<string, string | undefined>;
  taskPRsByTaskId: Record<string, TaskPR[] | undefined>;
  pendingFlags: Record<string, boolean>;
  titleById: Map<string, string>;
  workflowNameById: Map<string, string>;
  stepTitleById: Map<string, string>;
};

function resolveDiffStats(
  sessionDiffStats: { additions: number; deletions: number } | undefined,
  task: { primarySessionId?: string | null },
  envIdBySessionId: Record<string, string>,
  gitStatusByEnvId: Record<string, GitStatusEntry>,
): { additions: number; deletions: number } | undefined {
  if (sessionDiffStats || !task.primarySessionId) return sessionDiffStats;
  const envKey = envIdBySessionId[task.primarySessionId] ?? task.primarySessionId;
  const gitStatus = gitStatusByEnvId[envKey];
  if (!gitStatus) return undefined;
  const additions = gitStatus.branch_additions ?? 0;
  const deletions = gitStatus.branch_deletions ?? 0;
  return additions > 0 || deletions > 0 ? { additions, deletions } : undefined;
}

function toPrInfo(pr: TaskPR | undefined): { number: number; state: string } | undefined {
  if (!pr?.state) return undefined;
  return { number: pr.pr_number, state: pr.state[0].toUpperCase() + pr.state.slice(1) };
}

function toIssueInfo(
  task: KanbanState["tasks"][number],
): { url: string; number: number } | undefined {
  return task.issueUrl && task.issueNumber
    ? { url: task.issueUrl, number: task.issueNumber }
    : undefined;
}

/** Map a kanban task to a sidebar item with session info and repository metadata. */
export function buildSidebarItem(
  task: KanbanState["tasks"][number] & { _workflowId: string },
  context: SidebarItemContext,
) {
  const sessionInfo = getSessionInfoForTask(
    task.id,
    context.sessionsByTaskId,
    context.gitStatusByEnvId,
    context.envIdBySessionId,
  );
  const sessionState =
    sessionInfo.sessionState ?? (task.primarySessionState as TaskSessionState | undefined);
  const repositoryPath = task.repositoryId
    ? context.repositorySlugById.get(task.repositoryId)
    : undefined;
  const pr = context.taskPRsByTaskId[task.id]?.[0];
  const pending = readTaskPendingFlags(
    context.pendingFlags,
    context.sessionsByTaskId[task.id] ?? [],
    task.taskPendingAction,
  );

  return {
    id: task.id,
    title: task.title,
    state: task.state as TaskState | undefined,
    sessionState,
    // Use the task-level aggregate so multi-session and off-screen rows match other task views.
    foregroundActivity: task.foregroundActivity,
    description: task.description,
    workflowId: task._workflowId,
    workflowName: context.workflowNameById.get(task._workflowId),
    workflowStepId: task.workflowStepId as string | undefined,
    workflowStepTitle: workflowStepTitle(task, context.stepTitleById),
    repositoryPath: pr ? `${pr.owner}/${pr.repo}` : repositoryPath,
    diffStats: resolveDiffStats(
      sessionInfo.diffStats,
      task,
      context.envIdBySessionId,
      context.gitStatusByEnvId,
    ),
    isRemoteExecutor: task.isRemoteExecutor,
    remoteExecutorType: task.primaryExecutorType ?? undefined,
    remoteExecutorName: task.primaryExecutorName ?? undefined,
    primarySessionId: task.primarySessionId ?? null,
    hasPendingClarification: pending.clarification,
    hasPendingPermission: pending.permission,
    updatedAt: sessionInfo.updatedAt ?? task.updatedAt ?? task.createdAt,
    createdAt: task.createdAt,
    isArchived: false as boolean,
    parentTaskTitle: task.parentTaskId ? context.titleById.get(task.parentTaskId) : undefined,
    parentTaskId: task.parentTaskId ?? undefined,
    workspaceMode: task.workspaceMode,
    prInfo: toPrInfo(pr),
    isPRReview: task.isPRReview ?? false,
    isIssueWatch: task.isIssueWatch ?? false,
    issueInfo: toIssueInfo(task),
    agentErrorMessage: agentErrorMessageForTask(
      task,
      context.sessionsById,
      context.sessionsByTaskId,
      context,
    ),
  };
}
