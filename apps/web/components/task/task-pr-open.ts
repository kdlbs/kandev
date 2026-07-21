import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";

export type TaskPROpenAction =
  | { kind: "none" }
  | { kind: "open"; pr: TaskPR }
  | { kind: "pick"; prs: TaskPR[] };

/**
 * Decide what the "open task PR" shortcut should do: nothing when the task has
 * no linked PRs, open directly when there is exactly one, or show the picker
 * dialog when there are several.
 */
export function resolveTaskPROpenAction(prs: TaskPR[]): TaskPROpenAction {
  if (prs.length === 0) return { kind: "none" };
  if (prs.length === 1) return { kind: "open", pr: prs[0] };
  return { kind: "pick", prs };
}

export type TaskReviewOpenAction =
  | { kind: "none" }
  | { kind: "open"; url: string }
  | { kind: "pick" };

export function resolveTaskReviewOpenAction(prs: TaskPR[], mrs: TaskMR[]): TaskReviewOpenAction {
  const urls = [...prs.map((pr) => pr.pr_url), ...mrs.map((mr) => mr.mr_url)];
  if (urls.length === 0) return { kind: "none" };
  if (urls.length === 1) return { kind: "open", url: urls[0] };
  return { kind: "pick" };
}
