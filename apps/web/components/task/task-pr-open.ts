import type { TaskPR } from "@/lib/types/github";

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
