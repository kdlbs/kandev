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
  | { kind: "open"; url: string; target: TaskReviewTarget }
  | { kind: "pick"; targets: TaskReviewTarget[] };

export type TaskReviewTarget =
  | { type: "pr"; key: string; url: string; review: TaskPR }
  | { type: "mr"; key: string; url: string; review: TaskMR };

export function buildTaskReviewTargets(prs: TaskPR[], mrs: TaskMR[]): TaskReviewTarget[] {
  return [
    ...prs.map((pr) => ({ type: "pr" as const, key: `pr:${pr.id}`, url: pr.pr_url, review: pr })),
    ...mrs.map((mr) => ({ type: "mr" as const, key: `mr:${mr.id}`, url: mr.mr_url, review: mr })),
  ];
}

export function resolveTaskReviewOpenAction(prs: TaskPR[], mrs: TaskMR[]): TaskReviewOpenAction {
  const targets = buildTaskReviewTargets(prs, mrs);
  if (targets.length === 0) return { kind: "none" };
  if (targets.length === 1) return { kind: "open", url: targets[0].url, target: targets[0] };
  return { kind: "pick", targets };
}
