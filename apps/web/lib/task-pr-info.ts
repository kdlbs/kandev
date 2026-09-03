import type { TaskStatusSummary } from "@/lib/types/task-status-summary";

export type TaskPRInfo = {
  number: number;
  state: string;
  aggregateState?: string;
  autoFixEnabled?: boolean;
  autoMergeEnabled?: boolean;
};

function capitalize(value: string): string {
  return value.length > 0 ? value[0].toUpperCase() + value.slice(1) : value;
}

/** Map the bounded task-level PR projection to the shared task icon shape. */
export function taskPRInfoFromSummary(
  summary: TaskStatusSummary | null | undefined,
): TaskPRInfo | undefined {
  const pullRequest = summary?.pull_request;
  if (!pullRequest?.number) return undefined;
  return {
    number: pullRequest.number,
    state: capitalize(pullRequest.state ?? pullRequest.aggregate_state ?? "open"),
    aggregateState: pullRequest.aggregate_state,
    ...(pullRequest.auto_fix_enabled ? { autoFixEnabled: true } : {}),
    ...(pullRequest.auto_merge_enabled ? { autoMergeEnabled: true } : {}),
  };
}
