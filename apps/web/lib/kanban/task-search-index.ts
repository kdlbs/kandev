import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";

/**
 * One lowercase haystack per task built from its linked PR/MR numbers, tokenized
 * as `#<number>` so both a bare number and a `#`-prefixed number substring-match.
 */
export function buildTaskVcsSearchIndex(
  taskPRsByTaskId: Record<string, TaskPR[]>,
  taskMRsByTaskId: Record<string, TaskMR[]>,
): Record<string, string> {
  const index: Record<string, string> = {};

  for (const [taskId, prs] of Object.entries(taskPRsByTaskId)) {
    for (const pr of prs) {
      index[taskId] = `${index[taskId] ?? ""} #${pr.pr_number}`.trim();
    }
  }
  for (const [taskId, mrs] of Object.entries(taskMRsByTaskId)) {
    for (const mr of mrs) {
      index[taskId] = `${index[taskId] ?? ""} #${mr.mr_iid}`.trim();
    }
  }

  return index;
}
