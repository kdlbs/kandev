import type { TaskPRsState } from "@/lib/state/slices/github/types";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";

type TaskWithStatusSummary = {
  statusSummary?: TaskStatusSummary | null;
};

type TaskStatusSummaryByTaskId = Record<string, TaskStatusSummary | null | undefined>;

const EMPTY_TASK_PRS_BY_TASK_ID: Record<string, TaskPR[]> = {};

/** Return GitHub associations only when their store scope matches the active workspace. */
export function getTaskPRsByTaskIdForCurrentWorkspace(
  taskPRs: Pick<TaskPRsState, "byTaskId" | "workspaceId" | "workspaceContextGeneration">,
  activeWorkspaceId: string | null,
  workspaceContextGeneration: number,
): Record<string, TaskPR[]> {
  if (
    !activeWorkspaceId ||
    taskPRs.workspaceId !== activeWorkspaceId ||
    taskPRs.workspaceContextGeneration !== workspaceContextGeneration
  ) {
    return EMPTY_TASK_PRS_BY_TASK_ID;
  }
  return taskPRs.byTaskId;
}

/** GitHub PR and GitLab MR numbers linked to a task, de-duplicated and ascending. */
export function changeRequestNumbers(
  task: TaskWithStatusSummary,
  mrsForTask: TaskMR[] = [],
  prsForTask: TaskPR[] = [],
): number[] {
  const numbers = new Set<number>();
  const summaryPRNumber = task.statusSummary?.pull_request?.number;
  if (summaryPRNumber) numbers.add(summaryPRNumber);
  for (const pr of prsForTask) numbers.add(pr.pr_number);
  for (const mr of mrsForTask) numbers.add(mr.mr_iid);
  return [...numbers].sort((a, b) => a - b);
}

/**
 * One lowercase haystack per task built from its linked PR/MR numbers, tokenized
 * as `#<number>` so both a bare number and a `#`-prefixed number substring-match.
 */
export function buildTaskVcsSearchIndex(
  taskPRsByTaskId: Record<string, TaskPR[]>,
  taskMRsByTaskId: Record<string, TaskMR[]>,
  statusSummaryByTaskId: TaskStatusSummaryByTaskId = {},
): Record<string, string> {
  const index: Record<string, string> = {};
  const taskIds = new Set([
    ...Object.keys(taskPRsByTaskId),
    ...Object.keys(taskMRsByTaskId),
    ...Object.keys(statusSummaryByTaskId),
  ]);

  for (const taskId of taskIds) {
    const numbers = changeRequestNumbers(
      { statusSummary: statusSummaryByTaskId[taskId] },
      taskMRsByTaskId[taskId],
      taskPRsByTaskId[taskId],
    );
    if (numbers.length > 0) {
      index[taskId] = numbers.map((number) => `#${number}`).join(" ");
    }
  }

  return index;
}
